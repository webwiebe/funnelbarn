// Package auth — the OIDC login adapter used when FUNNELBARN_OIDC_* env vars
// are set. FunnelBarn is a confidential OIDC relying party: the browser only
// ever holds an opaque session handle, while the iambarn tokens live in the
// server-side web_sessions table and are renewed via the refresh_token grant.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	oidcv3 "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// OIDCConfig is the static configuration for the OIDC login flow.
type OIDCConfig struct {
	Issuer        string // iambarn issuer URL, e.g. https://iam.wiebe.xyz
	ClientID      string
	ClientSecret  string
	RedirectURL   string // e.g. https://funnelbarn.wiebe.xyz/api/v1/oidc/callback
	RequiredGroup string // group slug that grants access; bypass roles always win

	// PostLogoutRedirectURI is where iambarn returns the browser after an
	// RP-initiated /oauth2/end-session logout. It must be registered on the
	// client's post-logout allowlist.
	PostLogoutRedirectURI string
}

// Enabled reports whether all four required fields are present.
func (c OIDCConfig) Enabled() bool {
	return c.Issuer != "" && c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != ""
}

// OIDCClient lazily discovers the issuer's OpenID configuration and exposes
// the primitives the HTTP layer needs: an authorize URL, a code-exchange, a
// refresh grant, token revocation, RP-initiated logout, and back-channel
// logout-token verification.
type OIDCClient struct {
	cfg     OIDCConfig
	timeout time.Duration

	mu       sync.Mutex
	provider *oidcv3.Provider
	verifier *oidcv3.IDTokenVerifier
	oauth    *oauth2.Config
	// Optional endpoints from discovery, with conventional iambarn fallbacks
	// when the discovery document omits them.
	revocationEndpoint string
	endSessionEndpoint string

	// accessVerifiers caches one access-token verifier per audience (the MCP
	// resource URL). Guarded by mu.
	accessVerifiers map[string]*oidcv3.IDTokenVerifier

	// userinfoMu guards userinfoCache: groups/roles fetched from the userinfo
	// endpoint for access tokens that carry no groups claim, keyed by the
	// SHA-256 of the token and kept until the token expires.
	userinfoMu    sync.Mutex
	userinfoCache map[[32]byte]userinfoEntry
}

// NewOIDCClient returns a client. Discovery is deferred to the first call so
// that an unreachable issuer at startup does not crash the process.
func NewOIDCClient(cfg OIDCConfig) *OIDCClient {
	if cfg.RequiredGroup == "" {
		cfg.RequiredGroup = "funnelbarn-users"
	}
	return &OIDCClient{cfg: cfg, timeout: 10 * time.Second}
}

// Config returns the static configuration. Used to expose non-secret bits
// (issuer, client id) via client-config.
func (c *OIDCClient) Config() OIDCConfig { return c.cfg }

// AuthorizeURL builds the URL the browser should be redirected to. The caller
// is responsible for storing state + nonce (and the PKCE verifier) in
// short-lived cookies and matching them on callback. verifier is the PKCE
// code_verifier from oauth2.GenerateVerifier; its S256 challenge is sent with
// the authorization request even though FunnelBarn is a confidential client
// (PKCE-everywhere hardens against code injection at zero cost).
func (c *OIDCClient) AuthorizeURL(state, nonce, verifier string) (string, error) {
	if err := c.ensureReady(context.Background()); err != nil {
		return "", err
	}
	opts := []oauth2.AuthCodeOption{oidcv3.Nonce(nonce)}
	if verifier != "" {
		opts = append(opts, oauth2.S256ChallengeOption(verifier))
	}
	return c.oauth.AuthCodeURL(state, opts...), nil
}

// ExchangeResult holds the parsed claims and the raw OIDC tokens.
type ExchangeResult struct {
	Claims       OIDCClaims
	IDToken      string // raw id_token; kept for id_token_hint at logout
	AccessToken  string
	RefreshToken string    // empty if the client/grant did not include offline_access
	ExpiresAt    time.Time // zero if the token response omitted expires_in
}

