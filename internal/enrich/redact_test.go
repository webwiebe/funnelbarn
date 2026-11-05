package enrich

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestIsSensitiveParam(t *testing.T) {
	sensitive := []string{"token", "TOKEN", "code", "state", "invite", "access_token", "id_token",
		"refresh-token", "magicToken", "client_secret", "Secret", "password", "new_password"}
	for _, name := range sensitive {
		if !IsSensitiveParam(name) {
			t.Errorf("IsSensitiveParam(%q) = false, want true", name)
		}
	}
	ordinary := []string{"", "utm_source", "page", "q", "zipcode", "statement", "invited_by",
		"lang", "ref", "id"}
	for _, name := range ordinary {
		if IsSensitiveParam(name) {
			t.Errorf("IsSensitiveParam(%q) = true, want false", name)
		}
	}
}

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"no query", "https://example.com/docs", "https://example.com/docs"},
		{"ordinary params keep their values and order",
			"https://example.com/p?utm_source=mail&b=2&a=1",
			"https://example.com/p?utm_source=mail&b=2&a=1"},
		{"token value dropped, key and other params kept",
			"https://id.example.com/verify?utm_source=mail&token=abc123&lang=nl",
			"https://id.example.com/verify?utm_source=mail&token=redacted&lang=nl"},
		{"oauth code and state",
			"https://app.example.com/callback?code=xyz&state=s1",
			"https://app.example.com/callback?code=redacted&state=redacted"},
		{"invite and substring matches",
			"/join?invite=i1&access_token=t1&client_secret=c1&new_password=p1",
			"/join?invite=redacted&access_token=redacted&client_secret=redacted&new_password=redacted"},
		{"case-insensitive key keeps its spelling",
			"https://e.com/?Token=T&STATE=S",
			"https://e.com/?Token=redacted&STATE=redacted"},
		{"encoded key is matched decoded",
			"https://e.com/?%74oken=T",
			"https://e.com/?%74oken=redacted"},
		{"repeated key", "https://e.com/?token=a&token=b", "https://e.com/?token=redacted&token=redacted"},
		{"key without value left alone", "https://e.com/?token&x=1", "https://e.com/?token&x=1"},
		{"empty value still replaced", "https://e.com/?token=", "https://e.com/?token=redacted"},
		{"fragment parameters", "https://e.com/cb#access_token=t&expires_in=3600",
			"https://e.com/cb#access_token=redacted&expires_in=3600"},
		{"hash-router fragment", "https://e.com/#/reset?token=t&x=1", "https://e.com/#/reset?token=redacted&x=1"},
		{"plain anchor untouched", "https://e.com/p?token=t#pricing", "https://e.com/p?token=redacted#pricing"},
		{"zipcode is not code", "https://e.com/?zipcode=1234", "https://e.com/?zipcode=1234"},
		{"unparseable escape still redacted", "https://e.com/?bad=%zz&token=t", "https://e.com/?bad=%zz&token=redacted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactURL(tc.in); got != tc.want {
				t.Errorf("RedactURL(%q)\n got  %q\n want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactURLStrings(t *testing.T) {
	props := map[string]any{
		"page_url": "https://e.com/verify?token=t",
		"path":     "/reset?password_token=p",
		"note":     "what is the code? code=1",
		"plain":    "token=abc",
		"count":    float64(3),
		"nested": map[string]any{
			"href": "https://e.com/cb?code=c",
		},
		"list": []any{"https://e.com/?state=s", "ok"},
	}
	want := map[string]any{
		"page_url": "https://e.com/verify?token=redacted",
		"path":     "/reset?password_token=redacted",
		"note":     "what is the code? code=1",
		"plain":    "token=abc",
		"count":    float64(3),
		"nested": map[string]any{
			"href": "https://e.com/cb?code=redacted",
		},
		"list": []any{"https://e.com/?state=redacted", "ok"},
	}
	got := RedactURLStrings(props)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RedactURLStrings\n got  %#v\n want %#v", got, want)
	}
	if RedactURLStrings(nil) != nil {
		t.Error("RedactURLStrings(nil) should stay nil")
	}
}

func TestRedactURLsInJSON(t *testing.T) {
	t.Run("no marker returns input unchanged", func(t *testing.T) {
		in := []byte(`[{"type":4,"data":{"href":"https://e.com/p?x=1"},"timestamp":1730000000000}]`)
		out, changed, err := RedactURLsInJSON(in)
		if err != nil || changed || string(out) != string(in) {
			t.Fatalf("got %q changed=%v err=%v", out, changed, err)
		}
	})
	t.Run("marker without a sensitive URL returns input unchanged", func(t *testing.T) {
		in := []byte(`{"text":"enter your token below","href":"https://e.com/p?x=1"}`)
		out, changed, err := RedactURLsInJSON(in)
		if err != nil || changed || string(out) != string(in) {
			t.Fatalf("got %q changed=%v err=%v", out, changed, err)
		}
	})
	t.Run("meta href and nested attributes redacted, numbers and markup kept", func(t *testing.T) {
		in := []byte(`[{"type":4,"data":{"href":"https://id.e.com/verify?token=abc&lang=nl","width":1440},"timestamp":1730000000123},` +
			`{"type":2,"data":{"node":{"attributes":{"href":"/reset?password_token=p"},"textContent":"<b>a & b</b>"}},"timestamp":1730000000124}]`)
		out, changed, err := RedactURLsInJSON(in)
		if err != nil || !changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
		var got []map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("output is not JSON: %v", err)
		}
		s := string(out)
		for _, want := range []string{
			`"href":"https://id.e.com/verify?token=redacted&lang=nl"`,
			`"href":"/reset?password_token=redacted"`,
			`"timestamp":1730000000123`,
			`"width":1440`,
			`"textContent":"<b>a & b</b>"`,
		} {
			if !strings.Contains(s, want) {
				t.Errorf("output lacks %s:\n%s", want, s)
			}
		}
		if strings.Contains(s, "abc") {
			t.Errorf("token value still present:\n%s", s)
		}
	})
	t.Run("already redacted is unchanged", func(t *testing.T) {
		in := []byte(`{"href":"https://e.com/v?token=redacted"}`)
		_, changed, err := RedactURLsInJSON(in)
		if err != nil || changed {
			t.Fatalf("changed=%v err=%v", changed, err)
		}
	})
	t.Run("invalid JSON with a marker reports an error and returns input", func(t *testing.T) {
		in := []byte(`{"href":"https://e.com/v?token=x"`)
		out, changed, err := RedactURLsInJSON(in)
		if err == nil || changed || string(out) != string(in) {
			t.Fatalf("got %q changed=%v err=%v", out, changed, err)
		}
	})
}
