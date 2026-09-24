package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"quoteservice/internal/domain"
)

type workerMemUpdates struct {
	byID map[string]domain.UpdateRequest
}

func (m *workerMemUpdates) Create(_ context.Context, req domain.UpdateRequest) error {
	if m.byID == nil {
		m.byID = map[string]domain.UpdateRequest{}
	}
	m.byID[req.ID] = req
	return nil
}
func (m *workerMemUpdates) GetByID(_ context.Context, id string) (domain.UpdateRequest, error) {
	return m.byID[id], nil
}
func (m *workerMemUpdates) CountPending(context.Context) (int, error) { return 0, nil }
func (m *workerMemUpdates) GetByIdempotencyKey(context.Context, string) (domain.UpdateRequest, error) {
	return domain.UpdateRequest{}, domain.ErrUpdateNotFound
}
func (m *workerMemUpdates) ClaimNextPending(_ context.Context) (domain.UpdateRequest, error) {
	for id, r := range m.byID {
		if r.Status == domain.StatusPending {
			_ = r.MarkProcessing(time.Now().UTC())
			m.byID[id] = r
			return r, nil
		}
	}
	return domain.UpdateRequest{}, domain.ErrUpdateNotFound
}
func (m *workerMemUpdates) Save(_ context.Context, req domain.UpdateRequest) error {
	m.byID[req.ID] = req
	return nil
}

type workerMemQuotes struct{}

func (workerMemQuotes) UpsertLatest(context.Context, domain.Quote) error { return nil }
func (workerMemQuotes) GetLatest(context.Context, domain.CurrencyPair) (domain.Quote, error) {
	return domain.Quote{}, domain.ErrQuoteNotFound
}

type workerMemComplete struct{ u *workerMemUpdates }

func (m workerMemComplete) SaveCompleted(ctx context.Context, req domain.UpdateRequest, q domain.Quote) error {
	return m.u.Save(ctx, req)
}

type clk struct{}

func (clk) Now() time.Time { return time.Now().UTC() }

type blockingRates struct {
	started chan struct{}
	block   chan struct{}
	q       domain.Quote
}

func (b *blockingRates) Fetch(ctx context.Context, _ domain.CurrencyPair) (domain.Quote, error) {
	close(b.started)
	select {
	case <-b.block:
		return b.q, nil
	case <-ctx.Done():
		return domain.Quote{}, ctx.Err()
	}
}

func TestWorkerShutdownWaitsInFlightAndDoesNotFail(t *testing.T) {
	p, _ := domain.ParsePair("EUR/MXN")
	q, _ := domain.NewQuote(p, decimal.RequireFromString("21.5"), time.Now().UTC())
	req, _ := domain.NewUpdateRequest("u1", p, time.Now().UTC())
	repo := &workerMemUpdates{}
	_ = repo.Create(context.Background(), req)
	rates := &blockingRates{started: make(chan struct{}), block: make(chan struct{}), q: q}
	w := Worker{
		ProcessNext: ProcessNext{
			Updates: repo,
			Process: Process{
				Updates:  repo,
				Quotes:   workerMemQuotes{},
				Complete: workerMemComplete{u: repo},
				Rates:    rates,
				Clock:    clk{},
			},
		},
		Interval:   time.Hour,
		JobTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	select {
	case <-rates.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch never started")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("worker returned before in-flight fetch finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(rates.block)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not return after fetch")
	}
	saved, _ := repo.GetByID(context.Background(), "u1")
	if saved.Status == domain.StatusFailed {
		t.Fatalf("job failed on shutdown: %+v", saved)
	}
	if saved.Status != domain.StatusCompleted {
		t.Fatalf("status %s", saved.Status)
	}
}
