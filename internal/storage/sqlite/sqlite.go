// Package sqlite owns the shared SQLite connection (open, migrate, close) used
// by the per-feature repositories in this package. A single *sql.DB is shared
// so the connection cap and pragmas hold across all features. Schema changes
// live as versioned goose migrations under migrations/; feature files
// (craft.go, request.go) hold only their queries.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Repo is the shared SQLite store. It holds a single *sql.DB and satisfies the
// per-feature Repository interfaces implemented across this package's files.
type Repo struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies the
// connection pragmas, and runs all pending goose migrations.
func Open(path string) (*Repo, error) {
	// busy_timeout lets writes wait instead of failing with SQLITE_BUSY; capping
	// the pool at a single connection serialises concurrent access, eliminating
	// "database is locked". foreign_keys enables ON DELETE CASCADE.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Repo{db: db}, nil
}

// migrate applies every pending migration to db, in version order.
func migrate(db *sql.DB) error {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrations fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Close releases the database handle.
func (r *Repo) Close() error {
	return r.db.Close()
}
