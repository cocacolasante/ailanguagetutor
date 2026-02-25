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
}

func NewConversationHandler(cfg *config.Config, ss *store.SessionStore) *ConversationHandler {
	return &ConversationHandler{cfg: cfg, sessionStore: ss}
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

	topicName, topicDesc := TopicDetails(req.Topic)
	systemPrompt := buildSystemPrompt(req.Language, req.Level, topicName, topicDesc)
	session := h.sessionStore.Create(userID, req.Language, req.Topic, req.Level, systemPrompt)

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

	// Call IONOS
	payload := ionosPayload{
		Model:       h.cfg.IONOSModel,
		Messages:    messages,
		Stream:      true,
		MaxTokens:   512,
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

	// Stream chunks to client
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

	// Persist assistant response
	if fullResponse.Len() > 0 {
		_ = h.sessionStore.AddMessage(req.SessionID, store.Message{
			Role:    "assistant",
			Content: fullResponse.String(),
		})
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
		return `STUDENT LEVEL: Beginner (1/5)
- The student is brand new to this language. They may know almost nothing.
- Communicate primarily in English to ensure understanding. Introduce only 1–2 key phrases or vocabulary words in the target language per response, always accompanied by a clear English translation.
- Use extremely simple, short sentences when you do write in the target language.
- Praise every attempt enthusiastically — even a single correct word is worth celebrating.
- Never expect full sentences from the student; a single word response is completely fine.
- Explicitly teach: say the target-language word/phrase, provide the pronunciation hint if helpful, and use it in a simple example sentence.`
	case 2:
		return `STUDENT LEVEL: Elementary (2/5)
- The student knows basic greetings and very common vocabulary but still needs significant support.
- Write roughly 50% of your response in the target language and 50% in English. Place English translations in [square brackets] immediately after any word or phrase the student might not know.
- Use only simple, common grammar structures (present tense, basic questions).
- Gently model correct forms without calling out errors by name.
- Encourage the student to try responding in the target language, but accept English with a soft nudge.`
	case 3:
		return `STUDENT LEVEL: Intermediate (3/5)
- The student can handle everyday conversation on familiar topics.
- Speak primarily in the target language. Use English only for brief clarifications of complex ideas — place these in [square brackets].
- Use standard everyday vocabulary and a range of tenses.
- Subtly reinforce correct grammar by echoing corrected forms naturally in your own sentences.
- Push the student gently: ask follow-up questions that require a bit more than a yes/no answer.`
	case 4:
		return `STUDENT LEVEL: Advanced (4/5)
- The student is comfortable with the language and can discuss a wide range of topics.
- Speak entirely in the target language. Avoid English unless a cultural concept has no translation.
- Use rich vocabulary, idiomatic expressions, and varied grammar structures freely.
- Challenge the student with nuanced questions, hypotheticals, and opinions.
- Correct errors only by naturally and smoothly using the correct form in your response — never interrupt flow.`
	case 5:
		return `STUDENT LEVEL: Fluent (5/5)
- The student is near-native. Treat them as a fluent peer, not a learner.
- Speak entirely in the target language at natural native speed and complexity.
- Use idioms, colloquialisms, humor, cultural references, and register variation freely.
- Do not simplify vocabulary, grammar, or sentence length in any way.
- Engage as you would with any native speaker — debate, joke, tell stories, explore nuance.`
	default:
		return levelProfile(3)
	}
}

func buildSystemPrompt(langCode string, level int, topicName, topicDesc string) string {
	lang := LanguageName(langCode)
	return fmt.Sprintf(`You are an expert, warm, and encouraging language tutor specializing in %s. Your mission is to help the student practice %s through natural conversation about "%s".

Topic context: %s

%s

General guidelines (apply at all levels):
- Keep the conversation naturally centered on the topic without being rigid.
- Always end with a question or invitation that encourages the student to keep talking.
- Never use markdown formatting, asterisks, or bullet points — speak naturally in prose.
- Keep responses concise: typically 2–4 sentences.
- You may occasionally weave in a brief cultural note or fun fact relevant to the topic.`,
		lang, lang, topicName, topicDesc, levelProfile(level))
}

func buildGreetPrompt(langCode string, level int) string {
	lang := LanguageName(langCode)
	switch level {
	case 1:
		return fmt.Sprintf(
			"[Begin the session. Welcome the student warmly in English. Tell them you'll be learning %s together today and introduce the topic. Then teach them one simple greeting phrase in %s with its English translation to get started.]",
			lang, lang,
		)
	case 2:
		return fmt.Sprintf(
			"[Begin the session. Greet the student in a mix of English and %s. Introduce the topic simply and invite them to try responding — even in English is fine for now.]",
			lang,
		)
	case 3:
		return fmt.Sprintf(
			"[Begin the session. Greet the student primarily in %s and introduce the conversation topic naturally. Keep it brief, warm, and end with an easy question to get the conversation going.]",
			lang,
		)
	case 4, 5:
		return fmt.Sprintf(
			"[Begin the session. Greet the student entirely in %s and dive into the topic naturally, as you would with a confident speaker. Ask an engaging, open-ended question right away.]",
			lang,
		)
	default:
		return buildGreetPrompt(langCode, 3)
	}
}
