# Spec 007: Infrastructure Hardening + Ops Runbook

## Goal
Verify K8s manifests have resource limits, add missing probes, and write a backup restore runbook.

## Files to modify / create
- `deploy/k8s/production/deployment.yaml` — verify/add resource limits + probes
- `deploy/k8s/staging/deployment.yaml` — same
- `deploy/k8s/testing/deployment.yaml` — same
- `deploy/RUNBOOK.md` (new) — backup restore + ops procedures

## K8s Manifest Audit

For each deployment.yaml (service container), verify:

1. **Resource limits present** — every container must have requests + limits:
```yaml
resources:
  requests:
    cpu: 100m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi
```
Production limits should be higher than staging/testing.

2. **Liveness probe** on `/api/v1/health`:
```yaml
livenessProbe:
  httpGet:
    path: /api/v1/health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 20
  failureThreshold: 3
```

3. **Readiness probe** on `/api/v1/health`:
```yaml
readinessProbe:
  httpGet:
    path: /api/v1/health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
  failureThreshold: 3
```

4. For the web container, use a TCP probe on port 3000 (no health path available).

## Runbook (deploy/RUNBOOK.md)

Write a practical ops runbook covering:

### Sections required:
1. **Backup & Restore** — how to restore from Litestream S3 replication:
   - Command to list available restore points
   - Command to restore to a point in time
   - Validate restore procedure
2. **Deploy Production** — manual steps using `deploy-production.yml` workflow
3. **Rollback** — how to rollback a bad deployment (workflow has auto-rollback, document manual steps too)
4. **Database Inspection** — how to exec into the pod and run sqlite3 queries
5. **Log Access** — how to tail logs from K8s pods
6. **Emergency Shutdown** — how to scale to 0 replicas safely

## Acceptance criteria
- All three environment deployment.yaml files have resource limits, liveness, and readiness probes
- `deploy/RUNBOOK.md` exists with all 6 sections
- No functional code changes — purely config + docs
