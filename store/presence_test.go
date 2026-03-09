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

func newTestPresenceStore(t *testing.T) (*store.PresenceStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return store.NewPresenceStore(rdb), mr
}

func TestPresenceStore_SetAndGet(t *testing.T) {
	ps, _ := newTestPresenceStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	lp := store.LessonPresence{
		Type:      "conversation",
		Language:  "it",
		Topic:     "food",
		StartedAt: now,
	}
	require.NoError(t, ps.Set(ctx, "user1", lp))

	got, err := ps.Get(ctx, "user1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "conversation", got.Type)
	assert.Equal(t, "it", got.Language)
	assert.Equal(t, "food", got.Topic)
	assert.Equal(t, now, got.StartedAt)
}

func TestPresenceStore_Clear(t *testing.T) {
	ps, _ := newTestPresenceStore(t)
	ctx := context.Background()

	require.NoError(t, ps.Set(ctx, "user1", store.LessonPresence{Type: "vocab"}))
	require.NoError(t, ps.Clear(ctx, "user1"))

	got, err := ps.Get(ctx, "user1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPresenceStore_Get_Missing(t *testing.T) {
	ps, _ := newTestPresenceStore(t)

	got, err := ps.Get(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPresenceStore_Expires(t *testing.T) {
	ps, mr := newTestPresenceStore(t)
	ctx := context.Background()

	require.NoError(t, ps.Set(ctx, "user1", store.LessonPresence{Type: "listening"}))

	// Advance past 30-minute TTL
	mr.FastForward(35 * time.Minute)

	got, err := ps.Get(ctx, "user1")
	require.NoError(t, err)
	assert.Nil(t, got, "presence should have expired after 30min")
}

func TestPresenceStore_Set_RedisError(t *testing.T) {
	ps, mr := newTestPresenceStore(t)
	mr.Close()

	err := ps.Set(context.Background(), "user1", store.LessonPresence{Type: "vocab"})
	assert.Error(t, err, "should return error when Redis is down")
}

func TestPresenceStore_Get_RedisError(t *testing.T) {
	ps, mr := newTestPresenceStore(t)

	// Set a key first so it's not a redis.Nil miss, then close
	require.NoError(t, ps.Set(context.Background(), "user1", store.LessonPresence{Type: "vocab"}))
	mr.Close()

	_, err := ps.Get(context.Background(), "user1")
	assert.Error(t, err, "should return error on non-Nil Redis error")
}

func TestPresenceStore_Get_InvalidJSON(t *testing.T) {
	ps, mr := newTestPresenceStore(t)

	// Inject invalid JSON directly
	mr.Set("presence:user1", "not-valid-json")

	_, err := ps.Get(context.Background(), "user1")
	assert.Error(t, err, "should return error when stored JSON is invalid")
}

func TestPresenceStore_MultipleUsers_Isolated(t *testing.T) {
	ps, _ := newTestPresenceStore(t)
	ctx := context.Background()

	require.NoError(t, ps.Set(ctx, "user1", store.LessonPresence{Type: "vocab", Language: "it"}))
	require.NoError(t, ps.Set(ctx, "user2", store.LessonPresence{Type: "writing", Language: "es"}))

	got1, _ := ps.Get(ctx, "user1")
	got2, _ := ps.Get(ctx, "user2")

	require.NotNil(t, got1)
	require.NotNil(t, got2)
	assert.Equal(t, "vocab", got1.Type)
	assert.Equal(t, "writing", got2.Type)
	assert.Equal(t, "it", got1.Language)
	assert.Equal(t, "es", got2.Language)
}
