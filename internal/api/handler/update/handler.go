package update

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"quoteservice/internal/api/apictx"
	"quoteservice/internal/api/response"
)

const maxBodyBytes = 4096

type Handler struct {
	request RequestUseCase
	get     GetUseCase
	log     *slog.Logger
}

func New(request RequestUseCase, get GetUseCase, log *slog.Logger) *Handler {
	return &Handler{request: request, get: get, log: log}
}

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body requestBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(err.Error(), "http: request body too large") {
			response.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(body.IdempotencyKey)
	}
	correlationID := apictx.From(r.Context())
	id, err := h.request.Execute(r.Context(), body.Pair, key, correlationID)
	if err != nil {
		response.DomainError(w, err)
		return
	}
	if h.log != nil {
		h.log.Info("update accepted",
			"request_id", correlationID,
			"update_id", id,
			"pair", body.Pair,
		)
	}
	response.JSON(w, http.StatusAccepted, createResponse{UpdateID: id})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	req, err := h.get.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		response.DomainError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, newStatusResponse(req))
}
