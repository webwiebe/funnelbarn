# Task Backlog: Analytics & Funnel Tracking Foundation

## Phase 1 — Core Go Binary (MVP)

| ID | Task | Status |
|----|------|--------|
| T-001 | Go module `github.com/wiebe-xyz/trailpost` | DONE |
| T-002 | `internal/config`: env var config struct with loadConfigFiles | DONE |
| T-003 | `internal/spool`: durable NDJSON spool with cursor | DONE |
| T-004 | `internal/auth`: API key authorizer, user auth, session manager | DONE |
| T-005 | `internal/storage/db.go`: SQLite open + schema migration | DONE |
| T-006 | `internal/storage/schema.go`: full SQL DDL | DONE |
| T-007 | `internal/storage/events.go`: event CRUD + dashboard queries | DONE |
| T-008 | `internal/storage/sessions.go`: session CRUD | DONE |
| T-009 | `internal/storage/funnels.go`: funnel + step CRUD + analysis | DONE |
| T-010 | `internal/enrich`: UA parser, UTM extractor, referrer domain, user ID hash | DONE |
| T-011 | `internal/session/fingerprint.go`: IP-prefix + UA hash session ID | DONE |
| T-012 | `internal/ingest/handler.go`: auth → validate → queue → spool | DONE |
| T-013 | `internal/worker/worker.go`: decode + enrich + persist loop | DONE |
| T-014 | `internal/api/server.go`: HTTP server, CORS, route registration | DONE |
| T-015 | `internal/api/health.go`: GET /api/v1/health | DONE |
| T-016 | `internal/api/auth.go`: login, logout, me, projects, apikeys | DONE |
| T-017 | `internal/api/events.go`: paginated event list | DONE |
| T-018 | `internal/api/sessions.go`: paginated session list | DONE |
| T-019 | `internal/api/dashboard.go`: aggregate stats | DONE |
| T-020 | `internal/api/funnels.go`: funnel CRUD + analysis | DONE |
| T-021 | `cmd/trailpost/main.go`: entry point, CLI subcommands, BugBarn wiring | DONE |
| T-022 | `go.mod` with sqlite3, bugbarn-go, bcrypt deps | DONE |

## Phase 1 — Pending Go Work

| ID | Task | Status |
|----|------|--------|
| T-023 | `go.sum` file (requires running `go mod tidy`) | TODO |
| T-024 | Unit tests for `internal/enrich` (UA parsing, UTM extraction) | TODO |
| T-025 | Unit tests for `internal/session` (fingerprint) | TODO |
| T-026 | Unit tests for `internal/auth` (authorizer, session manager) | TODO |
| T-027 | Integration tests for ingest → worker → storage pipeline | TODO |
| T-028 | Integration tests for funnel analysis | TODO |
| T-029 | Funnel step property filters implementation | TODO |
| T-030 | Session-ordered funnel analysis (requires all steps completed in order) | TODO |
| T-031 | `worker.ProcessRecord` to extract UA from payload `user_agent` field via `RemoteAddr` | TODO |
| T-032 | Geo-IP lookup stub (populate `country_code` from IP, opt-in) | TODO |

## Phase 1 — Web UI

| ID | Task | Status |
|----|------|--------|
| T-040 | `web/package.json`, `vite.config.ts`, `index.html` | DONE |
| T-041 | `web/src/main.tsx`, `App.tsx` with React Router | DONE |
| T-042 | `web/src/pages/Dashboard.tsx` — stats + top pages + referrers | DONE |
| T-043 | `web/src/pages/Funnels.tsx` — funnel list + conversion bar chart | DONE |
| T-044 | Login page with session cookie auth | TODO |
| T-045 | Project selector / project list page | TODO |
| T-046 | Time range picker (last 7d / 30d / 90d / custom) | TODO |
| T-047 | Real time-series chart (canvas or SVG sparkline) | TODO |
| T-048 | Event list page with pagination | TODO |
| T-049 | Session list page | TODO |
| T-050 | API key management page | TODO |

## Phase 1 — JavaScript SDK

| ID | Task | Status |
|----|------|--------|
| T-060 | `sdks/js/src/index.ts` — TrailpostClient class | DONE |
| T-061 | `sdks/js/package.json`, `tsconfig.json` | DONE |
| T-062 | Unit tests for session ID generation | TODO |
| T-063 | Unit tests for UTM extraction | TODO |
| T-064 | Unit tests for event batching | TODO |
| T-065 | Build pipeline (ESM + CJS outputs) | TODO |
| T-066 | Publish to npm as `@trailpost/js` | TODO |

## Phase 1 — Go SDK

