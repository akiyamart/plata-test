package exchangerate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"quoteservice/internal/domain"
)

// Error is a typed FX provider failure. Status 0 means a transport/timeout error.
type Error struct {
	Status     int
	RetryAfter time.Duration
	err        error
}

func (e *Error) Error() string {
	if e == nil || e.err == nil {
		return "fx error"
	}
	return e.err.Error()
}

func (e *Error) Unwrap() error { return e.err }

func (e *Error) StatusCode() int { return e.Status }

func (e *Error) Retryable() bool {
	if e == nil {
		return false
	}
	if e.Status == 0 || e.Status == http.StatusTooManyRequests || e.Status >= 500 {
		return true
	}
	return false
}

func wrapErr(status int, retryAfter time.Duration, err error) error {
	return &Error{Status: status, RetryAfter: retryAfter, err: err}
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, domain.ErrInvalidQuote) {
		return false
	}
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Retryable()
	}
	return true
}

func retryAfterOf(err error) time.Duration {
	var fe *Error
	if errors.As(err, &fe) {
		return fe.RetryAfter
	}
	return 0
}

func fmtProviderErr(status int, code string) error {
	return fmt.Errorf("fx provider status=%d code=%s", status, code)
}
