package update

import (
	"time"

	"quoteservice/internal/domain"
)

type requestBody struct {
	Pair           string `json:"pair"`
	IdempotencyKey string `json:"idempotency_key"`
}

type createResponse struct {
	UpdateID string `json:"update_id"`
}

type statusResponse struct {
	UpdateID   string  `json:"update_id"`
	Pair       string  `json:"pair"`
	Status     string  `json:"status"`
	Rate       *string `json:"rate,omitempty"`
	ObservedAt *string `json:"observed_at,omitempty"`
	Error      *string `json:"error,omitempty"`
}

func newStatusResponse(req domain.UpdateRequest) statusResponse {
	result := statusResponse{
		UpdateID: req.ID,
		Pair:     req.Pair.String(),
		Status:   string(req.Status),
	}
	if req.Status == domain.StatusCompleted && req.Rate != nil && req.ObservedAt != nil {
		rate := req.Rate.String()
		observedAt := req.ObservedAt.UTC().Format(time.RFC3339Nano)
		result.Rate = &rate
		result.ObservedAt = &observedAt
	}
	if req.Status == domain.StatusFailed && req.Error != "" {
		message := req.Error
		result.Error = &message
	}
	return result
}
