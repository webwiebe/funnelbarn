package domain

// WebSession is one token-bound server-side browser session. The
// funnelbarn_session cookie holds an opaque handle; IDHash is its SHA-256 hex
// and the row key. OIDC sessions carry the iambarn token set; local
// password/admin sessions leave the token fields empty (stored as NULL).
type WebSession struct {
	IDHash              string
	Username            string
	AuthMethod          string // "oidc" or "local"
	IdpSub              string
	IdpSid              string
	IDToken             string
	AccessToken         string
	RefreshToken        string
	AccessExpiresAt     int64 // unix seconds; 0 = not applicable
	ClaimsJSON          string
	CreatedAt           int64 // unix seconds
	AbsoluteExpiresAt   int64 // unix seconds; hard cap regardless of refreshes
	LastRefreshAt       int64 // unix seconds; 0 = never refreshed
	RefreshFailingSince int64 // unix seconds; 0 = refresh not failing
}
