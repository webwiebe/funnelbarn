# Spec 004: Replace Panics with Proper Error Propagation

## Goal
`generateUUID()` and `generateUUIDLocal()` both call `panic()` on rand failure. Crypto/rand failure is extremely rare but crashing the server is never acceptable. Return errors instead.

## Files to modify
- `internal/storage/helpers.go` — change `generateUUID()` signature to `(string, error)`
- `internal/worker/uuid.go` — change `generateUUIDLocal()` signature to `(string, error)`
- `internal/worker/worker.go` — update callers of `generateUUIDLocal()`
- `internal/ingest/handler.go` — update `generateIngestID()` if it calls these
- `internal/storage/` — update any callers of `generateUUID()` in `db.go`, `events.go`, `sessions.go`, `abtests.go`, `funnels.go`

## Current code (internal/storage/helpers.go)
```go
func generateUUID() string {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        panic(err)
    }
    // RFC4122 version 4
    b[6] = (b[6] & 0x0f) | 0x40
    b[8] = (b[8] & 0x3f) | 0x80
    return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", ...)
}
```

## Target signature
```go
func generateUUID() (string, error)
func generateUUIDLocal() (string, error)
```

## Caller update pattern
Each caller currently does:
```go
id := generateUUID()
```
Should become:
```go
id, err := generateUUID()
if err != nil {
    return fmt.Errorf("generate uuid: %w", err)
}
```

Propagate errors up to HTTP handlers which return 500.

## Acceptance criteria
- No `panic()` calls remain in either UUID function
- All callers updated — `go build ./...` passes with no errors
- `go test ./...` passes
