package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"quoteservice/internal/domain"
	"quoteservice/internal/repository/postgres"
)

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{URL: url, ConnectTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	dir := filepath.Join("..", "..", "..", "..", "migrations")
	if err := postgres.Migrate(ctx, pool, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM update_requests WHERE id LIKE 'itest-%'`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func ancient(req domain.UpdateRequest) domain.UpdateRequest {
	req.CreatedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	req.UpdatedAt = req.CreatedAt
	return req
}

func TestClaimConcurrentSkipLocked(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	repo := postgres.NewUpdateRepository(pool, 30*time.Second, 3)
	p, _ := domain.ParsePair("EUR/MXN")
	now := time.Now().UTC()
	req, _ := domain.NewUpdateRequest("itest-conc-"+now.Format("150405.000000000"), p, now)
	req = ancient(req)
	if err := repo.Create(ctx, req); err != nil {
		t.Fatal(err)
	}
	type result struct {
		id  string
		err error
	}
	ch := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			got, err := repo.ClaimNextPending(ctx)
			if err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{id: got.ID}
		}()
	}
	var ours int
	for i := 0; i < 2; i++ {
		r := <-ch
		if errors.Is(r.err, domain.ErrUpdateNotFound) {
			continue
		}
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.id == req.ID {
			ours++
		}
	}
	if ours != 1 {
		t.Fatalf("our job claimed %d times", ours)
	}
}

func TestClaimSkipLockedAndReclaim(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	repo := postgres.NewUpdateRepository(pool, 50*time.Millisecond, 3)
	p, _ := domain.ParsePair("EUR/MXN")
	now := time.Now().UTC()
	req, _ := domain.NewUpdateRequest("itest-"+now.Format("150405.000000000"), p, now)
	req = ancient(req)
	if err := repo.Create(ctx, req); err != nil {
		t.Fatal(err)
	}

	a, err := repo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != req.ID || a.Status != domain.StatusProcessing {
		t.Fatalf("first claim %+v", a)
	}
	held, err := repo.GetByID(ctx, req.ID)
	if err != nil || held.Attempts != 1 {
		t.Fatalf("lease hold %+v %v", held, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE update_requests SET updated_at = NOW() - interval '2 seconds' WHERE id = $1`, req.ID); err != nil {
		t.Fatal(err)
	}
	b, err := repo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != req.ID {
		t.Fatalf("reclaim id %s", b.ID)
	}
	if b.Attempts < 2 {
		t.Fatalf("attempts %d", b.Attempts)
	}
}

func TestUpsertObservedAtGuard(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	quotes := postgres.NewQuoteRepository(pool)
	p, _ := domain.ParsePair("USD/EUR")
	newQ, _ := domain.NewQuote(p, decimal.RequireFromString("1.20"), time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC))
	oldQ, _ := domain.NewQuote(p, decimal.RequireFromString("1.10"), time.Date(2026, 9, 19, 11, 0, 0, 0, time.UTC))
	if err := quotes.UpsertLatest(ctx, newQ); err != nil {
		t.Fatal(err)
	}
	if err := quotes.UpsertLatest(ctx, oldQ); err != nil {
		t.Fatal(err)
	}
	got, err := quotes.GetLatest(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Rate.Equal(newQ.Rate) {
		t.Fatalf("rate %s want %s", got.Rate, newQ.Rate)
	}
}

func TestSaveCompletedAtomic(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	updatesRepo := postgres.NewUpdateRepository(pool, time.Second, 3)
	complete := postgres.NewCompletion(pool)
	quotes := postgres.NewQuoteRepository(pool)
	p, _ := domain.ParsePair("MXN/USD")
	now := time.Now().UTC()
	req, _ := domain.NewUpdateRequest("itest-complete-"+now.Format("150405.000000000"), p, now)
	_ = req.MarkProcessing(now)
	if err := updatesRepo.Create(ctx, req); err != nil {
		t.Fatal(err)
	}
	q, _ := domain.NewQuote(p, decimal.RequireFromString("0.05"), now)
	if err := req.Complete(q, now); err != nil {
		t.Fatal(err)
	}
	if err := complete.SaveCompleted(ctx, req, q); err != nil {
		t.Fatal(err)
	}
	got, err := updatesRepo.GetByID(ctx, req.ID)
	if err != nil || got.Status != domain.StatusCompleted {
		t.Fatalf("update %+v %v", got, err)
	}
	latest, err := quotes.GetLatest(ctx, p)
	if err != nil || !latest.Rate.Equal(q.Rate) {
		t.Fatalf("quote %+v %v", latest, err)
	}
}

func TestCorrelationIDPersisted(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	repo := postgres.NewUpdateRepository(pool, time.Second, 3)
	p, _ := domain.ParsePair("EUR/MXN")
	now := time.Now().UTC()
	req, _ := domain.NewUpdateRequest("itest-corr-"+now.Format("150405.000000000"), p, now)
	req.CorrelationID = "client-corr-xyz"
	if err := repo.Create(ctx, req); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CorrelationID != "client-corr-xyz" {
		t.Fatalf("correlation: %q", got.CorrelationID)
	}
	claimed, err := repo.ClaimNextPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID == req.ID && claimed.CorrelationID != "client-corr-xyz" {
		t.Fatalf("claim correlation: %q", claimed.CorrelationID)
	}
}

func TestSaveLostLease(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	repo := postgres.NewUpdateRepository(pool, time.Second, 3)
	complete := postgres.NewCompletion(pool)
	p, _ := domain.ParsePair("EUR/USD")
	now := time.Now().UTC()
	req, _ := domain.NewUpdateRequest("itest-lease-"+now.Format("150405.000000000"), p, now)
	_ = req.MarkProcessing(now)
	if err := repo.Create(ctx, req); err != nil {
		t.Fatal(err)
	}
	// Simulate another instance finishing the job first.
	if _, err := pool.Exec(ctx, `UPDATE update_requests SET status = 'completed', updated_at = NOW() WHERE id = $1`, req.ID); err != nil {
		t.Fatal(err)
	}
	q, _ := domain.NewQuote(p, decimal.RequireFromString("1.1"), now)
	if err := req.Complete(q, now); err != nil {
		t.Fatal(err)
	}
	if err := complete.SaveCompleted(ctx, req, q); !errors.Is(err, domain.ErrLostLease) {
		t.Fatalf("SaveCompleted want ErrLostLease, got %v", err)
	}
	if err := repo.Save(ctx, req); !errors.Is(err, domain.ErrLostLease) {
		t.Fatalf("Save want ErrLostLease, got %v", err)
	}
	got, err := repo.GetByID(ctx, req.ID)
	if err != nil || got.Status != domain.StatusCompleted {
		t.Fatalf("status should stay completed, got %+v %v", got, err)
	}
}

func TestIdempotencyKeyUnique(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	repo := postgres.NewUpdateRepository(pool, time.Second, 3)
	p, _ := domain.ParsePair("EUR/MXN")
	now := time.Now().UTC()
	a, _ := domain.NewUpdateRequest("itest-idem-a-"+now.Format("150405.000000000"), p, now)
	a.IdempotencyKey = "itest-key-" + now.Format("150405.000000000")
	b, _ := domain.NewUpdateRequest("itest-idem-b-"+now.Format("150405.000000000"), p, now)
	b.IdempotencyKey = a.IdempotencyKey
	if err := repo.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, b); !errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
		t.Fatalf("want duplicate, got %v", err)
	}
	got, err := repo.GetByIdempotencyKey(ctx, a.IdempotencyKey)
	if err != nil || got.ID != a.ID {
		t.Fatalf("got %+v %v", got, err)
	}
}
