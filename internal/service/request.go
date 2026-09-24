package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"quoteservice/internal/domain"
)

const maxIdempotencyKey = 128

// Request creates a pending quote update job and returns its id.
type Request struct {
	Updates    UpdateRepository
	Clock      Clock
	MaxPending int
}

func (uc Request) Execute(ctx context.Context, pairRaw, idempotencyKey, correlationID string) (string, error) {
	p, err := domain.ParsePair(pairRaw)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(idempotencyKey)
	if len(key) > maxIdempotencyKey {
		return "", domain.ErrInvalidIdempotencyKey
	}
	if key != "" {
		existing, err := uc.Updates.GetByIdempotencyKey(ctx, key)
		if err == nil {
			if existing.Pair != p {
				return "", domain.ErrIdempotencyConflict
			}
			return existing.ID, nil
		}
		if !errors.Is(err, domain.ErrUpdateNotFound) {
			return "", err
		}
	}
	if uc.MaxPending > 0 {
		n, err := uc.Updates.CountPending(ctx)
		if err != nil {
			return "", err
		}
		if n >= uc.MaxPending {
			return "", ErrQueueFull
		}
	}
	now := uc.Clock.Now()
	req, err := domain.NewUpdateRequest(uuid.NewString(), p, now)
	if err != nil {
		return "", err
	}
	req.IdempotencyKey = key
	req.CorrelationID = strings.TrimSpace(correlationID)
	if err := uc.Updates.Create(ctx, req); err != nil {
		if key != "" && errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			existing, getErr := uc.Updates.GetByIdempotencyKey(ctx, key)
			if getErr != nil {
				return "", getErr
			}
			if existing.Pair != p {
				return "", domain.ErrIdempotencyConflict
			}
			return existing.ID, nil
		}
		return "", err
	}
	return req.ID, nil
}
