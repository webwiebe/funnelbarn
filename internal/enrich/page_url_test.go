package enrich_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
)

func TestRecordingPageURL(t *testing.T) {
	cases := map[string]string{
		"":                             "",
		"https://a.example/auth/login": "https://a.example/auth/login",
		"https://a.example/auth/login?state=s&nonce=n&redirect_uri=%2Fx": "https://a.example/auth/login",
		"https://a.example/p?utm_source=nl&state=s&utm_campaign=x#frag":  "https://a.example/p?utm_source=nl&utm_campaign=x",
		"https://a.example/p#access_token=abc":                           "https://a.example/p",
		"https://a.example/p?UTM_medium=mail":                            "https://a.example/p?UTM_medium=mail",
		"/relative?code=1":                                               "/relative",
		"https://a.example/p?":                                           "https://a.example/p",
		"https://a.example/p?utm_source":                                 "https://a.example/p?utm_source",
	}
	for in, want := range cases {
		assert.Equal(t, want, enrich.RecordingPageURL(in), in)
	}
}
