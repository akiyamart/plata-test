package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"quoteservice/internal/domain"
)

const upsertLatestSQL = `
	INSERT INTO quotes (pair, rate, observed_at, updated_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (pair) DO UPDATE
	SET rate = EXCLUDED.rate,
	    observed_at = EXCLUDED.observed_at,
	    updated_at = EXCLUDED.updated_at
	WHERE quotes.observed_at <= EXCLUDED.observed_at`

type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func upsertLatest(ctx context.Context, db execer, q domain.Quote) error {
	now := time.Now().UTC()
	_, err := db.Exec(ctx, upsertLatestSQL, q.Pair.String(), q.Rate.String(), q.ObservedAt, now)
	return err
}
