package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	FXBaseURL         string
	FXAPIKey          string
	FXTimeout         time.Duration
	FXRPS             float64
	FXBurst           int
	FXRetryAttempts   int
	FXRetryBase       time.Duration
	MigrationsDir     string
	WorkerInterval    time.Duration
	JobTimeout        time.Duration
	ProcessingLease   time.Duration
	MaxAttempts       int
	MaxPending        int
	QuoteFreshness    time.Duration
	StaleFallback     time.Duration
	DBMaxConns        int32
	DBMaxConnLifetime time.Duration
	DBConnectTimeout  time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		FXBaseURL:         getenv("FX_BASE_URL", "https://api.exchangerate.dev"),
		FXAPIKey:          os.Getenv("EXCHANGERATE_API_KEY"),
		FXTimeout:         5 * time.Second,
		FXRPS:             2,
		FXBurst:           1,
		FXRetryAttempts:   3,
		FXRetryBase:       200 * time.Millisecond,
		MigrationsDir:     getenv("MIGRATIONS_DIR", "migrations"),
		WorkerInterval:    300 * time.Millisecond,
		JobTimeout:        20 * time.Second,
		ProcessingLease:   45 * time.Second,
		MaxAttempts:       3,
		MaxPending:        1000,
		QuoteFreshness:    30 * time.Second,
		StaleFallback:     5 * time.Minute,
		DBMaxConns:        8,
		DBMaxConnLifetime: 30 * time.Minute,
		DBConnectTimeout:  5 * time.Second,
	}
	var err error
	if cfg.WorkerInterval, err = durationMS("WORKER_INTERVAL_MS", cfg.WorkerInterval); err != nil {
		return Config{}, err
	}
	if cfg.FXTimeout, err = durationMS("FX_TIMEOUT_MS", cfg.FXTimeout); err != nil {
		return Config{}, err
	}
	if cfg.JobTimeout, err = durationMS("JOB_TIMEOUT_MS", cfg.JobTimeout); err != nil {
		return Config{}, err
	}
	if cfg.ProcessingLease, err = durationMS("PROCESSING_LEASE_MS", cfg.ProcessingLease); err != nil {
		return Config{}, err
	}
	if cfg.QuoteFreshness, err = durationMS("QUOTE_FRESHNESS_MS", cfg.QuoteFreshness); err != nil {
		return Config{}, err
	}
	if cfg.StaleFallback, err = durationMS("STALE_FALLBACK_MS", cfg.StaleFallback); err != nil {
		return Config{}, err
	}
	if cfg.FXRetryBase, err = durationMS("FX_RETRY_BASE_MS", cfg.FXRetryBase); err != nil {
		return Config{}, err
	}
	if cfg.FXRPS, err = floatEnv("FX_RPS", cfg.FXRPS); err != nil {
		return Config{}, err
	}
	if cfg.FXBurst, err = intEnv("FX_BURST", cfg.FXBurst); err != nil {
		return Config{}, err
	}
	if cfg.FXRetryAttempts, err = intEnv("FX_RETRY_ATTEMPTS", cfg.FXRetryAttempts); err != nil {
		return Config{}, err
	}
	if cfg.MaxAttempts, err = intEnv("MAX_ATTEMPTS", cfg.MaxAttempts); err != nil {
		return Config{}, err
	}
	if cfg.MaxPending, err = intEnv("MAX_PENDING", cfg.MaxPending); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("DB_MAX_CONNS: %w", err)
		}
		cfg.DBMaxConns = int32(n)
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationMS(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	ms, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func intEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func floatEnv(key string, fallback float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}
