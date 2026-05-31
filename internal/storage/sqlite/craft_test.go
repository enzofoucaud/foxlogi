package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"foxlogi/internal/craft"
)

func sampleCraft(item string, completion time.Time) craft.Craft {
	return craft.Craft{
		GuildID:    "g1",
		ChannelID:  "c1",
		UserID:     "u1",
		Item:       item,
		Quantity:   2,
		Completion: completion,
	}
}

func TestAddAssignsID(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.Add(context.Background(), sampleCraft("Bmat", time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.ID == 0 {
		t.Fatalf("expected an assigned id, got 0")
	}
}

func TestListByGuildSorted(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now()
	repo.Add(ctx, sampleCraft("late", now.Add(2*time.Hour)))
	repo.Add(ctx, sampleCraft("early", now.Add(30*time.Minute)))
	repo.Add(ctx, sampleCraft("mid", now.Add(time.Hour)))

	list, err := repo.ListByGuild(ctx, "g1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"early", "mid", "late"}
	if len(list) != len(want) {
		t.Fatalf("expected %d crafts, got %d", len(want), len(list))
	}
	for idx, w := range want {
		if list[idx].Item != w {
			t.Errorf("position %d: want %s, got %s", idx, w, list[idx].Item)
		}
	}
}

func TestListByGuildIsolation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	repo.Add(ctx, sampleCraft("a", time.Now().Add(time.Hour)))
	other := sampleCraft("b", time.Now().Add(time.Hour))
	other.GuildID = "g2"
	repo.Add(ctx, other)

	list, _ := repo.ListByGuild(ctx, "g1")
	if len(list) != 1 || list[0].Item != "a" {
		t.Fatalf("expected only the g1 craft, got %+v", list)
	}
}

func TestDue(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now()
	repo.Add(ctx, sampleCraft("ready", now.Add(-time.Minute)))
	repo.Add(ctx, sampleCraft("pending", now.Add(time.Hour)))

	due, err := repo.Due(ctx, now)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due) != 1 || due[0].Item != "ready" {
		t.Fatalf("expected only the ready craft, got %+v", due)
	}
}

func TestDelete(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	c, _ := repo.Add(ctx, sampleCraft("x", time.Now().Add(time.Hour)))

	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ := repo.ListByGuild(ctx, "g1")
	if len(list) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(list))
	}
	// Deleting a missing id must not be an error.
	if err := repo.Delete(ctx, 9999); err != nil {
		t.Fatalf("delete missing id: %v", err)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rt.db")

	repo, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	completion := time.Now().Add(time.Hour)
	if _, err := repo.Add(ctx, sampleCraft("persist", completion)); err != nil {
		t.Fatalf("add: %v", err)
	}
	repo.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	list, err := reopened.ListByGuild(ctx, "g1")
	if err != nil {
		t.Fatalf("list after reopen: %v", err)
	}
	if len(list) != 1 || list[0].Item != "persist" {
		t.Fatalf("expected the persisted craft, got %+v", list)
	}
	if list[0].Completion.Unix() != completion.Unix() {
		t.Errorf("completion mismatch: want %d, got %d", completion.Unix(), list[0].Completion.Unix())
	}
}
