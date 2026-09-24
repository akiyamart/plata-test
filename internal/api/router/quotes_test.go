package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"quoteservice/internal/api/router"
	"quoteservice/internal/domain"
	"quoteservice/internal/service"
)

type memUpdates struct {
	byID map[string]domain.UpdateRequest
}

func (m *memUpdates) Create(_ context.Context, req domain.UpdateRequest) error {
	if m.byID == nil {
		m.byID = map[string]domain.UpdateRequest{}
	}
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
	r, ok := m.byID[id]
	if !ok {
		return domain.UpdateRequest{}, domain.ErrUpdateNotFound
	}
	return r, nil
}
func (m *memUpdates) ClaimNextPending(context.Context) (domain.UpdateRequest, error) {
	return domain.UpdateRequest{}, domain.ErrUpdateNotFound
}
func (m *memUpdates) CountPending(context.Context) (int, error) { return len(m.byID), nil }
func (m *memUpdates) GetByIdempotencyKey(_ context.Context, key string) (domain.UpdateRequest, error) {
	for _, r := range m.byID {
		if r.IdempotencyKey == key && key != "" {
			return r, nil
		}
	}
	return domain.UpdateRequest{}, domain.ErrUpdateNotFound
}
func (m *memUpdates) Save(_ context.Context, req domain.UpdateRequest) error {
	m.byID[req.ID] = req
	return nil
}

type clk struct{ t time.Time }

func (c clk) Now() time.Time { return c.t }

type memQuotes struct{ q *domain.Quote }

func (m *memQuotes) UpsertLatest(_ context.Context, q domain.Quote) error { m.q = &q; return nil }
func (m *memQuotes) GetLatest(_ context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	if m.q == nil {
		return domain.Quote{}, domain.ErrQuoteNotFound
	}
	return *m.q, nil
}

func TestPostUpdateAccepted(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	repo := &memUpdates{}
	s := router.Server{
		Request: service.Request{Updates: repo, Clock: clk{t: now}},
	}
	body, _ := json.Marshal(map[string]string{"pair": "EUR/MXN"})
	req := httptest.NewRequest(http.MethodPost, "/v1/quotes/updates", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing request id")
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["update_id"] == "" {
		t.Fatal("empty update_id")
	}
}

func TestPostUpdateIdempotent(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	s := router.Server{
		Request: service.Request{Updates: &memUpdates{}, Clock: clk{t: now}},
	}
	body, _ := json.Marshal(map[string]string{"pair": "EUR/MXN"})
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/quotes/updates", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "k1")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}
	a := post()
	b := post()
	if a.Code != http.StatusAccepted || b.Code != http.StatusAccepted {
		t.Fatalf("status %d %d", a.Code, b.Code)
	}
	if a.Body.String() != b.Body.String() {
		t.Fatalf("bodies %s vs %s", a.Body.String(), b.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/quotes/updates", bytes.NewReader([]byte(`{"pair":"USD/EUR"}`)))
	req.Header.Set("Idempotency-Key", "k1")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d %s", rec.Code, rec.Body.Bytes())
	}
}

func TestPostUpdateInvalidPair(t *testing.T) {
	s := router.Server{
		Request: service.Request{Updates: &memUpdates{}, Clock: clk{t: time.Now()}},
	}
	body, _ := json.Marshal(map[string]string{"pair": "EUR/EUR"})
	req := httptest.NewRequest(http.MethodPost, "/v1/quotes/updates", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestPostUpdateUnknownField(t *testing.T) {
	s := router.Server{
		Request: service.Request{Updates: &memUpdates{}, Clock: clk{t: time.Now()}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/quotes/updates", strings.NewReader(`{"pair":"EUR/MXN","extra":1}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestPostUpdateTooLarge(t *testing.T) {
	s := router.Server{
		Request: service.Request{Updates: &memUpdates{}, Clock: clk{t: time.Now()}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/quotes/updates", strings.NewReader(`{"pair":"`+strings.Repeat("A", 5000)+`"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("want 413, got 400 %s", rec.Body.Bytes())
	}
}

func TestGetUpdateNotFound(t *testing.T) {
	s := router.Server{Get: service.Get{Updates: &memUpdates{}}}
	req := httptest.NewRequest(http.MethodGet, "/v1/quotes/updates/missing", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestGetLatest(t *testing.T) {
	p, _ := domain.ParsePair("EUR/MXN")
	q, _ := domain.NewQuote(p, decimal.RequireFromString("21.5"), time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC))
	s := router.Server{GetLatest: service.GetLatest{Quotes: &memQuotes{q: &q}}}
	req := httptest.NewRequest(http.MethodGet, "/v1/quotes/EUR-MXN", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.Bytes())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["pair"] != "EUR/MXN" || resp["rate"] != "21.5" {
		t.Fatalf("body %+v", resp)
	}
}
