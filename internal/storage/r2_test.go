package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestR2 points an R2Store at a local httptest.Server standing in for the
// R2/S3 endpoint, so Put/Get/Delete can be exercised (both success and error
// paths, covering the tracing/metrics instrumentation around each) without a
// real network dependency.
func newTestR2(t *testing.T, handler http.HandlerFunc) *R2Store {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	store, err := NewR2(srv.URL, "ak", "sk", "test-bucket")
	if err != nil {
		t.Fatalf("NewR2: %v", err)
	}
	return store
}

func TestR2Store_Put(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newTestR2(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		if err := store.Put(context.Background(), "k", []byte("data")); err != nil {
			t.Fatalf("Put: unexpected error: %v", err)
		}
	})
	t.Run("error", func(t *testing.T) {
		store := newTestR2(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		if err := store.Put(context.Background(), "k", []byte("data")); err == nil {
			t.Fatal("Put: expected error, got nil")
		}
	})
}

func TestR2Store_Get(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newTestR2(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		})
		data, err := store.Get(context.Background(), "k")
		if err != nil {
			t.Fatalf("Get: unexpected error: %v", err)
		}
		if string(data) != "hello" {
			t.Errorf("Get: data = %q, want %q", data, "hello")
		}
	})
	t.Run("error", func(t *testing.T) {
		store := newTestR2(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		if _, err := store.Get(context.Background(), "k"); err == nil {
			t.Fatal("Get: expected error, got nil")
		}
	})
}

func TestR2Store_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newTestR2(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		if err := store.Delete(context.Background(), "k"); err != nil {
			t.Fatalf("Delete: unexpected error: %v", err)
		}
	})
	t.Run("error", func(t *testing.T) {
		store := newTestR2(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		if err := store.Delete(context.Background(), "k"); err == nil {
			t.Fatal("Delete: expected error, got nil")
		}
	})
}

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
