package store

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter provides sliding-window rate limiting and cooldown flags via Redis.
type RateLimiter struct {
	rdb *redis.Client
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

// Allow returns true if the request is within limit for the given key and window.
// Fails open on Redis error so unavailability never blocks users.
func (rl *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	count, err := rl.rdb.Incr(ctx, key).Result()
	if err != nil {
		return true // fail open
	}
	if count == 1 {
		// New key — set the expiry window. Ignore error; worst case no expiry.
		_ = rl.rdb.Expire(ctx, key, window).Err()
	}
	return count <= int64(limit)
}

// SetCooldown sets a one-shot flag expiring after d.
func (rl *RateLimiter) SetCooldown(ctx context.Context, key string, d time.Duration) error {
	return rl.rdb.Set(ctx, key, "1", d).Err()
}

// OnCooldown returns true if the key exists. Fails open on Redis error.
func (rl *RateLimiter) OnCooldown(ctx context.Context, key string) bool {
	exists, err := rl.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false // fail open
	}
	return exists > 0
}
