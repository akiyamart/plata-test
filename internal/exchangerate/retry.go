package exchangerate

import (
	"context"
	"math/rand"
	"time"

	"quoteservice/internal/domain"
)

type Retry struct {
	next     Fetcher
	attempts int
	base     time.Duration
}

func NewRetry(next Fetcher, attempts int, base time.Duration) *Retry {
	if attempts <= 0 {
		attempts = 3
	}
	if base <= 0 {
		base = 200 * time.Millisecond
	}
	return &Retry{next: next, attempts: attempts, base: base}
}

func (r *Retry) Fetch(ctx context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	var last error
	for i := 0; i < r.attempts; i++ {
		if err := ctx.Err(); err != nil {
			return domain.Quote{}, err
		}
		q, err := r.next.Fetch(ctx, p)
		if err == nil {
			return q, nil
		}
		last = err
		if !retryable(err) || i == r.attempts-1 {
			return domain.Quote{}, err
		}
		delay := r.base * time.Duration(1<<i)
		if ra := retryAfterOf(err); ra > delay {
			delay = ra
		}
		// ponytail: jitter up to 50% of delay; upgrade to full decorrelated jitter if FX p99 wait matters.
		jitter := time.Duration(rand.Int63n(int64(delay/2 + 1)))
		timer := time.NewTimer(delay/2 + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return domain.Quote{}, ctx.Err()
		case <-timer.C:
		}
	}
	return domain.Quote{}, last
}
