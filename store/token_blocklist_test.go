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

func newTestBlocklist(t *testing.T) (*store.TokenBlocklist, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return store.NewTokenBlocklist(rdb), mr
}

func TestTokenBlocklist_NotBlocked_Initially(t *testing.T) {
	bl, _ := newTestBlocklist(t)

	blocked := bl.IsBlocked(context.Background(), "some.jwt.token")
	assert.False(t, blocked)
}

func TestTokenBlocklist_Add_ThenBlocked(t *testing.T) {
	bl, _ := newTestBlocklist(t)

	token := "header.payload.signature"
	exp := time.Now().Add(7 * 24 * time.Hour)

	err := bl.Add(context.Background(), token, exp)
	require.NoError(t, err)

	assert.True(t, bl.IsBlocked(context.Background(), token))
}

func TestTokenBlocklist_DifferentTokensAreIndependent(t *testing.T) {
	bl, _ := newTestBlocklist(t)

	token1 := "token.one.here"
	token2 := "token.two.here"
	exp := time.Now().Add(time.Hour)

	err := bl.Add(context.Background(), token1, exp)
	require.NoError(t, err)

	assert.True(t, bl.IsBlocked(context.Background(), token1), "token1 should be blocked")
	assert.False(t, bl.IsBlocked(context.Background(), token2), "token2 should not be blocked")
}

func TestTokenBlocklist_Add_AlreadyExpiredToken_NotStored(t *testing.T) {
	bl, mr := newTestBlocklist(t)

	token := "expired.jwt.token"
	exp := time.Now().Add(-1 * time.Hour) // already expired

	err := bl.Add(context.Background(), token, exp)
	require.NoError(t, err) // should not error, just no-op

	// Nothing written to Redis
	keys := mr.Keys()
	assert.Empty(t, keys, "no key should be stored for an already-expired token")
	assert.False(t, bl.IsBlocked(context.Background(), token))
}

func TestTokenBlocklist_BlocklistEntry_ExpiresAfterTTL(t *testing.T) {
	bl, mr := newTestBlocklist(t)

	token := "expiring.soon.token"
	exp := time.Now().Add(2 * time.Hour)

	err := bl.Add(context.Background(), token, exp)
	require.NoError(t, err)
	assert.True(t, bl.IsBlocked(context.Background(), token))

	// Advance past TTL
	mr.FastForward(3 * time.Hour)

	assert.False(t, bl.IsBlocked(context.Background(), token), "entry should expire from Redis")
}

func TestTokenBlocklist_FailOpen_OnRedisError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	bl := store.NewTokenBlocklist(rdb)

	// Kill Redis mid-flight
	mr.Close()

	// Should fail open (return false), not panic or error
	blocked := bl.IsBlocked(context.Background(), "some.token")
	assert.False(t, blocked, "should fail open when Redis is unreachable")
}

func TestTokenBlocklist_TTL_MatchesTokenLifetime(t *testing.T) {
	bl, mr := newTestBlocklist(t)

	token := "precise.ttl.token"
	lifetime := 30 * time.Minute
	exp := time.Now().Add(lifetime)

	err := bl.Add(context.Background(), token, exp)
	require.NoError(t, err)

	// Confirm a blocklist key exists with a TTL close to the token lifetime
	keys := mr.Keys()
	require.Len(t, keys, 1)
	ttl := mr.TTL(keys[0])
	assert.Greater(t, ttl, 25*time.Minute, "TTL should be close to token lifetime")
	assert.LessOrEqual(t, ttl, lifetime, "TTL should not exceed token lifetime")
}
