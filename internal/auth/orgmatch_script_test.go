package auth

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const orgMatchScript = "../../deploy/iambarn/oidc-org-match.sh"

// runOrgMatch runs the script with a stub sops that answers the extract with
// configured, and returns its combined output.
func runOrgMatch(t *testing.T, configured, given string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	bin := t.TempDir()
	stub := "#!/usr/bin/env bash\nprintf '%s' \"$STUB_CONFIGURED_ID\"\n"
	if err := os.WriteFile(filepath.Join(bin, "sops"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write stub sops: %v", err)
	}
	cmd := exec.Command("bash", orgMatchScript, "secret.yaml")
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_CONFIGURED_ID="+configured,
	)
	if given != "" {
		cmd.Env = append(cmd.Env, "GIVEN_ORGANIZATION_ID="+given)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if configured != "" && strings.Contains(out.String(), configured) {
		t.Errorf("output contains the configured id: %q", out.String())
	}
	return out.String(), err
}

func TestOrgMatchScriptPrintsYesForTheConfiguredID(t *testing.T) {
	out, err := runOrgMatch(t, "org-configured-1", "org-configured-1")
	if err != nil {
		t.Fatalf("script failed on a match: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "match=yes" {
		t.Errorf("output = %q, want match=yes", out)
	}
}

func TestOrgMatchScriptPrintsNoOtherwise(t *testing.T) {
	for name, c := range map[string]struct{ configured, given string }{
		"another organization": {"org-configured-1", "org-other-2"},
		"a prefix of the id":   {"org-configured-1", "org-configured"},
		"nothing configured":   {"", "org-other-2"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := runOrgMatch(t, c.configured, c.given)
			if err == nil {
				t.Fatalf("script passed:\n%s", out)
			}
			if !strings.HasPrefix(strings.TrimSpace(out), "match=no") {
				t.Errorf("output = %q, want match=no", out)
			}
		})
	}
}

func TestOrgMatchScriptRequiresAnID(t *testing.T) {
	out, err := runOrgMatch(t, "org-configured-1", "")
	if err == nil {
		t.Fatalf("script passed without GIVEN_ORGANIZATION_ID:\n%s", out)
	}
	if strings.Contains(out, "match=yes") {
		t.Errorf("output = %q, want no match=yes", out)
	}
}
