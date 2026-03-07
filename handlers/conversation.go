package handlers

import (
	"bufio"
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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ConversationHandler struct {
	cfg          *config.Config
	sessionStore *store.SessionStore
	contextStore *store.ContextStore
	userStore    *store.UserStore
	historyStore *store.ConversationHistoryStore
}

func NewConversationHandler(cfg *config.Config, ss *store.SessionStore, cs *store.ContextStore, us *store.UserStore, hs *store.ConversationHistoryStore) *ConversationHandler {
	return &ConversationHandler{cfg: cfg, sessionStore: ss, contextStore: cs, userStore: us, historyStore: hs}
}

// ── Start ─────────────────────────────────────────────────────────────────────

type startRequest struct {
	Language    string `json:"language"`
	Topic       string `json:"topic"`
	Level       int    `json:"level"`
	Personality string `json:"personality"`
}

type startResponse struct {
	SessionID   string `json:"session_id"`
	Language    string `json:"language"`
	Topic       string `json:"topic"`
	TopicName   string `json:"topic_name"`
	Level       int    `json:"level"`
	Personality string `json:"personality"`
}

func (h *ConversationHandler) Start(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req startRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if !IsValidLanguage(req.Language) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid language"})
		return
	}
	if !IsValidTopic(req.Topic) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic"})
		return
	}
	if req.Level < 1 || req.Level > 5 {
		req.Level = 3
	}
	if !IsValidPersonality(req.Personality) {
		req.Personality = ""
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
				"error": "Levels 4 and 5 require a full subscription. Upgrade to unlock advanced practice.",
			})
			return
		}
	}

	topicName, topicDesc := TopicDetails(req.Topic)

	priorMsgs := h.contextStore.Get(userID, req.Language, req.Level)
	systemPrompt := buildSystemPrompt(req.Language, req.Level, topicName, topicDesc, req.Topic, req.Personality, len(priorMsgs) > 0)
	session := h.sessionStore.Create(userID, req.Language, req.Topic, req.Level, req.Personality, systemPrompt)

	for _, m := range priorMsgs {
		_ = h.sessionStore.AddMessage(session.ID, m)
	}

	writeJSON(w, http.StatusCreated, startResponse{
		SessionID:   session.ID,
		Language:    session.Language,
		Topic:       session.Topic,
		TopicName:   topicName,
		Level:       session.Level,
		Personality: session.Personality,
	})
}

// ── End ───────────────────────────────────────────────────────────────────────

type endRequest struct {
	SessionID    string          `json:"session_id"`
	DurationSecs int             `json:"duration_secs"`
	// Agent flow: frontend sends the transcript collected from ElevenLabs WebSocket events.
	// Legacy flow: left empty; messages are read from the session store.
	Transcript   []store.Message `json:"transcript,omitempty"`
	MessageCount int             `json:"message_count,omitempty"`
}

type endResponse struct {
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
}

func (h *ConversationHandler) End(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req endRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
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

	// Agent flow: transcript provided by frontend from ElevenLabs WebSocket events.
	// Legacy flow: read messages from the in-memory session store.
	var msgs []store.Message
	var userMsgCount int

	if len(req.Transcript) > 0 {
		msgs = req.Transcript
		userMsgCount = req.MessageCount
		// Persist to session store so context is available for future sessions
		for _, m := range msgs {
			_ = h.sessionStore.AddMessage(req.SessionID, m)
		}
	} else {
		msgs, _ = h.sessionStore.GetMessages(req.SessionID)
		for _, m := range msgs {
			if m.Role == "user" {
				userMsgCount++
			}
		}
	}

	// FP = message_count * 3 + level * 5, minimum 5, maximum 100
	fp := userMsgCount*3 + session.Level*5
	if fp < 5 {
		fp = 5
	}
	if fp > 100 {
		fp = 100
	}

	topicName, _ := TopicDetails(session.Topic)

	// Generate AI summary (may be slow — acceptable since user just ended session)
	summaryResult := h.generateSummary(r.Context(), session.Language, session.Level, topicName, msgs, req.DurationSecs)

	// Update streak, FP, and achievements
	newStreak, newBadges, _ := h.userStore.UpdateActivity(userID, session.Language, fp)

	// Fetch updated total FP
	totalFP := 0
	if u, err := h.userStore.GetByID(userID); err == nil {
		totalFP = u.TotalFP
	}

	// Save conversation record
	record := &store.ConversationRecord{
		ID:           uuid.New().String(),
		UserID:       userID,
		SessionID:    session.ID,
		Language:     session.Language,
		Topic:        session.Topic,
		TopicName:    topicName,
		Level:        session.Level,
		Personality:  session.Personality,
		MessageCount: len(msgs),
		DurationSecs: req.DurationSecs,
		FPEarned:     fp,
		Summary:      summaryResult.Summary,
		Topics:       summaryResult.Topics,
		Vocabulary:   summaryResult.Vocabulary,
		Corrections:  summaryResult.Corrections,
		Suggestions:  summaryResult.Suggestions,
		CreatedAt:    session.CreatedAt,
		EndedAt:      time.Now(),
	}
	h.historyStore.Save(record)

	// Also save session context for future conversations
	if updated, err := h.sessionStore.Get(req.SessionID); err == nil {
		h.contextStore.Save(session.UserID, session.Language, session.Level, updated.Messages)
	}

	if newBadges == nil {
		newBadges = []string{}
	}

	writeJSON(w, http.StatusOK, endResponse{
		RecordID:        record.ID,
		FPEarned:        fp,
		NewStreak:       newStreak,
		NewAchievements: newBadges,
		TotalFP:         totalFP,
		Summary:         summaryResult.Summary,
		Topics:          summaryResult.Topics,
		Vocabulary:      summaryResult.Vocabulary,
		Corrections:     summaryResult.Corrections,
		Suggestions:     summaryResult.Suggestions,
		Language:        session.Language,
		Topic:           session.Topic,
		TopicName:       topicName,
		Level:           session.Level,
		Personality:     session.Personality,
		MessageCount:    len(msgs),
		DurationSecs:    req.DurationSecs,
	})
}

