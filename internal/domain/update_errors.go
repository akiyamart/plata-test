package domain

import "errors"

var (
	ErrUpdateNotFound          = errors.New("not found")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
	ErrInvalidStatus           = errors.New("invalid status")
	ErrEmptyID                 = errors.New("empty update request id")
	ErrEmptyFailMessage        = errors.New("empty fail message")
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	ErrIdempotencyConflict     = errors.New("idempotency key reused with a different pair")
	ErrInvalidIdempotencyKey   = errors.New("invalid idempotency key")
	ErrLostLease               = errors.New("lost processing lease")
)

// Stable fail codes persisted on UpdateRequest.Error and returned to clients.
const (
	FailFXUnavailable = "fx_unavailable"
	FailFXTimeout     = "fx_timeout"
	FailMaxAttempts   = "max_attempts"
	FailInvalidQuote  = "invalid_quote"
)
