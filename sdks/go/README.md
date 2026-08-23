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
