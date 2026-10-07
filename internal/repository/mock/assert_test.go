package mock_test

import (
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
)

// Compile-time checks: the in-memory store stands in for every port the
// service tests inject it as. The assertions live in a test file because the
// repository packages may not import ports (see internal/archtest).
var (
	_ ports.ProjectRepo          = (*mock.Store)(nil)
	_ ports.FunnelRepo           = (*mock.Store)(nil)
	_ ports.ABTestRepo           = (*mock.Store)(nil)
	_ ports.FlagRepo             = (*mock.Store)(nil)
	_ ports.EventRepo            = (*mock.Store)(nil)
	_ ports.SessionRepo          = (*mock.Store)(nil)
	_ ports.APIKeyRepo           = (*mock.Store)(nil)
	_ ports.WidgetRepo           = (*mock.Store)(nil)
	_ ports.SegmentRepo          = (*mock.Store)(nil)
	_ ports.ProjectHealthRepo    = (*mock.Store)(nil)
	_ ports.InstanceSettingsRepo = (*mock.Store)(nil)
	_ ports.EventPersister       = (*mock.Store)(nil)
)
