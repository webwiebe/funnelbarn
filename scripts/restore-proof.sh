#!/usr/bin/env bash
#
# restore-proof.sh — prove the production replica restores, without touching
# production beyond reading row counts. Run it through the Restore Proof workflow.
#
# Runs a one-off Job in the production namespace on layer7: `litestream restore`
# into an emptyDir, then integrity_check and a row count per table. Production is
# counted read-only before and after the restore, so a table's restored count
# should sit between the two. A table that shrinks through maintenance can fall
# outside that range; the report names it and a person reads it.
#
# The restored database is a full copy of production data, so it stays on layer7.
# It lives in the emptyDir and goes when the Job is deleted, on every exit path.
# The pod's log holds only exit codes, timing and a count per table: Litestream's
# and sqlite3's own output goes to /dev/null, so a failed restore reports its exit
# code and no message. ttlSecondsAfterFinished and activeDeadlineSeconds clear the
# Job if this script dies.
#
# This repository's Actions logs are public. Standard output carries exit codes,
# elapsed seconds, table totals and the names of tables out of range. Counts per
# table go to OUTPUT_DIR only.
#
# The R2 key reaches the pod by secretKeyRef from funnelbarn-secrets, the same
# reference the production Litestream sidecar uses, and this script never reads
# it. `litestream restore` makes object reads and listings under the replica path
# in /etc/litestream.yml. Setting the region skips the bucket location call, and
# the Job's command replaces the entrypoint, so `litestream replicate` and its
# retention deletes do not run. The pod carries only the Job's own labels, so no
# Service selects it.
#
# Usage: scripts/restore-proof.sh OUTPUT_DIR
#   OUTPUT_DIR  created mode 700; holds the counts and the Job's PROOF lines
#
# Exit codes: 0 restored, integrity ok, every table in range or named
#             1 restore failed, integrity failed, or a table is missing
#             2 usage or preflight
set -euo pipefail

OUT="${1:-}"
[ -n "$OUT" ] || { echo "usage: $0 OUTPUT_DIR" >&2; exit 2; }
SECRET=funnelbarn-secrets
NAMESPACE=funnelbarn-production
REMOTE=(ssh -o StrictHostKeyChecking=no deployer@layer7.wiebe.xyz)
DB=/var/lib/funnelbarn/funnelbarn.db
JOB="restore-proof-$(date -u +%Y%m%d%H%M%S)"

install -d -m 700 "$OUT"

# The list leaves out SQLite's own tables and Litestream's.
LIST_SQL="SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '_litestream_%' ORDER BY name;"

kube() {
  "${REMOTE[@]}" "kubectl -n $NAMESPACE $*"
}

count_production() {
  # Table names come from the database itself. Each count is its own read.
  "${REMOTE[@]}" "kubectl -n $NAMESPACE exec -i deploy/funnelbarn -c funnelbarn -- sh" <<SH
sqlite3 -readonly $DB "$LIST_SQL" | while read -r t; do
  printf '%s\t' "\$t"; sqlite3 -readonly $DB "SELECT count(*) FROM \"\$t\";"
done
SH
}

IMAGE=$(kube get deploy funnelbarn -o "jsonpath='{.spec.template.spec.containers[0].image}'")
PULL=$(kube get deploy funnelbarn -o "jsonpath='{.spec.template.spec.imagePullSecrets}'")
[ -n "$IMAGE" ] || { echo "no image on the production deployment" >&2; exit 2; }