// ExchangeFull swaps an authorization code for tokens, verifies the ID token,
// and returns both the parsed claims (including the IdP session id `sid`) and
// the raw tokens in one call. verifier is the PKCE code_verifier that produced
// the challenge sent at authorize time.
func (c *OIDCClient) ExchangeFull(ctx context.Context, code, nonce, verifier string) (ExchangeResult, error) {
	if err := c.ensureReady(ctx); err != nil {
		return ExchangeResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var opts []oauth2.AuthCodeOption
	if verifier != "" {
		opts = append(opts, oauth2.VerifierOption(verifier))
	}
	ctx, span := tracing.StartSpan(ctx, "oidc.exchange")
	defer span.End()
	start := time.Now()

	tok, err := c.oauth.Exchange(ctx, code, opts...)
	metrics.OIDCRequestDuration.WithLabelValues("exchange").Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		err = fmt.Errorf("oidc: token exchange: %w", err)
		tracing.RecordError(span, err)
		return ExchangeResult{}, err
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		err := errors.New("oidc: token response missing id_token")
		tracing.RecordError(span, err)
		return ExchangeResult{}, err
	}
	idToken, err := c.verifier.Verify(ctx, rawID)
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		err = fmt.Errorf("oidc: verify id_token: %w", err)
		tracing.RecordError(span, err)
		return ExchangeResult{}, err
	}
	if idToken.Nonce != nonce {
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		err := errors.New("oidc: nonce mismatch")
		tracing.RecordError(span, err)
		return ExchangeResult{}, err
	}
	var claims OIDCClaims
	if err := idToken.Claims(&claims); err != nil {
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		err = fmt.Errorf("oidc: decode claims: %w", err)
		tracing.RecordError(span, err)
		return ExchangeResult{}, err
	}
	metrics.OIDCRequests.WithLabelValues("exchange", "success").Inc()
	span.SetAttributes(attribute.String("oidc.sub", claims.Subject))
	return ExchangeResult{
		Claims:       claims,
		IDToken:      rawID,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.Expiry,
	}, nil
}

// ErrRefreshInvalid indicates iambarn rejected the refresh_token outright
// (invalid_grant: revoked, expired, already-rotated/replayed, or the user was
// suspended). The caller must not retry the same token — it is dead — and
// must destroy the local session immediately.
var ErrRefreshInvalid = errors.New("oidc: refresh token invalid")

// RefreshedTokens holds the renewed access/refresh token pair from a
// refresh_token grant. Iambarn rotates the refresh token on every use, so
// RefreshToken here always replaces whatever was previously stored.
type RefreshedTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	// IDToken and Claims are set when the token response included a fresh
	// id_token (iambarn does on refresh). Claims lets the caller re-snapshot
	// groups/roles so central role changes propagate on the next refresh
	// instead of the next login.
	IDToken string
	Claims  *OIDCClaims
}

// Refresh exchanges a refresh_token for a new access/refresh token pair.
//
// Refresh tokens are single-use: iambarn invalidates the one sent here the
// moment it issues the replacement. Callers MUST NOT invoke Refresh
// concurrently with the same refreshToken — a second call with an
// already-rotated token is treated as a replay and revokes the whole token
// family. Callers are responsible for serializing refreshes per session
// (e.g. via singleflight keyed on the session id hash).
func (c *OIDCClient) Refresh(ctx context.Context, refreshToken string) (RefreshedTokens, error) {
	if err := c.ensureReady(ctx); err != nil {
		return RefreshedTokens{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := tracing.StartSpan(ctx, "oidc.refresh")
	defer span.End()
	start := time.Now()

	tok, err := c.oauth.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	metrics.OIDCRequestDuration.WithLabelValues("refresh").Observe(time.Since(start).Seconds())
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
			metrics.OIDCRequests.WithLabelValues("refresh", "invalid_grant").Inc()
			tracing.RecordError(span, ErrRefreshInvalid)
			return RefreshedTokens{}, ErrRefreshInvalid
		}
		metrics.OIDCRequests.WithLabelValues("refresh", "error").Inc()
		err = fmt.Errorf("oidc: refresh token: %w", err)
		tracing.RecordError(span, err)
		return RefreshedTokens{}, err
	}
	metrics.OIDCRequests.WithLabelValues("refresh", "success").Inc()
	// golang.org/x/oauth2 falls back to the refresh_token we sent if the
	// response omits one (its accommodation for non-rotating providers), so
	// tok.RefreshToken is never empty here on a successful response —
	// iambarn always rotates, so in practice this is always the new token.
	refreshed := RefreshedTokens{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.Expiry,
	}
	// A refresh response may carry a fresh id_token with up-to-date claims.
	// Verify it like the login-time one (signature/issuer/audience), except
	// the nonce — refresh grants have no browser round-trip to bind one to.
	if rawID, ok := tok.Extra("id_token").(string); ok && rawID != "" {
		if idToken, verr := c.verifier.Verify(ctx, rawID); verr == nil {
			var claims OIDCClaims
			if cerr := idToken.Claims(&claims); cerr == nil {
				refreshed.IDToken = rawID
				refreshed.Claims = &claims
			}
		}
	}
	return refreshed, nil
}