| ID | Task | Status |
|----|------|--------|
| T-070 | `sdks/go/trailpost.go` — Init/Track/Page/Flush/Shutdown | DONE |
| T-071 | `sdks/go/go.mod` | DONE |
| T-072 | Unit tests for Go SDK transport | TODO |
| T-073 | Publish to GitHub as `github.com/wiebe-xyz/trailpost-go` | TODO |

## Phase 1 — Python SDK

| ID | Task | Status |
|----|------|--------|
| T-080 | `sdks/python/trailpost/__init__.py` — TrailpostClient stub | DONE |
| T-081 | `sdks/python/pyproject.toml` | DONE |
| T-082 | Unit tests for Python SDK | TODO |
| T-083 | Django middleware helper | TODO |
| T-084 | Flask/FastAPI integration example | TODO |
| T-085 | Publish to PyPI as `trailpost` | TODO |

## Phase 1 — Docker & Deployment

| ID | Task | Status |
|----|------|--------|
| T-090 | `deploy/docker/service.Dockerfile` | DONE |
| T-091 | `deploy/docker/web.Dockerfile` | DONE |
| T-092 | `deploy/docker/entrypoint.sh` — Litestream conditional | DONE |
| T-093 | `deploy/docker/litestream.yml` | DONE |
| T-094 | `deploy/docker/nginx.conf` | DONE |
| T-095 | `docker-compose.yml` | DONE |
| T-096 | Verify `docker compose up` starts successfully | TODO |

## Phase 1 — K8s Manifests

| ID | Task | Status |
|----|------|--------|
| T-100 | testing: namespace, pvc, deployment, web-deployment, service, web-service, ingress, kustomization | DONE |
| T-101 | staging: same structure | DONE |
| T-102 | production: same structure + Litestream env vars | DONE |
| T-103 | SOPS secret templates for testing/staging/production | TODO |
| T-104 | `.sops.yaml` age key configuration | DONE |

## Phase 1 — CI/CD

| ID | Task | Status |
|----|------|--------|
| T-110 | `.github/workflows/build-and-test.yml` — build + test + deploy testing | DONE |
| T-111 | `.github/workflows/binary-release.yml` — .deb + Homebrew release | DONE |
| T-112 | `.github/workflows/deploy-production.yml` — manual prod deploy | DONE |
| T-113 | GitHub repo and secrets setup | TODO |
| T-114 | GHCR package creation | TODO |
| T-115 | First binary release tag | TODO |

## Phase 1 — Packaging

| ID | Task | Status |
|----|------|--------|
| T-120 | `nfpm-trailpost.yaml` — .deb package config | DONE |
| T-121 | `deploy/systemd/trailpost.service` | DONE |
| T-122 | `deploy/deb/postinstall.sh` | DONE |
| T-123 | `deploy/deb/preremove.sh` | DONE |
| T-124 | `deploy/etc/trailpost.conf.example` | DONE |
| T-125 | APT repository integration via rapid-root dispatch | DONE (in CI) |
| T-126 | Homebrew tap formula at `webwiebe/homebrew-trailpost` | TODO |

## Phase 1 — Documentation

| ID | Task | Status |
|----|------|--------|
| T-130 | `README.md` with quick-start + config table | DONE |
| T-131 | `sdks/go/README.md` | DONE |
| T-132 | `sdks/js/README.md` | DONE |
| T-133 | `sdks/python/README.md` | DONE |
| T-134 | `site/src/pages/index.astro` — landing page | DONE |
| T-135 | Full API reference in docs site | TODO |
| T-136 | Self-hosting guide in docs site | TODO |

## Phase 2 — Funnels (Enhancements)

| ID | Task | Status |
|----|------|--------|
| T-200 | Property filter evaluation in funnel analysis | TODO |
| T-201 | Session-ordered funnel (user must complete steps in sequence) | TODO |
| T-202 | Funnel time-to-convert metric | TODO |
| T-203 | Funnel comparison across time windows | TODO |

## Phase 3 — Alerts & Digests

| ID | Task | Status |
|----|------|--------|
| T-300 | Alert rules schema and API | TODO |
| T-301 | Spike detection (event rate significantly above baseline) | TODO |
| T-302 | Threshold alerts (event count > N in window) | TODO |
| T-303 | Webhook delivery for alerts | TODO |
| T-304 | Weekly digest email (SMTP) | TODO |

## Phase 4 — Advanced

| ID | Task | Status |
|----|------|--------|
| T-400 | Geo-IP integration (MaxMind GeoLite2 free database) | TODO |
| T-401 | Real-time event stream (SSE) for live view | TODO |
| T-402 | Export events as CSV | TODO |
| T-403 | Custom retention policy (auto-delete old events) | TODO |
| T-404 | Multi-user dashboard access | TODO |
