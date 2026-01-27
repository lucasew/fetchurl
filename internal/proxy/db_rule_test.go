package proxy

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/lucasew/fetchurl/internal/db"
)

func TestDBRule(t *testing.T) {
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
	algo := "sha256"

	// Seed DB
	entries := map[string]string{
		"http://example.com/pkg2": "hash2",
	}
	if err := database.Insert(ctx, algo, entries); err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	// Test Rule
	rule := NewDBRule(database, algo)
	u, _ := url.Parse("http://example.com/pkg2")
	res := rule(ctx, u)
	if res == nil {
		t.Error("Rule expected to match http://example.com/pkg2")
	} else {
		if res.Hash != "hash2" {
			t.Errorf("Expected hash2, got %s", res.Hash)
		}
		if res.Algo != algo {
			t.Errorf("Expected %s, got %s", algo, res.Algo)
		}
	}

	u2, _ := url.Parse("http://example.com/pkg3")
	res2 := rule(ctx, u2)
	if res2 != nil {
		t.Error("Rule expected not to match http://example.com/pkg3")
	}
}
