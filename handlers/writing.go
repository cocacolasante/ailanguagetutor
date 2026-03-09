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

// ── Handler ────────────────────────────────────────────────────────────────────

type WritingHandler struct {
	cfg           *config.Config
	userStore     *store.UserStore
	profileStore  *store.StudentProfileStore
	historyStore  *store.ConversationHistoryStore
	sessionStore  *store.SessionStore
	pool          *store.ItemPool
	presenceStore *store.PresenceStore
	cacheStore    *store.CacheStore
}

func NewWritingHandler(
	cfg *config.Config,
	us *store.UserStore,
	ps *store.StudentProfileStore,
	hs *store.ConversationHistoryStore,
	ss *store.SessionStore,
	pool *store.ItemPool,
	presence *store.PresenceStore,
	cache *store.CacheStore,
) *WritingHandler {
	return &WritingHandler{
		cfg:           cfg,
		userStore:     us,
		profileStore:  ps,
		historyStore:  hs,
		sessionStore:  ss,
		pool:          pool,
		presenceStore: presence,
		cacheStore:    cache,
	}
}

// ── Types ──────────────────────────────────────────────────────────────────────

type writingSessionRequest struct {
	Language  string `json:"language"`
	Level     int    `json:"level"`
	Topic     string `json:"topic"`
	TopicName string `json:"topic_name"`
}

type writingSessionResponse struct {
	SessionID    string `json:"session_id"`
	FirstMessage string `json:"first_message"`
	Language     string `json:"language"`
	Level        int    `json:"level"`
	Topic        string `json:"topic"`
	TopicName    string `json:"topic_name"`
}

type writingMessageRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type writingMessageResponse struct {
	Reply        string   `json:"reply"`
	Misspellings []string `json:"misspellings"`
}

type writingCompleteRequest struct {
	SessionID    string   `json:"session_id"`
	DurationSecs int      `json:"duration_secs"`
	TopicName    string   `json:"topic_name"`
	Misspellings []string `json:"misspellings"`
}

type writingCompleteResponse struct {
	RecordID        string   `json:"record_id"`
	FPEarned        int      `json:"fp_earned"`
	NewStreak       int      `json:"new_streak"`
	NewAchievements []string `json:"new_achievements"`
	TotalFP         int      `json:"total_fp"`
	Summary         string   `json:"summary"`
	Topics          []string `json:"topics_discussed"`
	Vocabulary      []string `json:"vocabulary_learned"`
	Corrections     []string `json:"grammar_corrections"`
	Suggestions     []string `json:"suggested_next_lessons"`
	Language        string   `json:"language"`
	Topic           string   `json:"topic"`
	TopicName       string   `json:"topic_name"`
	Level           int      `json:"level"`
	Personality     string   `json:"personality"`
	MessageCount    int      `json:"message_count"`
	DurationSecs    int      `json:"duration_secs"`
	Misspellings    []string `json:"misspellings"`
}

// ── Session ────────────────────────────────────────────────────────────────────

