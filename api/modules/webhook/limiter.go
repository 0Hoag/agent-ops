package webhook

import (
	"sync"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

const (
	defaultRateLimit   = 50
	defaultRateWindow  = time.Second
	defaultRateEntries = 4096
)

// RateLimiter is a bounded, process-local token bucket keyed by repository ID.
type RateLimiter struct {
	mu             sync.Mutex
	limit, maxKeys float64
	window         time.Duration
	now            func() time.Time
	buckets        map[store.UUID]rateBucket
}

type rateBucket struct {
	tokens               float64
	lastRefill, lastSeen time.Time
}

func NewRateLimiter(limit int, window time.Duration, maxKeys int, now func() time.Time) *RateLimiter {
	if limit < 1 {
		limit = defaultRateLimit
	}
	if window <= 0 {
		window = defaultRateWindow
	}
	if maxKeys < 1 {
		maxKeys = defaultRateEntries
	}
	if now == nil {
		now = time.Now
	}
	return &RateLimiter{limit: float64(limit), window: window, maxKeys: float64(maxKeys), now: now, buckets: make(map[store.UUID]rateBucket)}
}

func (limiter *RateLimiter) Allow(repositoryID store.UUID) bool {
	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	bucket, exists := limiter.buckets[repositoryID]
	if !exists {
		limiter.evict()
		bucket = rateBucket{tokens: limiter.limit, lastRefill: now}
	}
	if elapsed := now.Sub(bucket.lastRefill); elapsed > 0 {
		bucket.tokens = min(limiter.limit, bucket.tokens+limiter.limit*elapsed.Seconds()/limiter.window.Seconds())
		bucket.lastRefill = now
	}
	bucket.lastSeen = now
	if bucket.tokens < 1 {
		limiter.buckets[repositoryID] = bucket
		return false
	}
	bucket.tokens--
	limiter.buckets[repositoryID] = bucket
	return true
}

func (limiter *RateLimiter) evict() {
	if float64(len(limiter.buckets)) < limiter.maxKeys {
		return
	}
	var oldestID store.UUID
	var oldest time.Time
	for id, bucket := range limiter.buckets {
		if oldestID == "" || bucket.lastSeen.Before(oldest) {
			oldestID, oldest = id, bucket.lastSeen
		}
	}
	if oldestID != "" {
		delete(limiter.buckets, oldestID)
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
