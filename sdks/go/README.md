# trailpost-go

Go SDK for [Trailpost](https://github.com/wiebe-xyz/trailpost) — self-hosted web analytics.

## Installation

```bash
go get github.com/wiebe-xyz/trailpost-go
```

## Usage

```go
package main

import (
    "github.com/wiebe-xyz/trailpost-go"
)

func main() {
    trailpost.Init(trailpost.Options{
        APIKey:      "your-api-key",
        Endpoint:    "https://analytics.example.com",
        ProjectName: "my-app",
    })
    defer trailpost.Shutdown(5 * time.Second)

    // Track a page view
    trailpost.Page("https://example.com/pricing", "https://google.com")

    // Track a custom event
    trailpost.Track("signup", map[string]any{
        "plan": "pro",
    })
}
```
