package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"foxlogi/internal/request"
)

func sampleRequest(guildID string) request.Request {
	return request.Request{
		GuildID:   guildID,
		ChannelID: "c1",
		UserID:    "u1",
		Location:  "West Stockpile",
		Priority:  request.PriorityMedium,
	}
}

func TestCreateRequestAssignsID(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.CreateRequest(context.Background(), sampleRequest("g1"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID == 0 {
		t.Fatalf("expected an assigned id, got 0")
	}
}

func TestGetRequestWithItems(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	req, _ := repo.CreateRequest(ctx, sampleRequest("g1"))
	repo.AddItem(ctx, req.ID, "Bmat", 200)
	repo.AddItem(ctx, req.ID, "Rmat", 50)

	got, found, err := repo.GetRequest(ctx, "g1", req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatal("expected request to be found")
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}

	_, found, err = repo.GetRequest(ctx, "g1", 9999)
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if found {
		t.Fatal("expected found=false for unknown id")
	}
}

func TestAddItemMergesDuplicates(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	req, _ := repo.CreateRequest(ctx, sampleRequest("g1"))

	repo.AddItem(ctx, req.ID, "Bmat", 200)
	merged, err := repo.AddItem(ctx, req.ID, "bmat", 100) // case-insensitive duplicate
	if err != nil {
		t.Fatalf("add duplicate: %v", err)
	}
	if merged.Quantity != 300 {
		t.Fatalf("expected merged quantity 300, got %d", merged.Quantity)
	}

	got, _, _ := repo.GetRequest(ctx, "g1", req.ID)
	if len(got.Items) != 1 {
		t.Fatalf("expected a single merged line, got %d", len(got.Items))
	}
}

func TestGetRequestGuildIsolation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	req, _ := repo.CreateRequest(ctx, sampleRequest("g1"))

	_, found, err := repo.GetRequest(ctx, "g2", req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found {
		t.Fatal("request from g1 must not be visible to g2")
	}
}

func TestListOpenOrderedByPriority(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	low := sampleRequest("g1")
	low.Priority = request.PriorityLow
	repo.CreateRequest(ctx, low)
	high := sampleRequest("g1")
	high.Priority = request.PriorityHigh
	repo.CreateRequest(ctx, high)
	med := sampleRequest("g1")
	med.Priority = request.PriorityMedium
	repo.CreateRequest(ctx, med)

	reqs, err := repo.ListOpenByGuild(ctx, "g1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{request.PriorityHigh, request.PriorityMedium, request.PriorityLow}
	if len(reqs) != len(want) {
		t.Fatalf("expected %d requests, got %d", len(want), len(reqs))
	}
	for idx, w := range want {
		if reqs[idx].Priority != w {
			t.Errorf("position %d: want %s, got %s", idx, w, reqs[idx].Priority)
		}
	}
}

func TestFillCumulativeWithSurplus(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	req, _ := repo.CreateRequest(ctx, sampleRequest("g1"))
	repo.AddItem(ctx, req.ID, "Bmat", 200)

	it, ok, err := repo.Fill(ctx, req.ID, "bmat", 50) // case-insensitive
	if err != nil || !ok {
		t.Fatalf("fill 1: ok=%v err=%v", ok, err)
	}
	if it.Delivered != 50 {
		t.Fatalf("expected 50 delivered, got %d", it.Delivered)
	}

	it, _, _ = repo.Fill(ctx, req.ID, "Bmat", 300) // surplus accepted
	if it.Delivered != 350 {
		t.Fatalf("expected cumulative 350, got %d", it.Delivered)
	}

	_, ok, err = repo.Fill(ctx, req.ID, "Unknown", 10)
	if err != nil {
		t.Fatalf("fill unknown: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for unknown item line")
	}
}

func TestIsFulfilledTransitions(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	req, _ := repo.CreateRequest(ctx, sampleRequest("g1"))

	// No items yet → not fulfilled.
	if ok, _ := repo.IsFulfilled(ctx, req.ID); ok {
		t.Fatal("empty request must not be fulfilled")
	}

	repo.AddItem(ctx, req.ID, "Bmat", 100)
	repo.AddItem(ctx, req.ID, "Rmat", 50)
	if ok, _ := repo.IsFulfilled(ctx, req.ID); ok {
		t.Fatal("unmet request must not be fulfilled")
	}

	repo.Fill(ctx, req.ID, "Bmat", 100)
	if ok, _ := repo.IsFulfilled(ctx, req.ID); ok {
		t.Fatal("partially met request must not be fulfilled")
	}

	repo.Fill(ctx, req.ID, "Rmat", 50)
	if ok, _ := repo.IsFulfilled(ctx, req.ID); !ok {
		t.Fatal("fully met request must be fulfilled")
	}
}

func TestDeleteRequestCascadesItems(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	req, _ := repo.CreateRequest(ctx, sampleRequest("g1"))
	repo.AddItem(ctx, req.ID, "Bmat", 100)

	if _, err := repo.DeleteRequest(ctx, req.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, found, _ := repo.GetRequest(ctx, "g1", req.ID); found {
		t.Fatal("request should be gone")
	}
	// Items must be removed via cascade — no orphans remain.
	var orphans int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM request_items WHERE request_id = ?`, req.ID).Scan(&orphans); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("expected items cascaded away, found %d", orphans)
	}
}

func TestRequestPersistenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rt.db")

	repo, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	deadline := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	r := sampleRequest("g1")
	r.Priority = request.PriorityHigh
	r.Deadline = &deadline
	created, _ := repo.CreateRequest(ctx, r)
	repo.AddItem(ctx, created.ID, "Bmat", 200)
	repo.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	got, found, err := reopened.GetRequest(ctx, "g1", created.ID)
	if err != nil || !found {
		t.Fatalf("get after reopen: found=%v err=%v", found, err)
	}
	if got.Priority != request.PriorityHigh || got.Deadline == nil || got.Deadline.Unix() != deadline.Unix() {
		t.Errorf("request fields not persisted: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].Item != "Bmat" {
		t.Errorf("items not persisted: %+v", got.Items)
	}
}
