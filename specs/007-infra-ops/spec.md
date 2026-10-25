# Spec 007: Infrastructure Hardening + Ops Runbook

**Spec**: `specs/007-infra-ops/`
**Status**: Shipped
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: none found
**Principles touched**: VIII

## Purpose

Every Kubernetes container declares resource requests and limits plus health probes, and `deploy/RUNBOOK.md` documents the operations an on-call person needs.

## Functional Requirements

- **FR-001**: Every service container in `deploy/k8s/{production,staging,testing}` (`deployment.yaml`, `reader-statefulset.yaml`, `web-deployment.yaml`) MUST declare resource requests and limits. Issues: none. [inferred]
- **FR-002**: The writer container MUST declare a liveness and a readiness probe on `/api/v1/health`. Issues: none. [inferred]
- **FR-003**: The reader container MUST declare a liveness probe on `/api/v1/health` and a readiness probe on `/api/v1/ready`. Issues: none. [inferred]
- **FR-004**: The web container MUST declare liveness and readiness TCP probes on port 3000. Issues: none. [inferred]
- **FR-005**: Production limits MUST be at least the staging and testing limits for the writer. Issues: none. [inferred]
- **FR-006**: `deploy/RUNBOOK.md` MUST cover backup and restore, production deploy, rollback, database inspection, log access and emergency shutdown. Issues: none. [inferred]

## Drift

| Claim | Source | Tree |
|---|---|---|
| Both probes use `/api/v1/health` | Original task | Readers use `/api/v1/ready` for readiness (`reader-statefulset.yaml` in each environment) |
| Production writer requests 100m CPU and 64Mi memory, limits 500m and 256Mi | Original task example | Production writer requests 128Mi (`deploy/k8s/production/deployment.yaml`); staging limits 128Mi, testing limits 250m CPU and 128Mi |
| Three deployment files, one per environment | Original task | Each environment also has `reader-statefulset.yaml` and `web-deployment.yaml`, and production adds `ingressroute-f-wildcard.yaml` |
| The runbook has 6 sections | Acceptance criteria | `deploy/RUNBOOK.md` has sections 1 to 6 plus 2b, 2d and 4b; there is no section 2c heading at level 2 (it is a level 3 heading under 2b) |
| Backup restore uses Litestream S3 replication | Original task | Holds; `deploy/docker/litestream.yml` and the writer manifests set `LITESTREAM_*` variables |

## Proof contract

- [ ] `grep -c 'livenessProbe\|readinessProbe'` on each deployment, statefulset and web manifest returns 2.
- [ ] Each of the 6 required runbook sections has a heading in `deploy/RUNBOOK.md`.
- [ ] The post-deploy version smoke passes on the writer and a reader (`scripts/test_version_smoke.py::DeployWiringTest.test_every_environment_runs_the_smoke_on_writer_and_reader`).

## Sources

- `deploy/k8s/*/deployment.yaml`, `deploy/k8s/*/reader-statefulset.yaml`, `deploy/k8s/*/web-deployment.yaml`, `deploy/RUNBOOK.md`
