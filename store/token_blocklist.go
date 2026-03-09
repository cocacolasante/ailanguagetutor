package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const blocklistKeyPrefix = "blocklist:jwt:"

type TokenBlocklist struct {
	rdb *redis.Client
}

func NewTokenBlocklist(rdb *redis.Client) *TokenBlocklist {
	return &TokenBlocklist{rdb: rdb}
}

func hashToken(tokenStr string) string {
	h := sha256.Sum256([]byte(tokenStr))
	return fmt.Sprintf("%x", h)
}

// Add revokes a token until its expiry time.
func (bl *TokenBlocklist) Add(ctx context.Context, tokenStr string, exp time.Time) error {
	ttl := time.Until(exp)
	if ttl <= 0 {
		return nil
	}
	return bl.rdb.Set(ctx, blocklistKeyPrefix+hashToken(tokenStr), "1", ttl).Err()
}

// IsBlocked returns true if the token has been revoked.
// Fails open on Redis error — JWT signature is the primary auth gate.
func (bl *TokenBlocklist) IsBlocked(ctx context.Context, tokenStr string) bool {
	val, err := bl.rdb.Get(ctx, blocklistKeyPrefix+hashToken(tokenStr)).Result()
	if err != nil {
		return false // redis.Nil (not blocked) OR redis unavailable (fail open)
	}
	return val == "1"
}
