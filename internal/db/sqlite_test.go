package db

import (
	"context"
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
	hash, found, err := db.Get(ctx, "http://example.com/pkg1", algo)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !found {
		t.Error("Expected to find http://example.com/pkg1")
	}
	if hash != "hash1" {
		t.Errorf("Expected hash1, got %s", hash)
	}

	// Test Get not found
	_, found, err = db.Get(ctx, "http://example.com/pkg3", algo)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if found {
		t.Error("Expected not to find http://example.com/pkg3")
	}

}
