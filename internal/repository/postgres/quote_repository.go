package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"quoteservice/internal/domain"
)

type QuoteRepository struct {
	pool *pgxpool.Pool
}

func NewQuoteRepository(pool *pgxpool.Pool) *QuoteRepository {
	return &QuoteRepository{pool: pool}
}

func (r *QuoteRepository) UpsertLatest(ctx context.Context, q domain.Quote) error {
	return upsertLatest(ctx, r.pool, q)
}

func (r *QuoteRepository) GetLatest(ctx context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	var (
		pairStr    string
		rateStr    string
		observedAt time.Time
	)
	err := r.pool.QueryRow(ctx, `
		SELECT pair, rate::text, observed_at FROM quotes WHERE pair = $1`, p.String(),
	).Scan(&pairStr, &rateStr, &observedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Quote{}, domain.ErrQuoteNotFound
	}
	if err != nil {
		return domain.Quote{}, err
	}
	parsed, err := domain.ParsePair(pairStr)
	if err != nil {
		return domain.Quote{}, err
	}
	rate, err := decimal.NewFromString(rateStr)
	if err != nil {
		return domain.Quote{}, err
	}
	return domain.NewQuote(parsed, rate, observedAt)
}