// summaryResult holds the parsed AI summary.
type summaryResult struct {
	Summary     string   `json:"summary"`
	Topics      []string `json:"topics_discussed"`
	Vocabulary  []string `json:"vocabulary_learned"`
	Corrections []string `json:"grammar_corrections"`
	Suggestions []string `json:"suggested_next_lessons"`
}

func (h *ConversationHandler) generateSummary(ctx context.Context, language string, level int, topicName string, msgs []store.Message, durationSecs int) summaryResult {
	fallback := summaryResult{
		Summary:     fmt.Sprintf("Great practice session in %s! Keep it up.", LanguageName(language)),
		Suggestions: []string{"Keep practicing to build fluency!", "Review any vocabulary from today's session."},
	}

	if len(msgs) < 2 {
		return fallback
	}

	// Build transcript from last 20 messages
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
	langName := LanguageName(language)

	prompt := fmt.Sprintf(`You are a language learning analytics assistant. Analyze this %s conversation session and return a JSON summary. Return ONLY valid JSON with no markdown or extra text.

{
  "summary": "2-3 sentence overview of what the student practiced",
  "topics_discussed": ["main topic or theme"],
  "vocabulary_learned": ["word: meaning"],
  "grammar_corrections": ["notable correction if any"],
  "suggested_next_lessons": ["specific next step recommendation"]
}

Student level: %s (%d/5)
Topic: %s
Duration: %s
Messages exchanged: %d

Conversation transcript:
%s`,
		langName, levelName, level, topicName, durationStr, len(msgs), transcript.String(),
	)

	payload := ionosPayload{
		Model: h.cfg.IONOSModel,
		Messages: []store.Message{
			{Role: "user", Content: prompt},
		},
		Stream:      false,
		MaxTokens:   512,
		Temperature: 0.3,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fallback
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.cfg.IONOSBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fallback
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.cfg.IONOSAPIKey)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
		return fallback
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	// Strip markdown code fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var sr summaryResult
	if err := json.Unmarshal([]byte(content), &sr); err != nil {
		log.Printf("summary parse error: %v — raw: %s", err, content)
		return summaryResult{
			Summary:     content,
			Suggestions: []string{"Keep practicing " + langName + " regularly!"},
		}
	}

	return sr
}

// ── Message (streaming) ───────────────────────────────────────────────────────

type messageRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	Greet     bool   `json:"greet"`
}

