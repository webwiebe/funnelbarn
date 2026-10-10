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

func TestValidateMode(t *testing.T) {
	cases := []struct {
		mode, redis string
		ok          bool
	}{
		{ModeStandalone, "", true},
		{ModeReader, "redis://valkey:6379/0", true},
		{ModeWriter, "redis://valkey:6379/0", true},
		{ModeReader, "", false},
		{ModeWriter, "", false},
		{"both", "redis://valkey:6379/0", false},
	}
	for _, c := range cases {
		err := Config{Mode: c.mode, RedisQueueURL: c.redis}.ValidateMode()
		if (err == nil) != c.ok {
			t.Errorf("mode %q redis %q: err = %v, want ok=%v", c.mode, c.redis, err, c.ok)
		}
	}
}
