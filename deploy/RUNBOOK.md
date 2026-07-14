# FunnelBarn Operations Runbook

Cluster host: `layer7.wiebe.xyz` (k3s), SSH user: `deployer`

Namespaces:
- Production: `funnelbarn-production`
- Staging: `funnelbarn-staging`
- Testing: `funnelbarn-testing`

---

## 1. Backup & Restore (Litestream)

Litestream replicates the SQLite database continuously to S3-compatible storage at `https://s3.wiebe.xyz`, bucket `funnelbarn-sqlite`.

Replica paths per environment:
- Production: `production/funnelbarn.db`
- Staging: (not configured — no Litestream env vars in staging manifest)
- Testing: (not configured — no Litestream env vars in testing manifest)

### List available restore points

```sh
# SSH into the cluster host, then list snapshots and WAL segments for the production replica:
ssh deployer@layer7.wiebe.xyz

litestream snapshots \
  -config /etc/litestream.yml \
  s3://funnelbarn-sqlite/production/funnelbarn.db

# List WAL segments (for point-in-time inspection):
litestream wal \
  -config /etc/litestream.yml \
  s3://funnelbarn-sqlite/production/funnelbarn.db
```

You must supply the S3 credentials via environment variables (same values used in the Kubernetes secret):

```sh
export LITESTREAM_ACCESS_KEY_ID=<value>
export LITESTREAM_SECRET_ACCESS_KEY=<value>
```

### Restore to latest (or point-in-time)

Scale down the service pod first to avoid write conflicts:

```sh
kubectl -n funnelbarn-production scale deployment/funnelbarn --replicas=0
kubectl -n funnelbarn-production rollout status deployment/funnelbarn
```

Run the restore (from the cluster host or any machine with network access to the S3 endpoint):

```sh
# Restore latest snapshot + WAL:
litestream restore \
  -config /etc/litestream.yml \
  -o /tmp/funnelbarn-restored.db \
  s3://funnelbarn-sqlite/production/funnelbarn.db

# Restore to a specific point in time (RFC3339):
litestream restore \
  -config /etc/litestream.yml \
  -timestamp "2026-04-30T12:00:00Z" \
  -o /tmp/funnelbarn-restored.db \
  s3://funnelbarn-sqlite/production/funnelbarn.db
```

### Validate the restore

```sh
sqlite3 /tmp/funnelbarn-restored.db "PRAGMA integrity_check;"
sqlite3 /tmp/funnelbarn-restored.db "SELECT count(*) FROM funnels;"
```

### Copy the restored database into the PVC

Exec into a temporary pod that mounts the PVC, copy the file, then scale back up:

```sh
# Spin up a debug pod mounting the existing PVC:
kubectl -n funnelbarn-production run db-restore --rm -it \
  --image=alpine --restart=Never \
  --overrides='{"spec":{"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"funnelbarn-data"}}],"containers":[{"name":"db-restore","image":"alpine","command":["sh"],"volumeMounts":[{"name":"data","mountPath":"/var/lib/funnelbarn"}]}]}}'

# Inside the pod, copy the restored file over the live db:
cp /tmp/funnelbarn-restored.db /var/lib/funnelbarn/funnelbarn.db
exit

# Scale the service back up:
kubectl -n funnelbarn-production scale deployment/funnelbarn --replicas=1
kubectl -n funnelbarn-production rollout status deployment/funnelbarn
```

---

## 2. Deploy Production

Production deploys are gated behind the `Deploy Production` GitHub Actions workflow (`deploy-production.yml`). It is manual-only (`workflow_dispatch`).

### Prerequisites

- The target version must already be validated on staging.
- The version tag (e.g. `v0.2.0`) must exist as a Docker image in GHCR (`ghcr.io/webwiebe/funnelbarn/service` and `ghcr.io/webwiebe/funnelbarn/web`).

### Steps

1. Go to **Actions → Deploy Production** in the GitHub repository.
2. Click **Run workflow**.
3. Fill in:
   - **Version to deploy** — e.g. `v0.2.0` (must match an existing GHCR image tag).
   - **Confirmed** — check the box to confirm the version is validated on staging. The workflow fails fast in the `preflight` job if this is left unchecked.
4. Click **Run workflow**.

Equivalent `gh` CLI invocation (note both inputs are required):

```sh
gh workflow run deploy-production.yml \
  -f production_version=v0.2.0 \
  -f confirmed=true
```

The workflow will:
- Verify the images exist in GHCR.
- Transfer K8s manifests to the cluster host via SSH.
- Apply the kustomization and update both deployment images.
- Wait for rollout (`--timeout=180s`). If the rollout fails, it automatically triggers a rollback (see Section 3).
- Post a release marker to BugBarn.

### Manual equivalent (emergency use only)