// RevokeRefreshToken revokes a refresh token at the issuer's revocation
// endpoint (RFC 7009), authenticating as the client. Used best-effort at
// logout so the token family dies server-side instead of merely being
// forgotten locally.
func (c *OIDCClient) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	if err := c.ensureReady(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := tracing.StartSpan(ctx, "oidc.revoke")
	defer span.End()
	start := time.Now()

	form := url.Values{}
	form.Set("token", refreshToken)
	form.Set("token_type_hint", "refresh_token")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.revocationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("revoke", "error").Inc()
		tracing.RecordError(span, err)
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	metrics.OIDCRequestDuration.WithLabelValues("revoke").Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("revoke", "error").Inc()
		err = fmt.Errorf("oidc: revoke refresh token: %w", err)
		tracing.RecordError(span, err)
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		metrics.OIDCRequests.WithLabelValues("revoke", "error").Inc()
		err := fmt.Errorf("oidc: revoke refresh token: status %d", resp.StatusCode)
		tracing.RecordError(span, err)
		return err
	}
	metrics.OIDCRequests.WithLabelValues("revoke", "success").Inc()
	return nil
}

// EndSessionURL builds the RP-initiated logout URL on the issuer
// (OpenID Connect RP-Initiated Logout 1.0): id_token_hint identifies the IdP
// session to end, client_id + post_logout_redirect_uri bring the browser back
// to the configured post-logout landing.
func (c *OIDCClient) EndSessionURL(idTokenHint string) (string, error) {
	if err := c.ensureReady(context.Background()); err != nil {
		return "", err
	}
	params := url.Values{}
	if idTokenHint != "" {
		params.Set("id_token_hint", idTokenHint)
	}
	params.Set("client_id", c.cfg.ClientID)
	if c.cfg.PostLogoutRedirectURI != "" {
		params.Set("post_logout_redirect_uri", c.cfg.PostLogoutRedirectURI)
	}
	return c.endSessionEndpoint + "?" + params.Encode(), nil
}

// backchannelLogoutEvent is the member the `events` claim of a logout token
// must contain (OIDC Back-Channel Logout 1.0 §2.4).
const backchannelLogoutEvent = "http://schemas.openid.net/event/backchannel-logout"

// logoutTokenMaxAge bounds how old a logout token's iat may be. Tokens are
// minted per delivery attempt, so anything older is a replay.
const logoutTokenMaxAge = 2 * time.Minute

// LogoutClaims carries the session-targeting claims of a validated
// back-channel logout token. At least one of Subject/SessionID is non-empty.
type LogoutClaims struct {
	Subject   string
	SessionID string
}

