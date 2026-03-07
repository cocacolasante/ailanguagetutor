package handlers

import (
	"net/http"

	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/go-chi/chi/v5"
)

type GamificationHandler struct {
	userStore    *store.UserStore
	historyStore *store.ConversationHistoryStore
}

func NewGamificationHandler(us *store.UserStore, hs *store.ConversationHistoryStore) *GamificationHandler {
	return &GamificationHandler{userStore: us, historyStore: hs}
}

// Stats returns the current user's gamification stats and recent conversations.
func (h *GamificationHandler) Stats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	user, err := h.userStore.GetByID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	records := h.historyStore.GetForUser(userID)
	recent := records
	if len(recent) > 3 {
		recent = recent[:3]
	}

	langFP := user.LanguageFP
	if langFP == nil {
		langFP = map[string]int{}
	}
	langLevel := user.LanguageLevel
	if langLevel == nil {
		langLevel = map[string]int{}
	}
	achievements := user.Achievements
	if achievements == nil {
		achievements = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"streak":               user.Streak,
		"last_activity_date":   user.LastActivityDate,
		"total_fp":             user.TotalFP,
		"language_fp":          langFP,
		"language_level":       langLevel,
		"achievements":         achievements,
		"conversation_count":   user.ConversationCount,
		"recent_conversations": recent,
	})
}

// Leaderboard returns the top 50 users by total FP.
func (h *GamificationHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	entries := h.userStore.GetLeaderboard(50)
	if entries == nil {
		entries = []store.LeaderboardEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"leaderboard": entries,
	})
}

// Records returns the current user's conversation history (last 10).
func (h *GamificationHandler) Records(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	records := h.historyStore.GetForUser(userID)
	if records == nil {
		records = []*store.ConversationRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"records": records,
	})
}

// GetRecord returns a single conversation record.
func (h *GamificationHandler) GetRecord(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	recordID := chi.URLParam(r, "id")

	record, err := h.historyStore.GetRecord(recordID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "record not found"})
		return
	}
	if record.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, record)
}

// Badges returns all available achievement badge definitions.
func (h *GamificationHandler) Badges(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"badges": store.AllBadges,
	})
}
