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

func newTestResetTokenStore(t *testing.T) (*store.ResetTokenStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return store.NewResetTokenStore(rdb), mr
}

func TestResetTokenStore_SaveAndGet(t *testing.T) {
	rs, _ := newTestResetTokenStore(t)
	ctx := context.Background()

	require.NoError(t, rs.Save(ctx, "token123", "user@example.com"))
	email, err := rs.Get(ctx, "token123")
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", email)
}

func TestResetTokenStore_Get_NotFound(t *testing.T) {
	rs, _ := newTestResetTokenStore(t)
	ctx := context.Background()

	_, err := rs.Get(ctx, "nonexistent-token")
	assert.ErrorIs(t, err, store.ErrTokenNotFound)
}

func TestResetTokenStore_Delete(t *testing.T) {
	rs, _ := newTestResetTokenStore(t)
	ctx := context.Background()

	require.NoError(t, rs.Save(ctx, "deletetoken", "del@example.com"))
	require.NoError(t, rs.Delete(ctx, "deletetoken"))

	_, err := rs.Get(ctx, "deletetoken")
	assert.ErrorIs(t, err, store.ErrTokenNotFound)
}

func TestResetTokenStore_Expiry(t *testing.T) {
	rs, mr := newTestResetTokenStore(t)
	ctx := context.Background()

	require.NoError(t, rs.Save(ctx, "expiretoken", "exp@example.com"))

	// Advance time past 1-hour TTL
	mr.FastForward(90 * time.Minute)

	_, err := rs.Get(ctx, "expiretoken")
	assert.ErrorIs(t, err, store.ErrTokenNotFound, "token should have expired after 1h")
}

func TestResetTokenStore_SaveWithTTL(t *testing.T) {
	rs, mr := newTestResetTokenStore(t)
	ctx := context.Background()

	// Save with a 48-hour TTL (admin invite)
	require.NoError(t, rs.SaveWithTTL(ctx, "invitetoken", "invite@example.com", 48*time.Hour))

	email, err := rs.Get(ctx, "invitetoken")
	require.NoError(t, err)
	assert.Equal(t, "invite@example.com", email)

	// 1h standard TTL should not expire it
	mr.FastForward(90 * time.Minute)
	email, err = rs.Get(ctx, "invitetoken")
	require.NoError(t, err, "token with 48h TTL should still be valid after 1.5h")
	assert.Equal(t, "invite@example.com", email)

	// But 49 hours should
	mr.FastForward(48 * time.Hour)
	_, err = rs.Get(ctx, "invitetoken")
	assert.ErrorIs(t, err, store.ErrTokenNotFound, "token should expire after 48h")
}

func TestResetTokenStore_Get_RedisError(t *testing.T) {
	rs, mr := newTestResetTokenStore(t)
	ctx := context.Background()

	require.NoError(t, rs.Save(ctx, "sometoken", "some@example.com"))
	mr.Close()

	_, err := rs.Get(ctx, "sometoken")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrTokenNotFound, "non-Nil Redis error should not wrap ErrTokenNotFound")
}
