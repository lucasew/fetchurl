package fetcher

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shogo82148/go-sfv"
)

func sha256Sum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestFetcher(t *testing.T) {
	content := []byte("test content")
	hash := sha256Sum(content)

	t.Run("Direct Download Success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		defer ts.Close()

		f := NewFetcher(nil, nil)
		var out bytes.Buffer
		err := f.Fetch(t.Context(), "sha256", hash, []string{ts.URL}, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.String() != string(content) {
			t.Errorf("got %q, want %q", out.String(), string(content))
		}
	})

	t.Run("Direct Download Hash Mismatch", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("wrong content"))
		}))
		defer ts.Close()

		f := NewFetcher(nil, nil)
		var out bytes.Buffer
		err := f.Fetch(t.Context(), "sha256", hash, []string{ts.URL}, &out)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("Server Fetch Success", func(t *testing.T) {
		// Mock Source
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Should not hit source if server succeeds")
		}))
		defer source.Close()

		// Mock FetchURL Server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != fmt.Sprintf("/api/fetchurl/sha256/%s", hash) {
				t.Errorf("unexpected path: %s", r.URL.Path)
				w.WriteHeader(404)
				return
			}
			// Verify X-Source-Urls header
			val := r.Header.Get("X-Source-Urls")
			list, err := sfv.DecodeList([]string{val})
			if err != nil {
				t.Errorf("failed to decode X-Source-Urls: %v", err)
			}
			found := false
			for _, item := range list {
				if s, ok := item.Value.(string); ok && s == source.URL {
					found = true
				}
			}
			if !found {
				t.Errorf("X-Source-Urls missing source URL, got %v", val)
			}

			w.Write(content)
		}))
		defer server.Close()

		f := NewFetcher(nil, []string{server.URL})
		var out bytes.Buffer
		err := f.Fetch(t.Context(), "sha256", hash, []string{source.URL}, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.String() != string(content) {
			t.Errorf("got %q, want %q", out.String(), string(content))
		}
	})

	t.Run("Server Fail Fallback", func(t *testing.T) {
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		defer source.Close()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
		}))
		defer server.Close()

		f := NewFetcher(nil, []string{server.URL})
		var out bytes.Buffer
		err := f.Fetch(t.Context(), "sha256", hash, []string{source.URL}, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.String() != string(content) {
			t.Errorf("got %q, want %q", out.String(), string(content))
		}
	})
}
