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

func newTestCacheStore(t *testing.T) (*store.CacheStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return store.NewCacheStore(rdb), mr
}

func TestCacheStore_Leaderboard_Miss(t *testing.T) {
	cs, _ := newTestCacheStore(t)
	_, ok := cs.GetLeaderboard(context.Background())
	assert.False(t, ok)
}

func TestCacheStore_Leaderboard_RoundTrip(t *testing.T) {
	cs, _ := newTestCacheStore(t)
	ctx := context.Background()

	entries := []store.LeaderboardEntry{
		{Rank: 1, Username: "alice", TotalFP: 500, Streak: 7},
		{Rank: 2, Username: "bob", TotalFP: 300, Streak: 3},
	}
	require.NoError(t, cs.SetLeaderboard(ctx, entries))

	got, ok := cs.GetLeaderboard(ctx)
	require.True(t, ok)
	assert.Equal(t, entries, got)
}

func TestCacheStore_Leaderboard_Expires(t *testing.T) {
	cs, mr := newTestCacheStore(t)
	ctx := context.Background()

	entries := []store.LeaderboardEntry{{Rank: 1, Username: "alice", TotalFP: 100}}
	require.NoError(t, cs.SetLeaderboard(ctx, entries))

	// Advance past 5-minute TTL
	mr.FastForward(6 * time.Minute)

	_, ok := cs.GetLeaderboard(ctx)
	assert.False(t, ok, "leaderboard cache should have expired after 5min")
}

func TestCacheStore_UserStats_RoundTrip(t *testing.T) {
	cs, _ := newTestCacheStore(t)
	ctx := context.Background()

	stats := map[string]any{"streak": 5, "total_fp": 200}
	require.NoError(t, cs.SetUserStats(ctx, "user1", stats))

	var got map[string]any
	ok := cs.GetUserStats(ctx, "user1", &got)
	require.True(t, ok)
	assert.Equal(t, float64(5), got["streak"])
	assert.Equal(t, float64(200), got["total_fp"])
}

func TestCacheStore_UserStats_Invalidate(t *testing.T) {
	cs, _ := newTestCacheStore(t)
	ctx := context.Background()

	require.NoError(t, cs.SetUserStats(ctx, "user2", map[string]any{"streak": 1}))
	require.NoError(t, cs.InvalidateUserStats(ctx, "user2"))

	var dest map[string]any
	assert.False(t, cs.GetUserStats(ctx, "user2", &dest))
}

func TestCacheStore_UserStats_Expires(t *testing.T) {
	cs, mr := newTestCacheStore(t)
	ctx := context.Background()

	require.NoError(t, cs.SetUserStats(ctx, "user3", map[string]any{"streak": 2}))

	mr.FastForward(3 * time.Minute)

	var dest map[string]any
	assert.False(t, cs.GetUserStats(ctx, "user3", &dest), "stats cache should have expired after 2min")
}

func TestCacheStore_SetLeaderboard_RedisError(t *testing.T) {
	cs, mr := newTestCacheStore(t)
	mr.Close()

	err := cs.SetLeaderboard(context.Background(), []store.LeaderboardEntry{})
	assert.Error(t, err, "should return error when Redis is down")
}

func TestCacheStore_GetLeaderboard_InvalidJSON(t *testing.T) {
	cs, mr := newTestCacheStore(t)

	// Inject invalid JSON directly into Redis
	mr.Set("cache:leaderboard", "not-valid-json")

	_, ok := cs.GetLeaderboard(context.Background())
	assert.False(t, ok, "should return false when cached value is invalid JSON")
}

func TestCacheStore_SetUserStats_MarshalError(t *testing.T) {
	cs, _ := newTestCacheStore(t)

	// chan cannot be JSON-marshalled — triggers the marshal error branch
	err := cs.SetUserStats(context.Background(), "user-x", make(chan int))
	assert.Error(t, err)
}

func TestCacheStore_SetUserStats_RedisError(t *testing.T) {
	cs, mr := newTestCacheStore(t)
	mr.Close()

	err := cs.SetUserStats(context.Background(), "user-y", map[string]any{"streak": 1})
	assert.Error(t, err)
}
