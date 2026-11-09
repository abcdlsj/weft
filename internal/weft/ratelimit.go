package weft

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// rateLimiter is a small sliding-window limiter. weft is a single-process
// control plane, so an in-memory map is enough and avoids another dependency.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]int64
	limit  int
	window time.Duration
	lastGC int64
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]int64{}, limit: limit, window: window}
}

func (l *rateLimiter) allow(key string) bool {
	if key == "" {
		key = "unknown"
	}
	at := time.Now().Unix()
	cutoff := at - int64(l.window.Seconds())
	l.mu.Lock()
	defer l.mu.Unlock()
	// Drop stale buckets occasionally instead of running a goroutine.
	if at-l.lastGC > 300 {
		for k, buckets := range l.hits {
			if len(buckets) == 0 || buckets[len(buckets)-1] < cutoff {
				delete(l.hits, k)
			}
		}
		l.lastGC = at
	}
	buckets := l.hits[key]
	kept := buckets[:0]
	for _, t := range buckets {
		if t >= cutoff {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, at)
	return true
}

func rateLimited(w http.ResponseWriter, retry time.Duration) {
	seconds := int(retry.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeErr(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
}
