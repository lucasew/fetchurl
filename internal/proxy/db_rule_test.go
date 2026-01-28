package proxy

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/lucasew/fetchurl/internal/db"
)

func TestDBMultiRule(t *testing.T) {
	// Create a temporary file for the database
	f, err := os.CreateTemp("", "testdb-rule-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := f.Name()
	f.Close()
	defer os.Remove(dbPath)

	// Initialize DB
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	// Seed DB with multiple hashes
	entries := map[string]string{
		"http://example.com/pkg2": "hash2",
	}
	if err := database.Insert(ctx, "sha256", entries); err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	// Add another hash for same URL
	entries2 := map[string]string{
		"http://example.com/pkg2": "hash2sha1",
	}
	if err := database.Insert(ctx, "sha1", entries2); err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	// Test Rule
	rule := NewDBMultiRule(database)
	u, _ := url.Parse("http://example.com/pkg2")
	results := rule(ctx, u)
	if len(results) == 0 {
		t.Error("Rule expected to match http://example.com/pkg2")
	} else {
		// Should have 2 results, ordered by priority (sha256 first)
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
		// First result should be sha256 (higher priority)
		if results[0].Algo != "sha256" {
			t.Errorf("Expected first result to be sha256, got %s", results[0].Algo)
		}
		if results[0].Hash != "hash2" {
			t.Errorf("Expected hash2, got %s", results[0].Hash)
		}
		// Second result should be sha1
		if results[1].Algo != "sha1" {
			t.Errorf("Expected second result to be sha1, got %s", results[1].Algo)
		}
	}

	u2, _ := url.Parse("http://example.com/pkg3")
	results2 := rule(ctx, u2)
	if len(results2) != 0 {
		t.Error("Rule expected not to match http://example.com/pkg3")
	}
}
