# funnelbarn-go

Go SDK for [FunnelBarn](https://github.com/wiebe-xyz/funnelbarn) — self-hosted web analytics.

## Installation

```bash
go get github.com/webwiebe/funnelbarn/sdks/go
```

The module is served from the funnelbarn monorepo. Releases are tagged as `sdks/go/vX.Y.Z` (so `go get` resolves the right version) and the binary-release workflow pushes those tags automatically alongside the main `vX.Y.Z` tag used for binaries.

## Usage

```go
package main

import (
    funnelbarn "github.com/webwiebe/funnelbarn/sdks/go"
)

func main() {
    funnelbarn.Init(funnelbarn.Options{
        APIKey:      "your-api-key",
        Endpoint:    "https://funnelbarn.example.com",
        ProjectName: "my-app",
        // Tags every event, so one key and project can be reused across
        // deployments and filtered apart in the dashboard.
        Environment: "production",
    })
    defer funnelbarn.Shutdown(5 * time.Second)

    // Track a page view
    funnelbarn.Page("https://example.com/pricing", "https://google.com")

    // Track a custom event
    funnelbarn.Track("signup", map[string]any{
        "plan": "pro",
    })
}
```

### Fully-specified events

`Track` and `Page` are conveniences over `TrackEvent`, which reaches every field
the payload carries:

```go
funnelbarn.TrackEvent(funnelbarn.Event{
    Name:        "outreach_opened",
    UTMSource:   "outreach",
    UTMMedium:   "email",
    UTMCampaign: "nl-intro",
    Properties:  map[string]any{"step": "intro-1"},
    UserID:      "lead-77607",
    SessionID:   "send-abc123",
    Timestamp:   openedAt, // zero means now
})
```

**`SessionID` is what funnel reports group on.** An event sent without one is
still recorded, but it cannot take part in a funnel — so a server-side sequence
(`sent` → `opened` → `clicked` → `converted`) must set it to something stable
for the subject being followed: a recipient, an order, a job. Browser traffic
gets this from the JS SDK; a Go service has to supply it.

Set `Timestamp` explicitly for anything replayed from a spool or a retry queue.
Left zero it stamps at enqueue, which records when you got around to sending the
event rather than when it happened.

### `Endpoint` is a base URL

```go
Endpoint: "https://funnelbarn.example.com"   // the SDK appends /api/v1/events
```

The full ingest URL is accepted too — a trailing `/api/v1/events` is stripped
before the path is appended, so one config value can feed this SDK and a browser
SDK without either of them 404ing. (It used to produce
`/api/v1/events/api/v1/events`, which 404s on every event and reported success.)

### Knowing when events are rejected

**Delivery is best-effort, not guaranteed — but it is no longer silent.** A
non-2xx response or a failed request is counted, and either handed to `OnError`
or, if you set no hook, written to the standard logger once per `Init`:

```go
funnelbarn.Init(funnelbarn.Options{
    // ...
    OnError: func(e funnelbarn.Event, err error) {
        slog.Error("funnelbarn rejected an event", "name", e.Name, "err", err)
    },
})

funnelbarn.Rejected() // cumulative since the last Init
```

This exists because it didn't. `send` used to discard the status code, so a
`404` (wrong endpoint), a `401` (wrong key) and a `403` (key minted for another
project) all returned success — a whole app's server-side events went nowhere
for two months with nothing logged, counted or returned ([#237][issue-237]).

`Rejected()` is the number to alert on. A count that tracks everything you send
is a configuration bug, not a blip: 4xx will not clear on its own, and the SDK
does not retry.

`OnError` runs on the SDK's background goroutine, once per failed event — it may
block, but a slow hook stalls delivery and fills the queue.

[issue-237]: https://github.com/webwiebe/funnelbarn/issues/237

### Knowing when events are dropped

Enqueueing is non-blocking: when the buffer (`Options.QueueSize`, default 256)
is full the event is discarded rather than stalling the caller. That is the
right trade for page views and the wrong one for a funnel-critical step — a lost
`converted` looks exactly like a conversion that never happened.

```go
funnelbarn.Init(funnelbarn.Options{
    // ...
    OnDrop: func(e funnelbarn.Event) {
        slog.Warn("funnelbarn dropped an event", "name", e.Name)
    },
})

funnelbarn.Dropped() // cumulative since the last Init
```

`OnDrop` runs on the caller's goroutine — keep it non-blocking.

`Dropped` and `Rejected` are different failures: `Dropped` means you produced
faster than the queue drained, `Rejected` means FunnelBarn would not take what
did get sent.

## Feature flags

```go
// Typed helpers. Every one returns the default you pass on any failure —
// network error, non-200, unknown flag, malformed body. Never an error you
// can accidentally ignore.
enabled := funnelbarn.EvaluateBool(ctx, "cold_email_enabled", false, nil)
cap     := funnelbarn.EvaluateInt(ctx, "cold_email_daily_cap", 25, nil)
mode    := funnelbarn.EvaluateString(ctx, "queue_mode", "slow", nil)

// Full resolution details when you care which variant you got and why.
res := funnelbarn.Evaluate(ctx, "checkout_redesign", false, map[string]any{
    "targeting_key": userID,
    "plan":          "pro",
})
if res.Reason == "TARGETING_MATCH" { /* ... */ }
```

**The fallback direction is the point.** FunnelBarn being unreachable means
*your* default, never "unset". Pass the safe value — `false` for a gate,
a conservative number for a cap — and an outage cannot widen the tap.

**`DISABLED` is not a failure.** A paused or not-yet-activated flag returns the
value the *server* holds (the flag's own default variant), not yours. That is
what makes an auto-registered flag configurable from the dashboard before
anyone activates it.

**Typed helpers matter.** Variant values are arbitrary JSON, so an int-valued
flag arrives as a `float64` through `any`. `EvaluateInt` does the
convert-and-truncate so every caller doesn't write it again.

### Caching

Each evaluation writes a row on the server, so a hot path must not evaluate per
call. The SDK caches in process, keyed by flag key *and* evaluation context
(two contexts can legitimately resolve to different variants).

The server decides the TTL: config flags advertise a polling interval,
experiments advertise `0` — don't cache — because each read is a data point.
`Options.FlagCacheTTL` is the fallback for a server that sends no hint at all:

| `FlagCacheTTL` | Effect |
|---|---|
| unset (`0`) | `DefaultFlagCacheTTL` (60s) when the server sends no hint |
| a duration | that duration when the server sends no hint |
| negative | never cache |

An `ERROR` result is your own default, not a resolved value, so it is never
cached — a blip can't pin the fallback in place for a minute.

### Config values

A value your service polls — a rate limit, a batch size, a daily cap — should
register as a config flag so the server doesn't record an evaluation row per
read:

```go
funnelbarn.Init(funnelbarn.Options{
    APIKey:   "your-api-key",
    Endpoint: "https://funnelbarn.example.com",
    FlagKind: "config", // honoured only when this call auto-registers the flag
})
```

The first evaluation registers the flag holding your default. Change the number
in the dashboard and the fleet picks it up within the cache TTL, no deploy.

### Headers, for anyone debugging a 401

The evaluate endpoint reads `x-funnelbarn-api-key` and `x-funnelbarn-project`.
`Authorization` and `x-api-key` both 401. The SDK sets both for you.
