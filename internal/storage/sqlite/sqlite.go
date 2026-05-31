// Package sqlite owns the shared SQLite connection (open, migrate, close) used
// by the per-feature repositories in this package. A single *sql.DB is shared
// so the connection cap and pragmas hold across all features. Feature-specific
// tables and queries live in their own files (craft.go, request.go); each
// registers its migration via registerMigration in an init function.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// migrations holds the DDL each feature registers; Open runs them all in order.
var migrations []string

// registerMigration queues a feature's schema DDL to run at Open. Called from
// feature files' init functions.
func registerMigration(ddl string) {
	migrations = append(migrations, ddl)
}

// Repo is the shared SQLite store. It holds a single *sql.DB and satisfies the
// per-feature Repository interfaces implemented across this package's files.
type Repo struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies the
// connection pragmas, and runs every registered migration.
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
	for _, ddl := range migrations {
		if _, err := db.Exec(ddl); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate schema: %w", err)
		}
	}
	return &Repo{db: db}, nil
}

// Close releases the database handle.
func (r *Repo) Close() error {
	return r.db.Close()
}
