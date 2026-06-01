package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGetUnknownGuildReturnsEmpty(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.Get(context.Background(), "g-unknown")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CraftChannelID != "" || got.RequestChannelID != "" {
		t.Fatalf("expected empty settings, got %+v", got)
	}
}

func TestSetChannelsIndependently(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.SetCraftChannel(ctx, "g1", "chan-craft"); err != nil {
		t.Fatalf("set craft: %v", err)
	}
	got, _ := repo.Get(ctx, "g1")
	if got.CraftChannelID != "chan-craft" || got.RequestChannelID != "" {
		t.Fatalf("after craft set: %+v", got)
	}

	// Setting the request channel must not clear the craft channel (upsert).
	if err := repo.SetRequestChannel(ctx, "g1", "chan-req"); err != nil {
		t.Fatalf("set request: %v", err)
	}
	got, _ = repo.Get(ctx, "g1")
	if got.CraftChannelID != "chan-craft" || got.RequestChannelID != "chan-req" {
		t.Fatalf("after request set: %+v", got)
	}

	// Updating an existing channel overwrites just that field.
	if err := repo.SetCraftChannel(ctx, "g1", "chan-craft-2"); err != nil {
		t.Fatalf("update craft: %v", err)
	}
	got, _ = repo.Get(ctx, "g1")
	if got.CraftChannelID != "chan-craft-2" || got.RequestChannelID != "chan-req" {
		t.Fatalf("after craft update: %+v", got)
	}
}

func TestSettingsGuildIsolation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	repo.SetCraftChannel(ctx, "g1", "chan-1")
	repo.SetCraftChannel(ctx, "g2", "chan-2")

	g1, _ := repo.Get(ctx, "g1")
	g2, _ := repo.Get(ctx, "g2")
	if g1.CraftChannelID != "chan-1" || g2.CraftChannelID != "chan-2" {
		t.Fatalf("guild isolation broken: g1=%q g2=%q", g1.CraftChannelID, g2.CraftChannelID)
	}
}

func TestSettingsPersistenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rt.db")

	repo, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo.SetCraftChannel(ctx, "g1", "chan-craft")
	repo.SetRequestChannel(ctx, "g1", "chan-req")
	repo.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.Get(ctx, "g1")
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got.CraftChannelID != "chan-craft" || got.RequestChannelID != "chan-req" {
		t.Fatalf("settings not persisted: %+v", got)
	}
}
