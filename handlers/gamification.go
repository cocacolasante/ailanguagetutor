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
	profileStore *store.StudentProfileStore
	cacheStore   *store.CacheStore
}

func NewGamificationHandler(us *store.UserStore, hs *store.ConversationHistoryStore, ps *store.StudentProfileStore, cs *store.CacheStore) *GamificationHandler {
	return &GamificationHandler{userStore: us, historyStore: hs, profileStore: ps, cacheStore: cs}
}

// Stats returns the current user's gamification stats and recent conversations.
func (h *GamificationHandler) Stats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	// Serve from cache if available
	var cached map[string]any
	if h.cacheStore.GetUserStats(r.Context(), userID, &cached) {
		writeJSON(w, http.StatusOK, cached)
		return
	}

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

	stats := map[string]any{
		"streak":               user.Streak,
		"last_activity_date":   user.LastActivityDate,
		"total_fp":             user.TotalFP,
		"language_fp":          langFP,
		"language_level":       langLevel,
		"achievements":         achievements,
		"conversation_count":   user.ConversationCount,
		"recent_conversations": recent,
	}

	_ = h.cacheStore.SetUserStats(r.Context(), userID, stats)
	writeJSON(w, http.StatusOK, stats)
}

// Leaderboard returns the top 50 users by total FP.
func (h *GamificationHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	// Serve from cache if available
	if entries, ok := h.cacheStore.GetLeaderboard(r.Context()); ok {
		writeJSON(w, http.StatusOK, map[string]any{"leaderboard": entries})
		return
	}

	entries := h.userStore.GetLeaderboard(50)
	if entries == nil {
		entries = []store.LeaderboardEntry{}
	}
	_ = h.cacheStore.SetLeaderboard(r.Context(), entries)
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

// GetMistakes returns the user's weak_vocab and weak_grammar for a given language.
func (h *GamificationHandler) GetMistakes(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	language := r.URL.Query().Get("language")
	if language == "" || !IsValidLanguage(language) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid language param required"})
		return
	}

	profile, err := h.profileStore.Get(r.Context(), userID, language)
	if err != nil || profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"language":    language,
			"weak_vocab":  []string{},
			"weak_grammar": []string{},
			"has_mistakes": false,
		})
		return
	}

	weakVocab := profile.WeakVocab
	if weakVocab == nil {
		weakVocab = []string{}
	}
	weakGrammar := profile.WeakGrammar
	if weakGrammar == nil {
		weakGrammar = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"language":     language,
		"weak_vocab":   weakVocab,
		"weak_grammar": weakGrammar,
		"has_mistakes": len(weakVocab) > 0 || len(weakGrammar) > 0,
	})
}
