package metrics

import "sync/atomic"

// Stats is process-local counters so we can see whether freshness/limit/retry work.
type Stats struct {
	CacheHits         atomic.Int64
	FXFetches         atomic.Int64
	FXErrors          atomic.Int64
	FXFetchNs         atomic.Int64
	RateLimitWaits    atomic.Int64
	RateLimitWaitNs   atomic.Int64
	JobsCompleted     atomic.Int64
	JobsFailed        atomic.Int64
	JobsCacheHit      atomic.Int64
	JobsStaleFallback atomic.Int64
	StuckReclaimed    atomic.Int64
}

func (s *Stats) Snapshot() map[string]int64 {
	if s == nil {
		return map[string]int64{}
	}
	return map[string]int64{
		"quote_cache_hit_total":            s.CacheHits.Load(),
		"fx_fetch_total":                   s.FXFetches.Load(),
		"fx_fetch_errors_total":            s.FXErrors.Load(),
		"fx_fetch_duration_ns_total":       s.FXFetchNs.Load(),
		"fx_rate_limit_wait_total":         s.RateLimitWaits.Load(),
		"fx_rate_limit_wait_ns_total":      s.RateLimitWaitNs.Load(),
		"update_jobs_completed_total":      s.JobsCompleted.Load(),
		"update_jobs_failed_total":         s.JobsFailed.Load(),
		"update_jobs_cache_hit_total":      s.JobsCacheHit.Load(),
		"update_jobs_stale_fallback_total": s.JobsStaleFallback.Load(),
		"worker_stuck_reclaimed_total":     s.StuckReclaimed.Load(),
	}
}
