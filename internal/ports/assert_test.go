package ports_test

import (
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Compile-time checks: the query ports are satisfied by *repository.ReadStore,
// which holds no write handle.
var _ ports.FlagQueries = (*repository.ReadStore)(nil)
var _ ports.ABTestQueries = (*repository.ReadStore)(nil)
var _ ports.APIKeyQueries = (*repository.ReadStore)(nil)
var _ ports.ProjectHealthQueries = (*repository.ReadStore)(nil)
var _ ports.InstanceSettingsQueries = (*repository.ReadStore)(nil)
var _ ports.EventQueries = (*repository.ReadStore)(nil)
var _ ports.OverviewQueries = (*repository.ReadStore)(nil)
var _ ports.SessionQueries = (*repository.ReadStore)(nil)
var _ ports.RecordingQueries = (*repository.ReadStore)(nil)

// The command ports stay satisfied by *repository.Store.
var _ ports.FlagCommands = (*repository.Store)(nil)
var _ ports.ABTestCommands = (*repository.Store)(nil)
var _ ports.APIKeyCommands = (*repository.Store)(nil)
var _ ports.ProjectHealthCommands = (*repository.Store)(nil)
var _ ports.InstanceSettingsRepo = (*repository.Store)(nil)
var _ ports.EventCommands = (*repository.Store)(nil)
var _ ports.SessionCommands = (*repository.Store)(nil)
var _ ports.RecordingCommands = (*repository.Store)(nil)