func (h *WritingHandler) Session(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req writingSessionRequest
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

	u, err := h.userStore.GetByID(userID)
	if err == nil {
		if !u.HasConversationAccess() {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "Your subscription has ended. Please visit your profile to resubscribe.",
				"code":  "subscription_ended",
			})
			return
		}
		if !u.HasFullAccess() && req.Level > 3 {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "Levels 4 and 5 require a full subscription.",
			})
			return
		}
	}

	langName := LanguageName(req.Language)
	topicName, topicDesc := TopicDetails(req.Topic)
	if req.TopicName != "" {
		topicName = req.TopicName
	}
	spec := levelSpec[req.Level]
	if spec == "" {
		spec = "INTERMEDIATE (B1)"
	}

	profile, _ := h.profileStore.Get(r.Context(), userID, req.Language)

	key := h.pool.Key(req.Language, req.Level, req.Topic)
	userIdx := 0
	if profile != nil && profile.WritingListIdx != nil {
		userIdx = profile.WritingListIdx[key]
	}

	var firstMessage string

	// Cache hit
	if userIdx < h.pool.Len(key) {
		raw, _ := h.pool.Get(key, userIdx)
		firstMessage = string(raw)
	}

	// Cache miss: generate opening message
	if firstMessage == "" {
		prompt := fmt.Sprintf(
			`You are a language tutor texting a student casually in %s.
Topic: %s — %s | Level: %s
Write ONE opening text (1–2 sentences) in %s that asks an engaging question about the topic.
Output ONLY the message text — no JSON, no quotes, no explanation.`,
			langName, topicName, topicDesc, spec, langName,
		)

		result, err := h.callAI(r.Context(), prompt, 150, 0.9)
		if err != nil {
			log.Printf("writing/session AI error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AI service error"})
			return
		}
		firstMessage = strings.TrimSpace(result)
		// Cache it as raw bytes
		h.pool.Append(key, []byte(firstMessage))
	}

	// Build system prompt for the session
	systemPrompt := fmt.Sprintf(
		`You are a language tutor texting a student in %s about %s.
Level: %s. Reply naturally in 1–3 sentences. Stay in %s only.
Also check the student's LAST message for clear spelling errors.
Return ONLY valid JSON (no markdown):
{ "reply": "your reply in %s", "misspellings": ["wrong → correct (brief note)"] }
If no misspellings, return empty array.`,
		langName, topicName, spec, langName, langName,
	)

	session := h.sessionStore.Create(userID, req.Language, req.Topic, req.Level, "writing-coach", systemPrompt)
	_ = h.sessionStore.AddMessage(session.ID, store.Message{Role: "assistant", Content: firstMessage})

	_ = h.presenceStore.Set(r.Context(), userID, store.LessonPresence{
		Type:      "writing",
		Language:  req.Language,
		Topic:     req.Topic,
		StartedAt: session.CreatedAt,
	})

	writeJSON(w, http.StatusOK, writingSessionResponse{
		SessionID:    session.ID,
		FirstMessage: firstMessage,
		Language:     req.Language,
		Level:        req.Level,
		Topic:        req.Topic,
		TopicName:    topicName,
	})
}

// ── Message ────────────────────────────────────────────────────────────────────

