package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/handlers"
	"github.com/ailanguagetutor/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const logoutTestSecret = "logout-test-secret-32-bytes-min!!"

func newLogoutTestHandler(t *testing.T) (*handlers.AuthHandler, *store.TokenBlocklist, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	bl := store.NewTokenBlocklist(rdb)
	cfg := &config.Config{JWTSecret: logoutTestSecret}
	h := handlers.NewAuthHandler(cfg, nil, nil, bl)
	return h, bl, mr
}

func makeLogoutToken(t *testing.T, userID string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": userID, "exp": exp.Unix()}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(logoutTestSecret))
	require.NoError(t, err)
	return tok
}

func TestLogout_AddsTokenToBlocklist(t *testing.T) {
	h, bl, _ := newLogoutTestHandler(t)

	exp := time.Now().Add(7 * 24 * time.Hour)
	tok := makeLogoutToken(t, "user1", exp)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.Logout(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "logged out", resp["message"])

	assert.True(t, bl.IsBlocked(context.Background(), tok), "token should be in blocklist after logout")
}

func TestLogout_CookieToken_AddsToBlocklist(t *testing.T) {
	h, bl, _ := newLogoutTestHandler(t)

	exp := time.Now().Add(time.Hour)
	tok := makeLogoutToken(t, "user2", exp)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tok})
	w := httptest.NewRecorder()
	h.Logout(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, bl.IsBlocked(context.Background(), tok))
}

func TestLogout_NoToken_ReturnsOK(t *testing.T) {
	h, bl, mr := newLogoutTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	h.Logout(w, req)

	// Should succeed even with no token
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, mr.Keys(), "no blocklist entry should be written")
	_ = bl // unused but needed to keep the helper consistent
}

func TestLogout_InvalidToken_ReturnsOK(t *testing.T) {
	h, _, mr := newLogoutTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	w := httptest.NewRecorder()
	h.Logout(w, req)

	// Should not error — just skip the blocklist add
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, mr.Keys(), "invalid token should not be added to blocklist")
}

func TestLogout_BlocklistEntryExpires_WithToken(t *testing.T) {
	h, bl, mr := newLogoutTestHandler(t)

	// Short-lived token
	exp := time.Now().Add(5 * time.Minute)
	tok := makeLogoutToken(t, "user3", exp)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	h.Logout(w(t), req)

	// Advance past token expiry
	mr.FastForward(10 * time.Minute)

	// Blocklist entry should also be gone (TTL tied to token lifetime)
	assert.False(t, bl.IsBlocked(context.Background(), tok), "blocklist entry should expire with the token")
}

// w is a convenience recorder factory for one-shot calls.
func w(t *testing.T) http.ResponseWriter {
	t.Helper()
	return httptest.NewRecorder()
}
