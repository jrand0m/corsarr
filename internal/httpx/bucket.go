package httpx

import (
	"sync"
	"time"
)

// bucket is a token bucket: rate tokens per second up to burst. reserve
// takes a token now and returns how long the caller must wait for it, so
// concurrent callers queue fairly without a background goroutine.
type bucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newBucket(rate float64, burst int, now time.Time) *bucket {
	return &bucket{rate: rate, burst: float64(burst), tokens: float64(burst), last: now}
}

func (b *bucket) reserve(now time.Time) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if el := now.Sub(b.last); el > 0 {
		b.tokens = min(b.burst, b.tokens+el.Seconds()*b.rate)
		b.last = now
	}
	b.tokens--
	if b.tokens >= 0 {
		return 0
	}
	return time.Duration(-b.tokens / b.rate * float64(time.Second))
}
