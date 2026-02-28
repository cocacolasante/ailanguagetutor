package handlers

import (
	"bufio"
	"bytes"
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
)

type ConversationHandler struct {
	cfg          *config.Config
	sessionStore *store.SessionStore
	contextStore *store.ContextStore
	userStore    *store.UserStore
}

func NewConversationHandler(cfg *config.Config, ss *store.SessionStore, cs *store.ContextStore, us *store.UserStore) *ConversationHandler {
	return &ConversationHandler{cfg: cfg, sessionStore: ss, contextStore: cs, userStore: us}
}

// ── Start ─────────────────────────────────────────────────────────────────────

type startRequest struct {
	Language string `json:"language"`
	Topic    string `json:"topic"`
	Level    int    `json:"level"`
}

type startResponse struct {
	SessionID string `json:"session_id"`
	Language  string `json:"language"`
	Topic     string `json:"topic"`
	TopicName string `json:"topic_name"`
	Level     int    `json:"level"`
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
		req.Level = 3 // default to intermediate
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

	// Load prior context for this user/language/level combo
	priorMsgs := h.contextStore.Get(userID, req.Language, req.Level)
	systemPrompt := buildSystemPrompt(req.Language, req.Level, topicName, topicDesc, len(priorMsgs) > 0)
	session := h.sessionStore.Create(userID, req.Language, req.Topic, req.Level, systemPrompt)

	// Inject prior session messages so the AI has continuity
	for _, m := range priorMsgs {
		_ = h.sessionStore.AddMessage(session.ID, m)
	}

	writeJSON(w, http.StatusCreated, startResponse{
		SessionID: session.ID,
		Language:  session.Language,
		Topic:     session.Topic,
		TopicName: topicName,
		Level:     session.Level,
	})
}

// ── Message (streaming) ───────────────────────────────────────────────────────

type messageRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	Greet     bool   `json:"greet"` // true on first turn to trigger an AI greeting
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

	// Build the messages list to send to IONOS
	messages := make([]store.Message, len(session.Messages))
	copy(messages, session.Messages)

	var userContent string
	if req.Greet {
		// Invisible trigger — not stored in history
		userContent = buildGreetPrompt(session.Language, session.Level)
		messages = append(messages, store.Message{Role: "user", Content: userContent})
	} else {
		if strings.TrimSpace(req.Message) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message cannot be empty"})
			return
		}
		userContent = req.Message
		// Persist user message
		userMsg := store.Message{Role: "user", Content: userContent}
		_ = h.sessionStore.AddMessage(req.SessionID, userMsg)
		messages = append(messages, userMsg)
	}

	// Set up SSE response
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

	// IONOS may use a reasoning model (e.g. gpt-oss-120b) that streams hundreds of
	// reasoning tokens before emitting any content. max_tokens must be large enough
	// to cover the full reasoning phase + the actual response. 4096 is safe for all
	// levels; the level profile instructions already constrain response length.
	maxTokens := 4096

	payload := ionosPayload{
		Model:       h.cfg.IONOSModel,
		Messages:    messages,
		Stream:      true,
		MaxTokens:   maxTokens,
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

	// Stream chunks to client.
	// IONOS may use a reasoning model that streams reasoning tokens in
	// delta.reasoning / delta.reasoning_content before emitting delta.content.
	// We skip reasoning chunks and only forward content to the browser.
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

		// Forward chunk to browser
		chunkJSON, _ := json.Marshal(map[string]string{"content": content})
		fmt.Fprintf(w, "data: %s\n\n", chunkJSON)
		flusher.Flush()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("IONOS scanner error (session %s): %v", req.SessionID, err)
	}
	if fullResponse.Len() == 0 {
		log.Printf("IONOS empty response (session %s, level %d, lang %s) — model may need higher max_tokens", req.SessionID, session.Level, session.Language)
	}

	// Persist assistant response
	if fullResponse.Len() > 0 {
		_ = h.sessionStore.AddMessage(req.SessionID, store.Message{
			Role:    "assistant",
			Content: fullResponse.String(),
		})
		// Save this session as the new context for (user, language, level)
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
		return `Student level: Beginner (1/5) — conversational language tutor.
LANGUAGE RULE — MANDATORY AND PERMANENT: Always speak mostly in English for the entire conversation, no matter what language the student uses. Do NOT switch to full target language just because the student responded in it. Your messages must be primarily English with only a few target-language words or one short phrase woven in per turn.
Each turn: introduce one target-language word or short phrase with a pronunciation hint, use it naturally in an English sentence, then ask the student to try it. Correct mistakes briefly: praise, correct form, one-line reason.`

	case 2:
		return `Student level: Elementary (2/5). Lead through prompts and scenarios.
Each turn: weave in one vocabulary word naturally (word = English meaning), give a situational prompt to respond to. After every 2–3 exchanges add one brief correction note. Write about 60% in the target language, 40% English. Translations in brackets after unfamiliar words.`

	case 3:
		return `Student level: Intermediate (3/5). Conversation partner who also coaches.
Have genuine exchanges and let minor errors slide for 4–5 turns. Then give one short coaching block: 1–2 grammar corrections with one-line reasons, one vocabulary upgrade, one fluency tip. Then continue naturally. Speak primarily in the target language; English only in brackets for quick clarifications.`

	case 4:
		return `Student level: Advanced (4/5). Natural conversation — no vocabulary preview.
Speak entirely in the target language. Let minor errors pass; only interrupt for errors that block understanding. After every 6–8 turns give a brief review: top 1–2 grammar corrections, one vocabulary upgrade, one fluency tip. Challenge the student with nuanced questions, hypotheticals, and opinions.`

	case 5:
		return `Student level: Fluent (5/5). Native-speaking peer, not a tutor.
Speak entirely in the target language at full native speed. Pursue opinions, debate, humor, storytelling, hypotheticals. Only correct when communication breaks down. After every 8–10 turns offer one brief tonal or idiomatic refinement, then resume immediately. No accommodations.`

	default:
		return levelProfile(3)
	}
}

