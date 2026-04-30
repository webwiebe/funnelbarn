package bblog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"
)

// ReportPanic sends a panic event to BugBarn if the endpoint and API key are configured.
// It reads from environment directly so it works even when the normal config is not initialized.
func ReportPanic(endpoint, apiKey string, r any) {
	if endpoint == "" || apiKey == "" {
		return
	}

	body, _ := json.Marshal(map[string]any{
		"event":   "panic",
		"project": "funnelbarn",
		"properties": map[string]any{
			"panic": fmt.Sprint(r),
			"stack": string(debug.Stack()),
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BugBarn-Api-Key", apiKey)
	_, _ = http.DefaultClient.Do(req)
}
