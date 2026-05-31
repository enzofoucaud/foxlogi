package sqlite

import (
	"path/filepath"
	"testing"
)

// newTestRepo opens a throwaway database in a temp dir, shared by the
// per-feature repository tests in this package.
func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	repo, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestOpenRunsMigrationsAndCloses(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "init.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Every registered migration ran, so a feature table is queryable.
	if _, err := repo.db.Exec(`SELECT 1 FROM crafts LIMIT 1`); err != nil {
		t.Errorf("crafts table not migrated: %v", err)
	}
	if _, err := repo.db.Exec(`SELECT 1 FROM request_items LIMIT 1`); err != nil {
		t.Errorf("request_items table not migrated: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