// VerifyLogoutToken validates a back-channel logout token per OIDC
// Back-Channel Logout 1.0: signature + issuer + audience (= our client_id)
// via the issuer's JWKS, iat within logoutTokenMaxAge, the mandatory
// backchannel-logout `events` member, the mandatory absence of `nonce`, and
// the presence of at least one of sub/sid.
func (c *OIDCClient) VerifyLogoutToken(ctx context.Context, raw string) (LogoutClaims, error) {
	if err := c.ensureReady(ctx); err != nil {
		return LogoutClaims{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := tracing.StartSpan(ctx, "oidc.verify_logout_token")
	defer span.End()
	start := time.Now()

	tok, err := c.verifier.Verify(ctx, raw)
	metrics.OIDCRequestDuration.WithLabelValues("verify_logout_token").Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("verify_logout_token", "error").Inc()
		err = fmt.Errorf("oidc: verify logout token: %w", err)
		tracing.RecordError(span, err)
		return LogoutClaims{}, err
	}
	var claims struct {
		Sub    string                     `json:"sub"`
		Sid    string                     `json:"sid"`
		Iat    int64                      `json:"iat"`
		Nonce  *string                    `json:"nonce"`
		Events map[string]json.RawMessage `json:"events"`
	}
	if err := tok.Claims(&claims); err != nil {
		metrics.OIDCRequests.WithLabelValues("verify_logout_token", "error").Inc()
		err = fmt.Errorf("oidc: decode logout token claims: %w", err)
		tracing.RecordError(span, err)
		return LogoutClaims{}, err
	}
	if claims.Nonce != nil {
		// The spec REQUIRES rejecting logout tokens with a nonce — it is what
		// distinguishes them from ID tokens, blocking cross-protocol replay.
		metrics.OIDCRequests.WithLabelValues("verify_logout_token", "error").Inc()
		err := errors.New("oidc: logout token must not contain nonce")
		tracing.RecordError(span, err)
		return LogoutClaims{}, err
	}
	if _, ok := claims.Events[backchannelLogoutEvent]; !ok {
		metrics.OIDCRequests.WithLabelValues("verify_logout_token", "error").Inc()
		err := errors.New("oidc: logout token missing backchannel-logout event")
		tracing.RecordError(span, err)
		return LogoutClaims{}, err
	}
	iat := time.Unix(claims.Iat, 0)
	now := time.Now()
	if claims.Iat == 0 || now.Sub(iat) > logoutTokenMaxAge || iat.Sub(now) > logoutTokenMaxAge {
		metrics.OIDCRequests.WithLabelValues("verify_logout_token", "error").Inc()
		err := errors.New("oidc: logout token iat outside acceptance window")
		tracing.RecordError(span, err)
		return LogoutClaims{}, err
	}
	if claims.Sub == "" && claims.Sid == "" {
		metrics.OIDCRequests.WithLabelValues("verify_logout_token", "error").Inc()
		err := errors.New("oidc: logout token has neither sub nor sid")
		tracing.RecordError(span, err)
		return LogoutClaims{}, err
	}
	metrics.OIDCRequests.WithLabelValues("verify_logout_token", "success").Inc()
	span.SetAttributes(attribute.String("oidc.sid", claims.Sid))
	return LogoutClaims{Subject: claims.Sub, SessionID: claims.Sid}, nil
}

// Allowed returns true if the claims grant access to this barn.
// Owner/organization_admin/operator roles bypass the group check.
func (c *OIDCClient) Allowed(claims OIDCClaims) bool {
	for _, role := range claims.Roles {
		switch role {
		case "owner", "organization_admin", "operator":
			return true
		}
	}
	for _, g := range claims.Groups {
		if g == c.cfg.RequiredGroup {
			return true
		}
	}
	return false
}

// ensureReady performs the one-time discovery + provider wiring. Safe for
// concurrent callers.
func (c *OIDCClient) ensureReady(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := tracing.StartSpan(ctx, "oidc.discover", attribute.String("oidc.issuer", c.cfg.Issuer))
	defer span.End()
	start := time.Now()

	issuer := strings.TrimRight(c.cfg.Issuer, "/")
	prov, err := oidcv3.NewProvider(ctx, issuer)
	metrics.OIDCRequestDuration.WithLabelValues("discover").Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("discover", "error").Inc()
		err = fmt.Errorf("oidc: discover issuer %q: %w", c.cfg.Issuer, err)
		tracing.RecordError(span, err)
		return err
	}
	metrics.OIDCRequests.WithLabelValues("discover", "success").Inc()
	c.provider = prov
	// Revocation + end-session endpoints are optional discovery fields; fall
	// back to iambarn's conventional paths when the document omits them.
	var extra struct {
		RevocationEndpoint string `json:"revocation_endpoint"`
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	_ = prov.Claims(&extra)
	c.revocationEndpoint = extra.RevocationEndpoint
	if c.revocationEndpoint == "" {
		c.revocationEndpoint = issuer + "/oauth2/revoke"
	}
	c.endSessionEndpoint = extra.EndSessionEndpoint
	if c.endSessionEndpoint == "" {
		c.endSessionEndpoint = issuer + "/oauth2/end-session"
	}
	c.verifier = prov.Verifier(&oidcv3.Config{ClientID: c.cfg.ClientID})
	c.oauth = &oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint:     prov.Endpoint(),
		RedirectURL:  c.cfg.RedirectURL,
		// offline_access asks iambarn for a refresh_token alongside the
		// short-lived (15m) access_token, so the session can be renewed
		// silently instead of forcing a full re-login every 15 minutes.
		// Requesting it here is required even though it's allowed on the
		// client record — iambarn only grants what's both allowed AND
		// explicitly requested.
		Scopes: []string{oidcv3.ScopeOpenID, "profile", "email", "offline_access"},
	}
	return nil
}