// ionosPayload matches the OpenAI-compatible request body
type ionosPayload struct {
	Model       string          `json:"model"`
	Messages    []store.Message `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
}

func (h *ConversationHandler) Message(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req messageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
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

	messages := make([]store.Message, len(session.Messages))
	copy(messages, session.Messages)

	var userContent string
	if req.Greet {
		userContent = buildGreetPrompt(session.Language, session.Level, session.Topic)
		messages = append(messages, store.Message{Role: "user", Content: userContent})
	} else {
		if strings.TrimSpace(req.Message) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message cannot be empty"})
			return
		}
		userContent = req.Message
		userMsg := store.Message{Role: "user", Content: userContent}
		_ = h.sessionStore.AddMessage(req.SessionID, userMsg)
		messages = append(messages, userMsg)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	payload := ionosPayload{
		Model:       h.cfg.IONOSModel,
		Messages:    messages,
		Stream:      true,
		MaxTokens:   4096,
		Temperature: 0.75,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"internal error\"}\n\n")
		flusher.Flush()
		return
	}

	ionosReq, err := http.NewRequestWithContext(r.Context(), "POST",
		h.cfg.IONOSBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"failed to reach AI service\"}\n\n")
		flusher.Flush()
		return
	}
	ionosReq.Header.Set("Content-Type", "application/json")
	ionosReq.Header.Set("Authorization", "Bearer "+h.cfg.IONOSAPIKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(ionosReq)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"AI service unavailable\"}\n\n")
		flusher.Flush()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("IONOS error %d: %s", resp.StatusCode, string(respBody))
		fmt.Fprintf(w, "data: {\"error\":\"AI service error\"}\n\n")
		flusher.Flush()
		return
	}

	var fullResponse strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		content := chunk.Choices[0].Delta.Content
		if content == "" {
			continue
		}
		fullResponse.WriteString(content)

		chunkJSON, _ := json.Marshal(map[string]string{"content": content})
		fmt.Fprintf(w, "data: %s\n\n", chunkJSON)
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("IONOS scanner error (session %s): %v", req.SessionID, err)
	}
	if fullResponse.Len() == 0 {
		log.Printf("IONOS empty response (session %s, level %d, lang %s)", req.SessionID, session.Level, session.Language)
	}

	if fullResponse.Len() > 0 {
		_ = h.sessionStore.AddMessage(req.SessionID, store.Message{
			Role:    "assistant",
			Content: fullResponse.String(),
		})
		if updated, err := h.sessionStore.Get(req.SessionID); err == nil {
			h.contextStore.Save(session.UserID, session.Language, session.Level, updated.Messages)
		}
	}

	fmt.Fprintf(w, "data: {\"done\":true}\n\n")
	flusher.Flush()
}

// ── Translate ─────────────────────────────────────────────────────────────────

type translateRequest struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

func (h *ConversationHandler) Translate(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text cannot be empty"})
		return
	}

	langName := LanguageName(req.Language)
	prompt := fmt.Sprintf(
		"Translate the following %s text to English. Respond with ONLY the translation — no explanations, no quotation marks, no additional commentary:\n\n%s",
		langName, req.Text,
	)

	payload := ionosPayload{
		Model: h.cfg.IONOSModel,
		Messages: []store.Message{
			{Role: "user", Content: prompt},
		},
		Stream:      false,
		MaxTokens:   512,
		Temperature: 0.1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	ionosReq, err := http.NewRequestWithContext(r.Context(), "POST",
		h.cfg.IONOSBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reach AI service"})
		return
	}
	ionosReq.Header.Set("Content-Type", "application/json")
	ionosReq.Header.Set("Authorization", "Bearer "+h.cfg.IONOSAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(ionosReq)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AI service unavailable"})
		return
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Choices) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse translation"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"translation": strings.TrimSpace(result.Choices[0].Message.Content),
	})
}

// ── History ───────────────────────────────────────────────────────────────────

func (h *ConversationHandler) History(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	sessionID := chi.URLParam(r, "sessionId")

	session, err := h.sessionStore.Get(sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if session.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	msgs, _ := h.sessionStore.GetMessages(sessionID)
	topicName, _ := TopicDetails(session.Topic)

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": session.ID,
		"language":   session.Language,
		"topic":      session.Topic,
		"topic_name": topicName,
		"level":      session.Level,
		"messages":   msgs,
	})
}

// ── System prompt builder ─────────────────────────────────────────────────────

func levelProfile(level int) string {
	switch level {

	case 1:
		return `Student level: Beginner (1/5) — Structured & Instructor-Led.
Conversation is short and highly guided. Speak primarily in English (about 85%) with only one short target-language word or phrase per turn, including a quick pronunciation hint.
Each turn: introduce one word or short phrase, model it in a simple sentence, then ask the student to try it. Correct immediately: brief praise, corrected form, one short reason. Keep language simple and controlled.`

	case 2:
		return `Student level: Elementary (2/5) — Guided Conversation.
Use about 60% target language and 40% English support. Weave in one useful vocabulary word naturally per turn (word = English meaning).
Use short situational prompts instead of scripts. After every 2–3 exchanges, insert one brief correction note (1 correction + short reason), then continue the conversation.`

	case 3:
		return `Student level: Intermediate (3/5) — Balanced Real Conversation.
Speak primarily in the target language with minimal English clarifications in brackets only when needed.
Allow natural back-and-forth for 4–5 turns without interrupting minor mistakes. Then give one short coaching block: 1–2 corrections (with very short reasons), one vocabulary upgrade, and one fluency tip. Resume conversation immediately.`

	case 4:
		return `Student level: Advanced (4/5) — Conversation First, Coaching Second.
Speak entirely in the target language. Do not preview vocabulary.
Let minor mistakes pass; only interrupt if meaning breaks down. After 6–8 turns, give a very brief performance review: top 1–2 corrections, one vocabulary refinement, one fluency suggestion. Keep conversation natural, nuanced, and opinion-based.`

	case 5:
		return `Student level: Fluent (5/5) — Full Immersion.
Speak entirely in the target language at natural native flow. No structured teaching.
Only correct if communication fails. After 8–10 turns, offer one subtle refinement (tone, idiom, cultural nuance), then immediately continue the conversation. Treat the student as an equal conversational partner.`

	default:
		return levelProfile(3)
	}
}

func personalityPrompt(personality string) string {
	switch personality {
	case "professor":
		return "TUTOR PERSONALITY: You are an academic Professor — formal, structured, and precise. Address the student respectfully, use formal vocabulary, and provide clear grammar explanations when correcting mistakes."
	case "friendly-partner":
		return "TUTOR PERSONALITY: You are a casual language exchange friend — warm, encouraging, and relaxed. Use informal language, share enthusiasm for the topic, and keep corrections light and positive."
	case "bartender":
		return "TUTOR PERSONALITY: You are a local bartender — laid-back, witty, and authentic. Use everyday expressions and slang naturally. Keep the vibe casual and fun, like chatting at the bar."
	case "business-executive":
		return "TUTOR PERSONALITY: You are a senior business executive — professional, direct, and formal. Focus on business vocabulary, formal register, and professional communication standards."
	case "travel-guide":
		return "TUTOR PERSONALITY: You are an enthusiastic travel guide — passionate, culturally rich, and storytelling. Weave in local culture, colorful expressions, and travel anecdotes naturally."
	default:
		return ""
	}
}

func buildSystemPrompt(langCode string, level int, topicName, topicDesc, topicID, personality string, hasPriorContext bool) string {
	lang := LanguageName(langCode)

	if strings.HasPrefix(topicID, "grammar-") {
		return buildGrammarSystemPrompt(lang, level, topicName, topicID, hasPriorContext)
	}
	if strings.HasPrefix(topicID, "cultural-") {
		return buildCulturalSystemPrompt(lang, level, topicName, topicID, hasPriorContext)
	}
	if strings.HasPrefix(topicID, "immersion-") {
		return buildImmersionSystemPrompt(lang, topicName, topicID, hasPriorContext)
	}

	personalityNote := ""
	if p := personalityPrompt(personality); p != "" {
		personalityNote = "\n\n" + p
	}

	contextNote := ""
	if hasPriorContext {
		contextNote = "\n\nNote: The conversation history below contains messages from this student's recent previous sessions. Use it to naturally acknowledge progress, avoid repeating vocabulary already mastered, and build on prior topics. Always begin this session with a warm but brief fresh greeting."
	}

	scenePreamble := ""
	if strings.HasPrefix(topicID, "role-") {
		scenePreamble = "\n\n" + rolePlayPrompt(topicID, lang)
	} else if strings.HasPrefix(topicID, "travel-") {
		scenePreamble = "\n\n" + travelPrompt(topicID, lang)
	}

	return fmt.Sprintf(`You are an expert 1-on-1 conversational language tutor specializing in %s. Your mission is to help the student actively practice %s through real conversation about "%s".

Topic context: %s

%s

FORMATTING RULE — MANDATORY:
Write in plain, natural prose only. No markdown whatsoever — no asterisks, no bold, no italics, no bullet points, no headers, no numbered lists. Write exactly as you would speak out loud to a student sitting across from you.

RESPONSE LENGTH RULE — MANDATORY:
Every reply must be 2 sentences. 3 sentences maximum. Never exceed this. Keep responses conversational and natural, not instructional lectures.

CONVERSATION RULES:
- Always end with one short follow-up question that keeps the conversation moving naturally.
- Teach through conversation, not explanation. Ask about the student's real life and opinions.
- When the student makes a mistake: briefly correct it inline (e.g., "— great, and we say 'fui' not 'iba' there —") then immediately continue the conversation. Never stop to lecture.
- No long grammar explanations. One quick correction note per mistake, woven naturally into your reply.
- Warm, encouraging tone — but concise.%s%s%s`,
		lang, lang, topicName, topicDesc, levelProfile(level), personalityNote, contextNote, scenePreamble)
}

func buildGrammarSystemPrompt(lang string, level int, topicName, topicID string, hasPriorContext bool) string {
	levelLabels := map[int]string{1: "Beginner", 2: "Elementary", 3: "Intermediate", 4: "Advanced", 5: "Fluent"}
	lvl := levelLabels[level]
	if lvl == "" {
		lvl = "Intermediate"
	}

	contextNote := ""
	if hasPriorContext {
		contextNote = "\n\nNote: The student has prior session history. Acknowledge any vocabulary or grammar previously practiced and avoid unnecessary repetition."
	}

	var exercises string
	switch topicID {
	case "grammar-vocabulary":
		exercises = fmt.Sprintf(`VOCABULARY BUILDER — %s %s learner.

Teach 1 new word per turn in a natural, conversational way — not as a formatted list or card. Here is how each turn should flow, written as natural speech:

Introduce the word naturally: say its name in %s, give the English meaning in parentheses right after it, then pronounce it in plain text with the stressed syllable in capitals (e.g., "Say it like this: KAH-sah"), then use it in one natural real-life sentence.

Then immediately give one short quiz exercise: either a fill-in-the-blank ("Complete this: Vivo en una _____ grande.") or ask them to translate a short English phrase using the word. Write the quiz as a plain sentence, not a formatted block.

After their answer, give a brief warm response — confirm what was right or gently correct — then move on to the next word.

Aim for 8–12 words across the full session. Choose everyday vocabulary that a %s learner will actually use.`, lvl, lang, lang)

	case "grammar-sentences":
		exercises = fmt.Sprintf(`SENTENCE CONSTRUCTION — %s %s learner.

Present each exercise in natural, conversational prose — not as numbered lists or formatted blocks. Here is how each turn should flow as natural speech:

Name the grammar pattern you are working on in one sentence (e.g., "Let's work on reflexive verbs"). Show one clear model sentence in %s with an English translation right after it in parentheses. Then give 2 exercises written as plain sentences: for a scramble say something like "Can you put these words in order: voy / hoy / al / supermercado?" and for fill-in-the-blank say "Complete this sentence: Ella _____ aprender."

After each answer, confirm or correct with one short natural sentence explaining the rule. After 2–3 successful exercises, move to the next pattern.

Focus on patterns that genuinely challenge English speakers learning %s. Always use natural, real-life sentences.`, lvl, lang, lang, lang)

	case "grammar-pronunciation":
		exercises = fmt.Sprintf(`PRONUNCIATION PRACTICE — %s %s learner.

Present each word and its pronunciation guide in natural, conversational prose — not as labeled blocks or bullet lists. Here is how each turn should flow as natural speech:

Introduce the word naturally: say "Today's word is [word]" and then give the phonetic breakdown in plain text with the stressed syllable in capitals (e.g., "Say it like this: res-tau-RAN-te — four syllables, stress on the third"). Then describe the trickiest sound compared to English in one plain sentence (e.g., "The double r in perro requires your tongue tip to vibrate, which has no equivalent in English"). Give one short physical placement tip. Then ask the student to type the word and explain how they would pronounce it, or use it in a short sentence.

After their response, confirm or gently correct with a specific note on what to adjust. After every 5 words, give a short recall drill by listing the session's words as a plain sentence: "Can you say all of these: [list]?"

Focus on sounds that genuinely challenge %s learners — rolled r's, nasal sounds, silent letters, tricky stress patterns.`, lvl, lang, lang)

	case "grammar-listening":
		exercises = fmt.Sprintf(`LISTENING COMPREHENSION — %s %s learner.

Present each passage and its questions in plain, natural prose — not as labeled blocks. Here is how each turn should flow as natural speech:

Begin by saying something like "Read this carefully:" and then write a short passage in %s (2–4 sentences, a natural everyday scenario appropriate for %s level — someone at the market, a phone call, two friends making plans). Then naturally ask two comprehension questions as plain sentences. For Beginner or Elementary levels, add a brief English hint in parentheses after each question.

After they answer, warmly acknowledge what they got right and gently correct any errors. Then explain one vocabulary word or grammar point from the passage in a single natural sentence before moving on to the next passage.

Gradually increase the complexity of passages across the session.`, lvl, lang, lang, lvl)

	case "grammar-writing":
		exercises = fmt.Sprintf(`WRITING COACH — %s %s learner.

Give all feedback in natural, conversational prose — not as emoji sections or formatted blocks. Here is how each turn should flow as natural speech:

Start by giving a clear, motivating writing prompt and asking the student to write 3–5 sentences in %s. When they submit their writing, give feedback as a flowing response: first acknowledge what they did well in one sentence, then point out the 1–2 most important corrections with a short plain explanation of why ("You wrote 'yo soy cansado' but with states that change, we use estar, so it would be 'yo estoy cansado'"), then suggest one vocabulary upgrade or more natural phrasing. End by asking them to rewrite one of the corrected sentences to reinforce it, then give a new prompt.

Focus only on the 1–2 most important errors. Be constructive, specific, and warm — never overwhelming.`, lvl, lang, lang)

	default:
		exercises = fmt.Sprintf(`GRAMMAR & SKILLS SESSION — %s %s learner. Run structured language exercises. Be clear, encouraging, and systematic. One exercise block per turn.`, lvl, lang)
	}

	return fmt.Sprintf(`You are an expert %s language teacher running a structured "%s" session.

%s

GENERAL RULES:
- Write in plain, natural prose only. No markdown — no asterisks, no bold, no italics, no bullet points, no headers, no numbered lists. Write exactly as you would speak to a student out loud.
- Follow the exercise format above precisely every turn, but express it in flowing sentences, not formatted lists.
- Be encouraging and specific — point to exactly what is right and wrong.
- Match difficulty to the student's %s level. Never skip ahead or lag behind.
- One exercise block per turn. Do not rush multiple topics into a single message.
- Always end each turn with a clear next step or question.%s`,
		lang, topicName, exercises, lvl, contextNote)
}

