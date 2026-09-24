package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"quoteservice/internal/domain"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type memUpdates struct {
	mu   sync.Mutex
	byID map[string]domain.UpdateRequest
}

func newMemUpdates() *memUpdates {
	return &memUpdates{byID: make(map[string]domain.UpdateRequest)}
}

func (m *memUpdates) Create(_ context.Context, req domain.UpdateRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if req.IdempotencyKey != "" {
		for _, r := range m.byID {
			if r.IdempotencyKey == req.IdempotencyKey {
				return domain.ErrDuplicateIdempotencyKey
			}
		}
	}
	m.byID[req.ID] = req
	return nil
}

func (m *memUpdates) GetByID(_ context.Context, id string) (domain.UpdateRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.byID[id]
	if !ok {
		return domain.UpdateRequest{}, domain.ErrUpdateNotFound
	}
	return r, nil
}

func (m *memUpdates) CountPending(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.byID {
		if r.Status == domain.StatusPending || r.Status == domain.StatusProcessing {
			n++
		}
	}
	return n, nil
}

func (m *memUpdates) GetByIdempotencyKey(_ context.Context, key string) (domain.UpdateRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.byID {
		if r.IdempotencyKey == key && key != "" {
			return r, nil
		}
	}
	return domain.UpdateRequest{}, domain.ErrUpdateNotFound
}

func (m *memUpdates) ClaimNextPending(_ context.Context) (domain.UpdateRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.byID {
		if r.Status == domain.StatusPending {
			r.Attempts++
			_ = r.MarkProcessing(time.Now().UTC())
			m.byID[id] = r
			return r, nil
		}
	}
	return domain.UpdateRequest{}, domain.ErrUpdateNotFound
}

func (m *memUpdates) Save(_ context.Context, req domain.UpdateRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.byID[req.ID]
	if !ok {
		return domain.ErrUpdateNotFound
	}
	if cur.Status != domain.StatusProcessing {
		return domain.ErrLostLease
	}
	m.byID[req.ID] = req
	return nil
}

type memQuotes struct {
	mu     sync.Mutex
	byPair map[string]domain.Quote
}

func newMemQuotes() *memQuotes {
	return &memQuotes{byPair: make(map[string]domain.Quote)}
}

func (m *memQuotes) UpsertLatest(_ context.Context, q domain.Quote) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.byPair[q.Pair.String()]
	if ok && cur.ObservedAt.After(q.ObservedAt) {
		return nil
	}
	m.byPair[q.Pair.String()] = q
	return nil
}

func (m *memQuotes) GetLatest(_ context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q, ok := m.byPair[p.String()]
	if !ok {
		return domain.Quote{}, domain.ErrQuoteNotFound
	}
	return q, nil
}

type memComplete struct {
	updates *memUpdates
	quotes  *memQuotes
}

func (m memComplete) SaveCompleted(ctx context.Context, req domain.UpdateRequest, q domain.Quote) error {
	if err := m.updates.Save(ctx, req); err != nil {
		return err
	}
	return m.quotes.UpsertLatest(ctx, q)
}

type stubRates struct {
	q       domain.Quote
	err     error
	calls   int
	block   chan struct{}
	started chan struct{}
}

func (s *stubRates) Fetch(ctx context.Context, _ domain.CurrencyPair) (domain.Quote, error) {
	s.calls++
	if s.started != nil {
		select {
		case <-s.started:
		default:
			close(s.started)
		}
	}
	if s.block != nil {
		select {
		case <-ctx.Done():
			return domain.Quote{}, ctx.Err()
		case <-s.block:
		}
	}
	if s.err != nil {
		return domain.Quote{}, s.err
	}
	return s.q, nil
}

