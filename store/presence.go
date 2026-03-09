package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const presenceKeyPrefix = "presence:"
const presenceTTL = 30 * time.Minute

// LessonPresence records what lesson a user is currently doing.
type LessonPresence struct {
	Type      string    `json:"type"`      // "conversation","writing","vocab","sentence","listening"
	Language  string    `json:"language"`
	Topic     string    `json:"topic"`
	StartedAt time.Time `json:"started_at"`
}

// PresenceStore tracks active lesson state per user in Redis with a 30-minute sliding TTL.
type PresenceStore struct {
	rdb *redis.Client
}

func NewPresenceStore(rdb *redis.Client) *PresenceStore {
	return &PresenceStore{rdb: rdb}
}

// Set records a user's active lesson. TTL is 30 minutes from call time.
func (p *PresenceStore) Set(ctx context.Context, userID string, lp LessonPresence) error {
	data, _ := json.Marshal(lp) // LessonPresence is always serialisable
	return p.rdb.Set(ctx, presenceKeyPrefix+userID, data, presenceTTL).Err()
}

// Get returns the active lesson for a user, or nil if not present.
func (p *PresenceStore) Get(ctx context.Context, userID string) (*LessonPresence, error) {
	data, err := p.rdb.Get(ctx, presenceKeyPrefix+userID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lp LessonPresence
	if err := json.Unmarshal(data, &lp); err != nil {
		return nil, err
	}
	return &lp, nil
}

// Clear removes the active lesson record for a user.
func (p *PresenceStore) Clear(ctx context.Context, userID string) error {
	return p.rdb.Del(ctx, presenceKeyPrefix+userID).Err()
}
