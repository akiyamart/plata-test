package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"quoteservice/internal/domain"
)

type latestMemQuotes struct {
	byPair map[string]domain.Quote
}

func (m *latestMemQuotes) UpsertLatest(_ context.Context, q domain.Quote) error {
	if m.byPair == nil {
		m.byPair = map[string]domain.Quote{}
	}
	m.byPair[q.Pair.String()] = q
	return nil
}

func (m *latestMemQuotes) GetLatest(_ context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	q, ok := m.byPair[p.String()]
	if !ok {
		return domain.Quote{}, domain.ErrQuoteNotFound
	}
	return q, nil
}

func TestGetLatest(t *testing.T) {
	p, err := domain.ParsePair("EUR/MXN")
	if err != nil {
		t.Fatal(err)
	}
	q, err := domain.NewQuote(p, decimal.RequireFromString("21.5"), time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	repo := &latestMemQuotes{byPair: map[string]domain.Quote{p.String(): q}}
	got, err := (GetLatest{Quotes: repo}).Execute(context.Background(), "EUR-MXN")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Rate.Equal(q.Rate) {
		t.Fatalf("rate: %s", got.Rate)
	}
}

func TestGetLatestInvalidPair(t *testing.T) {
	_, err := (GetLatest{Quotes: &latestMemQuotes{}}).Execute(context.Background(), "EUR/EUR")
	if !errors.Is(err, domain.ErrInvalidPair) {
		t.Fatalf("want ErrInvalidPair, got %v", err)
	}
}
