package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestNewQuote_invalid(t *testing.T) {
	p, err := ParsePair("EUR/MXN")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

	_, err = NewQuote(p, decimal.Zero, now)
	if !errors.Is(err, ErrInvalidQuote) {
		t.Errorf("zero rate: error = %v, want %v", err, ErrInvalidQuote)
	}
	_, err = NewQuote(p, decimal.NewFromInt(-1), now)
	if !errors.Is(err, ErrInvalidQuote) {
		t.Errorf("negative rate: error = %v, want %v", err, ErrInvalidQuote)
	}
	_, err = NewQuote(p, decimal.RequireFromString("1.2"), time.Time{})
	if !errors.Is(err, ErrInvalidQuote) {
		t.Errorf("zero time: error = %v, want %v", err, ErrInvalidQuote)
	}
}
