package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/google/uuid"
)

type SentenceHandler struct {
	cfg          *config.Config
	userStore    *store.UserStore
	profileStore *store.StudentProfileStore
	historyStore *store.ConversationHistoryStore
	pool         *store.ItemPool
}

func NewSentenceHandler(cfg *config.Config, us *store.UserStore, ps *store.StudentProfileStore, hs *store.ConversationHistoryStore, pool *store.ItemPool) *SentenceHandler {
	return &SentenceHandler{cfg: cfg, userStore: us, profileStore: ps, historyStore: hs, pool: pool}
}

// ── Types ─────────────────────────────────────────────────────────────────────

type Sentence struct {
	ID         string `json:"id"`
	English    string `json:"english"`
	Target     string `json:"target"`
	GrammarTip string `json:"grammar_tip"`
}

type sentenceSessionRequest struct {
	Language     string `json:"language"`
	Level        int    `json:"level"`
	Topic        string `json:"topic"`
	MistakesMode bool   `json:"mistakes_mode"`
}

type sentenceSessionResponse struct {
	Sentences []Sentence `json:"sentences"`
}

type sentenceCheckRequest struct {
	English        string `json:"english"`
	TargetExpected string `json:"target_expected"`
	UserAnswer     string `json:"user_answer"`
	Language       string `json:"language"`
}

type sentenceCheckResponse struct {
	Correct   bool   `json:"correct"`
	Feedback  string `json:"feedback"`
	Corrected string `json:"corrected"`
}

type sentenceResult struct {
	SentenceID string `json:"sentence_id"`
	GrammarTip string `json:"grammar_tip"`
	Correct    bool   `json:"correct"`
}

type sentenceCompleteRequest struct {
	Language  string           `json:"language"`
	Level     int              `json:"level"`
	Topic     string           `json:"topic"`
	TopicName string           `json:"topic_name"`
	Results   []sentenceResult `json:"results"`
}

type sentenceCompleteResponse struct {
	FPEarned     int      `json:"fp_earned"`
	WeakGrammar  []string `json:"weak_grammar"`
	CorrectCount int      `json:"correct_count"`
	RecordID     string   `json:"record_id"`
}

// ── Session ───────────────────────────────────────────────────────────────────

