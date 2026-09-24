package exchangerate

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"quoteservice/internal/api/apictx"
	"quoteservice/internal/domain"
	"quoteservice/internal/metrics"
)

// Observe records fetch duration and result around a Fetcher.
type Observe struct {
	next  Fetcher
	stats *metrics.Stats
	log   *slog.Logger
}

func NewObserve(next Fetcher, stats *metrics.Stats, log *slog.Logger) *Observe {
	return &Observe{next: next, stats: stats, log: log}
}

func (o *Observe) Fetch(ctx context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	start := time.Now()
	q, err := o.next.Fetch(ctx, p)
	d := time.Since(start)
	result := "success"
	if err != nil {
		result = "error"
		var fe *Error
		if errors.As(err, &fe) {
			switch {
			case fe.Status == 429:
				result = "429"
			case fe.Status >= 500:
				result = "5xx"
			case fe.Status == 0:
				result = "timeout"
			default:
				result = "4xx"
			}
		}
		if o.stats != nil {
			o.stats.FXErrors.Add(1)
		}
	}
	if o.stats != nil {
		o.stats.FXFetches.Add(1)
		o.stats.FXFetchNs.Add(d.Nanoseconds())
	}
	if o.log != nil {
		attrs := []any{
			"pair", p.String(),
			"result", result,
			"duration_ms", d.Milliseconds(),
			"correlation_id", apictx.From(ctx),
			"update_id", apictx.UpdateID(ctx),
		}
		if err != nil {
			attrs = append(attrs, "err", err)
			o.log.Error("fx fetch", attrs...)
		} else {
			o.log.Info("fx fetch", attrs...)
		}
	}
	return q, err
}