func rolePlayPrompt(topicID, lang string) string {
	scenes := map[string]string{
		"role-restaurant":   fmt.Sprintf("ROLE-PLAY SCENE: You are a waiter at a busy %s restaurant. The student is a customer who just sat down and opened the menu. Stay fully in character as the waiter throughout the conversation. Open by welcoming them warmly and asking if they have any questions about the menu.", lang),
		"role-job-interview": fmt.Sprintf("ROLE-PLAY SCENE: You are a professional interviewer at a %s-speaking company. The student is the job candidate. Stay in character as the interviewer throughout. Open by welcoming them to the interview and asking them to introduce themselves.", lang),
		"role-airport":      fmt.Sprintf("ROLE-PLAY SCENE: You are an airline check-in agent at a %s-speaking airport. The student is a traveler arriving to check in for their flight. Stay in character as the agent throughout. Open by greeting them and asking for their passport or booking reference.", lang),
		"role-doctor":       fmt.Sprintf("ROLE-PLAY SCENE: You are a doctor at a %s-speaking clinic. The student is the patient. Stay in character as the doctor throughout. Open by greeting them and asking what brings them in today.", lang),
		"role-business":     fmt.Sprintf("ROLE-PLAY SCENE: You are a senior executive at a %s-speaking company in a business meeting. The student is your colleague or client. Stay in character as the executive throughout. Open by welcoming them to the meeting and setting the agenda.", lang),
		"role-apartment":    fmt.Sprintf("ROLE-PLAY SCENE: You are a landlord showing an apartment in a %s-speaking city. The student is a potential tenant viewing the property. Stay in character as the landlord throughout. Open by welcoming them and beginning to show them around.", lang),
		"role-directions":   fmt.Sprintf("ROLE-PLAY SCENE: You are a friendly local resident in a %s-speaking city. The student is a tourist who has just approached you to ask for directions. Stay in character as the local throughout. Wait for them to ask their first question.", lang),
	}
	if scene, ok := scenes[topicID]; ok {
		return scene
	}
	return fmt.Sprintf("ROLE-PLAY SCENE: You are playing a character in a real-life %s scenario. Stay fully in character throughout the conversation.", lang)
}

