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
)

// ── Handler ────────────────────────────────────────────────────────────────────

type ListeningHandler struct {
	cfg          *config.Config
	userStore    *store.UserStore
	profileStore *store.StudentProfileStore
	pool         *store.ItemPool
	vocabPool    *store.ItemPool
	sentencePool *store.ItemPool
}

func NewListeningHandler(
	cfg *config.Config,
	us *store.UserStore,
	ps *store.StudentProfileStore,
	pool *store.ItemPool,
	vocabPool *store.ItemPool,
	sentencePool *store.ItemPool,
) *ListeningHandler {
	return &ListeningHandler{
		cfg:          cfg,
		userStore:    us,
		profileStore: ps,
		pool:         pool,
		vocabPool:    vocabPool,
		sentencePool: sentencePool,
	}
}

// ── Types ──────────────────────────────────────────────────────────────────────

type StoryQuestion struct {
	Type        string   `json:"type"`              // "multiple_choice"|"true_false"|"yes_no"
	Question    string   `json:"question"`
	Options     []string `json:"options,omitempty"`
	Answer      any      `json:"answer"`            // int or string
	Explanation string   `json:"explanation"`
}

type StorySegment struct {
	Text     string        `json:"text"`
	Question StoryQuestion `json:"question"`
}

type Story struct {
	Title    string         `json:"title"`
	Segments []StorySegment `json:"segments"`
}

type listeningSessionRequest struct {
	Language    string `json:"language"`
	Level       int    `json:"level"`
	Topic       string `json:"topic"`
	Personality string `json:"personality"`
}

type listeningSessionResponse struct {
	Story Story   `json:"story"`
	Speed float64 `json:"speed"`
}

type listeningResult struct {
	QuestionIndex int  `json:"question_index"`
	Correct       bool `json:"correct"`
}

type listeningCompleteRequest struct {
	Language    string            `json:"language"`
	Level       int               `json:"level"`
	Topic       string            `json:"topic"`
	TopicName   string            `json:"topic_name"`
	Personality string            `json:"personality"`
	Results     []listeningResult `json:"results"`
}

type listeningCompleteResponse struct {
	FPEarned     int `json:"fp_earned"`
	CorrectCount int `json:"correct_count"`
	TotalCount   int `json:"total_count"`
}

// ── Speed + segment count tables ──────────────────────────────────────────────

var levelSpeeds = map[int]float64{1: 0.75, 2: 0.85, 3: 1.0, 4: 1.0, 5: 1.1}
var levelSegments = map[int]int{1: 3, 2: 4, 3: 5, 4: 5, 5: 6}

func speedForLevel(level int) float64 {
	if s, ok := levelSpeeds[level]; ok {
		return s
	}
	return 1.0
}

func segmentCountForLevel(level int) int {
	if n, ok := levelSegments[level]; ok {
		return n
	}
	return 5
}

// ── Cultural context hints ─────────────────────────────────────────────────────

var culturalContext = map[string]string{
	"it": "Italian culture, daily life, food, and traditions",
	"es": "Hispanic/Latin American culture, daily life, and traditions",
	"pt": "Portuguese/Brazilian culture, daily life, and traditions",
}

// ── Personality descriptions ───────────────────────────────────────────────────

var personalityDescriptions = map[string]string{
	"professor":          "scholarly, precise, uses clear examples and cultural context",
	"friendly-partner":   "warm, conversational, encouraging, like a good friend",
	"bartender":          "casual, witty, uses colloquialisms and everyday language",
	"business-executive": "professional, concise, formal register",
	"travel-guide":       "enthusiastic, adventurous, rich in cultural details",
}

// ── Session ────────────────────────────────────────────────────────────────────

func (h *ListeningHandler) Session(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req listeningSessionRequest
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

	profile, _ := h.profileStore.Get(r.Context(), userID, req.Language)

	key := fmt.Sprintf("%s:%d:%s:%s", req.Language, req.Level, req.Topic, req.Personality)
	userIdx := 0
	if profile != nil && profile.ListeningListIdx != nil {
		userIdx = profile.ListeningListIdx[key]
	}

	// Cache hit
	if userIdx < h.pool.Len(key) {
		raw, _ := h.pool.Get(key, userIdx)
		var story Story
		if err := json.Unmarshal(raw, &story); err == nil {
			writeJSON(w, http.StatusOK, listeningSessionResponse{Story: story, Speed: speedForLevel(req.Level)})
			return
		}
	}

	// Cache miss: gather vocab words to weave into prompt
	vocabKey := h.vocabPool.Key(req.Language, req.Level, req.Topic)
	var reinforceWords []string
	for _, raw := range h.vocabPool.AllRaw(vocabKey) {
		var list []VocabWord
		if err := json.Unmarshal(raw, &list); err == nil {
			for _, w := range list {
				reinforceWords = append(reinforceWords, w.Word)
			}
		}
	}
	if len(reinforceWords) > 20 {
		reinforceWords = reinforceWords[:20]
	}

	story, err := h.generateStory(r.Context(), req.Language, req.Level, req.Topic, req.Personality, reinforceWords)
	if err != nil {
		log.Printf("listening/session AI error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AI service error"})
		return
	}

	raw, _ := json.Marshal(*story)
	h.pool.Append(key, raw)
	writeJSON(w, http.StatusOK, listeningSessionResponse{Story: *story, Speed: speedForLevel(req.Level)})
}

