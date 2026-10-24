package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateWindow struct {
	count    int
	resetAt  time.Time
	lastSeen time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	windows map[string]*rateWindow
	sweepAt time.Time
	log     *slog.Logger
}

func NewRateLimiter(limit int, window time.Duration, log *slog.Logger) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		windows: make(map[string]*rateWindow),
		sweepAt: time.Now().Add(window),
		log:     log,
	}
}

func (l *RateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.After(l.sweepAt) {
		for k, w := range l.windows {
			if now.After(w.resetAt) && now.Sub(w.lastSeen) > l.window {
				delete(l.windows, k)
			}
		}
		l.sweepAt = now.Add(l.window)
	}

	w, ok := l.windows[key]
	if !ok || now.After(w.resetAt) {
		l.windows[key] = &rateWindow{count: 1, resetAt: now.Add(l.window), lastSeen: now}
		return true
	}
	w.count++
	w.lastSeen = now
	return w.count <= l.limit
}

func RateLimit(limiter *RateLimiter, name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		key := name + "|" + c.ClientIP()
		if !limiter.allow(key, now) {
			retry := int(time.Until(limiter.resetAtFor(key, now)).Seconds()) + 1
			if retry < 1 {
				retry = 1
			}
			c.Header("Retry-After", strconv.Itoa(retry))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
			return
		}
		c.Next()
	}
}

func (l *RateLimiter) resetAtFor(key string, now time.Time) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if w, ok := l.windows[key]; ok {
		return w.resetAt
	}
	return now.Add(l.window)
}
