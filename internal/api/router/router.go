package router

import (
	"context"
	"log/slog"
	"net/http"

	quotehandler "quoteservice/internal/api/handler/quote"
	updatehandler "quoteservice/internal/api/handler/update"
	"quoteservice/internal/api/middleware"
	"quoteservice/internal/api/response"
	"quoteservice/internal/metrics"
)

type Deps struct {
	Request   updatehandler.RequestUseCase
	Get       updatehandler.GetUseCase
	GetLatest quotehandler.GetLatestUseCase
	Ready     func(context.Context) error
	Stats     *metrics.Stats
	Logger    *slog.Logger
}

func New(deps Deps) http.Handler {
	updates := updatehandler.New(deps.Request, deps.Get, deps.Logger)
	quotes := quotehandler.New(deps.GetLatest)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", ready(deps.Ready))
	mux.HandleFunc("GET /metrics", metricSnapshot(deps.Stats))
	mux.HandleFunc("POST /v1/quotes/updates", updates.Post)
	mux.HandleFunc("GET /v1/quotes/updates/{id}", updates.Get)
	mux.HandleFunc("GET /v1/quotes/{pair}", quotes.GetLatest)
	return middleware.Chain(deps.Logger, mux)
}

// Server preserves the small construction helper used by existing HTTP tests.
type Server struct {
	Request   updatehandler.RequestUseCase
	Get       updatehandler.GetUseCase
	GetLatest quotehandler.GetLatestUseCase
	Ready     func(context.Context) error
	Stats     *metrics.Stats
	Log       *slog.Logger
}

func (s Server) Handler() http.Handler {
	return New(Deps{
		Request: s.Request, Get: s.Get, GetLatest: s.GetLatest,
		Ready: s.Ready, Stats: s.Stats, Logger: s.Log,
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func ready(check func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			if err := check(r.Context()); err != nil {
				response.Error(w, http.StatusServiceUnavailable, "not ready")
				return
			}
		}
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func metricSnapshot(stats *metrics.Stats) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if stats == nil {
			response.JSON(w, http.StatusOK, map[string]int64{})
			return
		}
		response.JSON(w, http.StatusOK, stats.Snapshot())
	}
}
