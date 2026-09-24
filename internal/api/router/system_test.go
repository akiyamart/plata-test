package router_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"quoteservice/internal/api/router"
	"quoteservice/internal/metrics"
)

func TestReadyzNotReady(t *testing.T) {
	s := router.Server{Ready: func(context.Context) error { return context.DeadlineExceeded }}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s := router.Server{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestMetrics(t *testing.T) {
	stats := &metrics.Stats{}
	stats.CacheHits.Add(3)
	s := router.Server{Stats: stats}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"quote_cache_hit_total":3`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestRecoverPanic(t *testing.T) {
	s := router.Server{Ready: func(context.Context) error { panic("ready boom") }}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.Bytes())
	}
	if !strings.Contains(rec.Body.String(), `"error":"internal error"`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestRequestIDValidation(t *testing.T) {
	s := router.Server{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "bad\nid")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	got := rec.Header().Get("X-Request-Id")
	if got == "" || strings.Contains(got, "\n") {
		t.Fatalf("request id %q", got)
	}
	if got == "bad\nid" {
		t.Fatal("invalid inbound request id was trusted")
	}
}

func TestCORSPreflight(t *testing.T) {
	s := router.Server{}
	req := httptest.NewRequest(http.MethodOptions, "/v1/quotes/updates", nil)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Idempotency-Key")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("ACA-Origin %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
