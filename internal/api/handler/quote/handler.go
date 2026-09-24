package quote

import (
	"net/http"
	"strings"
	"time"

	"quoteservice/internal/api/response"
	"quoteservice/internal/domain"
)

type Handler struct {
	getLatest GetLatestUseCase
}

func New(getLatest GetLatestUseCase) *Handler {
	return &Handler{getLatest: getLatest}
}

func (h *Handler) GetLatest(w http.ResponseWriter, r *http.Request) {
	quote, err := h.getLatest.Execute(r.Context(), strings.ReplaceAll(r.PathValue("pair"), "/", "-"))
	if err != nil {
		response.DomainError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, newResponse(quote))
}

type latestResponse struct {
	Pair       string `json:"pair"`
	Rate       string `json:"rate"`
	ObservedAt string `json:"observed_at"`
}

func newResponse(quote domain.Quote) latestResponse {
	return latestResponse{
		Pair:       quote.Pair.String(),
		Rate:       quote.Rate.String(),
		ObservedAt: quote.ObservedAt.UTC().Format(time.RFC3339Nano),
	}
}
