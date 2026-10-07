package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
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

// newAPIAuthorizer builds the ingest/evaluate authorizer. With a dispatcher the
// last_used_at touch is a queued command instead of a write on the request path.
func newAPIAuthorizer(cfg config.Config, store *repository.Store, commands *command.Dispatcher) (*auth.Authorizer, error) {
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
	return base.WithDBLookup(store.ValidAPIKeySHA256, apiKeyToucher(store, commands)), nil
}

// apiKeyToucher returns the DBKeyTouch for the authorizer: a direct write when
// commands is nil, otherwise a TouchAPIKey command whose submit wait is added
// to the request's wait sum. Queued touches are throttled to one per key per
// apiKeyTouchInterval, so a busy SDK key does not put a write on the single
// consumer for every request.
func apiKeyToucher(store *repository.Store, commands *command.Dispatcher) auth.DBKeyTouch {
	if commands == nil {
		return store.TouchAPIKey
	}
	th := newTouchThrottle(apiKeyTouchInterval, maxTrackedKeyTouches)
	return func(ctx context.Context, keySHA256 string) error {
		if !th.due(keySHA256, time.Now()) {
			return nil
		}
		waited := commands.Submit(ctx, command.TouchAPIKey{Store: store, KeyHash: keySHA256})
		service.AddSubmitWait(ctx, waited)
		return nil
	}
}

// last_used_at is read in days ("unused for 90 days"), so minute resolution
// loses nothing.
const (
	apiKeyTouchInterval  = time.Minute
	maxTrackedKeyTouches = 4096
)

// touchThrottle remembers when each key was last touched. The map is reset
// rather than grown past max; losing it costs one extra write per key.
type touchThrottle struct {
	mu       sync.Mutex
	interval time.Duration
	max      int
	last     map[string]time.Time
}

func newTouchThrottle(interval time.Duration, max int) *touchThrottle {
	return &touchThrottle{interval: interval, max: max, last: make(map[string]time.Time)}
}

// due reports whether key is due for a touch and records it if so.
func (t *touchThrottle) due(key string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, ok := t.last[key]; ok && now.Sub(prev) < t.interval {
		return false
	}
	if len(t.last) >= t.max {
		t.last = make(map[string]time.Time, t.max)
	}
	t.last[key] = now
	return true
}

const (
	workerMaxRetries      = 3
	workerRotateThreshold = 64 << 20 // 64 MiB
)