```sh
ssh deployer@layer7.wiebe.xyz

VERSION=v0.2.0
NAMESPACE=funnelbarn-production
SERVICE_IMAGE=ghcr.io/webwiebe/funnelbarn/service
WEB_IMAGE=ghcr.io/webwiebe/funnelbarn/web

kubectl apply -k /tmp/funnelbarn-deploy/deploy/k8s/production
kubectl -n $NAMESPACE set image deployment/funnelbarn funnelbarn=${SERVICE_IMAGE}:${VERSION}
kubectl -n $NAMESPACE set image deployment/funnelbarn-web funnelbarn-web=${WEB_IMAGE}:${VERSION}
kubectl -n $NAMESPACE rollout status deployment/funnelbarn --timeout=180s
kubectl -n $NAMESPACE rollout status deployment/funnelbarn-web --timeout=180s
```

---

## 2b. Custom ingest domain (`f.<brand>` CNAME)

Consuming apps can front FunnelBarn under their own branded subdomain (e.g. BrandTrace uses
`f.brandtrace.net`) instead of exposing `funnelbarn.wiebe.xyz` in `<script>` tags and API calls. This
is handled generically at the edge — **no per-customer manifest change and no application config**.

**How it works.** `deploy/k8s/production/ingressroute-f-wildcard.yaml` is a Traefik `IngressRoute`
that matches any host of the form `f.<domain>` (`HostRegexp(`^f\..+$`)`) and routes `/sdk.js` (+ the
legacy `/sdk/funnelbarn.js`) to the web/nginx service and `/api/*` to the Go service. Traefik v3
issues a Let's Encrypt certificate per incoming SNI on-demand (`certResolver: letsencrypt`). This is
the same pattern the sibling barn tools use (`bt.*` for BrandTrace, `sb.*` for SpanBarn). Projects are
still resolved by API key + the `x-funnelbarn-project` header, so the custom host needs no app-side
mapping.

**Bare-host redirect.** Every other path on `f.<domain>` (the root `/`, or any stray browser
navigation) is routed to the Go service, which `301`-redirects it to `https://<domain>` — it strips
the `f.` label and sends the visitor to the app the host fronts (e.g. `https://f.profotograaf.nl/` →
`https://profotograaf.nl`). So `f.<brand>` doubles as a friendly link, not just a machine ingest
endpoint. The redirect lives in the Go app (`internal/api/server.go`) and fires for `GET`/`HEAD` on
any non-ingest path; the dashboard SPA is never exposed on customer domains.

**Co-located app collision (why the ingest routes run at `priority: 200`).** When the consuming app
is deployed on this *same* cluster and greedily claims all of its own subdomains, its IngressRoute
will otherwise swallow `f.<its-domain>` before FunnelBarn sees it. Real case: `profotograaf`'s app
(`profotograaf-production/profotograaf-profotograaf-subdomain`) matches `HostRegexp(^…\.profotograaf\.nl$)`
— i.e. *all* of `*.profotograaf.nl`, including `f.profotograaf.nl` — with priority up to 115 on `/api`.
FunnelBarn's ingest routes therefore run at **priority 200** so the reserved `f.` label wins ingest
regardless of any co-located app. The `f.` label is reserved for FunnelBarn; the root redirect stays
at `priority: 1`, so `f.<domain>/` still lands on a co-located app's own root (which is the point of
the redirect anyway). **The same collision breaks `sb.`/`bb.`/`iam.` on such a domain** — each barn
tool must assert its own prefix priority (or the app must exclude the reserved labels from its matcher).

**Debugging tip:** if `f.<domain>` 404s, `curl -sI` it and check whether the response is from your
cluster or the customer's edge. A `server: cloudflare` 404 with the host resolving into the *customer's*
Cloudflare zone means their DNS isn't pointed at the shared cluster yet (onboarding step 1). If it
resolves to the shared cluster and *still* 404s, hit the origin directly
(`curl -k --resolve f.<domain>:443:<cluster-ip> https://f.<domain>/api/v1/health`) — a 404 there is a
Traefik routing collision with a co-located app (see the priority note above).

**Onboarding a customer (one-time, DNS side only):**

1. Point `f.<brand>` at the shared cluster — a CNAME to the same target the customer's other barn
   subdomains (`bt.<brand>`, `sb.<brand>`) already use. Port 80 must reach Traefik for that host so
   the HTTP-01 ACME challenge can validate and the per-SNI cert can issue.
2. Install the snippet against the custom host: `<script src="https://f.<brand>/sdk.js"
   data-api-key="…" data-project-name="…" defer></script>`. The SDK derives its endpoint from the
   script's own origin, so events go to `https://f.<brand>/api/v1/events` automatically.
3. Verify (see checks in Section 2c below).

Only `/sdk.js` and `/api/*` are exposed on customer domains — the dashboard SPA is not.

### 2c. Verify a custom ingest domain

```sh
curl -sI https://f.<brand>/sdk.js            # expect HTTP 200 (served by nginx)
curl -sI https://f.<brand>/api/v1/health     # expect HTTP 200 (Go service)
curl -sI https://f.<brand>/                  # expect HTTP 301 → https://<brand> (bare-host redirect)
```

Then load a page embedding the snippet and confirm in the browser console that `window.funnelbarn`
initializes and a `track()` POST to `https://f.<brand>/api/v1/events` returns 2xx. Event volume for
the project should appear on the dashboard within a minute.

---

