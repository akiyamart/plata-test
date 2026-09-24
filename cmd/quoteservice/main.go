package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quoteservice/internal/api/router"
	"quoteservice/internal/clock"
	"quoteservice/internal/config"
	"quoteservice/internal/exchangerate"
	"quoteservice/internal/metrics"
	"quoteservice/internal/repository/postgres"
	"quoteservice/internal/service"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DBMaxConns,
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		ConnectTimeout:  cfg.DBConnectTimeout,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool, cfg.MigrationsDir); err != nil {
		return err
	}

	stats := &metrics.Stats{}
	updateRepo := postgres.NewUpdateRepository(pool, cfg.ProcessingLease, cfg.MaxAttempts)
	updateRepo.OnReclaim(func() { stats.StuckReclaimed.Add(1) })
	quoteRepo := postgres.NewQuoteRepository(pool)
	complete := postgres.NewCompletion(pool)
	raw := exchangerate.NewClient(exchangerate.Config{
		BaseURL: cfg.FXBaseURL,
		APIKey:  cfg.FXAPIKey,
		Timeout: cfg.FXTimeout,
	})
	rates := exchangerate.NewRetry(
		exchangerate.NewLimit(
			exchangerate.NewObserve(raw, stats, log),
			cfg.FXRPS,
			cfg.FXBurst,
			stats,
			log,
		),
		cfg.FXRetryAttempts,
		cfg.FXRetryBase,
	)
	sysClock := clock.NewSystemClock()

	request := service.Request{Updates: updateRepo, Clock: sysClock, MaxPending: cfg.MaxPending}
	get := service.Get{Updates: updateRepo}
	getLatest := service.GetLatest{Quotes: quoteRepo}
	process := service.Process{
		Updates:       updateRepo,
		Quotes:        quoteRepo,
		Complete:      complete,
		Rates:         rates,
		Clock:         sysClock,
		Freshness:     cfg.QuoteFreshness,
		StaleFallback: cfg.StaleFallback,
		OnCacheHit:    func() { stats.CacheHits.Add(1) },
	}

	w := service.Worker{
		ProcessNext: service.ProcessNext{Updates: updateRepo, Process: process},
		Interval:    cfg.WorkerInterval,
		JobTimeout:  cfg.JobTimeout,
		Log:         log,
		Stats:       stats,
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		w.Run(ctx)
	}()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: router.New(router.Deps{
			Request: request, Get: get, GetLatest: getLatest,
			Ready: pool.Ping, Stats: stats, Logger: log,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		select {
		case <-workerDone:
		case <-shutdownCtx.Done():
			log.Error("worker shutdown timeout")
		}
		return nil
	case err := <-errCh:
		stop()
		<-workerDone
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
