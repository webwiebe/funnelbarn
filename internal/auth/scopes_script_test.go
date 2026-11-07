package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const registerScript = "../../deploy/iambarn/register-mcp-resource.sh"

// The deploy check must cover every scope the authorize request asks for,
// except offline_access, which iambarn grants without registration.
func TestRegisterScriptRequiresEveryRequestedScope(t *testing.T) {
	src, err := os.ReadFile(registerScript)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	m := regexp.MustCompile(`(?m)^REQUIRED_SCOPES="([^"]*)"$`).FindSubmatch(src)
	if m == nil {
		t.Fatal("REQUIRED_SCOPES line not found in the script")
	}
	got := strings.Fields(string(m[1]))
	var want []string
	for _, s := range requestedScopes() {
		if s != "offline_access" {
			want = append(want, s)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("REQUIRED_SCOPES = %v, want %v (requestedScopes minus offline_access)", got, want)
	}
}

// runRegisterScript runs the script against a fake admin API that answers the
// PUT with the given registered scopes.
func runRegisterScript(t *testing.T, scopes []string) (string, error) {
	t.Helper()
	for _, bin := range []string{"bash", "curl", "jq"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	const resource = "https://fb.example.test/api/v1/mcp"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/admin/clients/client-1" {
			http.Error(w, `{"error":"unexpected"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"resource_identifier": resource, "scopes": scopes})
	}))
	t.Cleanup(srv.Close)

	cmd := exec.Command("bash", registerScript, "test")
	cmd.Env = append(os.Environ(),
		"IAMBARN_ADMIN_TOKEN=fake-admin-token",
		"IAMBARN_URL="+srv.URL,
		"CLIENT_ID=client-1",
		"RESOURCE_URL="+resource,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if strings.Contains(out.String(), "fake-admin-token") {
		t.Errorf("script output contains the admin token")
	}
	return out.String(), err
}

func TestRegisterScriptPassesWhenTheSignInScopesAreRegistered(t *testing.T) {
	out, err := runRegisterScript(t, []string{"openid", "profile", "email", "groups", "roles"})
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "OK: sign-in scopes registered") {
		t.Errorf("output lacks the scope OK line:\n%s", out)
	}
}

func TestRegisterScriptFailsOnAnUnregisteredSignInScope(t *testing.T) {
	for name, scopes := range map[string][]string{
		"groups and roles missing": {"openid", "profile", "email"},
		"no scopes field":          nil,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := runRegisterScript(t, scopes)
			if err == nil {
				t.Fatalf("script passed with scopes %v:\n%s", scopes, out)
			}
			if !strings.Contains(out, "groups roles") {
				t.Errorf("output does not name the missing scopes:\n%s", out)
			}
		})
	}
}
