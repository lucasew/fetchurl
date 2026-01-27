package db

import (
	"context"
	"net/url"
	"os"
	"testing"
)

func TestDB(t *testing.T) {
	// Create a temporary file for the database
	f, err := os.CreateTemp("", "testdb-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := f.Name()
	f.Close()
	defer os.Remove(dbPath)

	// Initialize DB
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Test Insert with algo
	entries := map[string]string{
		"http://example.com/pkg1": "hash1",
		"http://example.com/pkg2": "hash2",
	}
	algo := "sha256"
	if err := db.Insert(ctx, algo, entries); err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	// Test Get
	gotAlgo, hash, found, err := db.Get(ctx, "http://example.com/pkg1")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !found {
		t.Error("Expected to find http://example.com/pkg1")
	}
	if hash != "hash1" {
		t.Errorf("Expected hash1, got %s", hash)
	}
	if gotAlgo != algo {
		t.Errorf("Expected algo %s, got %s", algo, gotAlgo)
	}

	// Test Get not found
	_, _, found, err = db.Get(ctx, "http://example.com/pkg3")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if found {
		t.Error("Expected not to find http://example.com/pkg3")
	}

	// Test Rule
	rule := NewRule(db)
	u, _ := url.Parse("http://example.com/pkg2")
	res := rule(ctx, u)
	if res == nil {
		t.Error("Rule expected to match http://example.com/pkg2")
	} else {
		if res.Hash != "hash2" {
			t.Errorf("Expected hash2, got %s", res.Hash)
		}
		if res.Algo != "sha256" {
			t.Errorf("Expected sha256, got %s", res.Algo)
		}
	}

	u2, _ := url.Parse("http://example.com/pkg3")
	res2 := rule(ctx, u2)
	if res2 != nil {
		t.Error("Rule expected not to match http://example.com/pkg3")
	}
}
