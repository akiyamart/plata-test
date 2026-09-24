package service

import (
	"context"
	"log/slog"
	"quoteservice/internal/metrics"
	"time"
)

// Worker polls pending update requests and processes them serially.
// ponytail: one in-flight FX fetch per process is the bulkhead; add singleflight
// (key=domain.Slug) only if this loop is ever parallelized.
type Worker struct {
	ProcessNext ProcessNext
	Interval    time.Duration
	JobTimeout  time.Duration
	Log         *slog.Logger
	Stats       *metrics.Stats
}

func (w Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	jobTimeout := w.JobTimeout
	if jobTimeout <= 0 {
		jobTimeout = 20 * time.Second
	}
	log := w.Log
	if log == nil {
		log = slog.Default()
	}
	streak := 0
	for {
		if ctx.Err() != nil {
			return
		}
		jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobTimeout)
		start := time.Now()
		tick, err := w.ProcessNext.Execute(jobCtx)
		cancel()
		w.record(tick, err)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Error("worker tick", "err", err, "update_id", tick.UpdateID, "correlation_id", tick.CorrelationID, "pair", tick.Pair, "duration_ms", time.Since(start).Milliseconds())
			streak++
		} else if !tick.Skipped {
			attrs := []any{
				"update_id", tick.UpdateID,
				"correlation_id", tick.CorrelationID,
				"pair", tick.Pair,
				"result", string(tick.Outcome),
				"duration_ms", time.Since(start).Milliseconds(),
			}
			if tick.Outcome == OutcomeLostLease {
				log.Warn("worker tick", attrs...)
				streak = 0
			} else {
				log.Info("worker tick", attrs...)
				if tick.Outcome == OutcomeFailed {
					streak++
				} else {
					streak = 0
				}
			}
		}
		wait := interval
		switch {
		case tick.Skipped:
			wait = interval
		case streak > 0:
			wait = backoff(interval, streak)
		default:
			wait = 0
		}
		if wait <= 0 {
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (w Worker) record(tick TickResult, err error) {
	if w.Stats == nil || tick.Skipped || err != nil {
		return
	}
	switch tick.Outcome {
	case OutcomeCompleted:
		w.Stats.JobsCompleted.Add(1)
	case OutcomeFailed:
		w.Stats.JobsFailed.Add(1)
	case OutcomeCacheHit:
		w.Stats.JobsCacheHit.Add(1)
		w.Stats.JobsCompleted.Add(1)
	case OutcomeStaleFallback:
		w.Stats.JobsStaleFallback.Add(1)
		w.Stats.JobsCompleted.Add(1)
	}
}

func backoff(base time.Duration, streak int) time.Duration {
	if streak < 1 {
		streak = 1
	}
	d := base * time.Duration(1<<(streak-1))
	if d > 5*time.Second {
		return 5 * time.Second
	}
	return d
}
