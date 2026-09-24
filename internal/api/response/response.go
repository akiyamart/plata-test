package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"quoteservice/internal/domain"
	"quoteservice/internal/service"
)

type errorResponse struct {
	Error string `json:"error"`
}

func DomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidPair), errors.Is(err, domain.ErrEmptyID), errors.Is(err, domain.ErrInvalidIdempotencyKey):
		Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrQueueFull):
		Error(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, domain.ErrUpdateNotFound), errors.Is(err, domain.ErrQuoteNotFound):
		Error(w, http.StatusNotFound, err.Error())
	default:
		Error(w, http.StatusInternalServerError, "internal error")
	}
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, errorResponse{Error: message})
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
