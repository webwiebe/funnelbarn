package main

import (
	"errors"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// buildOIDCClient returns an OIDC adapter when all four FUNNELBARN_OIDC_* vars
// are set, or nil otherwise (in which case the local single-user login is the
// auth path). Discovery is lazy so an unreachable issuer at startup does not
// crash the process.
func buildOIDCClient(cfg config.Config) *auth.OIDCClient {
	oc := auth.OIDCConfig{
		Issuer:                cfg.OIDCIssuer,
		ClientID:              cfg.OIDCClientID,
		ClientSecret:          cfg.OIDCClientSecret,
		RedirectURL:           cfg.OIDCRedirectURL,
		RequiredGroup:         cfg.OIDCRequiredGroup,
		PostLogoutRedirectURI: cfg.PostLogoutRedirectURI,
	}
	if !oc.Enabled() {
		return nil
	}
	slog.Info("oidc: enabled", "issuer", oc.Issuer, "client_id", oc.ClientID, "required_group", oc.RequiredGroup)
	return auth.NewOIDCClient(oc)
}

// validateFailClosed refuses to start a production deployment whose auth
// surfaces would silently fail open: the ingest API accepting any key because
// none is configured, or the dashboard API serving every route unauthenticated
// because no login mechanism exists. Non-production tiers keep the permissive
// behaviour for local development and throwaway environments.
func validateFailClosed(env string, apiKeyConfigured, authConfigured bool) error {
	if env != environment.Production {
		return nil
	}
	if !apiKeyConfigured {
		return errors.New("refusing to start in production without an API key: ingest would accept ANY key — set FUNNELBARN_API_KEY(_SHA256) or create a key with 'funnelbarn apikey create'")
	}
	if !authConfigured {
		return errors.New("refusing to start in production without an authentication mechanism: dashboard routes would be served unauthenticated — set FUNNELBARN_ADMIN_*, configure FUNNELBARN_OIDC_*, or run 'funnelbarn user create'")
	}
	return nil
}

func newAPIAuthorizer(cfg config.Config, store *repository.Store) (*auth.Authorizer, error) {
	var base *auth.Authorizer
	var err error
	if cfg.APIKeySHA256 != "" {
		base, err = auth.NewHashed(cfg.APIKeySHA256)
		if err != nil {
			return nil, err
		}
	} else {
		base = auth.New(cfg.APIKey)
	}
	return base.WithDBLookup(store.ValidAPIKeySHA256, store.TouchAPIKey), nil
}

const (
	workerMaxRetries      = 3
	workerRotateThreshold = 64 << 20 // 64 MiB
)