func (h *SentenceHandler) Session(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req sentenceSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !IsValidLanguage(req.Language) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid language"})
		return
	}
	if req.Level < 1 || req.Level > 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level must be 1-5"})
		return
	}

	langName := LanguageName(req.Language)
	topicName, _ := TopicDetails(req.Topic)
	spec := levelSpec[req.Level]

	profile, _ := h.profileStore.Get(r.Context(), userID, req.Language)

	// Mistakes mode: generate sentences exclusively from weak grammar patterns
	if req.MistakesMode {
		weakGrammar := []string{}
		if profile != nil {
			weakGrammar = profile.WeakGrammar
		}
		if len(weakGrammar) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"sentences": []Sentence{},
				"message":   "No grammar mistakes on record yet. Complete some sentence sessions first!",
			})
			return
		}
		limit := weakGrammar
		if len(limit) > 10 {
			limit = limit[:10]
		}
		b, _ := json.Marshal(limit)
		prompt := fmt.Sprintf(`You are a language teacher creating translation exercises targeting specific grammar weaknesses.
Language: %s, Level: %s
The student previously struggled with these grammar patterns: %s

Generate exactly 10 English sentences for translation into %s that specifically target and practise EACH of these grammar patterns.
Return ONLY valid JSON — no markdown, no code fences, no explanation:
{"sentences":[{"id":"...","english":"...","target":"...","grammar_tip":"..."},...]}
Rules:
- "id": copy the English sentence verbatim
- "english": the English sentence the student will translate
- "target": the correct %s translation
- "grammar_tip": one concise note about the grammar pattern being practised (reference the weak area explicitly)
- Exactly 10 items`,
			langName, spec, string(b), langName, langName)

		result, err := h.callAI(r.Context(), prompt, 1200, 0.7)
		if err != nil {
			log.Printf("sentences/session mistakes AI error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AI service error"})
			return
		}
		result = strings.TrimSpace(result)
		if idx := strings.Index(result, "{"); idx > 0 {
			result = result[idx:]
		}
		if idx := strings.LastIndex(result, "}"); idx >= 0 && idx < len(result)-1 {
			result = result[:idx+1]
		}
		var parsed struct {
			Sentences []Sentence `json:"sentences"`
		}
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			log.Printf("sentences/session mistakes JSON parse error: %v\nraw: %s", err, result)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse AI response"})
			return
		}
		store.Shuffle(parsed.Sentences)
		writeJSON(w, http.StatusOK, sentenceSessionResponse{Sentences: parsed.Sentences})
		return
	}

	// Cache-first: serve from pool if the user hasn't exhausted this key
	key := h.pool.Key(req.Language, req.Level, req.Topic)
	userIdx := 0
	if profile != nil && profile.SentenceListIdx != nil {
		userIdx = profile.SentenceListIdx[key]
	}

	if userIdx < h.pool.Len(key) {
		raw, _ := h.pool.Get(key, userIdx)
		var sentences []Sentence
		if err := json.Unmarshal(raw, &sentences); err == nil {
			store.Shuffle(sentences)
			writeJSON(w, http.StatusOK, sentenceSessionResponse{Sentences: sentences})
			return
		}
	}

	// Cache miss: build exclusion list from all cached pool lists + profile history
	var excludeIDs []string
	for _, raw := range h.pool.AllRaw(key) {
		var list []Sentence
		if err := json.Unmarshal(raw, &list); err == nil {
			for _, s := range list {
				excludeIDs = append(excludeIDs, s.ID)
			}
		}
	}
	if profile != nil && len(profile.RecentSentences) > 0 {
		seen := profile.RecentSentences
		if len(seen) > 20 {
			seen = seen[:20]
		}
		excludeIDs = append(excludeIDs, seen...)
	}
	// Deduplicate
	seenMap := map[string]bool{}
	deduped := excludeIDs[:0]
	for _, id := range excludeIDs {
		if !seenMap[id] {
			seenMap[id] = true
			deduped = append(deduped, id)
		}
	}
	excludeIDs = deduped

	var excludeClause string
	if len(excludeIDs) > 0 {
		b, _ := json.Marshal(excludeIDs)
		excludeClause = fmt.Sprintf("\ndo NOT use these already-seen sentences: %s", string(b))
	}

	var reinforceClause string
	if profile != nil && len(profile.WeakAreas) > 0 {
		weak := profile.WeakAreas
		if len(weak) > 8 {
			weak = weak[:8]
		}
		b, _ := json.Marshal(weak)
		reinforceClause = fmt.Sprintf("\n- TARGET these previously weak grammar patterns in some exercises: %s", string(b))
	}

	prompt := fmt.Sprintf(`You are a language teacher creating translation exercises.
Language: %s, Topic: %s, Level: %s
Generate exactly 10 English sentences for translation into %s.
Return ONLY valid JSON — no markdown, no code fences, no explanation:
{"sentences":[{"id":"...","english":"...","target":"...","grammar_tip":"..."},...]}
Rules:
- "id": copy the English sentence verbatim
- "english": the English sentence the student will translate
- "target": the correct %s translation
- "grammar_tip": one concise grammar note about the key structure used (e.g. "uses subjunctive mood")
- vary structures: include statements, questions, conditionals, and imperatives
- match complexity to the level: %s%s%s
- Exactly 10 items`,
		langName, topicName, spec, langName, langName, spec, excludeClause, reinforceClause)

	result, err := h.callAI(r.Context(), prompt, 1200, 0.8)
	if err != nil {
		log.Printf("sentences/session AI error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AI service error"})
		return
	}

	result = strings.TrimSpace(result)
	if idx := strings.Index(result, "{"); idx > 0 {
		result = result[idx:]
	}
	if idx := strings.LastIndex(result, "}"); idx >= 0 && idx < len(result)-1 {
		result = result[:idx+1]
	}

	var parsed struct {
		Sentences []Sentence `json:"sentences"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		log.Printf("sentences/session JSON parse error: %v\nraw: %s", err, result)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse AI response"})
		return
	}

	// Cache the new list, then shuffle before returning
	if raw, err := json.Marshal(parsed.Sentences); err == nil {
		h.pool.Append(key, raw)
	}
	store.Shuffle(parsed.Sentences)
	writeJSON(w, http.StatusOK, sentenceSessionResponse{Sentences: parsed.Sentences})
}

// ── Check ─────────────────────────────────────────────────────────────────────

func (h *SentenceHandler) Check(w http.ResponseWriter, r *http.Request) {
	var req sentenceCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	langName := LanguageName(req.Language)
	prompt := fmt.Sprintf(`Evaluate this %s translation:
English: "%s"
Expected: "%s"
Student: "%s"
Reply ONLY with valid JSON: {"correct":true/false,"feedback":"grammar note if wrong, empty string if correct","corrected":"corrected form if wrong, empty string if correct"}
Minor spelling variants are OK if grammatically equivalent.`,
		langName, req.English, req.TargetExpected, req.UserAnswer)

	result, err := h.callAI(r.Context(), prompt, 200, 0.1)
	if err != nil {
		// Fallback: edit-distance check
		answer := strings.ToLower(strings.TrimSpace(req.UserAnswer))
		expected := strings.ToLower(strings.TrimSpace(req.TargetExpected))
		correct := answer == expected || editDistance(answer, expected) <= 3
		writeJSON(w, http.StatusOK, sentenceCheckResponse{Correct: correct})
		return
	}

	result = strings.TrimSpace(result)
	if idx := strings.Index(result, "{"); idx > 0 {
		result = result[idx:]
	}
	if idx := strings.LastIndex(result, "}"); idx >= 0 && idx < len(result)-1 {
		result = result[:idx+1]
	}

	var parsed sentenceCheckResponse
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		answer := strings.ToLower(strings.TrimSpace(req.UserAnswer))
		expected := strings.ToLower(strings.TrimSpace(req.TargetExpected))
		correct := answer == expected || editDistance(answer, expected) <= 3
		writeJSON(w, http.StatusOK, sentenceCheckResponse{Correct: correct})
		return
	}

	writeJSON(w, http.StatusOK, parsed)
}

// ── Complete ──────────────────────────────────────────────────────────────────

func (h *SentenceHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req sentenceCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	topicName, _ := TopicDetails(req.Topic)

	var correctCount int
	var weakGrammar []string
	var learnedIDs []string
	for _, res := range req.Results {
		if res.Correct {
			correctCount++
			learnedIDs = append(learnedIDs, res.SentenceID)
		} else if res.GrammarTip != "" {
			weakGrammar = append(weakGrammar, res.GrammarTip)
		}
	}

	fp := correctCount * 8
	if fp < 10 {
		fp = 10
	}

	if _, _, err := h.userStore.UpdateActivity(userID, req.Language, fp); err != nil {
		log.Printf("sentences/complete UpdateActivity error: %v", err)
	}

	ctx := context.Background()
	profile, err := h.profileStore.Get(ctx, userID, req.Language)
	if err != nil || profile == nil {
		profile = &store.StudentProfile{
			UserID:   userID,
			Language: req.Language,
		}
	}

	profile.WeakAreas       = prependUnique(weakGrammar, profile.WeakAreas, 20)
	profile.WeakGrammar     = prependUnique(weakGrammar, profile.WeakGrammar, 20)
	profile.RecentSentences = prependUnique(learnedIDs, profile.RecentSentences, 20)
	profile.RecentTopics    = prependUnique([]string{req.TopicName}, profile.RecentTopics, 10)
	profile.SessionCount++

	// Advance the user's list index for this pool key
	key := h.pool.Key(req.Language, req.Level, req.Topic)
	if profile.SentenceListIdx == nil {
		profile.SentenceListIdx = make(map[string]int)
	}
	profile.SentenceListIdx[key]++

	if err := h.profileStore.Upsert(ctx, profile); err != nil {
		log.Printf("sentences/complete Upsert error: %v", err)
	}

	total := len(req.Results)
	summary := fmt.Sprintf("Completed Sentence Builder on %s: %d/%d sentences correct.", topicName, correctCount, total)
	var suggestions []string
	if len(weakGrammar) > 0 {
		suggestions = []string{
			"Practice these grammar patterns in a conversation session",
			fmt.Sprintf("Focus on: %s", strings.Join(weakGrammar[:min(2, len(weakGrammar))], "; ")),
			"Repeat this topic to reinforce the grammar structures you missed",
		}
	} else {
		suggestions = []string{
			"Try a harder level to challenge your grammar skills",
			"Practice these sentence structures in a full conversation",
			"Explore a new topic to build more grammar variety",
		}
	}

	recordID := uuid.New().String()
	record := &store.ConversationRecord{
		ID:           recordID,
		UserID:       userID,
		Language:     req.Language,
		Topic:        req.Topic,
		TopicName:    topicName,
		Level:        req.Level,
		Personality:  "sentence-builder",
		MessageCount: total,
		FPEarned:     fp,
		Summary:      summary,
		Topics:       []string{topicName},
		Corrections:  weakGrammar,
		Suggestions:  suggestions,
		CreatedAt:    time.Now(),
		EndedAt:      time.Now(),
	}
	h.historyStore.Save(record)

	writeJSON(w, http.StatusOK, sentenceCompleteResponse{
		FPEarned:     fp,
		WeakGrammar:  weakGrammar,
		CorrectCount: correctCount,
		RecordID:     recordID,
	})
}

// ── AI helper ─────────────────────────────────────────────────────────────────

func (h *SentenceHandler) callAI(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	payload := ionosVocabPayload{
		Model: h.cfg.IONOSFastModel,
		Messages: []store.Message{
			{Role: "user", Content: prompt},
		},
		Stream:      false,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.cfg.IONOSBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.cfg.IONOSAPIKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI returned %d: %s", resp.StatusCode, string(raw))
	}

	var parsed ionosVocabResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("no choices in AI response")
	}
	content := parsed.Choices[0].Message.Content
	if content == "" {
		content = parsed.Choices[0].Message.ReasoningContent
	}
	return content, nil
}
