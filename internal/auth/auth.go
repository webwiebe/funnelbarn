package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const HeaderAPIKey = "x-trailpost-api-key"

// DBKeyLookupWithProject looks up an API key's project association and scope.
// Returns (projectID, scope, true, nil) on match, ("", "", false, nil) when not found.
type DBKeyLookupWithProject func(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error)

// DBKeyTouch is called after a successful DB-based auth to update last_used_at.
type DBKeyTouch func(ctx context.Context, keySHA256 string) error

// Authorizer validates API keys against a static hash or the database.
type Authorizer struct {
	apiKeyHash []byte
	dbLookup   DBKeyLookupWithProject
	dbTouch    DBKeyTouch
}

// New creates an Authorizer from a plaintext API key.
func New(apiKey string) *Authorizer {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return &Authorizer{}
	}
	sum := sha256.Sum256([]byte(apiKey))
	return &Authorizer{apiKeyHash: sum[:]}
}

// NewHashed creates an Authorizer from a pre-computed SHA256 hex digest.
func NewHashed(apiKeySHA256 string) (*Authorizer, error) {
	apiKeySHA256 = strings.TrimSpace(apiKeySHA256)
	if apiKeySHA256 == "" {
		return &Authorizer{}, nil
	}
	decoded, err := hex.DecodeString(apiKeySHA256)
	if err != nil {
		return nil, err
	}
	if len(decoded) != sha256.Size {
		return nil, errors.New("api key hash must be a sha256 hex digest")
	}
	return &Authorizer{apiKeyHash: decoded}, nil
}

// WithDBLookup returns a copy of the Authorizer that also accepts DB keys.
func (a *Authorizer) WithDBLookup(lookup DBKeyLookupWithProject, touch DBKeyTouch) *Authorizer {
	if a == nil {
		return &Authorizer{dbLookup: lookup, dbTouch: touch}
	}
	return &Authorizer{apiKeyHash: a.apiKeyHash, dbLookup: lookup, dbTouch: touch}
}

// Enabled returns true when at least one auth mechanism is configured.
func (a *Authorizer) Enabled() bool {
	if a == nil {
		return false
	}
	return len(a.apiKeyHash) == sha256.Size || a.dbLookup != nil
}

// ValidWithProject checks the provided key and returns the associated project ID and scope.
// For env-var static keys, projectID="" and scope="full" are returned (global/admin access).
// Returns (projectID, scope, true) on success, ("", "", false) on failure.
func (a *Authorizer) ValidWithProject(ctx context.Context, provided string) (projectID string, scope string, ok bool) {
	if a == nil || !a.Enabled() {
		return "", "full", true
	}

	provided = strings.TrimSpace(provided)
	sum := sha256.Sum256([]byte(provided))
	hexSum := hex.EncodeToString(sum[:])

	// Check static env-var hash first (global access, no project binding).
	if len(a.apiKeyHash) == sha256.Size {
		if subtle.ConstantTimeCompare(sum[:], a.apiKeyHash) == 1 {
			return "", "full", true
		}
	}

	// Check DB-stored keys.
	if a.dbLookup != nil {
		pid, sc, found, err := a.dbLookup(ctx, hexSum)
		if err == nil && found {
			if a.dbTouch != nil {
				_ = a.dbTouch(ctx, hexSum)
			}
			return pid, sc, true
		}
	}

	return "", "", false
}

// --------------------------------------------------------------------------
// User authentication
// --------------------------------------------------------------------------

// UserAuthenticator validates username/password for human logins.
type UserAuthenticator struct {
	username     string
	passwordHash []byte
}

// NewUserAuthenticator creates a UserAuthenticator from credentials.
func NewUserAuthenticator(username, password, passwordBcrypt string) (*UserAuthenticator, error) {
	username = strings.TrimSpace(username)
	passwordBcrypt = strings.TrimSpace(passwordBcrypt)
	if username == "" || (password == "" && passwordBcrypt == "") {
		return &UserAuthenticator{}, nil
	}
	if passwordBcrypt != "" {
		return &UserAuthenticator{username: username, passwordHash: []byte(passwordBcrypt)}, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &UserAuthenticator{username: username, passwordHash: hash}, nil
}

// Enabled returns true when credentials are configured.
func (a *UserAuthenticator) Enabled() bool {
	return a != nil && a.username != "" && len(a.passwordHash) > 0
}

// Valid returns true if username and password match.
func (a *UserAuthenticator) Valid(username, password string) bool {
	if a == nil || !a.Enabled() {
		return true
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) != 1 {
		return false
	}
	return bcrypt.CompareHashAndPassword(a.passwordHash, []byte(password)) == nil
}

// Username returns the configured admin username.
func (a *UserAuthenticator) Username() string {
	if a == nil {
		return ""
	}
	return a.username
}

// --------------------------------------------------------------------------
// Session management
// --------------------------------------------------------------------------

// SessionManager creates and validates HMAC-signed session tokens.
type SessionManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

type sessionClaims struct {
	Username string `json:"u"`
	Expires  int64  `json:"e"`
	Nonce    string `json:"n"`
}

// NewSessionManager creates a SessionManager with the given secret and TTL.
func NewSessionManager(secret string, ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		secret = randomSecret()
	}
	return &SessionManager{
		secret: []byte(secret),
		ttl:    ttl,
		now:    time.Now,
	}
}

// Create issues a new signed session token for username.
func (m *SessionManager) Create(username string) (string, time.Time, error) {
	if m == nil {
		return "", time.Time{}, errors.New("session manager is nil")
	}
	expires := m.now().UTC().Add(m.ttl)
	claims := sessionClaims{
		Username: username,
		Expires:  expires.Unix(),
		Nonce:    randomSecret(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	signature := sign(m.secret, payload)
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature)
	return token, expires, nil
}

// Valid validates a session token and returns the username.
func (m *SessionManager) Valid(token string) (string, bool) {
	if m == nil || strings.TrimSpace(token) == "" {
		return "", false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	expected := sign(m.secret, payload)
	if hmac.Equal(signature, expected) {
		var claims sessionClaims
		if err := json.Unmarshal(payload, &claims); err != nil {
			return "", false
		}
		if claims.Expires <= m.now().UTC().Unix() || claims.Username == "" {
			return "", false
		}
		return claims.Username, true
	}
	return "", false
}

// SessionCookie returns an HttpOnly session cookie.
func SessionCookie(token string, expires time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     "trailpost_session",
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}
}

// ClearSessionCookie returns a cookie that expires the session.
func ClearSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     "trailpost_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}
}

// CSRFCookie returns the companion CSRF cookie for a session token.
// HttpOnly=false so JavaScript can read and attach it as X-Trailpost-CSRF.
func CSRFCookie(sessionToken string, expires time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     "trailpost_csrf",
		Value:    CSRFToken(sessionToken),
		Path:     "/",
		Expires:  expires,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

// ClearCSRFCookie clears the CSRF cookie on logout.
func ClearCSRFCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     "trailpost_csrf",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

// CSRFToken derives a CSRF token from a session token using HMAC-SHA256.
func CSRFToken(sessionToken string) string {
	mac := hmac.New(sha256.New, []byte("csrf"))
	mac.Write([]byte(sessionToken))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

func sign(secret, payload []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return mac.Sum(nil)
}

func randomSecret() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
