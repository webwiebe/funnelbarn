package mcp

import (
	"context"
	"errors"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ResolveProject picks the project a tool works on: arg (slug or ID) when
// set, else header (the x-funnelbarn-project value), else an error listing
// the available projects.
func ResolveProject(ctx context.Context, projects service.Projects, arg, header string) (repository.Project, error) {
	return repository.Project{}, errors.New("not implemented")
}
