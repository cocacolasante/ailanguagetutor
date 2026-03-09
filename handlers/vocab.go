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

type VocabHandler struct {
	cfg          *config.Config
	userStore    *store.UserStore
	profileStore *store.StudentProfileStore
	historyStore *store.ConversationHistoryStore
	pool         *store.ItemPool
}

func NewVocabHandler(cfg *config.Config, us *store.UserStore, ps *store.StudentProfileStore, hs *store.ConversationHistoryStore, pool *store.ItemPool) *VocabHandler {
	return &VocabHandler{cfg: cfg, userStore: us, profileStore: ps, historyStore: hs, pool: pool}
}

// ── Types ─────────────────────────────────────────────────────────────────────

type VocabWord struct {
	Word        string `json:"word"`
	Translation string `json:"translation"`
	Phonetic    string `json:"phonetic"`
}

type vocabSessionRequest struct {
	Language     string `json:"language"`
	Level        int    `json:"level"`
	Topic        string `json:"topic"`
	MistakesMode bool   `json:"mistakes_mode"`
}

type vocabSessionResponse struct {
	Words []VocabWord `json:"words"`
}

type vocabCheckRequest struct {
	Word     string `json:"word"`
	Language string `json:"language"`
	Spoken   string `json:"spoken"`
}

type vocabCheckResponse struct {
	Correct  bool   `json:"correct"`
	Feedback string `json:"feedback"`
}

type wordResult struct {
	Word     string `json:"word"`
	Correct  bool   `json:"correct"`
	Attempts int    `json:"attempts"`
}

type vocabCompleteRequest struct {
	Language  string       `json:"language"`
	Level     int          `json:"level"`
	Topic     string       `json:"topic"`
	TopicName string       `json:"topic_name"`
	Results   []wordResult `json:"results"`
}

type vocabCompleteResponse struct {
	FPEarned     int      `json:"fp_earned"`
	WeakWords    []string `json:"weak_words"`
	LearnedCount int      `json:"learned_count"`
	RecordID     string   `json:"record_id"`
}

// ── Session ───────────────────────────────────────────────────────────────────

// levelSpec describes vocabulary complexity expectations per level.
var levelSpec = map[int]string{
	1: `BEGINNER (A1): extremely basic, high-frequency words only. Simple nouns and basic verbs a tourist or child would learn first (e.g. "hello", "water", "yes", "eat", "one"). No idioms, no compound words, no grammar structures.`,
	2: `ELEMENTARY (A2): common everyday words. Practical vocabulary for simple situations — ordering food, asking directions, describing family. Simple adjectives and verbs. Still no idioms.`,
	3: `INTERMEDIATE (B1): moderately complex vocabulary. Words needed for comfortable conversation — opinions, feelings, past/future events, common workplace or social words. May include short common phrases or collocations.`,
	4: `UPPER-INTERMEDIATE (B2): sophisticated vocabulary. Less common words, nuanced verbs, idiomatic expressions, compound nouns, and topic-specific terminology. Words that distinguish a good speaker from an average one.`,
	5: `ADVANCED/FLUENT (C1-C2): advanced and nuanced vocabulary. Rare words, formal register, subtle distinctions between synonyms, idiomatic expressions, proverbs, and domain-specific jargon. Words a native speaker uses that textbooks rarely teach.`,
}

