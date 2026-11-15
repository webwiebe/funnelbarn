package api

import "github.com/wiebe-xyz/funnelbarn/internal/httpx"

// The handlers in this package answer through the shared helpers in httpx.
var (
	mapServiceError = httpx.MapServiceError
	writeJSON       = httpx.WriteJSON
	jsonError       = httpx.JSONError
	readJSON        = httpx.ReadJSON
)
