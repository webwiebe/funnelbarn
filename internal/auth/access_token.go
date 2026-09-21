package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	oidcv3 "github.com/coreos/go-oidc/v3/oidc"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/oauth2"

	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

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