func mustPair(t *testing.T, s string) domain.CurrencyPair {
	t.Helper()
	p, err := domain.ParsePair(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func processingReq(t *testing.T, id, pairRaw string, now time.Time) domain.UpdateRequest {
	t.Helper()
	req, err := domain.NewUpdateRequest(id, mustPair(t, pairRaw), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := req.MarkProcessing(now); err != nil {
		t.Fatal(err)
	}
	return req
}

func newProcess(repo *memUpdates, quotes *memQuotes, rates RateProvider, now time.Time) Process {
	return Process{
		Updates:  repo,
		Quotes:   quotes,
		Complete: memComplete{updates: repo, quotes: quotes},
		Rates:    rates,
		Clock:    fixedClock{t: now},
	}
}

func TestRequestAndGet(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	repo := newMemUpdates()
	uc := Request{
		Updates: repo,
		Clock:   fixedClock{t: now},
	}
	id, err := uc.Execute(context.Background(), "EUR/MXN", "", "corr-1")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	got, err := (Get{Updates: repo}).Execute(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusPending || got.Pair.String() != "EUR/MXN" {
		t.Fatalf("got %+v", got)
	}
	if got.CorrelationID != "corr-1" {
		t.Fatalf("correlation: %q", got.CorrelationID)
	}
}

func TestRequestInvalidPair(t *testing.T) {
	uc := Request{
		Updates: newMemUpdates(),
		Clock:   fixedClock{t: nowish()},
	}
	_, err := uc.Execute(context.Background(), "EUR/EUR", "", "")
	if !errors.Is(err, domain.ErrInvalidPair) {
		t.Fatalf("want ErrInvalidPair, got %v", err)
	}
}

func TestRequestQueueFull(t *testing.T) {
	now := nowish()
	repo := newMemUpdates()
	uc := Request{
		Updates:    repo,
		Clock:      fixedClock{t: now},
		MaxPending: 1,
	}
	if _, err := uc.Execute(context.Background(), "EUR/MXN", "", "corr-q"); err != nil {
		t.Fatal(err)
	}
	_, err := uc.Execute(context.Background(), "USD/EUR", "", "corr-q")
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("want ErrQueueFull, got %v", err)
	}
}

func TestGetEmptyID(t *testing.T) {
	_, err := (Get{Updates: newMemUpdates()}).Execute(context.Background(), "  ")
	if !errors.Is(err, domain.ErrEmptyID) {
		t.Fatalf("got %v", err)
	}
}

func TestProcessSuccess(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	p := mustPair(t, "EUR/MXN")
	req := processingReq(t, "u1", "EUR/MXN", now)
	repo := newMemUpdates()
	_ = repo.Create(context.Background(), req)
	rate := decimal.RequireFromString("21.5")
	observed := now.Add(-time.Minute)
	q, err := domain.NewQuote(p, rate, observed)
	if err != nil {
		t.Fatal(err)
	}
	quotes := newMemQuotes()
	uc := newProcess(repo, quotes, &stubRates{q: q}, now)
	out, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out != OutcomeCompleted {
		t.Fatalf("outcome: %s", out)
	}
	saved, _ := repo.GetByID(context.Background(), "u1")
	if saved.Status != domain.StatusCompleted {
		t.Fatalf("status: %s", saved.Status)
	}
	latest, err := quotes.GetLatest(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !latest.Rate.Equal(rate) {
		t.Fatalf("rate: %s", latest.Rate)
	}
}

func TestProcessLostLease(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	p := mustPair(t, "EUR/MXN")
	req := processingReq(t, "u-lease", "EUR/MXN", now)
	repo := newMemUpdates()
	_ = repo.Create(context.Background(), req)
	stolen := req
	stolen.Status = domain.StatusCompleted
	repo.mu.Lock()
	repo.byID[stolen.ID] = stolen
	repo.mu.Unlock()

	rate := decimal.RequireFromString("21.5")
	q, err := domain.NewQuote(p, rate, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	out, err := newProcess(repo, newMemQuotes(), &stubRates{q: q}, now).Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out != OutcomeLostLease {
		t.Fatalf("outcome: %s", out)
	}
	saved, _ := repo.GetByID(context.Background(), "u-lease")
	if saved.Status != domain.StatusCompleted {
		t.Fatalf("status should stay completed, got %s", saved.Status)
	}
}

func TestProcessFetchFail(t *testing.T) {
	now := nowish()
	req := processingReq(t, "u2", "USD/EUR", now)
	repo := newMemUpdates()
	_ = repo.Create(context.Background(), req)
	uc := newProcess(repo, newMemQuotes(), &stubRates{err: errors.New("upstream down")}, now)
	out, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out != OutcomeFailed {
		t.Fatalf("outcome: %s", out)
	}
	saved, _ := repo.GetByID(context.Background(), "u2")
	if saved.Status != domain.StatusFailed {
		t.Fatalf("status: %s", saved.Status)
	}
	if saved.Error != domain.FailFXUnavailable {
		t.Fatalf("error: %q", saved.Error)
	}
}

func TestProcessCanceledDoesNotFail(t *testing.T) {
	now := nowish()
	req := processingReq(t, "u-cancel", "EUR/MXN", now)
	repo := newMemUpdates()
	_ = repo.Create(context.Background(), req)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	block := make(chan struct{})
	uc := newProcess(repo, newMemQuotes(), &stubRates{started: started, block: block}, now)
	errCh := make(chan error, 1)
	go func() {
		_, err := uc.Execute(ctx, req)
		errCh <- err
	}()
	<-started
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	saved, _ := repo.GetByID(context.Background(), "u-cancel")
	if saved.Status != domain.StatusProcessing {
		t.Fatalf("status: %s", saved.Status)
	}
}

func TestProcessFreshnessSkip(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	p := mustPair(t, "EUR/MXN")
	req := processingReq(t, "u-fresh", "EUR/MXN", now)
	repo := newMemUpdates()
	_ = repo.Create(context.Background(), req)
	q, err := domain.NewQuote(p, decimal.RequireFromString("21.5"), now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	quotes := newMemQuotes()
	_ = quotes.UpsertLatest(context.Background(), q)
	rates := &stubRates{q: q}
	uc := newProcess(repo, quotes, rates, now)
	uc.Freshness = 30 * time.Second
	out, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out != OutcomeCacheHit {
		t.Fatalf("outcome: %s", out)
	}
	if rates.calls != 0 {
		t.Fatalf("fx calls: %d", rates.calls)
	}
}

func TestProcessStaleFallback(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	p := mustPair(t, "EUR/MXN")
	req := processingReq(t, "u-stale", "EUR/MXN", now)
	repo := newMemUpdates()
	_ = repo.Create(context.Background(), req)
	q, err := domain.NewQuote(p, decimal.RequireFromString("21.5"), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	quotes := newMemQuotes()
	_ = quotes.UpsertLatest(context.Background(), q)
	uc := newProcess(repo, quotes, &stubRates{err: errors.New("down")}, now)
	uc.Freshness = time.Second
	uc.StaleFallback = 5 * time.Minute
	out, err := uc.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if out != OutcomeStaleFallback {
		t.Fatalf("outcome: %s", out)
	}
}

func TestProcessNextEmpty(t *testing.T) {
	uc := ProcessNext{Updates: newMemUpdates()}
	tick, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !tick.Skipped {
		t.Fatal("want skipped")
	}
}

func TestProcessNextClaimsAndCompletes(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	p := mustPair(t, "EUR/MXN")
	req, err := domain.NewUpdateRequest("u3", p, now)
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemUpdates()
	_ = repo.Create(context.Background(), req)
	q, err := domain.NewQuote(p, decimal.RequireFromString("21.5"), now)
	if err != nil {
		t.Fatal(err)
	}
	quotes := newMemQuotes()
	uc := ProcessNext{
		Updates: repo,
		Process: newProcess(repo, quotes, &stubRates{q: q}, now),
	}
	tick, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tick.Outcome != OutcomeCompleted {
		t.Fatalf("outcome: %s", tick.Outcome)
	}
	saved, _ := repo.GetByID(context.Background(), "u3")
	if saved.Status != domain.StatusCompleted {
		t.Fatalf("status: %s", saved.Status)
	}
}

func nowish() time.Time {
	return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
}

func TestRequestIdempotentReplay(t *testing.T) {
	now := nowish()
	repo := newMemUpdates()
	uc := Request{
		Updates: repo,
		Clock:   fixedClock{t: now},
	}
	id1, err := uc.Execute(context.Background(), "EUR/MXN", "client-1", "corr-a")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := uc.Execute(context.Background(), "EUR/MXN", "client-1", "corr-b")
	if err != nil {
		t.Fatal(err)
	}
	if id1 == "" || id2 != id1 {
		t.Fatalf("ids %q %q", id1, id2)
	}
	got, _ := repo.GetByID(context.Background(), id1)
	if got.CorrelationID != "corr-a" {
		t.Fatalf("correlation first-wins: %q", got.CorrelationID)
	}
	if _, err := uc.Execute(context.Background(), "USD/EUR", "client-1", "corr-c"); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestRequestIdempotentTooLong(t *testing.T) {
	uc := Request{
		Updates: newMemUpdates(),
		Clock:   fixedClock{t: nowish()},
	}
	_, err := uc.Execute(context.Background(), "EUR/MXN", strings.Repeat("k", 129), "")
	if !errors.Is(err, domain.ErrInvalidIdempotencyKey) {
		t.Fatalf("got %v", err)
	}
}
