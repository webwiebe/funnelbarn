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
   - **Confirmed** — check the box to confirm the version is validated on staging.
4. Click **Run workflow**.

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
