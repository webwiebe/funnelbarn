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
