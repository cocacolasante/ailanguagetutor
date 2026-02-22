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
}

type startResponse struct {
	SessionID string `json:"session_id"`
	Language  string `json:"language"`
	Topic     string `json:"topic"`
	TopicName string `json:"topic_name"`
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

	topicName, topicDesc := TopicDetails(req.Topic)
	systemPrompt := buildSystemPrompt(req.Language, topicName, topicDesc)
	session := h.sessionStore.Create(userID, req.Language, req.Topic, systemPrompt)

	writeJSON(w, http.StatusCreated, startResponse{
		SessionID: session.ID,
		Language:  session.Language,
		Topic:     session.Topic,
		TopicName: topicName,
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
		userContent = fmt.Sprintf(
			"[Begin the session. Greet the student warmly in %s and introduce the conversation topic naturally. Keep it brief and enthusiastic.]",
			LanguageName(session.Language),
		)
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
		"messages":   msgs,
	})
}

// ── System prompt builder ─────────────────────────────────────────────────────

func buildSystemPrompt(langCode, topicName, topicDesc string) string {
	lang := LanguageName(langCode)
	return fmt.Sprintf(`You are an expert, warm, and encouraging language tutor specializing in %s. Your mission is to help the student practice %s through natural, immersive conversation about "%s".

Topic context: %s

Tutor guidelines:
1. Speak primarily in %s. For students who seem to be struggling, include very brief English hints in [square brackets] only when necessary.
2. Keep the conversation naturally centered on the topic without being rigid about it.
3. Gently reinforce correct grammar by naturally weaving the correct form into your own response — never say "you made a mistake" explicitly.
4. Match your vocabulary complexity to the student's apparent level; introduce new words with context.
5. Always ask a follow-up question or make a comment that invites the student to continue talking.
6. Be warm, patient, and celebratory — language learning is hard and every effort counts.
7. If the student writes in English, respond in %s but gently acknowledge what they said and nudge them toward the target language.
8. Keep responses conversational and concise — typically 2–4 sentences.
9. Never use markdown formatting, asterisks, or bullet points in your responses; speak naturally.
10. You may occasionally add a brief cultural note or fun fact relevant to the topic.`,
		lang, lang, topicName, topicDesc, lang, lang)
}
