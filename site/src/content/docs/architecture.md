---
title: Architecture Overview
description: How FunnelBarn processes events — from HTTP ingest through the spool to SQLite.
order: 2
---

# Architecture Overview

FunnelBarn is a single Go binary with no external runtime dependencies. All data is stored in SQLite. The design prioritises fast, non-blocking ingest above all else.

## The ingest pipeline

```
Client (browser / SDK)
        │
        │  POST /api/v1/events
        ▼
┌───────────────────┐
│   Ingest Handler  │  validates API key, reads body
└────────┬──────────┘
         │
         │  enqueue (in-memory channel, size 32 768)
         ▼
┌───────────────────┐
│   Spool Writer    │  batches up to 64 records every 5 ms
└────────┬──────────┘
         │
         │  writes newline-delimited JSON to disk
         ▼
┌───────────────────┐
│   Spool Directory │  FUNNELBARN_SPOOL_DIR  (default: .data/spool)
└────────┬──────────┘
         │
         │  background worker reads files
         ▼
┌───────────────────┐
│   Worker          │  parses events, enriches, upserts sessions
└────────┬──────────┘
         │
         │  SQL writes
         ▼
┌───────────────────┐
│   SQLite          │  FUNNELBARN_DB_PATH  (default: .data/funnelbarn.db)
└───────────────────┘
         │
         │  SQL reads
         ▼
┌───────────────────┐
│   Dashboard API   │  GET /api/v1/projects/:id/dashboard
└───────────────────┘
```

## Ingest Handler

`POST /api/v1/events` is the only public write endpoint. The handler:

1. Validates the `x-funnelbarn-api-key` header against the configured API key or the database of per-project keys.
2. Reads the request body (capped at `FUNNELBARN_MAX_BODY_BYTES`, default 1 MiB).
3. Enqueues a `spool.Record` containing the raw body, the remote address, and metadata onto an in-memory channel.
4. Returns `202 Accepted` immediately with an `ingestId`.

The handler never blocks waiting for a database write. If the in-memory queue is full (e.g., during a traffic spike), it returns `429 Too Many Requests` with a `Retry-After: 1` header rather than blocking.

## Spool Writer

A background goroutine drains the in-memory queue in batches of up to 64 records, flushing every 5 ms. Records are appended to newline-delimited JSON files on disk in `FUNNELBARN_SPOOL_DIR`. This makes writes append-only and durable: even if the process crashes between ingest and database write, no events are lost.

## Background Worker

The worker continuously polls the spool directory for new files. For each file it:

1. Parses each `spool.Record`.
2. Decodes the raw JSON body into an event payload.
3. Enriches the event — parses the User-Agent for browser/OS/device type, extracts UTM parameters from the URL, resolves the session fingerprint.
4. Upserts the session record (create or extend the existing session based on the IP + User-Agent fingerprint).
5. Inserts the event into SQLite.
6. Deletes the processed spool file.

Session fingerprinting uses the IP address and User-Agent string. The raw values are never stored permanently — only the derived session ID hash.

## SQLite

All persistent data lives in a single SQLite file. Astro content collections are not used — there is no CMS. The schema contains:

- `projects` — multi-tenancy unit; each project has its own API key.
- `events` — one row per ingest call; stores event name, URL, referrer, UTMs, session ID, user ID hash, properties (JSON), device type, browser, OS, timestamps.
- `sessions` — one row per session; stores entry URL, referrer, UTMs, device/browser, first/last seen, event count.
- `funnels` + `funnel_steps` — funnel definitions.
- `ab_tests` — A/B test definitions.
- `api_keys` — per-project ingest keys with scope (`ingest` or `full`).
- `users` — dashboard user accounts.

## Dashboard reads

Dashboard queries hit SQLite directly via the `GET /api/v1/projects/:id/dashboard` endpoint. Aggregates (top pages, referrers, UTM sources, time-series counts, bounce rate, avg events per session) are computed with SQL queries at query time. There is no pre-aggregation layer; SQLite is fast enough for typical self-hosted workloads.

## Multi-project

One FunnelBarn instance supports multiple projects. Each project has its own API key. Events are routed to the correct project based on the key used at ingest. The `x-funnelbarn-project` header can optionally override the project slug.
