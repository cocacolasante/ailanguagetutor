package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

const leaderboardCacheKey = "cache:leaderboard"
const statsKeyPrefix = "cache:stats:"
const leaderboardTTL = 5 * time.Minute
const statsTTL = 2 * time.Minute

// CacheStore caches leaderboard results and per-user stats in Redis.
type CacheStore struct {
	rdb *redis.Client
}

func NewCacheStore(rdb *redis.Client) *CacheStore {
	return &CacheStore{rdb: rdb}
}

func (c *CacheStore) SetLeaderboard(ctx context.Context, entries []LeaderboardEntry) error {
	data, _ := json.Marshal(entries) // LeaderboardEntry is always serialisable
	return c.rdb.Set(ctx, leaderboardCacheKey, data, leaderboardTTL).Err()
}

// GetLeaderboard returns cached leaderboard entries. Returns false on miss or error.
func (c *CacheStore) GetLeaderboard(ctx context.Context) ([]LeaderboardEntry, bool) {
	data, err := c.rdb.Get(ctx, leaderboardCacheKey).Bytes()
	if err != nil {
		return nil, false
	}
	var entries []LeaderboardEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, false
	}
	return entries, true
}

// SetUserStats caches any stats value (must be JSON-serialisable) for 2 minutes.
func (c *CacheStore) SetUserStats(ctx context.Context, userID string, stats any) error {
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, statsKeyPrefix+userID, data, statsTTL).Err()
}

// GetUserStats unmarshals cached stats into dest. Returns false on miss or error.
func (c *CacheStore) GetUserStats(ctx context.Context, userID string, dest any) bool {
	data, err := c.rdb.Get(ctx, statsKeyPrefix+userID).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}

// InvalidateUserStats removes the cached stats for a user (call after FP update).
func (c *CacheStore) InvalidateUserStats(ctx context.Context, userID string) error {
	return c.rdb.Del(ctx, statsKeyPrefix+userID).Err()
}
