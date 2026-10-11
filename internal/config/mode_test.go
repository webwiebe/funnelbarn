package config

import "testing"

func TestLoad_ModeDefaultsToStandalone(t *testing.T) {
	t.Setenv("FUNNELBARN_MODE", "")
	if got := Load().Mode; got != ModeStandalone {
		t.Fatalf("Mode = %q, want %q", got, ModeStandalone)
	}
	t.Setenv("FUNNELBARN_MODE", " Reader ")
	if got := Load().Mode; got != ModeReader {
		t.Fatalf("Mode = %q, want %q", got, ModeReader)
	}
}

func TestLoad_WriterURLAndInternalToken(t *testing.T) {
	t.Setenv("FUNNELBARN_WRITER_URL", "http://funnelbarn-writer:8080/")
	t.Setenv("FUNNELBARN_INTERNAL_TOKEN", "s3cret")
	cfg := Load()
	if cfg.WriterURL != "http://funnelbarn-writer:8080" {
		t.Errorf("WriterURL = %q, want the URL without its trailing slash", cfg.WriterURL)
	}
	if cfg.InternalToken != "s3cret" {
		t.Errorf("InternalToken = %q", cfg.InternalToken)
	}
}

func TestValidateMode(t *testing.T) {
	const redis, writer, token = "redis://valkey:6379/0", "http://writer:8080", "t"
	cases := []struct {
		mode, redis, writer, token string
		ok                         bool
	}{
		{ModeStandalone, "", "", "", true},
		{ModeReader, redis, writer, token, true},
		{ModeWriter, redis, "", token, true},
		{ModeReader, "", writer, token, false},
		{ModeWriter, "", "", token, false},
		{ModeReader, redis, "", token, false},
		{ModeReader, redis, writer, "", false},
		{ModeWriter, redis, "", "", false},
		{"both", redis, writer, token, false},
	}
	for _, c := range cases {
		err := Config{Mode: c.mode, RedisQueueURL: c.redis, WriterURL: c.writer, InternalToken: c.token}.ValidateMode()
		if (err == nil) != c.ok {
			t.Errorf("%+v: err = %v, want ok=%v", c, err, c.ok)
		}
	}
}
