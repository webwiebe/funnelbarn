package enrich

import (
	"strings"
	"testing"
)

func TestHasSensitiveURLInJSON_MatchesRedactURLsInJSON(t *testing.T) {
	docs := []string{
		`[]`,
		`{}`,
		`[{"type":4,"data":{"href":"https://id.example.com/verify?token=one-time"},"timestamp":1}]`,
		`[{"type":4,"data":{"href":"https://id.example.com/verify?code=abc"}}]`,
		`[{"type":4,"data":{"href":"https://example.com/p?zipcode=1234"}}]`,
		`{"/reset?token=key-only":"value"}`,
		`{"a":{"b":[1,2,{"c":"/x#access_token=abc"}]},"d":true,"e":null}`,
		`{"a":[[],{},[{}]],"b":"plain text ? with token= words"}`,
		`{"href":"https://app.example.com/#/reset?password=x"}`,
		`[1.5e300,-0,"https://example.com/?q=1",{"k":"https://example.com/?invite=z"}]`,
		`["https://example.com/#pricing","/path?state=abc&x=1"]`,
	}
	for _, doc := range docs {
		_, want, err := RedactURLsInJSON([]byte(doc))
		if err != nil {
			t.Fatalf("RedactURLsInJSON(%s): %v", doc, err)
		}
		got, err := HasSensitiveURLInJSON(strings.NewReader(doc))
		if err != nil {
			t.Fatalf("HasSensitiveURLInJSON(%s): %v", doc, err)
		}
		if got != want {
			t.Errorf("%s: streaming says %v, decode says %v", doc, got, want)
		}
	}
}

func TestHasSensitiveURLInJSON_Errors(t *testing.T) {
	for _, doc := range []string{`[{"a":"b"`, `{"a":}`, `[1,,2]`} {
		if _, err := HasSensitiveURLInJSON(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: want an error", doc)
		}
	}
	// Only the first value counts, as with RedactURLsInJSON.
	got, err := HasSensitiveURLInJSON(strings.NewReader(`[] ["/x?token=a"]`))
	if err != nil || got {
		t.Errorf("trailing value: got %v, %v", got, err)
	}
}
