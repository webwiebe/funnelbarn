package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// webSessionService passes dashboard session reads and writes straight to the
// store, one call per method. The session middleware sits on the auth path, so
// it adds no queries, caching or logging; callers handle and log errors.
type webSessionService struct {
	repo ports.WebSessionRepo
}

// NewWebSessionService creates the WebSessions service.
func NewWebSessionService(repo ports.WebSessionRepo) WebSessions {
	return &webSessionService{repo: repo}
}

func (s *webSessionService) CreateWebSession(ctx context.Context, ws domain.WebSession) error {
	return s.repo.CreateWebSession(ctx, ws)
}

func (s *webSessionService) GetWebSession(ctx context.Context, idHash string) (domain.WebSession, error) {
	return s.repo.GetWebSession(ctx, idHash)
}

func (s *webSessionService) UpdateWebSessionTokens(ctx context.Context, idHash, idToken, accessToken, refreshToken string, accessExpiresAt int64, claimsJSON string, lastRefreshAt int64) error {
	return s.repo.UpdateWebSessionTokens(ctx, idHash, idToken, accessToken, refreshToken, accessExpiresAt, claimsJSON, lastRefreshAt)
}

func (s *webSessionService) MarkWebSessionRefreshFailing(ctx context.Context, idHash string, since int64) error {
	return s.repo.MarkWebSessionRefreshFailing(ctx, idHash, since)
}

func (s *webSessionService) DeleteWebSession(ctx context.Context, idHash string) error {
	return s.repo.DeleteWebSession(ctx, idHash)
}

func (s *webSessionService) DeleteWebSessionsByIdpSid(ctx context.Context, sid string) (int64, error) {
	return s.repo.DeleteWebSessionsByIdpSid(ctx, sid)
}

func (s *webSessionService) DeleteWebSessionsByIdpSub(ctx context.Context, sub string) (int64, error) {
	return s.repo.DeleteWebSessionsByIdpSub(ctx, sub)
}

// iambarnUserService creates the local user record behind an IAMBarn login.
type iambarnUserService struct {
	repo ports.IAMBarnUserRepo
}

// NewIAMBarnUserService creates the IAMBarnUsers service.
func NewIAMBarnUserService(repo ports.IAMBarnUserRepo) IAMBarnUsers {
	return &iambarnUserService{repo: repo}
}

func (s *iambarnUserService) CreateIAMBarnUser(ctx context.Context, sub, username string) (domain.User, error) {
	return s.repo.CreateIAMBarnUser(ctx, sub, username)
}

func (s *iambarnUserService) FindUserByIAMBarnSub(ctx context.Context, sub string) (domain.User, error) {
	return s.repo.FindUserByIAMBarnSub(ctx, sub)
}
