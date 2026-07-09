package storage

import "testing"

func TestNewR2_RejectsMissingArgs(t *testing.T) {
	cases := []struct {
		name                                   string
		endpoint, accessKey, secretKey, bucket string
	}{
		{"empty endpoint", "", "ak", "sk", "bucket"},
		{"empty access key", "https://x.r2.cloudflarestorage.com", "", "sk", "bucket"},
		{"empty secret key", "https://x.r2.cloudflarestorage.com", "ak", "", "bucket"},
		{"empty bucket", "https://x.r2.cloudflarestorage.com", "ak", "sk", ""},
		{"all empty", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewR2(tc.endpoint, tc.accessKey, tc.secretKey, tc.bucket)
			if err == nil {
				t.Fatal("expected error for missing argument, got nil")
			}
			if store != nil {
				t.Errorf("expected nil store on error, got %v", store)
			}
		})
	}
}

func TestNewR2_ConstructsWithAllArgs(t *testing.T) {
	// Construction does not touch the network; it only builds the S3 client.
	store, err := NewR2("https://acct.eu.r2.cloudflarestorage.com", "ak", "sk", "recordings")
	if err != nil {
		t.Fatalf("NewR2: unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if store.bucket != "recordings" {
		t.Errorf("bucket = %q, want %q", store.bucket, "recordings")
	}
	if store.client == nil {
		t.Error("expected non-nil s3 client")
	}
}
