package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"quoteservice/internal/domain"
)

type Completion struct {
	pool *pgxpool.Pool
}

func NewCompletion(pool *pgxpool.Pool) *Completion {
	return &Completion{pool: pool}
}

func (c *Completion) SaveCompleted(ctx context.Context, req domain.UpdateRequest, q domain.Quote) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, saveUpdateSQL,
		req.ID, string(req.Status), decimalPtr(req.Rate), req.ObservedAt, req.Error, req.UpdatedAt, req.Attempts,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrLostLease
	}
	if err := upsertLatest(ctx, tx, q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
