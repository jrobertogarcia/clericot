package security

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// RateLimiter manages distributed GCRA rate limiting with in-process fail-open fallback.
type RateLimiter struct {
	limiter      *redis_rate.Limiter
	localLimiter *rate.Limiter
	logger       *slog.Logger
}

// NewRateLimiter constructs a new distributed RateLimiter.
func NewRateLimiter(rdb *redis.Client, logger *slog.Logger) *RateLimiter {
	if logger == nil {
		logger = slog.Default()
	}
	var limiter *redis_rate.Limiter
	if rdb != nil {
		limiter = redis_rate.NewLimiter(rdb)
	}
	return &RateLimiter{
		limiter:      limiter,
		localLimiter: rate.NewLimiter(rate.Every(time.Second/100), 100),
		logger:       logger,
	}
}

// Allow checks if an action is permitted within the rate limit.
func (l *RateLimiter) Allow(ctx context.Context, key string, rps int) (bool, error) {
	if l.limiter == nil {
		return l.localLimiter.Allow(), nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	res, err := l.limiter.Allow(checkCtx, key, redis_rate.PerSecond(rps))
	if err != nil {
		l.logger.Warn("redis rate limiter unavailable, falling back to in-process fail-open", "error", err, "key", key)
		return l.localLimiter.Allow(), nil // Fail-open local fallback
	}
	return res.Allowed > 0, nil
}
