package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB represents the database connection.
type DB struct {
	*Queries
	db *sql.DB
}

// Open creates a new DB instance and initializes the schema.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	if err := migrateDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DB{
		Queries: New(db),
		db:      db,
	}, nil
}

func migrateDB(db *sql.DB) error {
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migrate driver: %w", err)
	}

	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create iofs source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", d, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to migrate up: %w", err)
	}

	slog.Info("Database migrations applied successfully")
	return nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Insert inserts multiple URL->Hash mappings in a single transaction.
func (d *DB) Insert(ctx context.Context, algo string, entries map[string]string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := d.Queries.WithTx(tx)

	for u, h := range entries {
		err := qtx.InsertHash(ctx, InsertHashParams{
			Url:  u,
			Hash: h,
			Algo: algo,
		})
		if err != nil {
			return fmt.Errorf("failed to insert %s: %w", u, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Get retrieves the hash for a given URL and algo.
func (d *DB) Get(ctx context.Context, u, algo string) (string, bool, error) {
	hash, err := d.Queries.GetHash(ctx, GetHashParams{
		Url:  u,
		Algo: algo,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get hash for url %s: %w", u, err)
	}
	return hash, true, nil
}
