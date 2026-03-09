package store_test

import (
	"testing"
	"time"

	"github.com/ailanguagetutor/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSessionStore(t *testing.T) (*store.SessionStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return store.NewSessionStore(rdb, 4*time.Hour), mr
}

func TestSessionStore_Create(t *testing.T) {
	ss, mr := newTestSessionStore(t)

	s := ss.Create("user1", "it", "food", 2, "professor", "You are a teacher.")

	assert.NotEmpty(t, s.ID)
	assert.Equal(t, "user1", s.UserID)
	assert.Equal(t, "it", s.Language)
	assert.Equal(t, "food", s.Topic)
	assert.Equal(t, 2, s.Level)
	assert.Equal(t, "professor", s.Personality)
	assert.Len(t, s.Messages, 1)
	assert.Equal(t, "system", s.Messages[0].Role)
	assert.Equal(t, "You are a teacher.", s.Messages[0].Content)

	// Confirm key exists in Redis with a TTL
	ttl := mr.TTL("conv_session:" + s.ID)
	assert.Greater(t, ttl, time.Duration(0))
}

func TestSessionStore_Get_Found(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	created := ss.Create("user1", "es", "travel", 3, "travel-guide", "You guide travelers.")

	got, err := ss.Get(created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "user1", got.UserID)
	assert.Equal(t, "es", got.Language)
	assert.Len(t, got.Messages, 1)
}

func TestSessionStore_Get_NotFound(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	_, err := ss.Get("nonexistent-id")
	assert.ErrorIs(t, err, store.ErrSessionNotFound)
}

func TestSessionStore_AddMessage(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	s := ss.Create("user1", "it", "food", 2, "professor", "System prompt.")

	err := ss.AddMessage(s.ID, store.Message{Role: "user", Content: "Ciao!"})
	require.NoError(t, err)

	err = ss.AddMessage(s.ID, store.Message{Role: "assistant", Content: "Ciao! Come stai?"})
	require.NoError(t, err)

	got, err := ss.Get(s.ID)
	require.NoError(t, err)
	assert.Len(t, got.Messages, 3) // system + user + assistant
	assert.Equal(t, "user", got.Messages[1].Role)
	assert.Equal(t, "Ciao!", got.Messages[1].Content)
	assert.Equal(t, "assistant", got.Messages[2].Role)
}

func TestSessionStore_AddMessage_NotFound(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	err := ss.AddMessage("nonexistent", store.Message{Role: "user", Content: "hello"})
	assert.ErrorIs(t, err, store.ErrSessionNotFound)
}

func TestSessionStore_GetMessages_ExcludesSystem(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	s := ss.Create("user1", "it", "food", 2, "professor", "You are a teacher.")
	_ = ss.AddMessage(s.ID, store.Message{Role: "user", Content: "Buongiorno!"})
	_ = ss.AddMessage(s.ID, store.Message{Role: "assistant", Content: "Buongiorno a te!"})

	msgs, err := ss.GetMessages(s.ID)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	for _, m := range msgs {
		assert.NotEqual(t, "system", m.Role, "system message should be excluded")
	}
}

func TestSessionStore_GetMessages_NotFound(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	_, err := ss.GetMessages("nonexistent")
	assert.ErrorIs(t, err, store.ErrSessionNotFound)
}

func TestSessionStore_GetMessages_EmptyConversation(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	s := ss.Create("user1", "it", "food", 2, "professor", "System prompt only.")

	msgs, err := ss.GetMessages(s.ID)
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestSessionStore_AddMessage_SlidingTTL(t *testing.T) {
	ss, mr := newTestSessionStore(t)

	s := ss.Create("user1", "it", "food", 2, "professor", "System prompt.")

	// Fast-forward time but not past TTL
	mr.FastForward(2 * time.Hour)

	// AddMessage should reset TTL back to 4h
	err := ss.AddMessage(s.ID, store.Message{Role: "user", Content: "Still active"})
	require.NoError(t, err)

	ttl := mr.TTL("conv_session:" + s.ID)
	assert.Greater(t, ttl, 3*time.Hour, "TTL should be reset close to 4h after AddMessage")
}

func TestSessionStore_Expiry(t *testing.T) {
	ss, mr := newTestSessionStore(t)

	s := ss.Create("user1", "it", "food", 2, "professor", "System prompt.")

	// Advance time past TTL
	mr.FastForward(5 * time.Hour)

	_, err := ss.Get(s.ID)
	assert.ErrorIs(t, err, store.ErrSessionNotFound, "expired session should return ErrSessionNotFound")
}

func TestSessionStore_MultipleUsers(t *testing.T) {
	ss, _ := newTestSessionStore(t)

	s1 := ss.Create("user1", "it", "food", 1, "professor", "Prompt 1.")
	s2 := ss.Create("user2", "es", "travel", 3, "travel-guide", "Prompt 2.")

	got1, err := ss.Get(s1.ID)
	require.NoError(t, err)
	assert.Equal(t, "user1", got1.UserID)

	got2, err := ss.Get(s2.ID)
	require.NoError(t, err)
	assert.Equal(t, "user2", got2.UserID)
}
