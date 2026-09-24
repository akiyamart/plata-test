package service

import (
	"context"
	"errors"
	"time"

	"quoteservice/internal/domain"
)

// UpdateRepository persists update request aggregates.
type UpdateRepository interface {
	Create(ctx context.Context, req domain.UpdateRequest) error
	GetByID(ctx context.Context, id string) (domain.UpdateRequest, error)
	ClaimNextPending(ctx context.Context) (domain.UpdateRequest, error)
	CountPending(ctx context.Context) (int, error)
	GetByIdempotencyKey(ctx context.Context, key string) (domain.UpdateRequest, error)
	Save(ctx context.Context, req domain.UpdateRequest) error
}

// QuoteRepository loads the latest stored quote for a currency pair.
type QuoteRepository interface {
	GetLatest(ctx context.Context, pair domain.CurrencyPair) (domain.Quote, error)
}

// RateProvider fetches a live quote from an external source.
type RateProvider interface {
	Fetch(ctx context.Context, pair domain.CurrencyPair) (domain.Quote, error)
}

// CompletionWriter atomically persists a completed update and its quote.
type CompletionWriter interface {
	SaveCompleted(ctx context.Context, req domain.UpdateRequest, quote domain.Quote) error
}

type Clock interface {
	Now() time.Time
}

var ErrQueueFull = errors.New("update queue full")

type Outcome string

const (
	OutcomeCompleted     Outcome = "completed"
	OutcomeFailed        Outcome = "failed"
	OutcomeCacheHit      Outcome = "cache_hit"
	OutcomeStaleFallback Outcome = "stale_fallback"
	OutcomeLostLease     Outcome = "lost_lease"
)
