package domain

import "regexp"

// projectSlugPattern is the shape of a project slug: lowercase alphanumerics
// joined by single hyphens.
var projectSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidProjectSlug reports whether s is a well-formed project slug. The CLI
// checks it before creating a project, and the ingest path checks it before
// auto-creating one, so a forged x-funnelbarn-project header cannot spawn
// projects with arbitrary names.
func ValidProjectSlug(s string) bool {
	return projectSlugPattern.MatchString(s)
}