func (h *VocabHandler) Session(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req vocabSessionRequest
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

	// Mistakes mode: generate flashcards exclusively from weak vocab
	if req.MistakesMode {
		weakVocab := []string{}
		if profile != nil {
			weakVocab = profile.WeakVocab
		}
		if len(weakVocab) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"words":   []VocabWord{},
				"message": "No weak words on record yet. Complete some vocab sessions first!",
			})
			return
		}
		limit := weakVocab
		if len(limit) > 15 {
			limit = limit[:15]
		}
		prompt := fmt.Sprintf(`You are a language teacher generating flashcard vocabulary for a student who needs to review specific words.

Language: %s
Level: %s

The student previously struggled with these specific words: %s

Generate flashcards ONLY for these specific words the student struggled with. Include ALL of them (up to 15).

Return ONLY valid JSON — no markdown, no code fences, no explanation:
{"words":[{"word":"...","translation":"...","phonetic":"..."},...]}

Rules:
- "word": the exact %s word from the list above
- "translation": concise English translation
- "phonetic": English-syllable pronunciation guide with stressed syllable in CAPS`,
			langName, spec, strings.Join(limit, ", "), langName)

		result, err := h.callAI(r.Context(), prompt, 900, 0.3)
		if err != nil {
			log.Printf("vocab/session mistakes AI error: %v", err)
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
			Words []VocabWord `json:"words"`
		}
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			log.Printf("vocab/session mistakes JSON parse error: %v\nraw: %s", err, result)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse AI response"})
			return
		}
		store.Shuffle(parsed.Words)
		writeJSON(w, http.StatusOK, vocabSessionResponse{Words: parsed.Words})
		return
	}

	// Cache-first: serve from pool if the user hasn't exhausted this key
	key := h.pool.Key(req.Language, req.Level, req.Topic)
	userIdx := 0
	if profile != nil && profile.VocabListIdx != nil {
		userIdx = profile.VocabListIdx[key]
	}

	if userIdx < h.pool.Len(key) {
		raw, _ := h.pool.Get(key, userIdx)
		var words []VocabWord
		if err := json.Unmarshal(raw, &words); err == nil {
			store.Shuffle(words)
			writeJSON(w, http.StatusOK, vocabSessionResponse{Words: words})
			return
		}
	}

	// Cache miss: build exclusion list from all cached pool lists + profile history
	var excludeWords []string
	for _, raw := range h.pool.AllRaw(key) {
		var list []VocabWord
		if err := json.Unmarshal(raw, &list); err == nil {
			for _, w := range list {
				excludeWords = append(excludeWords, w.Word)
			}
		}
	}
	if profile != nil && len(profile.RecentVocab) > 0 {
		seen := profile.RecentVocab
		if len(seen) > 60 {
			seen = seen[:60]
		}
		excludeWords = append(excludeWords, seen...)
	}
	// Deduplicate
	seen := map[string]bool{}
	deduped := excludeWords[:0]
	for _, w := range excludeWords {
		if !seen[w] {
			seen[w] = true
			deduped = append(deduped, w)
		}
	}
	excludeWords = deduped

	var excludeClause string
	if len(excludeWords) > 0 {
		seenJSON, _ := json.Marshal(excludeWords)
		excludeClause = fmt.Sprintf("\n- Do NOT use any of these already-learned words: %s", string(seenJSON))
	}

	var reinforceClause string
	if profile != nil && len(profile.WeakAreas) > 0 {
		weak := profile.WeakAreas
		if len(weak) > 10 {
			weak = weak[:10]
		}
		reinforceClause = fmt.Sprintf("\n- REINFORCE these previously weak words by including 2–3 of them in the list: %s", strings.Join(weak, ", "))
	}

	prompt := fmt.Sprintf(`You are a language teacher generating flashcard vocabulary for a student.

Language: %s
Topic: %s
Level: %s

Generate exactly 12 %s vocabulary words appropriate for this topic and level.

Return ONLY valid JSON — no markdown, no code fences, no explanation:
{"words":[{"word":"...","translation":"...","phonetic":"..."},...]}

Rules:
- "word": the %s word or short phrase (match the complexity described above)
- "translation": concise English translation
- "phonetic": English-syllable pronunciation guide with stressed syllable in CAPS (e.g. "MEH-sah" for mesa, "KWAHN-doh" for cuando)
- Order from easiest to hardest within the level
- Prioritise words the student will actually encounter and use
- Every session must use DIFFERENT words — avoid repetition%s%s
- Exactly 12 items`,
		langName, topicName, spec, langName, langName, excludeClause, reinforceClause)

	result, err := h.callAI(r.Context(), prompt, 900, 0.8)
	if err != nil {
		log.Printf("vocab/session AI error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AI service error"})
		return
	}

	// Strip markdown fences if present
	result = strings.TrimSpace(result)
	if idx := strings.Index(result, "{"); idx > 0 {
		result = result[idx:]
	}
	if idx := strings.LastIndex(result, "}"); idx >= 0 && idx < len(result)-1 {
		result = result[:idx+1]
	}

	var parsed struct {
		Words []VocabWord `json:"words"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		log.Printf("vocab/session JSON parse error: %v\nraw: %s", err, result)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse AI response"})
		return
	}

	// Cache the new list, then shuffle before returning
	if raw, err := json.Marshal(parsed.Words); err == nil {
		h.pool.Append(key, raw)
	}
	store.Shuffle(parsed.Words)
	writeJSON(w, http.StatusOK, vocabSessionResponse{Words: parsed.Words})
}

// ── Check ─────────────────────────────────────────────────────────────────────

func (h *VocabHandler) Check(w http.ResponseWriter, r *http.Request) {
	var req vocabCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	langName := LanguageName(req.Language)
	// Strip orthographic punctuation (¿, ¡, etc.) — speech recognition never produces these.
	cleanWord := strings.TrimSpace(stripOrthographic(req.Word))
	cleanSpoken := strings.TrimSpace(stripOrthographic(req.Spoken))
	prompt := fmt.Sprintf(`A language student was asked to pronounce the %s word "%s".
Speech recognition captured: "%s".

Note: orthographic markers like ¿ and ¡ are not spoken aloud and will not appear in speech recognition output — ignore their absence.

Answer true ONLY if "%s" is clearly the student's attempt to say "%s" — it must be the same word, possibly with a minor accent or pronunciation variance. Answer false if the student said a different word entirely, said nothing meaningful, or produced something unrelated to "%s".

Reply with ONLY valid JSON: {"correct": true/false, "feedback": "one short pronunciation tip if wrong, empty string if correct"}`,
		langName, cleanWord, cleanSpoken, cleanSpoken, cleanWord, cleanWord)

	result, err := h.callAI(r.Context(), prompt, 100, 0.1)
	if err != nil {
		// Fallback: edit distance only (on cleaned strings)
		spoken := strings.ToLower(cleanSpoken)
		word := strings.ToLower(cleanWord)
		threshold := 1
		if len([]rune(word)) > 10 {
			threshold = 2
		}
		correct := editDistance(spoken, word) <= threshold
		writeJSON(w, http.StatusOK, vocabCheckResponse{Correct: correct, Feedback: ""})
		return
	}

	result = strings.TrimSpace(result)
	if idx := strings.Index(result, "{"); idx > 0 {
		result = result[idx:]
	}
	if idx := strings.LastIndex(result, "}"); idx >= 0 && idx < len(result)-1 {
		result = result[:idx+1]
	}

	var parsed vocabCheckResponse
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		// Fallback: edit distance only (on cleaned strings)
		spoken := strings.ToLower(cleanSpoken)
		word := strings.ToLower(cleanWord)
		threshold := 1
		if len([]rune(word)) > 10 {
			threshold = 2
		}
		correct := editDistance(spoken, word) <= threshold
		writeJSON(w, http.StatusOK, vocabCheckResponse{Correct: correct, Feedback: ""})
		return
	}

	writeJSON(w, http.StatusOK, parsed)
}

// ── Complete ──────────────────────────────────────────────────────────────────

func (h *VocabHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req vocabCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	topicName, _ := TopicDetails(req.Topic)

	var weakWords []string
	var learnedWords []string
	for _, res := range req.Results {
		if res.Correct {
			learnedWords = append(learnedWords, res.Word)
		} else {
			weakWords = append(weakWords, res.Word)
		}
	}

	fp := len(req.Results) * 5
	if fp < 10 {
		fp = 10
	}

	// Update streak + FP
	if _, _, err := h.userStore.UpdateActivity(userID, req.Language, fp); err != nil {
		log.Printf("vocab/complete UpdateActivity error: %v", err)
	}

	// Update student profile
	ctx := context.Background()
	profile, err := h.profileStore.Get(ctx, userID, req.Language)
	if err != nil || profile == nil {
		profile = &store.StudentProfile{
			UserID:   userID,
			Language: req.Language,
		}
	}

	profile.WeakAreas    = prependUnique(weakWords, profile.WeakAreas, 20)
	profile.WeakVocab    = prependUnique(weakWords, profile.WeakVocab, 30)
	profile.RecentVocab  = prependUnique(learnedWords, profile.RecentVocab, 30)
	profile.RecentTopics = prependUnique([]string{req.TopicName}, profile.RecentTopics, 10)
	profile.SessionCount++

	// Advance the user's list index for this pool key
	key := h.pool.Key(req.Language, req.Level, req.Topic)
	if profile.VocabListIdx == nil {
		profile.VocabListIdx = make(map[string]int)
	}
	profile.VocabListIdx[key]++

	if err := h.profileStore.Upsert(ctx, profile); err != nil {
		log.Printf("vocab/complete Upsert error: %v", err)
	}

	// Build summary text
	total := len(req.Results)
	summary := fmt.Sprintf("Completed Vocabulary Builder on %s: %d/%d words learned.", topicName, len(learnedWords), total)
	corrections := weakWords
	var suggestions []string
	if len(weakWords) > 0 {
		suggestions = []string{
			fmt.Sprintf("Review these weak words in context: %s", strings.Join(weakWords[:min(3, len(weakWords))], ", ")),
			"Try a conversation session to use today's vocabulary naturally",
			"Repeat this topic to reinforce the words you missed",
		}
	} else {
		suggestions = []string{
			"Try the next level to challenge yourself",
			"Practice these words in a conversation session",
			"Explore a new topic to expand your vocabulary",
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
		Personality:  "vocab-builder",
		MessageCount: total,
		FPEarned:     fp,
		Summary:      summary,
		Topics:       []string{topicName},
		Vocabulary:   learnedWords,
		Corrections:  corrections,
		Suggestions:  suggestions,
		CreatedAt:    time.Now(),
		EndedAt:      time.Now(),
	}
	h.historyStore.Save(record)

	writeJSON(w, http.StatusOK, vocabCompleteResponse{
		FPEarned:     fp,
		WeakWords:    weakWords,
		LearnedCount: len(learnedWords),
		RecordID:     recordID,
	})
}

// ── AI helper ─────────────────────────────────────────────────────────────────

type ionosVocabPayload struct {
	Model       string         `json:"model"`
	Messages    []store.Message `json:"messages"`
	Stream      bool           `json:"stream"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature float64        `json:"temperature"`
}

type ionosVocabResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
}

func (h *VocabHandler) callAI(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
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

// ── Edit distance (Levenshtein) fallback ──────────────────────────────────────

// stripOrthographic removes punctuation that speech recognition never produces
// (¿, ¡, ?, !, ., ,) so comparisons are not thrown off by Spanish inverted marks.
func stripOrthographic(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '¿', '¡', '?', '!', '.', ',', ';', ':', '"', '\'':
			return -1
		}
		return r
	}, s)
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			if ra[i-1] == rb[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = 1 + min3(dp[i-1][j], dp[i][j-1], dp[i-1][j-1])
			}
		}
	}
	return dp[la][lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
