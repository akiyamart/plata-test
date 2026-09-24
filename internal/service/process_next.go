package service

import (
	"context"
	"errors"

	"quoteservice/internal/domain"
)

// ProcessNext claims the next pending update and finishes it.
type ProcessNext struct {
	Updates UpdateRepository
	Process Process
}

type TickResult struct {
	Skipped       bool
	UpdateID      string
	CorrelationID string
	Pair          string
	Outcome       Outcome
}

func (uc ProcessNext) Execute(ctx context.Context) (TickResult, error) {
	req, err := uc.Updates.ClaimNextPending(ctx)
	if errors.Is(err, domain.ErrUpdateNotFound) {
		return TickResult{Skipped: true}, nil
	}
	if err != nil {
		return TickResult{}, err
	}
	out, err := uc.Process.Execute(ctx, req)
	return TickResult{
		UpdateID:      req.ID,
		CorrelationID: req.CorrelationID,
		Pair:          req.Pair.String(),
		Outcome:       out,
	}, err
}