func travelPrompt(topicID, lang string) string {
	scenes := map[string]string{
		"travel-rome":      "TRAVEL IMMERSION: You are a friendly Roman local. The student has just arrived in Rome. Help them navigate the city, recommend authentic food spots, local sights, and everyday Roman life — all through natural conversation. Stay in character as a Roman local throughout.",
		"travel-barcelona": "TRAVEL IMMERSION: You are a local Barcelonan. The student has just arrived in Barcelona. Help them discover tapas bars, the beach, Gaudí architecture, and navigate the city through natural conversation. Stay in character as a local throughout.",
		"travel-paris":     "TRAVEL IMMERSION: You are a Parisian local. The student has just arrived in Paris. Help them experience authentic café culture, museum tips, and everyday Parisian life through natural conversation. Stay in character as a local throughout.",
		"travel-tokyo":     "TRAVEL IMMERSION: You are a Tokyo local. The student has just arrived in Tokyo. Help them navigate the subway, find great restaurants, and understand cultural etiquette through natural conversation. Stay in character as a local throughout.",
		"travel-lisbon":    "TRAVEL IMMERSION: You are a Lisbon local. The student has just arrived in Lisbon. Help them explore the historic neighborhoods, iconic trams, and traditional cuisine through natural conversation. Stay in character as a local throughout.",
	}
	if scene, ok := scenes[topicID]; ok {
		return scene
	}
	return fmt.Sprintf("TRAVEL IMMERSION: You are a friendly local in a %s-speaking destination. The student has just arrived. Help them navigate the city and culture through natural conversation. Stay in character as a local throughout.", lang)
}

