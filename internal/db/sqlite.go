package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB represents the database connection.
type DB struct {
	db *sql.DB
}

// New creates a new DB instance and initializes the schema.
func New(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &DB{db: db}
	if err := d.init(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

func (d *DB) init() error {
	query := `
	CREATE TABLE IF NOT EXISTS urls (
		url TEXT PRIMARY KEY,
		hash TEXT NOT NULL
	);
	`
	_, err := d.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Insert inserts multiple URL->Hash mappings in a single transaction.
func (d *DB) Insert(ctx context.Context, entries map[string]string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, "INSERT OR REPLACE INTO urls (url, hash) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for u, h := range entries {
		_, err := stmt.ExecContext(ctx, u, h)
		if err != nil {
			return fmt.Errorf("failed to insert %s: %w", u, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Get retrieves the hash for a given URL.
func (d *DB) Get(ctx context.Context, u string) (string, bool, error) {
	var hash string
	err := d.db.QueryRowContext(ctx, "SELECT hash FROM urls WHERE url = ?", u).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get hash for url %s: %w", u, err)
	}
	return hash, true, nil
}
