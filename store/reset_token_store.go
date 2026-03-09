package store

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrTokenNotFound = errors.New("token not found")

const resetKeyPrefix = "reset:pwd:"
const resetTokenTTL = time.Hour

// ResetTokenStore stores password-reset tokens in Redis with a 1-hour TTL.
type ResetTokenStore struct {
	rdb *redis.Client
}

func NewResetTokenStore(rdb *redis.Client) *ResetTokenStore {
	return &ResetTokenStore{rdb: rdb}
}

// Save stores token → email with the default 1-hour TTL.
func (s *ResetTokenStore) Save(ctx context.Context, token, email string) error {
	return s.rdb.Set(ctx, resetKeyPrefix+token, email, resetTokenTTL).Err()
}

// SaveWithTTL stores token → email with a custom TTL (e.g. 48h for admin invites).
func (s *ResetTokenStore) SaveWithTTL(ctx context.Context, token, email string, ttl time.Duration) error {
	return s.rdb.Set(ctx, resetKeyPrefix+token, email, ttl).Err()
}

// Get returns the email associated with a token, or ErrTokenNotFound if absent/expired.
func (s *ResetTokenStore) Get(ctx context.Context, token string) (string, error) {
	email, err := s.rdb.Get(ctx, resetKeyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", err
	}
	return email, nil
}

// Delete removes a token after use.
func (s *ResetTokenStore) Delete(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, resetKeyPrefix+token).Err()
}