func buildGreetPrompt(langCode string, level int, topicID string) string {
	lang := LanguageName(langCode)

	if strings.HasPrefix(topicID, "grammar-") {
		return buildGrammarGreet(topicID, lang, level)
	}
	if strings.HasPrefix(topicID, "cultural-") {
		return buildCulturalGreet(topicID, lang, level)
	}
	if strings.HasPrefix(topicID, "immersion-") {
		return buildImmersionGreet(topicID, lang)
	}

	switch level {

	case 1:
		return fmt.Sprintf(
			"[Open warmly in English. Introduce the topic briefly, teach one simple %s word or short phrase with pronunciation guidance, use it in a model sentence, then ask the student to try it. Keep it structured and beginner-safe. 2–3 sentences only.]",
			lang,
		)

	case 2:
		return fmt.Sprintf(
			"[Greet using a mix of %s and English. Introduce 1–2 useful words naturally (word = English meaning), then set up a simple real-life scenario related to the topic and give the student their first short situational prompt. Keep it conversational, not instructional.]",
			lang,
		)

	case 3:
		return fmt.Sprintf(
			"[Begin naturally in %s. Set the conversational context in one sentence and ask an engaging open-ended question that invites a real response. No vocabulary preview — let the conversation flow.]",
			lang,
		)

	case 4:
		return fmt.Sprintf(
			"[Start immediately in %s with a natural, opinion-based or reflective question about the topic. No greeting ritual, no vocabulary preview — make it feel like a real ongoing conversation.]",
			lang,
		)

	case 5:
		return fmt.Sprintf(
			"[Begin entirely in %s at full natural tone. Open with a thought-provoking opinion, cultural reference, or hypothetical related to the topic that invites genuine engagement between equals.]",
			lang,
		)

	default:
		return buildGreetPrompt(langCode, 3, topicID)
	}
}

func buildGrammarGreet(topicID, lang string, level int) string {
	levelNames := map[int]string{1: "beginner", 2: "elementary", 3: "intermediate", 4: "advanced", 5: "fluent"}
	lvl := levelNames[level]
	if lvl == "" {
		lvl = "intermediate"
	}

	switch topicID {
	case "grammar-vocabulary":
		return fmt.Sprintf(
			"[Start the vocabulary session. Greet the student warmly in 1 sentence, then immediately present the first vocabulary word using the exact Palabra / Significa / Pronunciación / Ejemplo card format. Choose an everyday word suited for a %s %s learner. Then give 1 quiz exercise on that word right away.]",
			lvl, lang,
		)
	case "grammar-sentences":
		return fmt.Sprintf(
			"[Start the sentence construction session. Greet briefly in 1 sentence, name the first grammar pattern, show one clear model sentence in %s with English translation, then give the first exercise using the exact 'Ordena las palabras: word1 / word2 / word3' scramble format or a fill-in-the-blank. Appropriate for a %s learner.]",
			lang, lvl,
		)
	case "grammar-pronunciation":
		return fmt.Sprintf(
			"[Start the pronunciation session. Greet in 1 sentence, then introduce the first word using the exact 'La palabra es: \"[word]\"' format, followed by phonetic breakdown, syllable count, and an English comparison for the trickiest sound. Choose a genuinely challenging sound for a %s learner of %s — like a rolled r, nasal vowel, or silent letter.]",
			lvl, lang,
		)
	case "grammar-listening":
		return fmt.Sprintf(
			"[Start the listening session. Greet briefly and explain the format in one sentence (student reads a passage and answers questions), then immediately present the first passage using the exact Escucha: / Pregunta 1: / Pregunta 2: format. Choose a simple, natural everyday scenario in %s appropriate for a %s learner.]",
			lang, lvl,
		)
	case "grammar-writing":
		return fmt.Sprintf(
			"[Start the writing coach session. Greet in 1 sentence, explain the format briefly, then give the first writing prompt for a %s %s learner. Make the prompt clear and motivating.]",
			lvl, lang,
		)
	default:
		return fmt.Sprintf(
			"[Start the grammar exercise session. Greet the student and begin the first exercise appropriate for a %s %s learner.]",
			lvl, lang,
		)
	}
}

func buildCulturalGreet(topicID, lang string, level int) string {
	levelNames := map[int]string{1: "beginner", 2: "elementary", 3: "intermediate", 4: "advanced", 5: "fluent"}
	lvl := levelNames[level]
	if lvl == "" {
		lvl = "intermediate"
	}

	switch topicID {
	case "cultural-context":
		return fmt.Sprintf(
			"[Start the cultural lesson. Greet warmly in 1 sentence, then introduce the first cultural topic — pick a social norm, unwritten rule, or everyday custom from %s culture that surprises English speakers. Give 2-3 sentences of rich cultural context, then ask an engaging question to start the discussion. Appropriate for a %s learner.]",
			lang, lvl,
		)
	case "cultural-stories":
		return fmt.Sprintf(
			"[Start the interactive story. Skip a long greeting — just one warm sentence, then immediately set the opening scene in %s (3–4 sentences, %s level, vivid authentic location). End with a branching moment: the student's character is spoken to or faces a choice. Make it feel like they're stepping into a real place. Their first response will shape what happens next.]",
			lang, lvl,
		)
	case "cultural-idioms":
		return fmt.Sprintf(
			"[Start the idioms session. Greet in 1 sentence, then introduce the first idiom using the exact 'La frase es: \"[expression]\"' format followed by Literal / Significa / Origen, and 2 example sentences. Ask the student to try using it. Choose an idiom that's genuinely common in everyday %s and appropriate for a %s learner.]",
			lang, lvl,
		)
	case "cultural-food":
		return fmt.Sprintf(
			"[Start the food culture session. Greet warmly, then introduce the first food culture topic — pick something vivid and specific from %s food culture (a meal tradition, regional dish, dining custom, or market culture). Give 2-3 sentences of enthusiastic cultural detail, introduce 2 vocabulary words, and invite the student to ask or share. Appropriate for a %s learner.]",
			lang, lvl,
		)
	case "cultural-history":
		return fmt.Sprintf(
			"[Start the history and traditions session. Greet in 1 sentence, then introduce the first cultural topic — pick a festival, historical event, or regional tradition from %s culture. Give 2-3 sentences of context including why it matters today, share one surprising fact, teach one relevant vocabulary word, and ask an engaging discussion question. Appropriate for a %s learner.]",
			lang, lvl,
		)
	default:
		return fmt.Sprintf(
			"[Start the cultural learning session. Greet the student warmly and begin with an interesting cultural insight about %s culture appropriate for a %s learner.]",
			lang, lvl,
		)
	}
}

