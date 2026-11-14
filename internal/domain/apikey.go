package domain

import "time"

// APIKey represents a stored API key.
type APIKey struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"-"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
	// LastUsedAt is when a request last authenticated with this key. Nil means
	// never used — which is how you tell a token that was issued and forgotten
	// from one something depends on before revoking it.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// APIKeyScopeFull allows full API access.
const APIKeyScopeFull = "full"

// APIKeyScopeIngest allows only event ingest.
const APIKeyScopeIngest = "ingest"

// APIKeyScopeFlagsRead allows reading a project's feature flags without a
// dashboard session, so a service can report what the project actually holds
// rather than what its own client resolved — "the flag is off" and "the flag
// does not exist and you are seeing your own default" are very different
// answers when the question is whether mail is about to go out.
const APIKeyScopeFlagsRead = "flags:read"

// APIKeyScopeAnalyticsRead allows reading a project's analytics — event
// counts, the event catalog, funnels and funnel-step conversion — without a
// dashboard session, so a scheduled job can publish a weekly readout instead
// of asking someone to open the dashboard and copy numbers out of it. It is
// read-only by construction: no route that mutates anything accepts it.
const APIKeyScopeAnalyticsRead = "analytics:read"

// APIKeyScopeFlagsWrite additionally allows updating an existing flag
// (enable / disable / change its value). It deliberately does not allow
// creating or deleting flags: auto-registration already covers creation, and a
// token that can turn an outbound-email gate on should not also be able to
// delete the gate.
const APIKeyScopeFlagsWrite = "flags:write"

// APIKeyScopeEvaluatesFlags reports whether a key with this scope is served
// from the in-memory key snapshot on the flag evaluate path. SDKs evaluate
// with ingest keys; analytics:read keys keep the live lookup.
func APIKeyScopeEvaluatesFlags(scope string) bool {
	switch scope {
	case APIKeyScopeIngest, APIKeyScopeFull, APIKeyScopeFlagsRead, APIKeyScopeFlagsWrite:
		return true
	}
	return false
}