// ── Complete ───────────────────────────────────────────────────────────────────

func (h *ListeningHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req listeningCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	correctCount := 0
	for _, res := range req.Results {
		if res.Correct {
			correctCount++
		}
	}
	totalCount := len(req.Results)

	fp := correctCount * 15
	if fp < 20 {
		fp = 20
	}
	if correctCount == totalCount && totalCount > 0 {
		fp += 20
	}

	if _, _, err := h.userStore.UpdateActivity(userID, req.Language, fp); err != nil {
		log.Printf("listening/complete UpdateActivity error: %v", err)
	}

	ctx := context.Background()
	profile, err := h.profileStore.Get(ctx, userID, req.Language)
	if err != nil || profile == nil {
		profile = &store.StudentProfile{
			UserID:   userID,
			Language: req.Language,
		}
	}

	profile.RecentTopics = prependUnique([]string{req.TopicName}, profile.RecentTopics, 10)
	profile.SessionCount++

	key := fmt.Sprintf("%s:%d:%s:%s", req.Language, req.Level, req.Topic, req.Personality)
	if profile.ListeningListIdx == nil {
		profile.ListeningListIdx = make(map[string]int)
	}
	profile.ListeningListIdx[key]++

	if err := h.profileStore.Upsert(ctx, profile); err != nil {
		log.Printf("listening/complete Upsert error: %v", err)
	}

	writeJSON(w, http.StatusOK, listeningCompleteResponse{
		FPEarned:     fp,
		CorrectCount: correctCount,
		TotalCount:   totalCount,
	})
}

// ── AI story generation ────────────────────────────────────────────────────────

func (h *ListeningHandler) generateStory(ctx context.Context, language string, level int, topic string, personality string, reinforceWords []string) (*Story, error) {
	langName := LanguageName(language)
	topicName, _ := TopicDetails(topic)
	n := segmentCountForLevel(level)
	spec := levelSpec[level]
	if spec == "" {
		spec = "INTERMEDIATE (B1)"
	}

	cultural := culturalContext[language]
	if cultural == "" {
		cultural = "the target language's culture, daily life, and traditions"
	}
	personalityDesc := personalityDescriptions[personality]
	if personalityDesc == "" {
		personalityDesc = "friendly and clear"
	}

	var reinforceClause string
	if len(reinforceWords) > 0 {
		reinforceClause = fmt.Sprintf("\nVocabulary to weave in naturally (use as many as appropriate): %s", strings.Join(reinforceWords, ", "))
	}

	prompt := fmt.Sprintf(`You are a language tutor creating a listening comprehension story.
Language: %s
Topic: %s
Level: %s
Narrator personality: %s — %s
Cultural context: %s%s

Write a short, engaging, FAMILY-FRIENDLY story in %s that:
- Is culturally authentic and relevant to the context above
- Is told by a narrator with the personality described above
- Contains exactly %d segments (paragraphs of 60–90 words each)
- Naturally incorporates as many vocabulary words from the list as possible
- Is entirely appropriate for all ages

After each segment, write one comprehension question. Use a mix of types across segments:
- "yes_no": yes or no question about a fact in the segment
- "true_false": true/false statement about the segment
- "multiple_choice": 4-option question (exactly 4 options)

Return ONLY valid JSON — no markdown, no code fences, no explanation:
{
  "title": "Story title in target language",
  "segments": [
    {
      "text": "Paragraph in target language...",
      "question": {
        "type": "multiple_choice",
        "question": "Question in target language",
        "options": ["A","B","C","D"],
        "answer": 0,
        "explanation": "Brief explanation in target language"
      }
    }
  ]
}
For yes_no: omit options, answer is "yes" or "no"
For true_false: omit options, answer is "true" or "false"
Exactly %d segments. Family-friendly only.`,
		langName, topicName, spec,
		personality, personalityDesc,
		cultural, reinforceClause,
		langName, n, n,
	)

	result, err := h.callAI(ctx, prompt, 2500, 0.85)
	if err != nil {
		return nil, err
	}

	// Strip markdown fences if present
	result = strings.TrimSpace(result)
	if idx := strings.Index(result, "{"); idx > 0 {
		result = result[idx:]
	}
	if idx := strings.LastIndex(result, "}"); idx >= 0 && idx < len(result)-1 {
		result = result[:idx+1]
	}

	var story Story
	if err := json.Unmarshal([]byte(result), &story); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w\nraw: %s", err, result)
	}
	return &story, nil
}

// ── AI helper (mirrors VocabHandler.callAI) ────────────────────────────────────

type ionosListeningPayload struct {
	Model       string          `json:"model"`
	Messages    []store.Message `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
}

type ionosListeningResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
}

func (h *ListeningHandler) callAI(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	payload := ionosListeningPayload{
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

	var parsed ionosListeningResponse
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