func buildCulturalSystemPrompt(lang string, level int, topicName, topicID string, hasPriorContext bool) string {
	levelLabels := map[int]string{1: "Beginner", 2: "Elementary", 3: "Intermediate", 4: "Advanced", 5: "Fluent"}
	lvl := levelLabels[level]
	if lvl == "" {
		lvl = "Intermediate"
	}

	contextNote := ""
	if hasPriorContext {
		contextNote = "\n\nNote: The student has prior session history. Avoid repeating cultural topics already covered. Build on what they know."
	}

	var guide string
	switch topicID {
	case "cultural-context":
		guide = fmt.Sprintf(`CULTURAL CONTEXT LESSONS — %s level %s learner.

Your approach each turn:
1. Introduce one specific cultural topic: a social norm, unwritten rule, etiquette point, or everyday custom from %s culture.
2. Explain it in 2-3 sentences with authentic detail. Briefly contrast with typical English-speaking culture where relevant.
3. Teach one natural phrase or expression tied to this cultural point.
4. Ask the student a reflective or personal question to invite discussion.

Cover a wide range of topics: greetings, personal space, punctuality, gift-giving, family roles, work culture, social hierarchies, taboos, gestures. Make it feel like an enlightening conversation, not a lecture.`, lvl, lang, lang)

	case "cultural-stories":
		guide = fmt.Sprintf(`STORY-BASED INTERACTIVE LEARNING — %s level %s learner.

You are an interactive storyteller. The student's responses directly shape how the story unfolds — this is a choose-your-path narrative.

Your format each turn:
1. Set or continue the scene in %s (3–5 sentences, level-appropriate). Use vivid, authentic cultural settings: markets, train stations, family dinners, plazas, workplaces.
2. End the scene with a branching moment: the character (the student) faces a choice or is spoken to. Example: "El camarero te pregunta: '¿Qué desea tomar?'" or "Ves dos calles. ¿A la derecha, al museo? ¿O a la izquierda, al mercado?"
3. After the student responds, continue the story based on what they said — their answer genuinely changes what happens next.
4. After every 2–3 exchanges, briefly highlight one cultural detail from the scene or introduce 1 vocabulary word that appeared naturally.

Keep the story moving and the student in the role of the main character. Build narrative momentum — their choices matter. Stories should reveal culture through experience, not explanation.`, lvl, lang, lang)

	case "cultural-idioms":
		guide = fmt.Sprintf(`IDIOMS & EXPRESSIONS COACH — %s level %s learner.

Introduce one idiom per turn in natural, conversational prose — not as a formatted list. Here is how each turn should flow as natural speech:

Introduce the phrase naturally: say "The expression is [phrase in %s]" and immediately explain what it literally means word-for-word, then explain what it actually means in real life. Follow with the cultural backstory in 1–2 sentences — why this phrase exists, what it reveals about how %s speakers think or see the world. Then give 2 short natural sentences showing it used in real conversation. Finally, ask the student to try using it in their own sentence.

After their attempt, give warm specific feedback on whether they used it correctly, and then move to the next idiom.

Choose idioms that are genuinely common in everyday %s. Prioritize expressions that reveal something interesting about the culture, sense of humor, or values of %s speakers.`, lvl, lang, lang, lang, lang, lang)

	case "cultural-food":
		guide = fmt.Sprintf(`FOOD & CUISINE CULTURE GUIDE — %s level %s learner.

You are an enthusiastic %s food culture guide. Your approach each turn:
1. Introduce one aspect of %s food culture: a regional dish, meal tradition, market custom, dining etiquette rule, street food culture, or food-related social norm.
2. Share 2-3 sentences of rich, authentic cultural detail — regional variations, history, social context, why it matters.
3. Introduce 2-3 key vocabulary words related to the topic (with English translations).
4. Invite the student to ask questions or share their own experience with food.

Cover the full richness of food culture: not just dishes, but when people eat, how meals are structured, what food means socially, regional pride, market culture, and food-related expressions.`, lvl, lang, lang, lang)

	case "cultural-history":
		guide = fmt.Sprintf(`HISTORY & TRADITIONS GUIDE — %s level %s learner.

You are a knowledgeable cultural historian specializing in %s-speaking cultures. Your approach each turn:
1. Introduce one festival, historical event, cultural tradition, or regional practice.
2. Explain its significance in 2-3 sentences — why it matters, how it's celebrated or observed today.
3. Share one surprising or little-known fact about it.
4. Teach one relevant vocabulary word or phrase tied to this tradition (with pronunciation if helpful).
5. Ask an engaging question to invite personal reflection or discussion.

Cover a wide range: national holidays, local festivals, historical milestones, seasonal traditions, regional customs, and the stories behind everyday cultural practices.`, lvl, lang, lang)

	default:
		guide = fmt.Sprintf(`CULTURAL LANGUAGE LEARNING — %s level %s learner.
Explore the rich culture behind the %s language. Each turn: share one cultural insight, teach relevant vocabulary, and invite discussion.`, lvl, lang, lang)
	}

	return fmt.Sprintf(`You are an expert %s cultural guide and language teacher running a "%s" session.

%s

GENERAL RULES:
- Write in plain, natural prose only. No markdown — no asterisks, no bold, no italics, no bullet points, no headers, no numbered lists. Write exactly as you would speak to a student out loud.
- Be warm, enthusiastic, and culturally rich. Share genuine knowledge, not surface-level facts.
- Adapt language complexity to the student's %s level. Use %s naturally in examples and prompts.
- Balance cultural teaching with active language practice every turn.
- Keep responses focused: 3-4 sentences of content per turn maximum, then engage the student.
- Always end with a question or invitation to respond.%s`,
		lang, topicName, guide, lvl, lang, contextNote)
}

