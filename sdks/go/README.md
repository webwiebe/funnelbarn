# funnelbarn-go

Go SDK for [FunnelBarn](https://github.com/wiebe-xyz/funnelbarn) — self-hosted web analytics.

## Installation

```bash
go get github.com/wiebe-xyz/funnelbarn-go
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
