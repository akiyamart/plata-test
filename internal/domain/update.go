package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case StatusPending, StatusProcessing, StatusCompleted, StatusFailed:
		return Status(s), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidStatus, s)
	}
}

type UpdateRequest struct {
	ID             string
	Pair           CurrencyPair
	Status         Status
	Rate           *decimal.Decimal // nil until completed
	ObservedAt     *time.Time
	Error          string // only failed
	Attempts       int
	IdempotencyKey string
	CorrelationID  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewUpdateRequest(id string, p CurrencyPair, now time.Time) (UpdateRequest, error) {
	if strings.TrimSpace(id) == "" {
		return UpdateRequest{}, ErrEmptyID
	}
	return UpdateRequest{
		ID:        id,
		Pair:      p,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (u *UpdateRequest) MarkProcessing(now time.Time) error {
	if u.Status != StatusPending {
		return fmt.Errorf("%w: %s -> processing", ErrInvalidStatusTransition, u.Status)
	}
	u.Status = StatusProcessing
	u.UpdatedAt = now
	return nil
}

func (u *UpdateRequest) Complete(q Quote, now time.Time) error {
	if u.Status != StatusProcessing {
		return fmt.Errorf("%w: %s -> completed", ErrInvalidStatusTransition, u.Status)
	}
	if _, err := NewQuote(q.Pair, q.Rate, q.ObservedAt); err != nil {
		return err
	}
	if q.Pair != u.Pair {
		return fmt.Errorf("%w: quote pair %s != %s", ErrInvalidPair, q.Pair, u.Pair)
	}
	rate := q.Rate
	observed := q.ObservedAt
	u.Status = StatusCompleted
	u.Rate = &rate
	u.ObservedAt = &observed
	u.Error = ""
	u.UpdatedAt = now
	return nil
}

func (u *UpdateRequest) Fail(msg string, now time.Time) error {
	if u.Status != StatusProcessing {
		return fmt.Errorf("%w: %s -> failed", ErrInvalidStatusTransition, u.Status)
	}
	if strings.TrimSpace(msg) == "" {
		return ErrEmptyFailMessage
	}
	u.Status = StatusFailed
	u.Error = msg
	u.Rate = nil
	u.ObservedAt = nil
	u.UpdatedAt = now
	return nil
}
