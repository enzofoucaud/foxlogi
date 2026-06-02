package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"foxlogi/internal/building"
)

func sampleBuilding(label string) building.Building {
	return building.Building{
		GuildID:  "g1",
		Label:    label,
		Hexagon:  "Great March",
		Town:     "Sitaria",
		Type:     "Depot",
		Role:     "Faci/Logi HUB",
		Password: "155523",
	}
}

func TestAddBuildingAndGet(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	got, err := repo.AddBuilding(ctx, sampleBuilding("LIV GREAT"))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got.ID == 0 {
		t.Fatal("expected an assigned id")
	}

	b, found, err := repo.GetBuilding(ctx, "g1", got.ID)
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if b.Password != "155523" || b.Town != "Sitaria" {
		t.Fatalf("unexpected building: %+v", b)
	}

	if _, found, _ := repo.GetBuilding(ctx, "g1", 9999); found {
		t.Fatal("expected not found for unknown id")
	}
}

func TestBuildingGuildIsolation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	a, _ := repo.AddBuilding(ctx, sampleBuilding("A"))

	if _, found, _ := repo.GetBuilding(ctx, "g2", a.ID); found {
		t.Fatal("g1 building must not be visible to g2")
	}
	list, _ := repo.ListBuildingsByGuild(ctx, "g2")
	if len(list) != 0 {
		t.Fatalf("expected no buildings for g2, got %d", len(list))
	}
}

func TestListBuildingsOrderedAndDelete(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	repo.AddBuilding(ctx, sampleBuilding("Bravo"))
	a, _ := repo.AddBuilding(ctx, sampleBuilding("Alpha"))

	list, err := repo.ListBuildingsByGuild(ctx, "g1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Label != "Alpha" {
		t.Fatalf("expected label-ordered list, got %+v", list)
	}

	if err := repo.DeleteBuilding(ctx, "g1", a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ = repo.ListBuildingsByGuild(ctx, "g1")
	if len(list) != 1 || list[0].Label != "Bravo" {
		t.Fatalf("expected only Bravo after delete, got %+v", list)
	}
	// Deleting a missing id is not an error.
	if err := repo.DeleteBuilding(ctx, "g1", 9999); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	// A building can't be deleted from another guild's scope.
	other, _ := repo.AddBuilding(ctx, sampleBuilding("Charlie"))
	if err := repo.DeleteBuilding(ctx, "g2", other.ID); err != nil {
		t.Fatalf("delete wrong guild: %v", err)
	}
	if _, found, _ := repo.GetBuilding(ctx, "g1", other.ID); !found {
		t.Fatal("building must survive a delete scoped to another guild")
	}
}

func TestBuildingPersistenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rt.db")

	repo, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	saved, _ := repo.AddBuilding(ctx, sampleBuilding("Persist"))
	repo.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	b, found, err := reopened.GetBuilding(ctx, "g1", saved.ID)
	if err != nil || !found {
		t.Fatalf("get after reopen: found=%v err=%v", found, err)
	}
	if b.Password != "155523" {
		t.Fatalf("password not persisted: %+v", b)
	}
}

func TestBuildingRoles(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if roles, _ := repo.ListBuildingRoles(ctx, "g1"); len(roles) != 0 {
		t.Fatalf("expected no roles initially, got %v", roles)
	}

	if err := repo.AddBuildingRole(ctx, "g1", "r1"); err != nil {
		t.Fatalf("add role: %v", err)
	}
	// Idempotent: re-adding the same role does not error or duplicate.
	if err := repo.AddBuildingRole(ctx, "g1", "r1"); err != nil {
		t.Fatalf("re-add role: %v", err)
	}
	repo.AddBuildingRole(ctx, "g1", "r2")

	roles, _ := repo.ListBuildingRoles(ctx, "g1")
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %v", roles)
	}

	// Guild isolation.
	if other, _ := repo.ListBuildingRoles(ctx, "g2"); len(other) != 0 {
		t.Fatalf("roles leaked to g2: %v", other)
	}

	if err := repo.RemoveBuildingRole(ctx, "g1", "r1"); err != nil {
		t.Fatalf("remove role: %v", err)
	}
	roles, _ = repo.ListBuildingRoles(ctx, "g1")
	if len(roles) != 1 || roles[0] != "r2" {
		t.Fatalf("expected only r2 after remove, got %v", roles)
	}
}
