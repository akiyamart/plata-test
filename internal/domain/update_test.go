package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func mustPair(t *testing.T, s string) CurrencyPair {
	t.Helper()
	p, err := ParsePair(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustQuote(t *testing.T, p CurrencyPair, rate string, at time.Time) Quote {
	t.Helper()
	r, err := decimal.NewFromString(rate)
	if err != nil {
		t.Fatal(err)
	}
	q, err := NewQuote(p, r, at)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestUpdateRequest_complete(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	p := mustPair(t, "EUR/MXN")
	u, err := NewUpdateRequest("upd-1", p, now)
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != StatusPending {
		t.Fatalf("status = %s, want pending", u.Status)
	}

	later := now.Add(time.Second)
	if err := u.MarkProcessing(later); err != nil {
		t.Fatal(err)
	}
	if u.Status != StatusProcessing {
		t.Fatalf("status = %s, want processing", u.Status)
	}

	q := mustQuote(t, p, "21.45", later)
	if err := u.Complete(q, later.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if u.Status != StatusCompleted {
		t.Fatalf("status = %s, want completed", u.Status)
	}
	if u.Rate == nil || !u.Rate.Equal(q.Rate) {
		t.Fatalf("rate = %v, want %s", u.Rate, q.Rate)
	}
	if u.ObservedAt == nil || !u.ObservedAt.Equal(q.ObservedAt) {
		t.Fatalf("observedAt = %v, want %s", u.ObservedAt, q.ObservedAt)
	}
	if u.Error != "" {
		t.Fatalf("error = %q, want empty", u.Error)
	}
}

func TestUpdateRequest_fail(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	u, err := NewUpdateRequest("upd-2", mustPair(t, "USD/EUR"), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.MarkProcessing(now); err != nil {
		t.Fatal(err)
	}
	if err := u.Fail("fx timeout", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if u.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", u.Status)
	}
	if u.Error != "fx timeout" {
		t.Fatalf("error = %q", u.Error)
	}
	if u.Rate != nil || u.ObservedAt != nil {
		t.Fatalf("rate/observedAt must be nil after fail")
	}
}

func TestUpdateRequest_invalidTransitions(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	p := mustPair(t, "EUR/MXN")
	q := mustQuote(t, p, "20", now)

	u, err := NewUpdateRequest("upd-3", p, now)
	if err != nil {
		t.Fatal(err)
	}
	before := u

	if err := u.Complete(q, now); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Errorf("complete from pending: %v", err)
	}
	if err := u.Fail("nope", now); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Errorf("fail from pending: %v", err)
	}
	if u != before {
		t.Fatal("failed transition mutated request")
	}

	if err := u.MarkProcessing(now); err != nil {
		t.Fatal(err)
	}
	if err := u.Fail("  ", now); !errors.Is(err, ErrEmptyFailMessage) {
		t.Fatalf("empty fail message: %v", err)
	}
	if u.Status != StatusProcessing {
		t.Fatal("empty fail message mutated status")
	}

	other := mustQuote(t, mustPair(t, "USD/MXN"), "18", now)
	if err := u.Complete(other, now); !errors.Is(err, ErrInvalidPair) {
		t.Errorf("complete other pair: %v", err)
	}
	if u.Status != StatusProcessing {
		t.Fatal("mismatched pair mutated status")
	}

	if err := u.Complete(q, now); err != nil {
		t.Fatal(err)
	}
	if err := u.MarkProcessing(now); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Errorf("processing from completed: %v", err)
	}
}

func TestParseStatus(t *testing.T) {
	for _, s := range []Status{StatusPending, StatusProcessing, StatusCompleted, StatusFailed} {
		got, err := ParseStatus(string(s))
		if err != nil || got != s {
			t.Fatalf("ParseStatus(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := ParseStatus("nope"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("invalid: %v", err)
	}
}

func TestNewUpdateRequest_emptyID(t *testing.T) {
	_, err := NewUpdateRequest("  ", mustPair(t, "EUR/MXN"), time.Now())
	if !errors.Is(err, ErrEmptyID) {
		t.Fatalf("empty id: %v", err)
	}
}
