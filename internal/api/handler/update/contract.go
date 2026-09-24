package update

import (
	"context"

	"quoteservice/internal/domain"
)

type RequestUseCase interface {
	Execute(ctx context.Context, pair, idempotencyKey, correlationID string) (string, error)
}

type GetUseCase interface {
	Execute(ctx context.Context, id string) (domain.UpdateRequest, error)
}