func buildImmersionGreet(topicID, lang string) string {
	scenes := map[string]string{
		"immersion-daily":  fmt.Sprintf("[Open entirely in %s. Begin mid-scene — you're already in the middle of a daily life moment (e.g., bumping into someone at the market, asking a neighbour for help). No English, no introduction, no explanation. Just start the scene naturally.]", lang),
		"immersion-social": fmt.Sprintf("[Open entirely in %s. You're already at a social gathering — warmly greet the student as you would a new person at a dinner party or get-together. Introduce yourself, make small talk. No English at all.]", lang),
		"immersion-work":   fmt.Sprintf("[Open entirely in %s. You're a colleague in a %s-speaking workplace. Start naturally — perhaps arriving at the office, starting a meeting, or catching up in the break room. No English at all.]", lang, lang),
		"immersion-city":   fmt.Sprintf("[Open entirely in %s. You're a local in a %s-speaking city. The student has just approached you — perhaps to ask directions or find something. React naturally as a local would. No English at all.]", lang, lang),
		"immersion-media":  fmt.Sprintf("[Open entirely in %s. You're a friend who just watched a great film or listened to a new album. Start talking about it with enthusiasm, as you would with any native-speaking friend. No English at all.]", lang),
		"immersion-debate": fmt.Sprintf("[Open entirely in %s. Jump straight into an interesting topic — share a strong opinion on something current, cultural, or philosophical and invite the student's view. Speak as a native. No English at all.]", lang),
	}
	if scene, ok := scenes[topicID]; ok {
		return scene
	}
	return fmt.Sprintf("[Begin immediately and entirely in %s. No English, no introduction — just start naturally as a native speaker would.]", lang)
}

func buildImmersionSystemPrompt(lang, topicName, topicID string, hasPriorContext bool) string {
	scenes := map[string]string{
		"immersion-daily":  fmt.Sprintf("SCENE: You are in everyday daily life with a %s speaker. Scenarios can include shopping, running errands, home life, asking for help, or any mundane real-world situation.", lang),
		"immersion-social": fmt.Sprintf("SCENE: You are at a casual social gathering — a dinner party, night out, or informal get-together with %s speakers. Be warm, funny, and social.", lang),
		"immersion-work":   fmt.Sprintf("SCENE: You are in a %s-speaking workplace. Conversations may include meetings, collaborating with colleagues, work updates, or break-room chat.", lang),
		"immersion-city":   fmt.Sprintf("SCENE: You are a local navigating a %s-speaking city. Help the student find places, use transit, explore neighbourhoods, and interact with the city.", lang),
		"immersion-media":  fmt.Sprintf("SCENE: You are discussing films, TV shows, music, books, and pop culture entirely in %s, as native-speaking friends would.", lang),
		"immersion-debate": fmt.Sprintf("SCENE: You are engaging in natural opinion-sharing and debate in %s — current events, ethics, culture, society. Take genuine positions, challenge the student's arguments politely but directly, and invite genuine discourse. If the student's meaning is unclear due to a language error, ask them to clarify (in %s only) rather than ignoring it.", lang, lang),
	}
	scene := scenes[topicID]
	if scene == "" {
		scene = fmt.Sprintf("SCENE: You are having a natural, native-level conversation in %s.", lang)
	}

	contextNote := ""
	if hasPriorContext {
		contextNote = "\n\nNote: The student has conversed with you before. Continue naturally from where things left off."
	}

	return fmt.Sprintf(`You are a native %s speaker in a full immersion conversation. The student has chosen to be completely immersed in %s.

%s

IMMERSION RULES — ABSOLUTE AND NON-NEGOTIABLE:
- Respond ONLY in %s. Never write a single word of English under any circumstances.
- Do not translate, explain grammar, or provide hints in any language other than %s.
- If a word or concept might be unfamiliar, explain it using only %s — describe, paraphrase, or give context within the target language itself. Never use English as a crutch.
- Do not correct grammar errors unless the student's meaning is completely unclear.
- Speak at a natural native pace and register — use contractions, colloquialisms, natural rhythm.
- If the student writes in English, do not acknowledge it. Simply continue the scene in %s as though they responded in the target language.
- Treat the student as a fully capable speaker who belongs in this conversation.
- Never break character or reference that this is a language learning exercise.

RESPONSE STYLE:
- Write in plain prose only. No markdown — no asterisks, no bold, no italics, no headers, no bullet points. Speak as a native would, not as a formatted document.
- 2–4 sentences per turn. Keep the conversation moving naturally.
- End each turn with a question, reaction, or natural conversational hook.
- Match the register of the scene: casual for social/daily, professional for work, lively for debate.%s`,
		lang, lang, scene, lang, lang, lang, contextNote)
}
