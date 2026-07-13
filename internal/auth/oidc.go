// Package auth — the OIDC login adapter used when FUNNELBARN_OIDC_* env vars
// are set. FunnelBarn is a confidential OIDC relying party: the browser only
// ever holds an opaque session handle, while the iambarn tokens live in the
// server-side web_sessions table and are renewed via the refresh_token grant.
package auth

import (
	"context"
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

	ctx, span := tracing.StartSpan(ctx, "oidc.exchange",
		attribute.String("oidc.issuer", c.cfg.Issuer),
		attribute.String("oidc.client_id", c.cfg.ClientID),
	)
	defer span.End()

	start := time.Now()
	tok, err := c.oauth.Exchange(ctx, code)
	metrics.OIDCRequestDuration.WithLabelValues("exchange").Observe(time.Since(start).Seconds())
	if err != nil {
		err = fmt.Errorf("oidc: token exchange: %w", err)
		tracing.RecordError(span, err)
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		return OIDCClaims{}, err
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		err := errors.New("oidc: token response missing id_token")
		tracing.RecordError(span, err)
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		return OIDCClaims{}, err
	}
	idToken, err := c.verifier.Verify(ctx, rawID)
	if err != nil {
		err = fmt.Errorf("oidc: verify id_token: %w", err)
		tracing.RecordError(span, err)
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		return OIDCClaims{}, err
	}
	if idToken.Nonce != nonce {
		err := errors.New("oidc: nonce mismatch")
		tracing.RecordError(span, err)
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		return OIDCClaims{}, err
	}
	var claims OIDCClaims
	if err := idToken.Claims(&claims); err != nil {
		err = fmt.Errorf("oidc: decode claims: %w", err)
		tracing.RecordError(span, err)
		metrics.OIDCRequests.WithLabelValues("exchange", "error").Inc()
		return OIDCClaims{}, err
	}
	metrics.OIDCRequests.WithLabelValues("exchange", "success").Inc()
	return claims, nil
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

	ctx, span := tracing.StartSpan(ctx, "oidc.discover",
		attribute.String("oidc.issuer", c.cfg.Issuer),
	)
	defer span.End()

	start := time.Now()
	prov, err := oidcv3.NewProvider(ctx, strings.TrimRight(c.cfg.Issuer, "/"))
	metrics.OIDCRequestDuration.WithLabelValues("discover").Observe(time.Since(start).Seconds())
	if err != nil {
		err = fmt.Errorf("oidc: discover issuer %q: %w", c.cfg.Issuer, err)
		tracing.RecordError(span, err)
		metrics.OIDCRequests.WithLabelValues("discover", "error").Inc()
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
