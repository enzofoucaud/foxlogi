package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRecordEventInsertsRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.RecordEvent(ctx, "g1", "u1", "craft.add", map[string]string{"item": "Bmat", "quantity": "200"}); err != nil {
		t.Fatalf("record: %v", err)
	}

	var (
		userID, etype, payload string
		createdAt              int64
	)
	if err := repo.db.QueryRowContext(ctx,
		`SELECT user_id, type, payload, created_at FROM events WHERE guild_id = ?`, "g1").
		Scan(&userID, &etype, &payload, &createdAt); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if userID != "u1" || etype != "craft.add" {
		t.Fatalf("unexpected event: user=%q type=%q", userID, etype)
	}
	if createdAt == 0 {
		t.Fatal("expected a non-zero created_at")
	}

	var got map[string]string
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("payload not valid JSON: %v (%q)", err, payload)
	}
	if got["item"] != "Bmat" || got["quantity"] != "200" {
		t.Fatalf("unexpected payload: %v", got)
	}
}

func TestRecordEventEmptyPayload(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if err := repo.RecordEvent(ctx, "g1", "u1", "request.cancel", nil); err != nil {
		t.Fatalf("record: %v", err)
	}
	var payload string
	if err := repo.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE guild_id = ?`, "g1").Scan(&payload); err != nil {
		t.Fatalf("query: %v", err)
	}
	if payload != "" {
		t.Fatalf("expected empty payload, got %q", payload)
	}
}

func TestRecordEventGuildIsolation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	repo.RecordEvent(ctx, "g1", "u1", "craft.add", nil)
	repo.RecordEvent(ctx, "g2", "u2", "craft.add", nil)

	var count int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE guild_id = ?`, "g1").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event for g1, got %d", count)
	}
}

func TestRecordEventPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rt.db")

	repo, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.RecordEvent(ctx, "g1", "u1", "building.show", map[string]string{"building_id": "4", "label": "LIV GREAT"})
	repo.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var etype string
	if err := reopened.db.QueryRowContext(ctx, `SELECT type FROM events WHERE guild_id = ?`, "g1").Scan(&etype); err != nil {
		t.Fatalf("query after reopen: %v", err)
	}
	if etype != "building.show" {
		t.Fatalf("event not persisted: %q", etype)
	}
}
