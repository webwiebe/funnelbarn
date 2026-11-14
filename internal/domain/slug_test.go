package domain

import "testing"

func TestValidProjectSlug(t *testing.T) {
	for _, s := range []string{"default", "my-project", "a1", "team-42-alpha"} {
		if !ValidProjectSlug(s) {
			t.Errorf("ValidProjectSlug should accept %q", s)
		}
	}
	for _, s := range []string{"", "-lead", "trail-", "Upper", "has space", "double--dash", "under_score"} {
		if ValidProjectSlug(s) {
			t.Errorf("ValidProjectSlug should reject %q", s)
		}
	}
}
