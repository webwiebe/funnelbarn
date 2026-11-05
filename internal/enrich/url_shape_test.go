package enrich

import (
	"reflect"
	"strings"
	"testing"
)

func TestMaskPath(t *testing.T) {
	cases := map[string]string{
		"/auth/magic-links/consume":               "/auth/magic-links/consume",
		"/newsletter/confirm":                     "/newsletter/confirm",
		"/orders/42/items":                        "/orders/:id/items",
		"/reset/9f8e7d6c5b4a/confirm":             "/reset/:id/confirm",
		"/u/550e8400-e29b-41d4-a716-446655440000": "/u/:id",
		"/s/aGVsbG8gd29ybGQ1/view":                "/s/:id/view",
		"/a-very-long-segment-name-here/x":        "/:id/x",
		"/decade/facade":                          "/decade/facade",
		"/":                                       "/",
	}
	for in, want := range cases {
		if got := MaskPath(in); got != want {
			t.Errorf("MaskPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSensitiveURLShapes(t *testing.T) {
	cases := []struct {
		raw  string
		want []URLShape
	}{
		{"https://iam.example.com/auth/magic-links/consume?token=abc123&lang=nl",
			[]URLShape{{"iam.example.com", "/auth/magic-links/consume", "token"}}},
		{"https://user:pw@App.Example.com/reset/9f8e7d6c5b4a?Access_Token=x&state=y",
			[]URLShape{{"app.example.com", "/reset/:id", "access_token"}, {"app.example.com", "/reset/:id", "state"}}},
		{"https://app.example.com/#/invite/12345?invite=zz",
			[]URLShape{{"app.example.com", "/#/invite/:id", "invite"}}},
		{"https://app.example.com/cb#access_token=x&token_type=bearer",
			[]URLShape{{"app.example.com", "/cb", "access_token"}, {"app.example.com", "/cb", "token_type"}}},
		{"/login?code=abc", []URLShape{{"(relative)", "/login", "code"}}},
		{"//cdn.example.com/a.js?token=t", []URLShape{{"cdn.example.com", "/a.js", "token"}}},
		{"https://app.example.com/page?tab=overview#s", []URLShape{}},
		{"not a url ?token=x", nil},
	}
	for _, c := range cases {
		got := SensitiveURLShapes(c.raw)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("SensitiveURLShapes(%q) = %+v, want %+v", c.raw, got, c.want)
		}
	}
}

// A shape never carries any part of a parameter value.
func TestSensitiveURLShapes_NoValues(t *testing.T) {
	raw := "https://app.example.com/reset/VALUEPATH1234567/x?token=SECRETVALUE&code=SECRETCODE#/r?state=SECRETSTATE"
	for _, sh := range SensitiveURLShapes(raw) {
		joined := sh.Host + sh.Path + sh.Param
		for _, secret := range []string{"SECRET", "VALUEPATH"} {
			if strings.Contains(joined, secret) {
				t.Errorf("shape %+v holds %q", sh, secret)
			}
		}
	}
}

func TestWalkSensitiveURLsInJSON(t *testing.T) {
	doc := `[{"href":"https://a.example/x?token=1"},{"k":"https://a.example/y?tab=1"},` +
		`{"nested":{"src":"/cb?code=2","token=key":"plain"}},"https://b.example/z?secret=3"] trailing`
	var got []string
	if err := WalkSensitiveURLsInJSON(strings.NewReader(doc), func(s string) { got = append(got, s) }); err != nil {
		t.Fatal(err)
	}
	want := []string{"https://a.example/x?token=1", "/cb?code=2", "https://b.example/z?secret=3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("walk = %q, want %q", got, want)
	}
	if err := WalkSensitiveURLsInJSON(strings.NewReader(`[{"a":"https://x/?token=1"`), func(string) {}); err == nil {
		t.Error("truncated document: want an error")
	}
}