cleanup() {
  kube delete job "$JOB" --ignore-not-found --cascade=foreground --wait=true >/dev/null 2>&1 || true
  left=$(kube get pods -l "job-name=$JOB" -o name 2>/dev/null | wc -l | tr -d ' ')
  echo "throwaway pods left: $left"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM

count_production > "$OUT/production-before.tsv"

"${REMOTE[@]}" "kubectl -n $NAMESPACE apply -f -" >/dev/null <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: $JOB
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 3600
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      imagePullSecrets: ${PULL:-[]}
      volumes:
        - name: restore
          emptyDir:
            sizeLimit: 8Gi
      containers:
        - name: restore
          image: $IMAGE
          imagePullPolicy: IfNotPresent
          command: ["sh", "-c"]
          args:
            - |
              set -eu
              start=\$(date +%s)
              rc=0
              litestream restore -config /etc/litestream.yml -o /restore/proof.db $DB >/dev/null 2>&1 || rc=\$?
              echo "PROOF restore_exit \$rc"
              echo "PROOF elapsed_seconds \$((\$(date +%s) - start))"
              [ "\$rc" -eq 0 ] || exit "\$rc"
              rc=1
              [ "\$(sqlite3 -readonly /restore/proof.db 'PRAGMA integrity_check;' 2>/dev/null | head -1)" = ok ] && rc=0
              echo "PROOF integrity_exit \$rc"
              sqlite3 -readonly /restore/proof.db "$LIST_SQL" 2>/dev/null | while read -r t; do
                echo "PROOFROW \$t \$(sqlite3 -readonly /restore/proof.db "SELECT count(*) FROM \\"\$t\\";" 2>/dev/null)"
              done
              echo "PROOF done"
          env:
            - name: LITESTREAM_REPLICA_PATH
              value: funnelbarn/litestream/production/funnelbarn.db
            - name: LITESTREAM_ACCESS_KEY_ID
              valueFrom:
                secretKeyRef: {name: $SECRET, key: FUNNELBARN_R2_ACCESS_KEY_ID}
            - name: LITESTREAM_SECRET_ACCESS_KEY
              valueFrom:
                secretKeyRef: {name: $SECRET, key: FUNNELBARN_R2_SECRET_ACCESS_KEY}
          volumeMounts:
            - name: restore
              mountPath: /restore
          resources:
            requests: {cpu: 100m, memory: 128Mi}
            limits: {cpu: "1", memory: 1Gi}
YAML

# activeDeadlineSeconds fails the Job at the hour, so this loop ends. The
# workflow's timeout ends it if layer7 stops answering.
until [ -n "$(kube get job "$JOB" -o "jsonpath='{.status.conditions[*].type}'")" ]; do
  sleep 10
done

kube logs "job/$JOB" -c restore 2>/dev/null | grep -E '^PROOF(ROW)? ' > "$OUT/job.log" || true
chmod 600 "$OUT/job.log"
count_production > "$OUT/production-after.tsv"

python3 - "$OUT" <<'PY'
import pathlib
import sys

out = pathlib.Path(sys.argv[1])


def counts(name):
    rows = {}
    for line in (out / name).read_text().splitlines():
        table, _, n = line.partition("\t")
        if n.isdigit():
            rows[table] = int(n)
    return rows


before, after = counts("production-before.tsv"), counts("production-after.tsv")
proof, restored = {}, {}
for line in (out / "job.log").read_text(errors="replace").splitlines():
    parts = line.split(" ")
    if parts[0] == "PROOF" and len(parts) >= 3:
        proof[parts[1]] = " ".join(parts[2:])
    elif parts[0] == "PROOF" and parts[1:] == ["done"]:
        proof["done"] = "yes"
    elif parts[0] == "PROOFROW" and len(parts) == 3 and parts[2].isdigit():
        restored[parts[1]] = int(parts[2])

missing = sorted(set(after) - set(restored))
extra = sorted(set(restored) - set(after))
outside = []
for table, n in sorted(restored.items()):
    if table in after:
        low, high = sorted((before.get(table, after[table]), after[table]))
        if not low <= n <= high:
            outside.append(f"{table} {'below' if n < low else 'above'}")

print(f"restore_exit={proof.get('restore_exit')} elapsed_seconds={proof.get('elapsed_seconds')} integrity_exit={proof.get('integrity_exit')}")
print(f"tables={len(after)} restored={len(restored)} missing={missing} extra={extra}")
print(f"in_range={len(restored) - len(outside) - len(extra)} outside={len(outside)}")
for line in outside:
    print(f"  outside: {line}")
failed = not after or proof.get("done") != "yes" or proof.get("integrity_exit") != "0" or missing
sys.exit(1 if failed else 0)
PY
