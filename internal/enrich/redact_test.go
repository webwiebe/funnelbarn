package enrich

import (
	"reflect"
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
