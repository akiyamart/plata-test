package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"quoteservice/internal/domain"
)

const updateCols = `id, pair, status, rate::text, observed_at, error, attempts, idempotency_key, correlation_id, created_at, updated_at`

// attempts is the claim generation: reclaim increments it, Complete and Fail do not.
const saveUpdateSQL = `
	UPDATE update_requests
	SET status = $2, rate = $3, observed_at = $4, error = $5, updated_at = $6, attempts = $7
	WHERE id = $1 AND status = 'processing' AND attempts = $7`

// claimSaveSQL updates a row held under FOR UPDATE (pending claim or reclaim).
const claimSaveSQL = `
	UPDATE update_requests
	SET status = $2, rate = $3, observed_at = $4, error = $5, updated_at = $6, attempts = $7
	WHERE id = $1 AND status IN ('pending', 'processing')`

type UpdateRepository struct {
	pool        *pgxpool.Pool
	lease       time.Duration
	maxAttempts int
	onReclaim   func()
}

func NewUpdateRepository(pool *pgxpool.Pool, lease time.Duration, maxAttempts int) *UpdateRepository {
	if lease <= 0 {
		lease = 45 * time.Second
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &UpdateRepository{pool: pool, lease: lease, maxAttempts: maxAttempts}
}

func (r *UpdateRepository) OnReclaim(fn func()) { r.onReclaim = fn }

func (r *UpdateRepository) Create(ctx context.Context, req domain.UpdateRequest) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO update_requests (id, pair, status, rate, observed_at, error, attempts, idempotency_key, correlation_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		req.ID, req.Pair.String(), string(req.Status), decimalPtr(req.Rate), req.ObservedAt, req.Error, req.Attempts, nullIfEmpty(req.IdempotencyKey), req.CorrelationID, req.CreatedAt, req.UpdatedAt,
	)
	if isUniqueViolation(err) {
		return domain.ErrDuplicateIdempotencyKey
	}
	return err
}

func (r *UpdateRepository) GetByID(ctx context.Context, id string) (domain.UpdateRequest, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+updateCols+` FROM update_requests WHERE id = $1`, id)
	req, err := scanUpdate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UpdateRequest{}, domain.ErrUpdateNotFound
	}
	return req, err
}

func (r *UpdateRepository) GetByIdempotencyKey(ctx context.Context, key string) (domain.UpdateRequest, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+updateCols+` FROM update_requests WHERE idempotency_key = $1`, key)
	req, err := scanUpdate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UpdateRequest{}, domain.ErrUpdateNotFound
	}
	return req, err
}

func (r *UpdateRepository) CountPending(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM update_requests WHERE status IN ('pending', 'processing')`).Scan(&n)
	return n, err
}

func (r *UpdateRepository) ClaimNextPending(ctx context.Context) (domain.UpdateRequest, error) {
	for {
		req, err := r.claimOne(ctx)
		if err != nil {
			return domain.UpdateRequest{}, err
		}
		if req.Status == domain.StatusFailed {
			continue
		}
		return req, nil
	}
}

func (r *UpdateRepository) claimOne(ctx context.Context) (domain.UpdateRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.UpdateRequest{}, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT `+updateCols+`
		FROM update_requests
		WHERE status = 'pending'
		   OR (status = 'processing' AND updated_at < NOW() - $1::interval)
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1`, fmt.Sprintf("%d milliseconds", r.lease.Milliseconds()))
	req, err := scanUpdate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UpdateRequest{}, domain.ErrUpdateNotFound
	}
	if err != nil {
		return domain.UpdateRequest{}, err
	}

	reclaimed := req.Status == domain.StatusProcessing
	now := time.Now().UTC()
	req.Attempts++
	if req.Status == domain.StatusPending {
		if err := req.MarkProcessing(now); err != nil {
			return domain.UpdateRequest{}, err
		}
	} else {
		req.UpdatedAt = now
	}

	if req.Attempts > r.maxAttempts {
		if err := req.Fail(domain.FailMaxAttempts, now); err != nil {
			return domain.UpdateRequest{}, err
		}
	}

	_, err = tx.Exec(ctx, claimSaveSQL,
		req.ID, string(req.Status), decimalPtr(req.Rate), req.ObservedAt, req.Error, req.UpdatedAt, req.Attempts,
	)
	if err != nil {
		return domain.UpdateRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UpdateRequest{}, err
	}
	if reclaimed && r.onReclaim != nil {
		r.onReclaim()
	}
	return req, nil
}

func (r *UpdateRepository) Save(ctx context.Context, req domain.UpdateRequest) error {
	tag, err := r.pool.Exec(ctx, saveUpdateSQL,
		req.ID, string(req.Status), decimalPtr(req.Rate), req.ObservedAt, req.Error, req.UpdatedAt, req.Attempts,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrLostLease
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUpdate(row scannable) (domain.UpdateRequest, error) {
	var (
		req        domain.UpdateRequest
		pairStr    string
		status     string
		rateStr    *string
		observedAt *time.Time
		idemKey    *string
	)
	err := row.Scan(
		&req.ID, &pairStr, &status, &rateStr, &observedAt, &req.Error, &req.Attempts, &idemKey, &req.CorrelationID, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		return domain.UpdateRequest{}, err
	}
	p, err := domain.ParsePair(pairStr)
	if err != nil {
		return domain.UpdateRequest{}, fmt.Errorf("stored pair: %w", err)
	}
	st, err := domain.ParseStatus(status)
	if err != nil {
		return domain.UpdateRequest{}, fmt.Errorf("stored status: %w", err)
	}
	req.Pair = p
	req.Status = st
	if rateStr != nil {
		d, err := decimal.NewFromString(*rateStr)
		if err != nil {
			return domain.UpdateRequest{}, fmt.Errorf("rate: %w", err)
		}
		req.Rate = &d
	}
	req.ObservedAt = observedAt
	if idemKey != nil {
		req.IdempotencyKey = *idemKey
	}
	return req, nil
}

func decimalPtr(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return d.String()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