// ErrIssuerUnavailable wraps a failure to reach the issuer's discovery
// document. It is a server-side problem, so callers must not report it to the
// client as a bad token.
var ErrIssuerUnavailable = errors.New("oidc: issuer unavailable")

// AccessTokenClaims are the claims of a verified IAMBarn access token.
type AccessTokenClaims struct {
	OIDCClaims
	Scope    string    // space-delimited granted scopes
	TokenUse string    // "access_token" for an access token
	Expiry   time.Time // exp
}

// userinfoEntry is a cached userinfo lookup for one access token.
type userinfoEntry struct {
	groups  []string
	roles   []string
	expires time.Time
}

// maxUserinfoCache bounds the userinfo cache; expired entries are pruned first.
const maxUserinfoCache = 1024

// VerifyAccessToken verifies an IAMBarn access token (a JWT signed with a key
// from the issuer's JWKS) for audience: signature, issuer, expiry, and that
// the aud claim contains audience. A token minted for the dashboard client has
// aud=[client_id] and fails here. It does not check token_use or group access;
// the caller does, with Allowed.
//
// When the token has no groups claim at all, groups (and roles, when also
// absent) are read from the userinfo endpoint with the token itself, and
// cached per token until it expires.
func (c *OIDCClient) VerifyAccessToken(ctx context.Context, raw, audience string) (AccessTokenClaims, error) {
	if audience == "" {
		return AccessTokenClaims{}, errors.New("oidc: access token audience not configured")
	}
	if err := c.ensureReady(ctx); err != nil {
		return AccessTokenClaims{}, fmt.Errorf("%w: %w", ErrIssuerUnavailable, err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := tracing.StartSpan(ctx, "oidc.verify_access_token")
	defer span.End()
	start := time.Now()

	tok, err := c.accessVerifier(audience).Verify(ctx, raw)
	metrics.OIDCRequestDuration.WithLabelValues("verify_access_token").Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("verify_access_token", "error").Inc()
		err = fmt.Errorf("oidc: verify access token: %w", err)
		tracing.RecordError(span, err)
		return AccessTokenClaims{}, err
	}
	// groups/roles are decoded twice: into the claims, and as raw presence
	// markers, because an absent groups claim sends us to userinfo while an
	// empty one does not.
	var raws struct {
		OIDCClaims
		Scope    string `json:"scope"`
		TokenUse string `json:"token_use"`
	}
	var present struct {
		Groups json.RawMessage `json:"groups"`
		Roles  json.RawMessage `json:"roles"`
	}
	if err := errors.Join(tok.Claims(&raws), tok.Claims(&present)); err != nil {
		metrics.OIDCRequests.WithLabelValues("verify_access_token", "error").Inc()
		err = fmt.Errorf("oidc: decode access token claims: %w", err)
		tracing.RecordError(span, err)
		return AccessTokenClaims{}, err
	}
	claims := AccessTokenClaims{
		OIDCClaims: raws.OIDCClaims,
		Scope:      raws.Scope,
		TokenUse:   raws.TokenUse,
		Expiry:     tok.Expiry,
	}
	if present.Groups == nil {
		groups, roles, err := c.userinfoGroups(ctx, raw, tok.Expiry)
		if err != nil {
			metrics.OIDCRequests.WithLabelValues("verify_access_token", "error").Inc()
			tracing.RecordError(span, err)
			return AccessTokenClaims{}, err
		}
		claims.Groups = groups
		if present.Roles == nil {
			claims.Roles = roles
		}
	}
	metrics.OIDCRequests.WithLabelValues("verify_access_token", "success").Inc()
	span.SetAttributes(attribute.String("oidc.sub", claims.Subject))
	return claims, nil
}

// accessVerifier returns the cached verifier for audience. ensureReady must
// have succeeded.
func (c *OIDCClient) accessVerifier(audience string) *oidcv3.IDTokenVerifier {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.accessVerifiers[audience]; ok {
		return v
	}
	if c.accessVerifiers == nil {
		c.accessVerifiers = make(map[string]*oidcv3.IDTokenVerifier)
	}
	v := c.provider.Verifier(&oidcv3.Config{
		ClientID: audience,
		// IAMBarn signs access tokens with Ed25519; the asymmetric fallbacks
		// keep a key rotation to another algorithm from locking everyone out.
		SupportedSigningAlgs: []string{oidcv3.EdDSA, oidcv3.ES256, oidcv3.RS256},
	})
	c.accessVerifiers[audience] = v
	return v
}

// userinfoGroups reads groups and roles for an access token from the userinfo
// endpoint, cached per token until exp.
func (c *OIDCClient) userinfoGroups(ctx context.Context, raw string, exp time.Time) ([]string, []string, error) {
	key := sha256.Sum256([]byte(raw))
	now := time.Now()
	c.userinfoMu.Lock()
	if e, ok := c.userinfoCache[key]; ok && now.Before(e.expires) {
		c.userinfoMu.Unlock()
		return e.groups, e.roles, nil
	}
	c.userinfoMu.Unlock()

	start := time.Now()
	info, err := c.provider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: raw, TokenType: "Bearer"}))
	metrics.OIDCRequestDuration.WithLabelValues("userinfo").Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.OIDCRequests.WithLabelValues("userinfo", "error").Inc()
		return nil, nil, fmt.Errorf("oidc: userinfo: %w", err)
	}
	var extra struct {
		Groups []string `json:"groups"`
		Roles  []string `json:"roles"`
	}
	if err := info.Claims(&extra); err != nil {
		metrics.OIDCRequests.WithLabelValues("userinfo", "error").Inc()
		return nil, nil, fmt.Errorf("oidc: decode userinfo: %w", err)
	}
	metrics.OIDCRequests.WithLabelValues("userinfo", "success").Inc()

	c.userinfoMu.Lock()
	defer c.userinfoMu.Unlock()
	if c.userinfoCache == nil {
		c.userinfoCache = make(map[[32]byte]userinfoEntry)
	}
	if len(c.userinfoCache) >= maxUserinfoCache {
		for k, e := range c.userinfoCache {
			if !now.Before(e.expires) {
				delete(c.userinfoCache, k)
			}
		}
		for k := range c.userinfoCache {
			if len(c.userinfoCache) < maxUserinfoCache {
				break
			}
			delete(c.userinfoCache, k)
		}
	}
	c.userinfoCache[key] = userinfoEntry{groups: extra.Groups, roles: extra.Roles, expires: exp}
	return extra.Groups, extra.Roles, nil
}

// OIDCClaims is the subset of ID-token claims this barn cares about.
type OIDCClaims struct {
	Subject           string   `json:"sub"`
	SessionID         string   `json:"sid"` // IdP session id; keys back-channel logout
	Email             string   `json:"email"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
	Roles             []string `json:"roles"`
}

// PreferredName returns the best human-readable identifier from the claims.
func (c OIDCClaims) PreferredName() string {
	for _, v := range []string{c.PreferredUsername, c.Email, c.Name, c.Subject} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
