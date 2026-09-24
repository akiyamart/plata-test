package service

import (
	"context"

	"quoteservice/internal/domain"
)

// GetLatest returns the latest stored quote for a pair.
type GetLatest struct {
	Quotes QuoteRepository
}

func (uc GetLatest) Execute(ctx context.Context, pairRaw string) (domain.Quote, error) {
	p, err := domain.ParsePair(pairRaw)
	if err != nil {
		return domain.Quote{}, err
	}
	return uc.Quotes.GetLatest(ctx, p)
}
