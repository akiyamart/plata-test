package quote

import (
	"context"

	"quoteservice/internal/domain"
)

type GetLatestUseCase interface {
	Execute(ctx context.Context, pair string) (domain.Quote, error)
}
