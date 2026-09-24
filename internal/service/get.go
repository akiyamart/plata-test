package service

import (
	"context"
	"strings"

	"quoteservice/internal/domain"
)

// Get returns an update request by id.
type Get struct {
	Updates UpdateRepository
}

func (uc Get) Execute(ctx context.Context, id string) (domain.UpdateRequest, error) {
	if strings.TrimSpace(id) == "" {
		return domain.UpdateRequest{}, domain.ErrEmptyID
	}
	return uc.Updates.GetByID(ctx, id)
}