func (h *WritingHandler) Message(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req writingMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message cannot be empty"})
		return
	}
	if len(req.Message) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message too long"})
		return
	}

	session, err := h.sessionStore.Get(req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if session.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	_ = h.sessionStore.AddMessage(req.SessionID, store.Message{Role: "user", Content: req.Message})

	msgs, _ := h.sessionStore.GetMessages(req.SessionID)

	// Build messages for AI: system prompt + all conversation messages
	aiMessages := make([]store.Message, 0, len(session.Messages))
	aiMessages = append(aiMessages, session.Messages[0]) // system prompt
	aiMessages = append(aiMessages, msgs...)

	langName := LanguageName(session.Language)
	topicName, _ := TopicDetails(session.Topic)
	spec := levelSpec[session.Level]
	if spec == "" {
		spec = "INTERMEDIATE (B1)"
	}

	// Override the system message to include full instructions
	aiMessages[0] = store.Message{
		Role: "system",
		Content: fmt.Sprintf(
			`You are a language tutor texting a student in %s about %s.
Level: %s. Reply naturally in 1–3 sentences. Stay in %s only.
Also check the student's LAST message for clear spelling errors.
Return ONLY valid JSON (no markdown):
{ "reply": "your reply in %s", "misspellings": ["wrong → correct (brief note)"] }
If no misspellings, return empty array.`,
			langName, topicName, spec, langName, langName,
		),
	}

	result, err := h.callAIMessages(r.Context(), aiMessages, 400, 0.75)
	if err != nil {
		log.Printf("writing/message AI error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AI service error"})
		return
	}

	// Parse JSON response
	result = strings.TrimSpace(result)
	if idx := strings.Index(result, "{"); idx > 0 {
		result = result[idx:]
	}
	if idx := strings.LastIndex(result, "}"); idx >= 0 && idx < len(result)-1 {
		result = result[:idx+1]
	}

	var parsed struct {
		Reply        string   `json:"reply"`
		Misspellings []string `json:"misspellings"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		// Fallback: treat raw content as reply
		parsed.Reply = strings.TrimSpace(result)
		parsed.Misspellings = []string{}
	}
	if parsed.Misspellings == nil {
		parsed.Misspellings = []string{}
	}

	_ = h.sessionStore.AddMessage(req.SessionID, store.Message{Role: "assistant", Content: parsed.Reply})

	writeJSON(w, http.StatusOK, writingMessageResponse{
		Reply:        parsed.Reply,
		Misspellings: parsed.Misspellings,
	})
}

// ── Complete ───────────────────────────────────────────────────────────────────

func (h *WritingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req writingCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	session, err := h.sessionStore.Get(req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if session.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	msgs, _ := h.sessionStore.GetMessages(req.SessionID)
	userMsgCount := 0
	for _, m := range msgs {
		if m.Role == "user" {
			userMsgCount++
		}
	}

	fp := 10 + userMsgCount*3 + session.Level*5
	if fp < 10 {
		fp = 10
	}
	if fp > 80 {
		fp = 80
	}

	topicName := req.TopicName
	if topicName == "" {
		topicName, _ = TopicDetails(session.Topic)
	}

	summaryRes := h.generateSummary(r.Context(), session.Language, session.Level, topicName, msgs, req.DurationSecs)

	newStreak, newBadges, _ := h.userStore.UpdateActivity(userID, session.Language, fp)

	totalFP := 0
	if u, err := h.userStore.GetByID(userID); err == nil {
		totalFP = u.TotalFP
	}

	if req.Misspellings == nil {
		req.Misspellings = []string{}
	}
	if newBadges == nil {
		newBadges = []string{}
	}

	record := &store.ConversationRecord{
		ID:           uuid.New().String(),
		UserID:       userID,
		SessionID:    session.ID,
		Language:     session.Language,
		Topic:        session.Topic,
		TopicName:    topicName,
		Level:        session.Level,
		Personality:  "writing-coach",
		MessageCount: len(msgs),
		DurationSecs: req.DurationSecs,
		FPEarned:     fp,
		Summary:      summaryRes.Summary,
		Topics:       summaryRes.Topics,
		Vocabulary:   summaryRes.Vocabulary,
		Corrections:  summaryRes.Corrections,
		Suggestions:  summaryRes.Suggestions,
		Misspellings: req.Misspellings,
		CreatedAt:    session.CreatedAt,
		EndedAt:      time.Now(),
	}
	h.historyStore.Save(record)

	_ = h.presenceStore.Clear(r.Context(), userID)
	_ = h.cacheStore.InvalidateUserStats(r.Context(), userID)

	// Advance writing list index
	ctx := context.Background()
	profile, err := h.profileStore.Get(ctx, userID, session.Language)
	if err != nil || profile == nil {
		profile = &store.StudentProfile{
			UserID:   userID,
			Language: session.Language,
		}
	}
	key := h.pool.Key(session.Language, session.Level, session.Topic)
	if profile.WritingListIdx == nil {
		profile.WritingListIdx = make(map[string]int)
	}
	profile.WritingListIdx[key]++
	profile.RecentTopics = prependUnique([]string{topicName}, profile.RecentTopics, 10)
	profile.SessionCount++
	if err := h.profileStore.Upsert(ctx, profile); err != nil {
		log.Printf("writing/complete Upsert error: %v", err)
	}

	writeJSON(w, http.StatusOK, writingCompleteResponse{
		RecordID:        record.ID,
		FPEarned:        fp,
		NewStreak:       newStreak,
		NewAchievements: newBadges,
		TotalFP:         totalFP,
		Summary:         summaryRes.Summary,
		Topics:          summaryRes.Topics,
		Vocabulary:      summaryRes.Vocabulary,
		Corrections:     summaryRes.Corrections,
		Suggestions:     summaryRes.Suggestions,
		Language:        session.Language,
		Topic:           session.Topic,
		TopicName:       topicName,
		Level:           session.Level,
		Personality:     "writing-coach",
		MessageCount:    len(msgs),
		DurationSecs:    req.DurationSecs,
		Misspellings:    req.Misspellings,
	})
}

// ── Summary generation ─────────────────────────────────────────────────────────

type writingSummaryResult struct {
	Summary     string   `json:"summary"`
	Topics      []string `json:"topics_discussed"`
	Vocabulary  []string `json:"vocabulary_learned"`
	Corrections []string `json:"grammar_corrections"`
	Suggestions []string `json:"suggested_next_lessons"`
}

func (h *WritingHandler) generateSummary(ctx context.Context, language string, level int, topicName string, msgs []store.Message, durationSecs int) writingSummaryResult {
	langName := LanguageName(language)
	fallback := writingSummaryResult{
		Summary:     fmt.Sprintf("Great writing practice session in %s! Keep it up.", langName),
		Suggestions: []string{"Keep practicing to build fluency!", "Review any vocabulary from today's session."},
	}

	if len(msgs) < 2 {
		return fallback
	}

	startIdx := 0
	if len(msgs) > 20 {
		startIdx = len(msgs) - 20
	}
	var transcript strings.Builder
	for _, m := range msgs[startIdx:] {
		role := "Student"
		if m.Role == "assistant" {
			role = "Tutor"
		}
		transcript.WriteString(fmt.Sprintf("[%s]: %s\n", role, m.Content))
	}

	levelNames := []string{"", "Beginner", "Elementary", "Intermediate", "Advanced", "Fluent"}
	levelName := "Intermediate"
	if level >= 1 && level <= 5 {
		levelName = levelNames[level]
	}
	durationStr := fmt.Sprintf("%d min %d sec", durationSecs/60, durationSecs%60)

	prompt := fmt.Sprintf(`You are a language learning analytics assistant. Analyze the %s writing conversation transcript below and return a JSON object. Return ONLY valid JSON — no markdown, no code fences, no extra text.

RULES — you MUST follow these exactly:
- "summary": Write 2-3 complete sentences describing what the student practiced in writing.
- "topics_discussed": List 2-4 specific topics or themes that came up.
- "vocabulary_learned": List every %s word or phrase that appeared (format: "word: English meaning"). If fewer than 3, infer 2-3 relevant words.
- "grammar_corrections": List any grammar mistakes from the student's written messages with a brief correction. If no mistakes, write one writing tip relevant to their level.
- "suggested_next_lessons": Always provide exactly 3 specific, actionable next steps.

Student level: %s (%d/5)
Topic: %s
Duration: %s
Messages exchanged: %d

Transcript:
%s`,
		langName, langName, levelName, level, topicName, durationStr, len(msgs), transcript.String(),
	)

	result, err := h.callAI(ctx, prompt, 1024, 0.3)
	if err != nil {
		return fallback
	}

	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var sr writingSummaryResult
	if err := json.Unmarshal([]byte(result), &sr); err != nil {
		log.Printf("writing summary parse error: %v — raw: %s", err, result)
		return writingSummaryResult{
			Summary:     result,
			Suggestions: []string{"Keep practicing " + langName + " regularly!"},
		}
	}
	return sr
}

// ── AI helpers ─────────────────────────────────────────────────────────────────

type ionosWritingPayload struct {
	Model       string          `json:"model"`
	Messages    []store.Message `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
}

type ionosWritingResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
}

func (h *WritingHandler) callAI(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	return h.callAIMessages(ctx, []store.Message{{Role: "user", Content: prompt}}, maxTokens, temperature)
}

func (h *WritingHandler) callAIMessages(ctx context.Context, messages []store.Message, maxTokens int, temperature float64) (string, error) {
	payload := ionosWritingPayload{
		Model:       h.cfg.IONOSFastModel,
		Messages:    messages,
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

	var parsed ionosWritingResponse
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
