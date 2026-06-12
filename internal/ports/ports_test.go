package ports_test

import (
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
)

// Compile-time checks: *repository.Store satisfies every port interface.
var _ ports.ProjectRepo = (*repository.Store)(nil)
var _ ports.FunnelRepo = (*repository.Store)(nil)
var _ ports.ABTestRepo = (*repository.Store)(nil)
var _ ports.FlagRepo = (*repository.Store)(nil)
var _ ports.EventRepo = (*repository.Store)(nil)
var _ ports.SessionRepo = (*repository.Store)(nil)
var _ ports.APIKeyRepo = (*repository.Store)(nil)
var _ ports.WidgetRepo = (*repository.Store)(nil)
var _ ports.EventPersister = (*repository.Store)(nil)
var _ ports.ProjectHealthRepo = (*repository.Store)(nil)

// Compile-time checks: *mock.Store satisfies every port interface.
var _ ports.ProjectRepo = (*mock.Store)(nil)
var _ ports.FunnelRepo = (*mock.Store)(nil)
var _ ports.ABTestRepo = (*mock.Store)(nil)
var _ ports.FlagRepo = (*mock.Store)(nil)
var _ ports.EventRepo = (*mock.Store)(nil)
var _ ports.SessionRepo = (*mock.Store)(nil)
var _ ports.APIKeyRepo = (*mock.Store)(nil)
var _ ports.WidgetRepo = (*mock.Store)(nil)
var _ ports.EventPersister = (*mock.Store)(nil)
var _ ports.ProjectHealthRepo = (*mock.Store)(nil)
