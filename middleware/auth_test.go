package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-jwt-secret-32-bytes-minimum!!"

func newTestMiddleware(t *testing.T) (*middleware.AuthMiddleware, *store.TokenBlocklist, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	bl := store.NewTokenBlocklist(rdb)
	cfg := &config.Config{JWTSecret: testSecret}
	return middleware.NewAuthMiddleware(cfg, bl), bl, mr
}

func makeToken(t *testing.T, userID string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": userID, "exp": exp.Unix()}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	require.NoError(t, err)
	return tok
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(userID))
}

// ── Missing / malformed token ──────────────────────────────────────────────

func TestProtect_NoToken_Returns401(t *testing.T) {
	am, _, _ := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProtect_InvalidToken_Returns401(t *testing.T) {
	am, _, _ := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProtect_ExpiredToken_Returns401(t *testing.T) {
	am, _, _ := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	tok := makeToken(t, "user123", time.Now().Add(-1*time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ── Valid token flows ──────────────────────────────────────────────────────

func TestProtect_ValidBearerToken_Passes(t *testing.T) {
	am, _, _ := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	tok := makeToken(t, "user42", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user42", w.Body.String())
}

func TestProtect_ValidCookieToken_Passes(t *testing.T) {
	am, _, _ := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	tok := makeToken(t, "user99", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: tok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user99", w.Body.String())
}

// ── Blocklist ──────────────────────────────────────────────────────────────

func TestProtect_BlockedToken_Returns401(t *testing.T) {
	am, bl, _ := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	exp := time.Now().Add(7 * 24 * time.Hour)
	tok := makeToken(t, "user1", exp)

	err := bl.Add(context.Background(), tok, exp)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProtect_BlocklistExpires_TokenAllowedAgain(t *testing.T) {
	am, bl, mr := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	shortExp := time.Now().Add(1 * time.Minute)
	tok := makeToken(t, "user1", time.Now().Add(time.Hour))

	err := bl.Add(context.Background(), tok, shortExp)
	require.NoError(t, err)

	// Blocked now
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Advance past blocklist TTL
	mr.FastForward(2 * time.Minute)

	// Token JWT itself is still valid (1h exp) — blocklist entry gone
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestProtect_UnblockedTokenAfterOtherRevoked(t *testing.T) {
	am, bl, _ := newTestMiddleware(t)
	handler := am.Protect(http.HandlerFunc(okHandler))

	exp := time.Now().Add(time.Hour)
	blockedTok := makeToken(t, "user1", exp)
	cleanTok := makeToken(t, "user2", exp)

	err := bl.Add(context.Background(), blockedTok, exp)
	require.NoError(t, err)

	// blocked token → 401
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.Header.Set("Authorization", "Bearer "+blockedTok)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	assert.Equal(t, http.StatusUnauthorized, w1.Code)

	// clean token → 200
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("Authorization", "Bearer "+cleanTok)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "user2", w2.Body.String())
}

// ── UserIDKey propagation ──────────────────────────────────────────────────

func TestProtect_SetsUserIDInContext(t *testing.T) {
	am, _, _ := newTestMiddleware(t)

	var capturedID string
	capture := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = r.Context().Value(middleware.UserIDKey).(string)
		w.WriteHeader(http.StatusOK)
	})
	handler := am.Protect(capture)

	tok := makeToken(t, "user-abc-123", time.Now().Add(time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "user-abc-123", capturedID)
}