## 3. Rollback

### Automatic rollback

The `deploy-production.yml` workflow has an automatic rollback step (`if: failure()`). If either `rollout status` call times out or fails, the workflow runs:

```sh
kubectl -n funnelbarn-production rollout undo deployment/funnelbarn
kubectl -n funnelbarn-production rollout undo deployment/funnelbarn-web
```

### Manual rollback

SSH into the cluster host and undo the last rollout:

```sh
ssh deployer@layer7.wiebe.xyz

# Roll back both deployments to the previous ReplicaSet:
kubectl -n funnelbarn-production rollout undo deployment/funnelbarn
kubectl -n funnelbarn-production rollout undo deployment/funnelbarn-web

# Confirm the rollback completed:
kubectl -n funnelbarn-production rollout status deployment/funnelbarn
kubectl -n funnelbarn-production rollout status deployment/funnelbarn-web

# Check which image version is now running:
kubectl -n funnelbarn-production get deployment funnelbarn \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
```

### Roll back to a specific revision

```sh
# List revision history:
kubectl -n funnelbarn-production rollout history deployment/funnelbarn

# Roll back to revision 3:
kubectl -n funnelbarn-production rollout undo deployment/funnelbarn --to-revision=3
```

Note: `revisionHistoryLimit: 2` is set on all deployments, so only the two most recent ReplicaSets are kept.

---

## 4. Database Inspection

The SQLite database lives at `/var/lib/funnelbarn/funnelbarn.db` inside the service pod. The service deployment uses `strategy: Recreate`, so there is always at most one pod running.

### Find the running pod

```sh
kubectl -n funnelbarn-production get pods -l app.kubernetes.io/name=funnelbarn
```

### Exec in and run SQLite queries

```sh
POD=$(kubectl -n funnelbarn-production get pods \
  -l app.kubernetes.io/name=funnelbarn \
  -o jsonpath='{.items[0].metadata.name}')

kubectl -n funnelbarn-production exec -it $POD -- \
  sqlite3 /var/lib/funnelbarn/funnelbarn.db
```

Useful queries inside the sqlite3 shell:

```sql
-- List tables:
.tables

-- Row counts:
SELECT count(*) FROM funnels;
SELECT count(*) FROM funnel_steps;
SELECT count(*) FROM events;

-- Database integrity check:
PRAGMA integrity_check;

-- WAL size:
PRAGMA wal_checkpoint(TRUNCATE);
```

### Staging / Testing

Replace `funnelbarn-production` with `funnelbarn-staging` or `funnelbarn-testing` as needed.

---

## 5. Log Access

### Tail service logs (production)

```sh
kubectl -n funnelbarn-production logs -f \
  -l app.kubernetes.io/name=funnelbarn \
  --tail=100
```

### Tail web (Next.js) logs (production)

```sh
kubectl -n funnelbarn-production logs -f \
  -l app.kubernetes.io/name=funnelbarn-web \
  --tail=100
```

### Previous container logs (after a crash/restart)

```sh
POD=$(kubectl -n funnelbarn-production get pods \
  -l app.kubernetes.io/name=funnelbarn \
  -o jsonpath='{.items[0].metadata.name}')

kubectl -n funnelbarn-production logs $POD --previous
```

### All namespaces at once

```sh
# Service logs across all environments:
for ns in funnelbarn-production funnelbarn-staging funnelbarn-testing; do
  echo "=== $ns ==="
  kubectl -n $ns logs -l app.kubernetes.io/name=funnelbarn --tail=20 2>/dev/null || echo "(no pods)"
done
```

### Staging / Testing

Replace `funnelbarn-production` with `funnelbarn-staging` or `funnelbarn-testing`.

---

## 6. Emergency Shutdown

Scale all deployments in a namespace to zero replicas. This stops all traffic immediately without deleting any resources.

### Scale down production

```sh
ssh deployer@layer7.wiebe.xyz

kubectl -n funnelbarn-production scale deployment/funnelbarn --replicas=0
kubectl -n funnelbarn-production scale deployment/funnelbarn-web --replicas=0

# Confirm pods are terminated:
kubectl -n funnelbarn-production get pods
```

### Scale back up

```sh
kubectl -n funnelbarn-production scale deployment/funnelbarn --replicas=1
kubectl -n funnelbarn-production scale deployment/funnelbarn-web --replicas=1

kubectl -n funnelbarn-production rollout status deployment/funnelbarn
kubectl -n funnelbarn-production rollout status deployment/funnelbarn-web
```

### Shutdown staging or testing

```sh
NAMESPACE=funnelbarn-staging  # or funnelbarn-testing

kubectl -n $NAMESPACE scale deployment/funnelbarn --replicas=0
kubectl -n $NAMESPACE scale deployment/funnelbarn-web --replicas=0
```

### Full namespace lockdown (remove ingress)

To stop external traffic while keeping pods running for inspection:

```sh
kubectl -n funnelbarn-production delete ingress funnelbarn
# Restore with:
kubectl -n funnelbarn-production apply -f /tmp/funnelbarn-deploy/deploy/k8s/production/ingress.yaml
```
