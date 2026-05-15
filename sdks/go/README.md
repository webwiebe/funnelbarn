# funnelbarn-go

Go SDK for [FunnelBarn](https://github.com/wiebe-xyz/funnelbarn) — self-hosted web analytics.

## Installation

> **Not yet a standalone Go module.** The SDK lives in the funnelbarn monorepo at `sdks/go/`. The `go get` path advertised in earlier drafts (`github.com/wiebe-xyz/funnelbarn-go`) does not resolve. Until a separate module is published, vendor `sdks/go/` into your project or use a `replace` directive against a local checkout:

```bash
# in your project's go.mod
replace github.com/webwiebe/funnelbarn/sdks/go => /path/to/funnelbarn/sdks/go
```

## Usage

```go
package main

import (
    "github.com/wiebe-xyz/funnelbarn-go"
)

func main() {
    funnelbarn.Init(funnelbarn.Options{
        APIKey:      "your-api-key",
        Endpoint:    "https://funnelbarn.example.com",
        ProjectName: "my-app",
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
