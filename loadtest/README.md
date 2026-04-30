# FunnelBarn Load Tests

k6 scripts for validating throughput, rate limiting, and query performance.

## Prerequisites

Install [k6](https://k6.io/docs/get-started/installation/):

```bash
# macOS
brew install k6

# Linux (Debian/Ubuntu)
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update && sudo apt-get install k6

# Or download a binary from https://github.com/grafana/k6/releases
```

k6 v0.46+ is recommended. No other dependencies are required.

## Running ingest test

Tests that the event ingest endpoint sustains 200 RPS for 30 seconds with less than 1% errors and p95 latency under 200 ms.

```bash
BASE_URL=http://localhost:8080 \
API_KEY=your-api-key \
k6 run loadtest/scenarios/ingest.js
```

Optional: set `PROJECT_ID` if your ingest endpoint requires it.

The test will fail (non-zero exit) if either threshold is breached:
- `http_req_failed < 1%`
- `http_req_duration p(95) < 200ms`

## Running rate-limit validation

This is a **correctness test**, not a throughput test. It deliberately hammers the login endpoint with 30 rapid attempts across 10 VUs to confirm the rate limiter fires 429 responses.

```bash
BASE_URL=http://localhost:8080 \
k6 run loadtest/scenarios/rate-limit.js
```

The test passes when at least 30% of requests are rate-limited (HTTP 429). Seeing 429s is expected and desired — a run with zero 429s indicates the rate limiter is not working.

## Running analysis test

Tests the funnel analysis query endpoint under 20 concurrent users for 30 seconds. This endpoint is session-authenticated, so you must provide a valid session cookie, project ID, and funnel ID.

**How to get the required values:**

1. **SESSION_COOKIE** — Log in to FunnelBarn in your browser, open DevTools (Application > Cookies), and copy the value of `funnelbarn_session`.
2. **PROJECT_ID** — Find this in the URL when viewing a project: `/projects/<PROJECT_ID>/...`
3. **FUNNEL_ID** — Find this in the URL when viewing a funnel: `/projects/<PROJECT_ID>/funnels/<FUNNEL_ID>`

```bash
BASE_URL=http://localhost:8080 \
SESSION_COOKIE=your-session-cookie-value \
PROJECT_ID=your-project-id \
FUNNEL_ID=your-funnel-id \
k6 run loadtest/scenarios/analysis.js
```

The test will fail if either threshold is breached:
- `http_req_failed < 1%`
- `http_req_duration p(95) < 2000ms`

## Interpreting results

k6 prints a summary table at the end of each run. Key metrics to watch:

| Metric | What it means |
|---|---|
| `http_req_duration` | End-to-end request latency. Check `p(50)`, `p(95)`, `p(99)`. |
| `http_req_failed` | Fraction of requests that returned a network error or 4xx/5xx. |
| `http_reqs` | Total requests and throughput (RPS). |
| `checks` | Pass/fail rate of your explicit `check()` assertions. |
| `iterations` | Number of times the default function completed. |

**Thresholds** appear at the bottom of the summary with a tick or cross. A cross causes k6 to exit with a non-zero status code, which will fail a CI step.

**Reading the output:**

```
✓ http_req_duration.............: avg=45ms  p(95)=182ms
✓ http_req_failed...............: 0.00%
✗ checks........................: 98.50% (985/1000)
```

A failed `checks` line with passing thresholds means individual assertions failed (e.g. wrong status code) but did not breach the configured limits — investigate the check name to understand which assertion is failing.

## Baseline

Record results here after each release to detect regressions.

| Date | Version | p50 (ms) | p95 (ms) | p99 (ms) | RPS | Error rate |
|---|---|---|---|---|---|---|
| — | — | — | — | — | — | — |
