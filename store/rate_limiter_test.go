package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/ailanguagetutor/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRateLimiter(t *testing.T) (*store.RateLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return store.NewRateLimiter(rdb), mr
}

func TestRateLimiter_Allow_BelowLimit(t *testing.T) {
	rl, _ := newTestRateLimiter(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow(ctx, "ratelimit:test:ip1", 10, time.Minute))
	}
}

func TestRateLimiter_Allow_ExceedsLimit(t *testing.T) {
	rl, _ := newTestRateLimiter(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		rl.Allow(ctx, "ratelimit:test:ip2", 10, time.Minute) //nolint:errcheck
	}
	// 11th request should be blocked
	assert.False(t, rl.Allow(ctx, "ratelimit:test:ip2", 10, time.Minute))
}

func TestRateLimiter_Allow_WindowReset(t *testing.T) {
	rl, mr := newTestRateLimiter(t)
	ctx := context.Background()
	key := "ratelimit:test:ip3"
	for i := 0; i < 10; i++ {
		rl.Allow(ctx, key, 10, time.Minute) //nolint:errcheck
	}
	assert.False(t, rl.Allow(ctx, key, 10, time.Minute), "should be blocked")

	// Advance past the window
	mr.FastForward(2 * time.Minute)
	assert.True(t, rl.Allow(ctx, key, 10, time.Minute), "window reset: should be allowed again")
}

func TestRateLimiter_SetAndOnCooldown(t *testing.T) {
	rl, _ := newTestRateLimiter(t)
	ctx := context.Background()
	key := "cooldown:email-send:user@example.com"

	assert.False(t, rl.OnCooldown(ctx, key), "not on cooldown before set")
	require.NoError(t, rl.SetCooldown(ctx, key, 60*time.Second))
	assert.True(t, rl.OnCooldown(ctx, key), "should be on cooldown after set")
}

func TestRateLimiter_Cooldown_Expires(t *testing.T) {
	rl, mr := newTestRateLimiter(t)
	ctx := context.Background()
	key := "cooldown:email-send:expire@example.com"

	require.NoError(t, rl.SetCooldown(ctx, key, 30*time.Second))
	assert.True(t, rl.OnCooldown(ctx, key))

	mr.FastForward(60 * time.Second)
	assert.False(t, rl.OnCooldown(ctx, key), "cooldown should have expired")
}

func TestRateLimiter_FailOpen_OnRedisError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rl := store.NewRateLimiter(rdb)
	ctx := context.Background()

	// Close Redis to simulate error
	mr.Close()

	// Should fail open (return true = allow)
	assert.True(t, rl.Allow(ctx, "ratelimit:test:down", 1, time.Minute))
	assert.False(t, rl.OnCooldown(ctx, "cooldown:test:down"))
}
