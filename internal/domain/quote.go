package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

type Quote struct {
	Pair       CurrencyPair
	Rate       decimal.Decimal
	ObservedAt time.Time
}

func NewQuote(p CurrencyPair, rate decimal.Decimal, observedAt time.Time) (Quote, error) {
	if !rate.IsPositive() {
		return Quote{}, fmt.Errorf("%w: rate must be positive", ErrInvalidQuote)
	}
	if observedAt.IsZero() {
		return Quote{}, fmt.Errorf("%w: observedAt required", ErrInvalidQuote)
	}
	return Quote{Pair: p, Rate: rate, ObservedAt: observedAt}, nil
}
