package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"

	fbauth "github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// tokenUseAccess is the token_use claim value of an IAMBarn access token. An
// ID token signed by the same key carries "id_token" and must not open MCP.
const tokenUseAccess = "access_token"

// AccessTokenVerifier verifies IAMBarn access tokens and applies the barn's
// group rule. *auth.OIDCClient satisfies it.
type AccessTokenVerifier interface {
	VerifyAccessToken(ctx context.Context, raw, audience string) (fbauth.AccessTokenClaims, error)
	Allowed(claims fbauth.OIDCClaims) bool
}

// UserResolver maps an IAMBarn subject to its FunnelBarn user, creating the
// user on first sight. *repository.Store satisfies it.
type UserResolver interface {
	FindUserByIAMBarnSub(ctx context.Context, sub string) (repository.User, error)
	CreateIAMBarnUser(ctx context.Context, sub, username string) (repository.User, error)
}

// NewTokenVerifier returns the bearer-token verifier for the MCP endpoint. It
// accepts an IAMBarn access token whose audience contains audience (the MCP
// resource URL), whose token_use is access_token and whose user passes the
// barn's group rule. Every rejection wraps auth.ErrInvalidToken, so the client
// gets a 401 with the resource-metadata pointer and can sign in again. users
// may be nil.
func NewTokenVerifier(v AccessTokenVerifier, audience string, users UserResolver, log *slog.Logger) auth.TokenVerifier {
	if log == nil {
		log = slog.Default()
	}
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		claims, err := v.VerifyAccessToken(ctx, token, audience)
		if err != nil {
			if errors.Is(err, fbauth.ErrIssuerUnavailable) {
				// IAMBarn is unreachable: a server-side fault, so a 500 and an
				// Error log (selflog reports it), not a 401 that sends the
				// client into a pointless re-login.
				log.ErrorContext(ctx, "mcp token verification: issuer unavailable", "err", err, "handled", false)
				return nil, errors.New("authorization server unavailable")
			}
			// The details (audience, issuer, expiry) go to the log only; the
			// 401 body says no more than "invalid token".
			log.WarnContext(ctx, "mcp token rejected", "reason", "verify", "err", err)
			return nil, auth.ErrInvalidToken
		}
		if claims.TokenUse != tokenUseAccess {
			log.WarnContext(ctx, "mcp token rejected", "reason", "token_use", "token_use", claims.TokenUse, "sub", claims.Subject)
			return nil, fmt.Errorf("%w: not an access token", auth.ErrInvalidToken)
		}
		if claims.Subject == "" {
			log.WarnContext(ctx, "mcp token rejected", "reason", "no_sub")
			return nil, fmt.Errorf("%w: token has no subject", auth.ErrInvalidToken)
		}
		if !v.Allowed(claims.OIDCClaims) {
			log.WarnContext(ctx, "mcp token rejected", "reason", "group",
				"sub", claims.Subject, "groups", claims.Groups, "roles", claims.Roles)
			return nil, fmt.Errorf("%w: user is not a member of the required group", auth.ErrInvalidToken)
		}

		return &auth.TokenInfo{
			UserID:     claims.Subject,
			Scopes:     strings.Fields(claims.Scope),
			Expiration: claims.Expiry,
			Extra:      map[string]any{ExtraUsername: resolveUsername(ctx, users, claims, log)},
		}, nil
	}
}

// resolveUsername returns the caller's FunnelBarn username. An existing user
// keeps the name the dashboard login gave it: access tokens often lack
// preferred_username, and CreateIAMBarnUser would overwrite the name with
// whatever the token has. A first-time caller is created the same way the OIDC
// callback does. Failures are logged and fall back to the token's name.
func resolveUsername(ctx context.Context, users UserResolver, claims fbauth.AccessTokenClaims, log *slog.Logger) string {
	name := claims.PreferredName()
	if name == "" {
		name = "oidc-user"
	}
	if users == nil {
		return name
	}
	if u, err := users.FindUserByIAMBarnSub(ctx, claims.Subject); err == nil {
		return u.Username
	}
	u, err := users.CreateIAMBarnUser(ctx, claims.Subject, name)
	if err != nil {
		log.WarnContext(ctx, "mcp: upsert user", "sub", claims.Subject, "err", err)
		return name
	}
	return u.Username
}
