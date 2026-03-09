package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "conv_session:"

type SessionStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewSessionStore(rdb *redis.Client, ttl time.Duration) *SessionStore {
	return &SessionStore{rdb: rdb, ttl: ttl}
}

func sessionKey(id string) string { return sessionKeyPrefix + id }

func (ss *SessionStore) Create(userID, language, topic string, level int, personality, systemPrompt string) *Session {
	s := &Session{
		ID:          uuid.New().String(),
		UserID:      userID,
		Language:    language,
		Topic:       topic,
		Level:       level,
		Personality: personality,
		Messages:    []Message{{Role: "system", Content: systemPrompt}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	data, _ := json.Marshal(s)
	_ = ss.rdb.Set(context.Background(), sessionKey(s.ID), data, ss.ttl).Err()
	return s
}

func (ss *SessionStore) Get(id string) (*Session, error) {
	data, err := ss.rdb.Get(context.Background(), sessionKey(id)).Bytes()
	if err == redis.Nil {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("session get: %w", err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("session decode: %w", err)
	}
	return &s, nil
}

func (ss *SessionStore) AddMessage(id string, msg Message) error {
	s, err := ss.Get(id)
	if err != nil {
		return err
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("session encode: %w", err)
	}
	// Reset TTL on every message (sliding expiry — keeps active sessions alive)
	return ss.rdb.Set(context.Background(), sessionKey(id), data, ss.ttl).Err()
}

func (ss *SessionStore) GetMessages(id string) ([]Message, error) {
	s, err := ss.Get(id)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for _, m := range s.Messages {
		if m.Role != "system" {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}
