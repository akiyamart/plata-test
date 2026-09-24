package service

import (
	"context"
	"errors"
	"quoteservice/internal/api/apictx"
	"quoteservice/internal/domain"
	"time"
)

// Process finishes a claimed (processing) update request.
type Process struct {
	Updates       UpdateRepository
	Quotes        QuoteRepository
	Complete      CompletionWriter
	Rates         RateProvider
	Clock         Clock
	Freshness     time.Duration
	StaleFallback time.Duration
	OnCacheHit    func()
}

func (uc Process) Execute(ctx context.Context, req domain.UpdateRequest) (Outcome, error) {
	now := uc.Clock.Now()
	latest, err := uc.Quotes.GetLatest(ctx, req.Pair)
	hasLatest := err == nil
	if err != nil && !errors.Is(err, domain.ErrQuoteNotFound) {
		return "", err
	}
	if hasLatest && uc.Freshness > 0 && now.Sub(latest.ObservedAt) <= uc.Freshness {
		if err := req.Complete(latest, now); err != nil {
			return "", err
		}
		if uc.OnCacheHit != nil {
			uc.OnCacheHit()
		}
		return finish(OutcomeCacheHit, uc.Complete.SaveCompleted(ctx, req, latest))
	}
	q, err := uc.Rates.Fetch(apictx.WithUpdateID(apictx.With(ctx, req.CorrelationID), req.ID), req.Pair)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if hasLatest && uc.StaleFallback > 0 && now.Sub(latest.ObservedAt) <= uc.StaleFallback {
			if err := req.Complete(latest, now); err != nil {
				return "", err
			}
			return finish(OutcomeStaleFallback, uc.Complete.SaveCompleted(ctx, req, latest))
		}
		if failErr := req.Fail(failCode(err), now); failErr != nil {
			return "", failErr
		}
		return finish(OutcomeFailed, uc.Updates.Save(ctx, req))
	}
	if err := req.Complete(q, now); err != nil {
		if failErr := req.Fail(domain.FailInvalidQuote, now); failErr != nil {
			return "", failErr
		}
		return finish(OutcomeFailed, uc.Updates.Save(ctx, req))
	}
	return finish(OutcomeCompleted, uc.Complete.SaveCompleted(ctx, req, q))
}

func finish(out Outcome, err error) (Outcome, error) {
	if errors.Is(err, domain.ErrLostLease) {
		return OutcomeLostLease, nil
	}
	return out, err
}

type statusCoder interface {
	StatusCode() int
}

func failCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return domain.FailFXTimeout
	}
	var sc statusCoder
	if errors.As(err, &sc) {
		if sc.StatusCode() == 429 || sc.StatusCode() >= 500 || sc.StatusCode() == 0 {
			return domain.FailFXUnavailable
		}
	}
	return domain.FailFXUnavailable
}
