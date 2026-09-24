package exchangerate_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"quoteservice/internal/domain"
	"quoteservice/internal/exchangerate"
	"quoteservice/internal/metrics"
)

func TestClientFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rate/eur-mxn" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result":          "success",
			"base":            "EUR",
			"quote":           "MXN",
			"rate":            "21.45",
			"data_updated_at": time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC),
		})
	}))
	defer srv.Close()

	client := exchangerate.NewClient(exchangerate.Config{BaseURL: srv.URL, Timeout: time.Second})
	p, err := domain.ParsePair("EUR/MXN")
	if err != nil {
		t.Fatal(err)
	}
	q, err := client.Fetch(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if q.Pair != p || q.Rate.String() != "21.45" {
		t.Fatalf("quote: %+v", q)
	}
}

func TestClientFetch429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "error", "code": "rate_limit", "message": "slow down"})
	}))
	defer srv.Close()
	client := exchangerate.NewClient(exchangerate.Config{BaseURL: srv.URL, Timeout: time.Second})
	p, _ := domain.ParsePair("EUR/MXN")
	_, err := client.Fetch(context.Background(), p)
	var fe *exchangerate.Error
	if !errors.As(err, &fe) || !fe.Retryable() || fe.StatusCode() != 429 {
		t.Fatalf("got %v", err)
	}
}

func TestRetryStopsOn400(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "error", "code": "bad"})
	}))
	defer srv.Close()
	inner := exchangerate.NewClient(exchangerate.Config{BaseURL: srv.URL, Timeout: time.Second})
	r := exchangerate.NewRetry(inner, 3, time.Millisecond)
	p, _ := domain.ParsePair("USD/EUR")
	_, err := r.Fetch(context.Background(), p)
	if err == nil {
		t.Fatal("want error")
	}
	if n != 1 {
		t.Fatalf("calls: %d", n)
	}
}

func TestRetryOn500(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"result": "error"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result":          "success",
			"rate":            "1.1",
			"data_updated_at": time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC),
		})
	}))
	defer srv.Close()
	r := exchangerate.NewRetry(exchangerate.NewClient(exchangerate.Config{BaseURL: srv.URL, Timeout: time.Second}), 3, time.Millisecond)
	p, _ := domain.ParsePair("USD/EUR")
	q, err := r.Fetch(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || q.Rate.String() != "1.1" {
		t.Fatalf("n=%d q=%v", n, q)
	}
}

func TestLimitWaits(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result":          "success",
			"rate":            "1.1",
			"data_updated_at": time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC),
		})
	}))
	defer srv.Close()
	stats := &metrics.Stats{}
	inner := exchangerate.NewClient(exchangerate.Config{BaseURL: srv.URL, Timeout: time.Second})
	lim := exchangerate.NewLimit(inner, 10, 1, stats, nil)
	p, _ := domain.ParsePair("USD/EUR")
	if _, err := lim.Fetch(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := lim.Fetch(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatalf("expected limiter to delay second call, took %s", time.Since(start))
	}
	if n != 2 {
		t.Fatalf("calls: %d", n)
	}
}
