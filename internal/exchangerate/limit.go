package exchangerate

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/time/rate"

	"quoteservice/internal/domain"
	"quoteservice/internal/metrics"
)

type Fetcher interface {
	Fetch(ctx context.Context, p domain.CurrencyPair) (domain.Quote, error)
}

// Limit is a token-bucket decorator around Fetcher.
type Limit struct {
	next  Fetcher
	lim   *rate.Limiter
	stats *metrics.Stats
	log   *slog.Logger
}

func NewLimit(next Fetcher, rps float64, burst int, stats *metrics.Stats, log *slog.Logger) *Limit {
	if rps <= 0 {
		rps = 2
	}
	if burst <= 0 {
		burst = 1
	}
	return &Limit{next: next, lim: rate.NewLimiter(rate.Limit(rps), burst), stats: stats, log: log}
}

func (l *Limit) Fetch(ctx context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	start := time.Now()
	if err := l.lim.Wait(ctx); err != nil {
		return domain.Quote{}, wrapErr(0, 0, err)
	}
	if waited := time.Since(start); waited > time.Millisecond && l.stats != nil {
		l.stats.RateLimitWaits.Add(1)
		l.stats.RateLimitWaitNs.Add(waited.Nanoseconds())
	}
	return l.next.Fetch(ctx, p)
}
