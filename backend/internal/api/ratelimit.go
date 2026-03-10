package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type ipRecord struct {
	count    int
	resetAt  time.Time
}

// RateLimiter provides IP-based request rate limiting
type RateLimiter struct {
	mu       sync.Mutex
	records  map[string]*ipRecord
	limit    int           // max requests per window
	window   time.Duration // time window
}

// NewRateLimiter creates a rate limiter.
// limit: max requests per window per IP. window: time window duration.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		records: make(map[string]*ipRecord),
		limit:   limit,
		window:  window,
	}
	go rl.cleanup()
	return rl
}

// cleanup periodically removes expired entries
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for ip, rec := range rl.records {
			if now.After(rec.resetAt) {
				delete(rl.records, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns a gin middleware that rate-limits by client IP
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		rl.mu.Lock()
		rec, exists := rl.records[ip]
		if !exists || now.After(rec.resetAt) {
			rl.records[ip] = &ipRecord{count: 1, resetAt: now.Add(rl.window)}
			rl.mu.Unlock()
			c.Next()
			return
		}

		rec.count++
		if rec.count > rl.limit {
			rl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, APIResponse{
				Success: false,
				Error:   "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		rl.mu.Unlock()
		c.Next()
	}
}
