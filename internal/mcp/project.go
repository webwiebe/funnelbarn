package mcp

import (
	"context"
	"sort"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ResolveProject picks the project a tool works on: arg (slug or ID) when
// set, else header (the x-funnelbarn-project value), else the only project
// when the instance has exactly one, else an error listing the available
// projects. The header is optional, so a server added without it still works
// on a single-project instance and tells the caller what to pass otherwise.
//
// This is the single place project selection happens, so it is also the
// single place a future per-project access check (restricting a caller to a
// subset of projects) belongs.
func ResolveProject(ctx context.Context, projects service.Projects, arg, header string) (repository.Project, error) {
	value := arg
	if value == "" {
		value = header
	}
	if value == "" {
		return onlyProject(ctx, projects)
	}

	p, err := projects.GetProjectBySlug(ctx, value)
	if err == nil {
		return p, nil
	}
	if !domain.IsNotFound(err) {
		return repository.Project{}, err
	}

	p, err = projects.GetProject(ctx, value)
	if err == nil {
		return p, nil
	}
	if !domain.IsNotFound(err) {
		return repository.Project{}, err
	}

	slugs, lerr := availableSlugs(ctx, projects)
	if lerr != nil {
		return repository.Project{}, lerr
	}
	return repository.Project{}, invalidInput("project %q not found; available: %s", value, strings.Join(slugs, ", "))
}

// onlyProject resolves a call that names no project: the single project when
// there is exactly one, else an input error listing the slugs to choose from.
func onlyProject(ctx context.Context, projects service.Projects) (repository.Project, error) {
	list, err := projects.ListProjects(ctx)
	if err != nil {
		return repository.Project{}, err
	}
	if len(list) == 1 {
		return list[0], nil
	}
	slugs := make([]string, len(list))
	for i, p := range list {
		slugs[i] = p.Slug
	}
	sort.Strings(slugs)
	return repository.Project{}, invalidInput(
		"no project is selected: pass a project argument (slug or ID), or set the %s header to pick a default; available: %s",
		ProjectHeader, strings.Join(slugs, ", "))
}

// availableSlugs lists every project's slug, sorted, for use in error
// messages.
func availableSlugs(ctx context.Context, projects service.Projects) ([]string, error) {
	list, err := projects.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, len(list))
	for i, p := range list {
		slugs[i] = p.Slug
	}
	sort.Strings(slugs)
	return slugs, nil
}