func buildSystemPrompt(langCode string, level int, topicName, topicDesc string, hasPriorContext bool) string {
	lang := LanguageName(langCode)
	contextNote := ""
	if hasPriorContext {
		contextNote = "\n\nNote: The conversation history below contains messages from this student's recent previous sessions. Use it to remember what vocabulary and topics were already covered, acknowledge their progress naturally, and avoid re-teaching things they already know. Always open this new session with a warm, fresh greeting."
	}
	return fmt.Sprintf(`You are an expert language tutor specializing in %s. Your mission is to help the student practice %s through engaging conversation about "%s".

Topic context: %s

%s

RESPONSE LENGTH RULE — THIS IS MANDATORY: Every reply must be 2 sentences. 3 sentences absolute maximum. Never more. Do not explain, elaborate, or add extra context beyond those sentences. If you are tempted to write more, stop and cut it down.

Other guidelines:
- End each turn with one short question or prompt.
- Plain prose only. No bullet points except inside correction or review blocks.
- Acknowledge effort warmly but briefly.%s`,
		lang, lang, topicName, topicDesc, levelProfile(level), contextNote)
}

func buildGreetPrompt(langCode string, level int) string {
	lang := LanguageName(langCode)
	switch level {
	case 1:
		return fmt.Sprintf(
			"[Open in English. Welcome the student briefly, name the topic, introduce one %s word or short phrase with a pronunciation hint, and ask them to try it. Speak primarily in English — this is a beginner lesson taught in English with %s words woven in. 2–3 sentences only.]",
			lang, lang,
		)
	case 2:
		return fmt.Sprintf(
			"[Begin the session. Greet the student in a mix of %s and English. Drop in 2–3 useful vocabulary words for the topic (word = English meaning). Then set up a simple scenario related to the topic and give the student their first situational prompt to respond to.]",
			lang,
		)
	case 3:
		return fmt.Sprintf(
			"[Begin the session. Set the context naturally — describe the conversational scenario related to the topic in 1 sentence. Greet the student primarily in %s and open with an engaging question that invites a real response. Keep it brief, warm, and natural.]",
			lang,
		)
	case 4:
		return fmt.Sprintf(
			"[Begin the session. Jump straight into natural conversation in %s — no preamble. Ask an interesting, open-ended question related to the topic that requires a real opinion or thought, not a yes/no answer.]",
			lang,
		)
	case 5:
		return fmt.Sprintf(
			"[Begin the session entirely in %s. Start immediately — no greeting ritual. Open with something that invites real engagement: a bold opinion on the topic, a hypothetical, a cultural reference, or a question worth debating. Set the tone of a real conversation between equals.]",
			lang,
		)
	default:
		return buildGreetPrompt(langCode, 3)
	}
}
