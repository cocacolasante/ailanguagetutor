package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
)

const elevenLabsAPI = "https://api.elevenlabs.io"

type AgentHandler struct {
	cfg          *config.Config
	sessionStore *store.SessionStore
}

func NewAgentHandler(cfg *config.Config, ss *store.SessionStore) *AgentHandler {
	return &AgentHandler{cfg: cfg, sessionStore: ss}
}

// ── Setup Agent (admin, one-time) ─────────────────────────────────────────────

func (h *AgentHandler) SetupAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := h.createAgent()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"agent_id": agentID,
		"message":  "Agent created. Add ELEVENLABS_AGENT_ID=" + agentID + " to your .env and restart.",
	})
}

func (h *AgentHandler) createAgent() (string, error) {
	payload := map[string]any{
		"name": "LinguaAI Language Tutor",
		"conversation_config": map[string]any{
			"agent": map[string]any{
				"prompt": map[string]any{
					"prompt":      agentBasePrompt,
					"llm":         "gemini-2.0-flash",
					"temperature": 0.8,
					"max_tokens":  600,
				},
				"first_message": "",
				"language":      "en",
			},
			"asr": map[string]any{
				"quality":                  "high",
				"provider":                 "elevenlabs",
				"user_input_audio_format":  "pcm_16000",
			},
			"turn": map[string]any{
				"turn_timeout": 7,
				"mode":         "turn",
			},
			"tts": map[string]any{
				"model_id":                    "eleven_multilingual_v2",
				"voice_id":                    "21m00Tcm4TlvDq8ikWAM",
				"optimize_streaming_latency":  3,
				"output_format":               "pcm_16000",
			},
		},
		"platform_settings": map[string]any{
			"auth": map[string]any{
				"allow_api_key_auth": false,
			},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", elevenLabsAPI+"/v1/convai/agents/create", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("xi-api-key", h.cfg.ElevenLabsAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ElevenLabs error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || result.AgentID == "" {
		return "", fmt.Errorf("unexpected response: %s", string(respBody))
	}
	return result.AgentID, nil
}

// ── Get signed conversation URL ───────────────────────────────────────────────

type agentURLRequest struct {
	SessionID string `json:"session_id"`
}

type agentURLResponse struct {
	SignedURL    string `json:"signed_url"`
	SystemPrompt string `json:"system_prompt"`
	FirstMessage string `json:"first_message"`
	VoiceID      string `json:"voice_id"`
	Language     string `json:"language"` // BCP-47 language code for native ASR + TTS accent
}

func (h *AgentHandler) GetConversationURL(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)

	var req agentURLRequest
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

	if h.cfg.ElevenLabsAgentID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "ElevenLabs agent not configured — run POST /api/admin/setup-agent first.",
		})
		return
	}

	topicName, topicDesc := TopicDetails(session.Topic)

	// Build the session-specific system prompt.
	// The opening utterance is handled by first_message, not the system prompt,
	// so we don't inject a greet instruction here.
	systemPrompt := buildSystemPrompt(
		session.Language, session.Level, topicName, topicDesc,
		session.Topic, session.Personality, false,
	)
	// Strip any bracket wrappers left over from the text-based prompt builders
	systemPrompt = strings.ReplaceAll(systemPrompt, "[", "")
	systemPrompt = strings.ReplaceAll(systemPrompt, "]", "")

	voiceID := h.cfg.VoiceForLanguage(session.Language)

	signedURL, err := h.getSignedURL()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to get conversation URL: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, agentURLResponse{
		SignedURL:    signedURL,
		SystemPrompt: systemPrompt,
		FirstMessage: buildFirstMessage(session.Language, session.Topic, session.Level),
		VoiceID:      voiceID,
		Language:     session.Language,
	})
}

func (h *AgentHandler) getSignedURL() (string, error) {
	url := elevenLabsAPI + "/v1/convai/conversation/get_signed_url?agent_id=" + h.cfg.ElevenLabsAgentID
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("xi-api-key", h.cfg.ElevenLabsAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ElevenLabs error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		SignedURL string `json:"signed_url"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.SignedURL == "" {
		return "", fmt.Errorf("unexpected response: %s", string(body))
	}
	return result.SignedURL, nil
}

// buildFirstMessage returns the opening utterance for the ElevenLabs agent.
// It is level-aware, topic-specific, and in the target language.
// Levels 1-2 → beginner-safe questions; levels 3-5 → engaging topic-specific questions.
func buildFirstMessage(langCode, topicID string, level int) string {

	// ── Role-play: in-character opening ───────────────────────────────────────
	rolePlays := map[string]string{
		"role-restaurant":    "Buenas tardes, bienvenido. ¿Tiene reservación o prefiere mesa libre?",
		"role-job-interview": "Buenas tardes. Gracias por venir hoy. Por favor, cuénteme un poco sobre usted.",
		"role-airport":       "Good afternoon! Passport and booking reference, please.",
		"role-doctor":        "Good morning. Please take a seat — what brings you in today?",
		"role-business":      "Good morning, glad you could make it. Shall we get started?",
		"role-apartment":     "Hi, welcome! I'll show you around. Any questions before we begin?",
		"role-directions":    "¡Hola! ¿En qué puedo ayudarte?",
	}
	if msg, ok := rolePlays[topicID]; ok {
		return msg
	}

	// ── Travel destinations: local guide in target language ────────────────────
	travelGreets := map[string]string{
		"travel-rome":      "Ciao! Benvenuto a Roma. È la tua prima volta qui, o sei già stato?",
		"travel-barcelona": "¡Hola! Bienvenido a Barcelona. ¿Es tu primera vez aquí o ya conoces la ciudad?",
		"travel-paris":     "Bonjour! Bienvenue à Paris. C'est votre première visite, ou connaissez-vous déjà la ville?",
		"travel-tokyo":     "こんにちは！東京へようこそ。東京は初めてですか、それとも以前に来たことがありますか？",
		"travel-lisbon":    "Olá! Bem-vindo a Lisboa. É a sua primeira vez aqui ou já conhece a cidade?",
	}
	if msg, ok := travelGreets[topicID]; ok {
		return msg
	}

	// ── Immersion: full target-language opener ─────────────────────────────────
	if strings.HasPrefix(topicID, "immersion-") {
		immersionGreets := map[string]string{
			"es": "¡Hola! Hoy hablamos solo en español. ¿Sobre qué tema te gustaría conversar hoy?",
			"it": "Ciao! Oggi parliamo solo in italiano. Di cosa ti va di parlare oggi?",
			"pt": "Olá! Hoje falamos só em português. Sobre o que você gostaria de conversar hoje?",
			"fr": "Bonjour! Aujourd'hui on parle uniquement en français. De quoi voulez-vous parler aujourd'hui?",
			"de": "Hallo! Heute sprechen wir nur Deutsch. Worüber möchtest du heute sprechen?",
			"ja": "こんにちは！今日は日本語だけで話しましょう。今日は何について話したいですか？",
			"zh": "你好！今天我们只说中文。你今天想聊什么话题？",
			"ru": "Привет! Сегодня говорим только по-русски. О чём хочешь поговорить сегодня?",
			"ro": "Bună! Azi vorbim doar în română. Despre ce vrei să vorbești azi?",
		}
		if g, ok := immersionGreets[langCode]; ok {
			return g
		}
		return "Hello! Today we speak only in the target language. What shall we talk about?"
	}

	// ── Grammar: structured practice opener ───────────────────────────────────
	if strings.HasPrefix(topicID, "grammar-") {
		grammarGreets := map[string]map[string]string{
			"grammar-vocabulary": {
				"es": "¡Hola! Hoy ampliamos tu vocabulario en español. ¿Qué temas te interesan más — la vida cotidiana, el trabajo, o los viajes?",
				"it": "Ciao! Oggi arricchiamo il tuo vocabolario italiano. Che argomenti ti interessano di più?",
				"pt": "Olá! Hoje vamos ampliar seu vocabulário em português. Quais temas te interessam mais?",
				"fr": "Bonjour! Aujourd'hui on enrichit votre vocabulaire français. Quels sujets vous intéressent le plus?",
				"de": "Hallo! Heute erweitern wir deinen deutschen Wortschatz. Welche Themen interessieren dich am meisten?",
				"ja": "こんにちは！今日は日本語の語彙を増やしましょう。どんなテーマに興味がありますか？",
				"zh": "你好！今天我们来扩充你的中文词汇。你对哪些话题最感兴趣？",
				"ru": "Привет! Сегодня расширяем твой словарный запас. Какие темы тебя интересуют больше всего?",
				"ro": "Bună! Azi ne extindem vocabularul în română. Ce subiecte te interesează cel mai mult?",
			},
			"grammar-sentences": {
				"es": "¡Hola! Hoy practicamos la construcción de frases en español. ¿Estás listo para tu primer ejercicio?",
				"it": "Ciao! Oggi pratichiamo la costruzione di frasi in italiano. Pronto per il primo esercizio?",
				"pt": "Olá! Hoje praticamos a construção de frases em português. Você está pronto para o primeiro exercício?",
				"fr": "Bonjour! Aujourd'hui on pratique la construction de phrases en français. Prêt pour le premier exercice?",
				"de": "Hallo! Heute üben wir den Satzbau auf Deutsch. Bist du bereit für die erste Übung?",
				"ja": "こんにちは！今日は日本語の文章の作り方を練習しましょう。最初の練習の準備はできていますか？",
				"zh": "你好！今天我们练习中文造句。准备好第一个练习了吗？",
				"ru": "Привет! Сегодня тренируем построение предложений. Готов к первому упражнению?",
				"ro": "Bună! Azi exersăm construcția de propoziții în română. Ești gata pentru primul exercițiu?",
			},
			"grammar-pronunciation": {
				"es": "¡Hola! Hoy trabajamos la pronunciación del español. ¿Hay algún sonido que te cueste especialmente?",
				"it": "Ciao! Oggi lavoriamo sulla pronuncia dell'italiano. C'è qualche suono che trovi particolarmente difficile?",
				"pt": "Olá! Hoje trabalhamos a pronúncia do português. Há algum som que você acha especialmente difícil?",
				"fr": "Bonjour! Aujourd'hui on travaille la prononciation du français. Y a-t-il des sons qui vous posent problème?",
				"de": "Hallo! Heute arbeiten wir an der deutschen Aussprache. Gibt es Laute, die dir besonders schwer fallen?",
				"ja": "こんにちは！今日は日本語の発音を練習しましょう。特に難しいと感じる音はありますか？",
				"zh": "你好！今天我们练习中文发音。有没有你觉得特别困难的声音？",
				"ru": "Привет! Сегодня работаем над произношением. Есть звуки, которые даются тебе особенно сложно?",
				"ro": "Bună! Azi lucrăm la pronunția în română. Există sunete care ți se par deosebit de dificile?",
			},
			"grammar-listening": {
				"es": "¡Hola! Hoy entrenamos la comprensión auditiva en español. ¿Estás listo para escuchar el primer pasaje?",
				"it": "Ciao! Oggi alleniamo la comprensione dell'ascolto in italiano. Pronto per il primo brano?",
				"pt": "Olá! Hoje treinamos a compreensão auditiva em português. Você está pronto para ouvir o primeiro trecho?",
				"fr": "Bonjour! Aujourd'hui on entraîne la compréhension orale en français. Prêt pour le premier passage?",
				"de": "Hallo! Heute trainieren wir das Hörverstehen auf Deutsch. Bist du bereit für den ersten Text?",
				"ja": "こんにちは！今日は日本語のリスニングを練習しましょう。最初のパッセージを聞く準備はできていますか？",
				"zh": "你好！今天我们练习中文听力理解。准备好听第一段了吗？",
				"ru": "Привет! Сегодня тренируем аудирование. Готов к первому отрывку?",
				"ro": "Bună! Azi antrenăm înțelegerea la auz în română. Ești gata pentru primul pasaj?",
			},
			"grammar-writing": {
				"es": "¡Hola! Hoy trabajamos la escritura en español. ¿Prefieres practicar con mensajes cotidianos, correos formales, o textos creativos?",
				"it": "Ciao! Oggi lavoriamo sulla scrittura in italiano. Preferisci messaggi quotidiani, email formali, o testi creativi?",
				"pt": "Olá! Hoje trabalhamos a escrita em português. Você prefere mensagens do dia a dia, e-mails formais ou textos criativos?",
				"fr": "Bonjour! Aujourd'hui on travaille l'écriture en français. Préférez-vous des messages quotidiens, des e-mails formels, ou des textes créatifs?",
				"de": "Hallo! Heute arbeiten wir am Schreiben auf Deutsch. Möchtest du alltägliche Nachrichten, formelle E-Mails oder kreative Texte üben?",
				"ja": "こんにちは！今日は日本語の作文を練習しましょう。日常のメッセージ、フォーマルなメール、それとも創作文どれを練習したいですか？",
				"zh": "你好！今天我们练习中文写作。你更喜欢练习日常消息、正式邮件还是创意写作？",
				"ru": "Привет! Сегодня работаем над письмом. Хочешь практиковать повседневные сообщения, деловые письма или творческие тексты?",
				"ro": "Bună! Azi lucrăm la scriere în română. Preferi mesaje cotidiene, e-mailuri formale sau texte creative?",
			},
		}
		if byLang, ok := grammarGreets[topicID]; ok {
			if msg, ok := byLang[langCode]; ok {
				return msg
			}
		}
		return "Hello! Welcome to your practice session. What would you like to work on today?"
	}

	// ── Cultural: target-language cultural opener ──────────────────────────────
	if strings.HasPrefix(topicID, "cultural-") {
		culturalGreets := map[string]map[string]string{
			"cultural-context": {
				"es": "¡Hola! Hoy exploramos la cultura hispanohablante. ¿Qué aspectos de la cultura en español te generan más curiosidad?",
				"it": "Ciao! Oggi esploriamo la cultura italiana. Cosa ti incuriosisce di più della cultura italiana?",
				"pt": "Olá! Hoje exploramos a cultura lusófona. Que aspectos da cultura portuguesa ou brasileira te geram mais curiosidade?",
				"fr": "Bonjour! Aujourd'hui on explore la culture francophone. Quels aspects de la culture française vous intriguent le plus?",
				"de": "Hallo! Heute erkunden wir die deutschsprachige Kultur. Welche Aspekte der deutschen Kultur interessieren dich am meisten?",
				"ja": "こんにちは！今日は日本文化を探求しましょう。日本文化のどんなところが一番気になりますか？",
				"zh": "你好！今天我们探索中文文化。中国文化的哪些方面让你最感好奇？",
				"ru": "Привет! Сегодня исследуем русскую культуру. Какие аспекты русской культуры тебя интересуют больше всего?",
				"ro": "Bună! Azi explorăm cultura română. Ce aspecte ale culturii române te fascinează cel mai mult?",
			},
			"cultural-stories": {
				"es": "¡Hola! Hoy viajamos juntos a través de una historia en español. ¿Estás listo para entrar en escena?",
				"it": "Ciao! Oggi viaggiamo insieme attraverso una storia in italiano. Sei pronto per entrare in scena?",
				"pt": "Olá! Hoje viajamos juntos por uma história em português. Você está pronto para entrar em cena?",
				"fr": "Bonjour! Aujourd'hui on voyage ensemble à travers une histoire en français. Êtes-vous prêt à entrer en scène?",
				"de": "Hallo! Heute reisen wir gemeinsam durch eine Geschichte auf Deutsch. Bist du bereit, die Bühne zu betreten?",
				"ja": "こんにちは！今日は一緒に日本語のストーリーの旅に出ましょう。舞台に上がる準備はできていますか？",
				"zh": "你好！今天我们一起通过中文故事旅行。准备好登上舞台了吗？",
				"ru": "Привет! Сегодня путешествуем вместе через историю на русском. Готов выйти на сцену?",
				"ro": "Bună! Azi călătorim împreună printr-o poveste în română. Ești gata să intri în scenă?",
			},
			"cultural-idioms": {
				"es": "¡Hola! Hoy descubrimos expresiones idiomáticas del español. ¿Has escuchado alguna frase en español que te haya dejado confundido?",
				"it": "Ciao! Oggi scopriamo i modi di dire italiani. Hai mai sentito un'espressione italiana che ti ha lasciato perplesso?",
				"pt": "Olá! Hoje descobrimos expressões idiomáticas do português. Já ouviu alguma frase em português que te deixou confuso?",
				"fr": "Bonjour! Aujourd'hui on découvre les expressions idiomatiques françaises. Avez-vous entendu une expression française qui vous a laissé perplexe?",
				"de": "Hallo! Heute entdecken wir deutsche Redewendungen. Hast du schon mal einen deutschen Ausdruck gehört, der dich verwirrt hat?",
				"ja": "こんにちは！今日は日本語の慣用句を探りましょう。意味が分からなかった日本語の表現を聞いたことはありますか？",
				"zh": "你好！今天我们来了解中文成语和惯用语。你有没有听到过让你困惑的中文表达？",
				"ru": "Привет! Сегодня изучаем идиомы русского языка. Ты когда-нибудь слышал выражение на русском, которое тебя озадачило?",
				"ro": "Bună! Azi descoperim expresii idiomatice în română. Ai auzit vreodată o expresie în română care te-a lăsat nedumerit?",
			},
			"cultural-food": {
				"es": "¡Hola! Hoy exploramos la gastronomía del mundo hispanohablante. ¿Qué platos en español conoces o te dan más curiosidad?",
				"it": "Ciao! Oggi esploriamo la gastronomia italiana. Quali piatti italiani conosci già o ti incuriosiscono di più?",
				"pt": "Olá! Hoje exploramos a gastronomia lusófona. Que pratos da cozinha portuguesa ou brasileira você conhece ou tem curiosidade?",
				"fr": "Bonjour! Aujourd'hui on explore la gastronomie francophone. Quels plats français connaissez-vous déjà ou vous intriguent?",
				"de": "Hallo! Heute erkunden wir die deutschsprachige Küche. Welche deutschen Gerichte kennst du schon oder interessieren dich?",
				"ja": "こんにちは！今日は日本の食文化を探りましょう。すでに知っている日本料理はありますか、それとも気になる料理は？",
				"zh": "你好！今天我们探索中国饮食文化。你已经了解哪些中国菜，或者最想了解哪些？",
				"ru": "Привет! Сегодня исследуем кухню русскоязычного мира. Какие блюда ты уже знаешь или тебе интересны?",
				"ro": "Bună! Azi explorăm gastronomia română. Ce mâncăruri românești cunoști deja sau te intrigă?",
			},
			"cultural-history": {
				"es": "¡Hola! Hoy exploramos la historia y tradiciones del mundo hispanohablante. ¿Hay algún período histórico o tradición que te genere especial curiosidad?",
				"it": "Ciao! Oggi esploriamo la storia e le tradizioni italiane. C'è un periodo storico o una tradizione che ti affascina in modo speciale?",
				"pt": "Olá! Hoje exploramos a história e tradições lusófonas. Há algum período histórico ou tradição que te gera especial curiosidade?",
				"fr": "Bonjour! Aujourd'hui on explore l'histoire et les traditions francophones. Y a-t-il une période historique ou une tradition qui vous fascine particulièrement?",
				"de": "Hallo! Heute erkunden wir die Geschichte und Traditionen der deutschsprachigen Welt. Gibt es eine Epoche oder Tradition, die dich besonders fasziniert?",
				"ja": "こんにちは！今日は日本の歴史と伝統を探りましょう。特に興味のある時代や伝統はありますか？",
				"zh": "你好！今天我们探索中国历史和传统。有没有特别让你感兴趣的历史时期或传统？",
				"ru": "Привет! Сегодня исследуем историю и традиции русскоязычного мира. Есть ли период или традиция, которые тебя особенно притягивают?",
				"ro": "Bună! Azi explorăm istoria și tradițiile lumii vorbitoare de română. Există o perioadă istorică sau o tradiție care te fascinează în mod special?",
			},
		}
		if byLang, ok := culturalGreets[topicID]; ok {
			if msg, ok := byLang[langCode]; ok {
				return msg
			}
		}
		return "Hello! I'm excited to explore the language and culture with you. What aspect are you most curious about?"
	}

	// ── Conversational topics ──────────────────────────────────────────────────
	// Beginner (levels 1-2): greeting + simple universal question in target language.
	// Advanced (levels 3-5): engaging topic-specific question in target language.

	greetings := map[string]string{
		"es": "¡Hola! Soy tu tutor de español.",
		"it": "Ciao! Sono il tuo tutor di italiano.",
		"pt": "Olá! Sou seu tutor de português.",
		"fr": "Bonjour! Je suis votre tuteur de français.",
		"de": "Hallo! Ich bin dein Deutschlehrer.",
		"ja": "こんにちは！日本語の先生です。",
		"zh": "你好！我是你的汉语老师。",
		"ru": "Привет! Я твой репетитор по русскому.",
		"ro": "Bună! Sunt tutorele tău de română.",
	}

	// beginnerQ: simple topic-relevant question in target language
	beginnerQ := map[string]map[string]string{
		"general": {
			"es": "¿Cómo te llamas?",
			"it": "Come ti chiami?",
			"pt": "Como você se chama?",
			"fr": "Comment vous appelez-vous?",
			"de": "Wie heißt du?",
			"ja": "お名前は何ですか？",
			"zh": "你叫什么名字？",
			"ru": "Как тебя зовут?",
			"ro": "Cum te numești?",
		},
		"daily-recap": {
			"es": "¿Cómo estás hoy?",
			"it": "Come stai oggi?",
			"pt": "Como você está hoje?",
			"fr": "Comment allez-vous aujourd'hui?",
			"de": "Wie geht es dir heute?",
			"ja": "今日はお元気ですか？",
			"zh": "你今天好吗？",
			"ru": "Как ты сегодня?",
			"ro": "Cum ești azi?",
		},
		"future-plans": {
			"es": "¿Qué te gusta hacer en tu tiempo libre?",
			"it": "Cosa ti piace fare nel tempo libero?",
			"pt": "O que você gosta de fazer no tempo livre?",
			"fr": "Qu'aimez-vous faire pendant votre temps libre?",
			"de": "Was machst du gerne in deiner Freizeit?",
			"ja": "暇な時間に何をするのが好きですか？",
			"zh": "你空闲时喜欢做什么？",
			"ru": "Что ты любишь делать в свободное время?",
			"ro": "Ce îți place să faci în timpul liber?",
		},
		"home": {
			"es": "¿Dónde vives?",
			"it": "Dove abiti?",
			"pt": "Onde você mora?",
			"fr": "Où habitez-vous?",
			"de": "Wo wohnst du?",
			"ja": "どこに住んでいますか？",
			"zh": "你住在哪里？",
			"ru": "Где ты живёшь?",
			"ro": "Unde locuiești?",
		},
		"food-dining": {
			"es": "¿Cuál es tu comida favorita?",
			"it": "Qual è il tuo cibo preferito?",
			"pt": "Qual é sua comida favorita?",
			"fr": "Quel est votre plat préféré?",
			"de": "Was ist dein Lieblingsessen?",
			"ja": "好きな食べ物は何ですか？",
			"zh": "你最喜欢的食物是什么？",
			"ru": "Какая твоя любимая еда?",
			"ro": "Care este mâncarea ta preferată?",
		},
		"shopping": {
			"es": "¿Te gusta ir de compras?",
			"it": "Ti piace fare shopping?",
			"pt": "Você gosta de fazer compras?",
			"fr": "Aimez-vous faire du shopping?",
			"de": "Magst du einkaufen gehen?",
			"ja": "買い物は好きですか？",
			"zh": "你喜欢购物吗？",
			"ru": "Тебе нравится делать покупки?",
			"ro": "Îți place să faci cumpărături?",
		},
		"family": {
			"es": "¿Tienes hermanos o hermanas?",
			"it": "Hai fratelli o sorelle?",
			"pt": "Você tem irmãos ou irmãs?",
			"fr": "Avez-vous des frères et sœurs?",
			"de": "Hast du Geschwister?",
			"ja": "兄弟や姉妹がいますか？",
			"zh": "你有兄弟姐妹吗？",
			"ru": "У тебя есть братья или сёстры?",
			"ro": "Ai frați sau surori?",
		},
		"culture": {
			"es": "¿Qué música te gusta?",
			"it": "Che tipo di musica ti piace?",
			"pt": "Que tipo de música você gosta?",
			"fr": "Quel genre de musique aimez-vous?",
			"de": "Welche Musik magst du?",
			"ja": "どんな音楽が好きですか？",
			"zh": "你喜欢什么音乐？",
			"ru": "Какую музыку ты любишь?",
			"ro": "Ce tip de muzică îți place?",
		},
		"sports": {
			"es": "¿Te gustan los deportes?",
			"it": "Ti piacciono gli sport?",
			"pt": "Você gosta de esportes?",
			"fr": "Aimez-vous le sport?",
			"de": "Magst du Sport?",
			"ja": "スポーツは好きですか？",
			"zh": "你喜欢运动吗？",
			"ru": "Тебе нравится спорт?",
			"ro": "Îți place sportul?",
		},
		"entertainment": {
			"es": "¿Qué películas te gustan?",
			"it": "Che tipo di film ti piacciono?",
			"pt": "Que filmes você gosta?",
			"fr": "Quels films aimez-vous?",
			"de": "Welche Filme magst du?",
			"ja": "どんな映画が好きですか？",
			"zh": "你喜欢什么类型的电影？",
			"ru": "Какие фильмы тебе нравятся?",
			"ro": "Ce tipuri de filme îți plac?",
		},
		"news": {
			"es": "¿Ves las noticias?",
			"it": "Guardi le notizie?",
			"pt": "Você acompanha as notícias?",
			"fr": "Regardez-vous les informations?",
			"de": "Schaust du Nachrichten?",
			"ja": "ニュースを見ますか？",
			"zh": "你看新闻吗？",
			"ru": "Ты смотришь новости?",
			"ro": "Te uiți la știri?",
		},
		"travel": {
			"es": "¿Has viajado al extranjero?",
			"it": "Hai viaggiato all'estero?",
			"pt": "Você já viajou para o exterior?",
			"fr": "Avez-vous voyagé à l'étranger?",
			"de": "Bist du schon ins Ausland gereist?",
			"ja": "海外に行ったことがありますか？",
			"zh": "你去过国外吗？",
			"ru": "Ты ездил за рубеж?",
			"ro": "Ai călătorit în străinătate?",
		},
		"environment": {
			"es": "¿Te preocupa el medioambiente?",
			"it": "Ti preoccupi per l'ambiente?",
			"pt": "Você se preocupa com o meio ambiente?",
			"fr": "Vous préoccupez-vous de l'environnement?",
			"de": "Machst du dir Sorgen um die Umwelt?",
			"ja": "環境問題は気になりますか？",
			"zh": "你关心环境问题吗？",
			"ru": "Тебя беспокоит экология?",
			"ro": "Ești îngrijorat de mediu?",
		},
		"health": {
			"es": "¿Haces ejercicio regularmente?",
			"it": "Fai sport regolarmente?",
			"pt": "Você pratica exercícios regularmente?",
			"fr": "Faites-vous de l'exercice régulièrement?",
			"de": "Treibst du regelmäßig Sport?",
			"ja": "定期的に運動していますか？",
			"zh": "你经常锻炼吗？",
			"ru": "Ты регулярно занимаешься спортом?",
			"ro": "Faci sport în mod regulat?",
		},
		"education": {
			"es": "¿Qué estudias o estudiaste?",
			"it": "Cosa studi o hai studiato?",
			"pt": "O que você estuda ou estudou?",
			"fr": "Qu'étudiez-vous ou qu'avez-vous étudié?",
			"de": "Was studierst du oder hast du studiert?",
			"ja": "何を勉強していますか？",
			"zh": "你学什么专业？",
			"ru": "Что ты изучаешь или изучал?",
			"ro": "Ce studiezi sau ai studiat?",
		},
		"work": {
			"es": "¿En qué trabajas?",
			"it": "Che lavoro fai?",
			"pt": "Em que você trabalha?",
			"fr": "Quel est votre métier?",
			"de": "Was machst du beruflich?",
			"ja": "お仕事は何ですか？",
			"zh": "你是做什么工作的？",
			"ru": "Кем ты работаешь?",
			"ro": "Ce muncă faci?",
		},
		"technology": {
			"es": "¿Usas mucho el teléfono inteligente?",
			"it": "Usi molto lo smartphone?",
			"pt": "Você usa muito o smartphone?",
			"fr": "Utilisez-vous beaucoup votre smartphone?",
			"de": "Benutzt du viel dein Smartphone?",
			"ja": "スマートフォンをよく使いますか？",
			"zh": "你经常用智能手机吗？",
			"ru": "Ты много пользуешься смартфоном?",
			"ro": "Folosești mult smartphone-ul?",
		},
		"cloud": {
			"es": "¿Usas computadoras en tu trabajo?",
			"it": "Usi computer nel tuo lavoro?",
			"pt": "Você usa computadores no trabalho?",
			"fr": "Utilisez-vous des ordinateurs au travail?",
			"de": "Benutzt du Computer bei der Arbeit?",
			"ja": "仕事でコンピューターを使いますか？",
			"zh": "你工作中用电脑吗？",
			"ru": "Ты используешь компьютер на работе?",
			"ro": "Folosești computere la muncă?",
		},
		"marketing": {
			"es": "¿Qué anuncios o marcas te llaman la atención?",
			"it": "Quali pubblicità o marchi ti colpiscono?",
			"pt": "Quais anúncios ou marcas chamam sua atenção?",
			"fr": "Quelles publicités ou marques attirent votre attention?",
			"de": "Welche Werbung oder Marken fallen dir auf?",
			"ja": "どんな広告やブランドが気になりますか？",
			"zh": "哪些广告或品牌引起了你的注意？",
			"ru": "Какая реклама или бренды привлекают твоё внимание?",
			"ro": "Ce reclame sau mărci îți atrag atenția?",
		},
		"finance": {
			"es": "¿Sueles ahorrar dinero?",
			"it": "Di solito risparmi denaro?",
			"pt": "Você costuma guardar dinheiro?",
			"fr": "Économisez-vous habituellement de l'argent?",
			"de": "Sparst du normalerweise Geld?",
			"ja": "普段お金を貯めていますか？",
			"zh": "你平时存钱吗？",
			"ru": "Ты обычно откладываешь деньги?",
			"ro": "De obicei economisești bani?",
		},
	}

	// advancedQ: topic-specific engaging question (level 3-5)
	advancedQ := map[string]map[string]string{
		"general": {
			"es": "¡Bienvenido! Cuéntame sobre ti. ¿De dónde eres y qué te motivó a aprender español?",
			"it": "Benvenuto! Raccontami di te. Da dove vieni e cosa ti ha spinto a imparare l'italiano?",
			"pt": "Bem-vindo! Me conta sobre você. De onde você é e o que te motivou a aprender português?",
			"fr": "Bienvenue! Parlez-moi de vous. D'où venez-vous et qu'est-ce qui vous a poussé à apprendre le français?",
			"de": "Willkommen! Erzähl mir von dir. Woher kommst du und was hat dich dazu gebracht, Deutsch zu lernen?",
			"ja": "ようこそ！あなたについて教えてください。どこから来ましたか、日本語を学ぼうと思ったきっかけは？",
			"zh": "欢迎！跟我说说你自己吧。你来自哪里，是什么让你想学汉语的？",
			"ru": "Добро пожаловать! Расскажи о себе. Откуда ты и что побудило тебя учить русский?",
			"ro": "Bun venit! Spune-mi despre tine. De unde ești și ce te-a motivat să înveți română?",
		},
		"daily-recap": {
			"es": "¡Hola! ¿Qué tal el día? Cuéntame algo interesante o inesperado que hayas vivido hoy.",
			"it": "Ciao! Com'è andata la giornata? Raccontami qualcosa di interessante o inaspettato.",
			"pt": "Olá! Como foi o dia? Me conta algo interessante ou inesperado que aconteceu hoje.",
			"fr": "Bonjour! Comment s'est passée votre journée? Racontez-moi quelque chose d'intéressant ou d'inattendu.",
			"de": "Hallo! Wie war dein Tag? Erzähl mir etwas Interessantes oder Unerwartetes, das heute passiert ist.",
			"ja": "こんにちは！今日はどんな一日でしたか？何か面白いことや意外なことはありましたか？",
			"zh": "你好！今天过得怎么样？有没有什么有趣或出乎意料的事情发生？",
			"ru": "Привет! Как прошёл день? Расскажи что-нибудь интересное или неожиданное, что случилось сегодня.",
			"ro": "Bună! Cum a decurs ziua? Povestește-mi ceva interesant sau neașteptat care s-a întâmplat azi.",
		},
		"future-plans": {
			"es": "¿Qué sueños o planes tienes para los próximos años? ¿Hay algo que quieras lograr o explorar?",
			"it": "Quali sogni o piani hai per i prossimi anni? C'è qualcosa che vuoi realizzare o esplorare?",
			"pt": "Que sonhos ou planos você tem para os próximos anos? Há algo que queira alcançar ou explorar?",
			"fr": "Quels rêves ou projets avez-vous pour les prochaines années? Y a-t-il quelque chose que vous voulez accomplir?",
			"de": "Welche Träume oder Pläne hast du für die nächsten Jahre? Gibt es etwas, das du erreichen oder erkunden möchtest?",
			"ja": "これからの数年間でどんな夢や計画がありますか？実現したいことや探求したいことは何ですか？",
			"zh": "你对未来几年有什么梦想或计划？有什么想要实现或探索的事情吗？",
			"ru": "Какие мечты или планы есть у тебя на ближайшие годы? Есть что-то, чего ты хочешь достичь?",
			"ro": "Ce vise sau planuri ai pentru următorii ani? Există ceva ce vrei să realizezi sau să explorezi?",
		},
		"home": {
			"es": "Descríbeme el lugar ideal donde te gustaría vivir. ¿Qué lo haría perfecto para ti?",
			"it": "Descrivimi il posto ideale dove vorresti vivere. Cosa lo renderebbe perfetto per te?",
			"pt": "Descreva o lugar ideal onde gostaria de morar. Que características tornariam esse lugar perfeito para você?",
			"fr": "Décrivez l'endroit idéal où vous aimeriez vivre. Quelles caractéristiques le rendraient parfait pour vous?",
			"de": "Beschreib mir den idealen Ort, an dem du leben möchtest. Was würde ihn für dich perfekt machen?",
			"ja": "あなたの理想の住まいはどんな場所ですか？何があればあなたにとって完璧な場所になりますか？",
			"zh": "描述一下你理想的居住地。是什么让那个地方对你来说完美？",
			"ru": "Опиши идеальное место, где ты хотел бы жить. Что сделало бы его для тебя идеальным?",
			"ro": "Descrie-mi locul ideal unde ți-ar plăcea să locuiești. Ce l-ar face perfect pentru tine?",
		},
		"food-dining": {
			"es": "¿Cuál es el plato que más te identifica o el que más recuerdos te trae? ¿Qué historia hay detrás?",
			"it": "Qual è il piatto che più ti rappresenta o che più ricordi ti evoca? Che storia c'è dietro?",
			"pt": "Qual é o prato que mais te representa ou traz mais lembranças? Qual é a história por trás?",
			"fr": "Quel est le plat qui vous représente le mieux ou qui vous rappelle le plus de souvenirs? Quelle est son histoire?",
			"de": "Welches Gericht steht am meisten für dich oder ruft die meisten Erinnerungen hervor? Was steckt dahinter?",
			"ja": "あなたらしさを最もよく表す料理や、一番懐かしい気持ちにさせる料理は何ですか？その食べ物にはどんな思い出がありますか？",
			"zh": "最能代表你的菜肴或让你有最多回忆的食物是什么？这道菜背后有什么故事？",
			"ru": "Какое блюдо больше всего характеризует тебя или приносит больше всего воспоминаний? Какая история за ним стоит?",
			"ro": "Care este mâncarea care te caracterizează cel mai bine sau îți aduce cele mai multe amintiri? Ce poveste stă în spatele ei?",
		},
		"shopping": {
			"es": "¿Cómo tomas la decisión de comprar algo — por impulso o con mucha reflexión? ¿Qué dice eso de ti?",
			"it": "Come decidi di comprare qualcosa — per impulso o dopo riflessione? Cosa dice questo di te?",
			"pt": "Como você decide comprar algo — por impulso ou com reflexão? O que isso diz sobre você?",
			"fr": "Comment décidez-vous d'acheter quelque chose — par impulsion ou après réflexion? Qu'est-ce que cela dit de vous?",
			"de": "Wie entscheidest du dich etwas zu kaufen — impulsiv oder nach langem Nachdenken? Was sagt das über dich aus?",
			"ja": "何かを買うときはどう決めますか——衝動買いですか、じっくり考えてからですか？それはあなたについて何を教えてくれますか？",
			"zh": "你如何决定购买某物——是冲动消费还是深思熟虑？这说明了你什么？",
			"ru": "Как ты принимаешь решение о покупке — импульсивно или после долгих раздумий? Что это говорит о тебе?",
			"ro": "Cum decizi să cumperi ceva — impulsiv sau după multă gândire? Ce spune asta despre tine?",
		},
		"family": {
			"es": "Cuéntame sobre tu familia. ¿Qué tradiciones o valores han pasado de generación en generación?",
			"it": "Parlami della tua famiglia. Quali tradizioni o valori si sono tramandati di generazione in generazione?",
			"pt": "Fale sobre sua família. Que tradições ou valores foram passados de geração em geração?",
			"fr": "Parlez-moi de votre famille. Quelles traditions ou valeurs se sont transmises de génération en génération?",
			"de": "Erzähl mir von deiner Familie. Welche Traditionen oder Werte werden von Generation zu Generation weitergegeben?",
			"ja": "あなたの家族について話してください。世代から世代へと受け継がれている伝統や価値観はありますか？",
			"zh": "跟我说说你的家人吧。有什么传统或价值观是世代相传的？",
			"ru": "Расскажи мне о своей семье. Какие традиции или ценности передаются из поколения в поколение?",
			"ro": "Povestește-mi despre familia ta. Ce tradiții sau valori s-au transmis din generație în generație?",
		},
		"culture": {
			"es": "¿Hay alguna expresión artística o tradición del mundo hispanohablante que te fascine especialmente?",
			"it": "C'è qualche espressione artistica o tradizione italiana che ti affascina in modo particolare?",
			"pt": "Há alguma expressão artística ou tradição lusófona que te chame a atenção de forma especial?",
			"fr": "Y a-t-il une expression artistique ou une tradition francophone qui vous fascine particulièrement?",
			"de": "Gibt es einen künstlerischen Ausdruck oder eine Tradition der deutschsprachigen Welt, die dich besonders fasziniert?",
			"ja": "日本の文化の中で特に魅了される芸術表現や伝統はありますか？",
			"zh": "中文世界里有什么特别吸引你的艺术表现或文化传统吗？",
			"ru": "Есть ли в русскоязычной культуре художественное выражение или традиция, которые особенно тебя привлекают?",
			"ro": "Există vreo expresie artistică sau tradiție din lumea vorbitoare de română care te fascinează în mod special?",
		},
		"sports": {
			"es": "¿Qué papel juega el deporte en tu vida — como practicante, aficionado o espectador ocasional?",
			"it": "Che ruolo ha lo sport nella tua vita — come praticante, tifoso o semplice spettatore?",
			"pt": "Que papel o esporte desempenha na sua vida — como praticante, fã ou espectador ocasional?",
			"fr": "Quel rôle joue le sport dans votre vie — en tant que pratiquant, fan ou spectateur occasionnel?",
			"de": "Welche Rolle spielt Sport in deinem Leben — als Sportler, Fan oder gelegentlicher Zuschauer?",
			"ja": "スポーツはあなたの生活でどんな役割を果たしていますか——選手として、ファンとして、たまの観戦者として？",
			"zh": "体育在你生活中扮演什么角色——作为运动员、球迷，还是偶尔的观众？",
			"ru": "Какую роль спорт играет в твоей жизни — как спортсмена, болельщика или просто зрителя?",
			"ro": "Ce rol joacă sportul în viața ta — ca practicant, fan sau spectator ocazional?",
		},
		"entertainment": {
			"es": "¿Qué película, serie o música te ha marcado recientemente y por qué te resonó tan profundamente?",
			"it": "Quale film, serie o musica ti ha colpito di recente e perché ti ha toccato così profondamente?",
			"pt": "Que filme, série ou música te marcou recentemente e por que ressoou tão profundamente?",
			"fr": "Quel film, série ou morceau de musique vous a marqué récemment et pourquoi vous a-t-il touché si profondément?",
			"de": "Welcher Film, welche Serie oder Musik hat dich zuletzt beeindruckt und warum hat sie dich so berührt?",
			"ja": "最近感銘を受けた映画、ドラマ、または音楽は何ですか？なぜそんなに深く心に響きましたか？",
			"zh": "最近有什么电影、剧集或音乐给你留下了深刻印象？为什么它对你触动如此之深？",
			"ru": "Какой фильм, сериал или музыка произвели на тебя впечатление в последнее время и почему они так тебя задели?",
			"ro": "Ce film, serial sau muzică te-a impresionat recent și de ce ți-a rezonat atât de profund?",
		},
		"news": {
			"es": "¿Qué tema de la actualidad te genera más reflexión? ¿Cómo te mantienes informado sin abrumarte?",
			"it": "Quale argomento di attualità ti fa riflettere di più? Come ti mantieni informato senza sentirti sopraffatto?",
			"pt": "Que tema atual te gera mais reflexão? Como você se mantém informado sem se sobrecarregar?",
			"fr": "Quel sujet d'actualité vous fait le plus réfléchir? Comment restez-vous informé sans vous sentir submergé?",
			"de": "Welches aktuelle Thema regt dich am meisten zum Nachdenken an? Wie bleibst du informiert, ohne dich zu überlasten?",
			"ja": "どんな時事問題が最も考えさせられますか？情報過多にならずにどうやって情報収集していますか？",
			"zh": "哪个当前话题让你最有思考？你如何保持信息灵通而不被淹没？",
			"ru": "Какое текущее событие заставляет тебя больше всего задуматься? Как ты остаёшься в курсе, не перегружаясь?",
			"ro": "Ce subiect actual te face să reflectezi cel mai mult? Cum te menții informat fără să te simți copleșit?",
		},
		"travel": {
			"es": "¿Cuál ha sido tu experiencia de viaje más memorable? ¿Qué descubriste sobre ti mismo en ese viaje?",
			"it": "Qual è stata la tua esperienza di viaggio più memorabile? Cosa hai scoperto su te stesso?",
			"pt": "Qual foi a sua experiência de viagem mais memorável? O que você descobriu sobre si mesmo nessa viagem?",
			"fr": "Quelle a été votre expérience de voyage la plus mémorable? Qu'avez-vous découvert sur vous-même?",
			"de": "Was war deine unvergesslichste Reiseerfahrung? Was hast du dabei über dich selbst entdeckt?",
			"ja": "最も忘れられない旅の経験はどんなものでしたか？その旅で自分自身について何か発見しましたか？",
			"zh": "你最难忘的旅行经历是什么？在那次旅行中你对自己有什么发现？",
			"ru": "Какое путешествие было самым незабываемым? Что ты открыл о себе в этой поездке?",
			"ro": "Care a fost cea mai memorabilă experiență de călătorie a ta? Ce ai descoperit despre tine?",
		},
		"environment": {
			"es": "¿Cómo ves el equilibrio entre crecimiento económico y sostenibilidad ambiental? ¿Qué cambios has adoptado?",
			"it": "Come vedi l'equilibrio tra crescita economica e sostenibilità ambientale? Che cambiamenti hai adottato?",
			"pt": "Como você vê o equilíbrio entre crescimento econômico e sustentabilidade ambiental? Que mudanças você adotou?",
			"fr": "Comment voyez-vous l'équilibre entre croissance économique et durabilité? Quels changements avez-vous adoptés?",
			"de": "Wie siehst du das Gleichgewicht zwischen Wirtschaftswachstum und Umweltnachhaltigkeit? Welche Veränderungen hast du angenommen?",
			"ja": "経済成長と環境の持続可能性のバランスについてどう思いますか？生活の中でどんな変化を取り入れましたか？",
			"zh": "你如何看待经济增长与环境可持续性的平衡？你在生活中做出了哪些改变？",
			"ru": "Как ты видишь баланс между экономическим ростом и экологической устойчивостью? Какие изменения ты принял?",
			"ro": "Cum vezi echilibrul dintre creșterea economică și sustenabilitatea mediului? Ce schimbări ai adoptat în viața ta?",
		},
		"health": {
			"es": "¿Qué hábitos son los pilares de tu bienestar físico y mental? ¿Cuál te costó más desarrollar?",
			"it": "Quali abitudini sono i pilastri del tuo benessere fisico e mentale? Quale hai fatto più fatica a sviluppare?",
			"pt": "Que hábitos são os pilares do seu bem-estar físico e mental? Qual foi mais difícil de desenvolver?",
			"fr": "Quelles habitudes sont les piliers de votre bien-être physique et mental? Laquelle a été la plus difficile à développer?",
			"de": "Welche Gewohnheiten sind die Grundlage deines Wohlbefindens? Welche war am schwierigsten zu entwickeln?",
			"ja": "体と心の健康を支える習慣は何ですか？一番身につけるのが難しかったのはどれですか？",
			"zh": "哪些习惯是你身心健康的支柱？哪个最难养成？",
			"ru": "Какие привычки являются основой твоего благополучия? Какую из них было труднее всего выработать?",
			"ro": "Ce obiceiuri sunt pilonii bunăstării tale fizice și mentale? Care a fost cel mai greu de dezvoltat?",
		},
		"education": {
			"es": "¿Qué aprendizaje ha marcado más tu vida — dentro o fuera del aula? ¿Qué te reveló sobre ti mismo?",
			"it": "Quale apprendimento ha segnato di più la tua vita — dentro o fuori dall'aula? Cosa ti ha rivelato di te?",
			"pt": "Qual aprendizado marcou mais sua vida — dentro ou fora da sala de aula? O que revelou sobre você?",
			"fr": "Quel apprentissage a le plus marqué votre vie — en classe ou en dehors? Qu'est-ce que cela vous a révélé?",
			"de": "Welches Lernen hat dein Leben am meisten geprägt — im Unterricht oder außerhalb? Was hat es dir über dich offenbart?",
			"ja": "教室の内外を問わず、あなたの人生に最も影響を与えた学びは何ですか？自分について何が分かりましたか？",
			"zh": "无论在课堂内外，哪种学习经历对你的生活影响最大？它揭示了你什么？",
			"ru": "Какое обучение оказало наибольшее влияние на твою жизнь — в классе или за его пределами?",
			"ro": "Ce învățare ți-a marcat cel mai mult viața — în clasă sau în afara ei? Ce ți-a dezvăluit despre tine?",
		},
		"work": {
			"es": "¿Cómo describirías tu trayectoria profesional? ¿Qué te ha enseñado sobre tus prioridades y lo que realmente valoras?",
			"it": "Come describeresti la tua carriera? Cosa ti ha insegnato sulle tue priorità e su ciò che davvero valorizzi?",
			"pt": "Como você descreveria sua trajetória profissional? O que te ensinou sobre suas prioridades?",
			"fr": "Comment décririez-vous votre parcours professionnel? Qu'est-ce que cela vous a appris sur vos priorités?",
			"de": "Wie würdest du deinen beruflichen Werdegang beschreiben? Was hat er dir über deine Prioritäten gelehrt?",
			"ja": "あなたのキャリアをどのように表現しますか？それはあなたの優先事項について何を教えてくれましたか？",
			"zh": "你会如何描述你的职业经历？它教会了你什么关于你的优先事项的东西？",
			"ru": "Как бы ты описал свой профессиональный путь? Чему он тебя научил о твоих приоритетах?",
			"ro": "Cum ți-ai descrie parcursul profesional? Ce ți-a arătat despre prioritățile și valorile tale?",
		},
		"technology": {
			"es": "¿Cómo ha transformado la tecnología tu manera de comunicarte, trabajar o aprender? ¿Hay algo que hayas ganado o perdido?",
			"it": "Come ha trasformato la tecnologia il tuo modo di comunicare, lavorare o imparare? C'è qualcosa che hai guadagnato o perso?",
			"pt": "Como a tecnologia transformou sua maneira de comunicar, trabalhar ou aprender? Algo que ganhou ou perdeu?",
			"fr": "Comment la technologie a-t-elle transformé votre façon de communiquer ou de travailler? Y a-t-il quelque chose que vous avez gagné ou perdu?",
			"de": "Wie hat die Technologie deine Art zu kommunizieren oder zu arbeiten verändert? Hast du etwas gewonnen oder verloren?",
			"ja": "技術はあなたのコミュニケーション、仕事、学習をどのように変えましたか？何か得たものや失ったものはありますか？",
			"zh": "技术如何改变了你的沟通、工作或学习方式？有什么你得到的或失去的？",
			"ru": "Как технологии изменили твой способ общения и работы? Есть что-то, что ты приобрёл или потерял?",
			"ro": "Cum a transformat tehnologia modul în care comunici sau lucrezi? Există ceva ce ai câștigat sau pierdut?",
		},
		"cloud": {
			"es": "¿Qué herramientas o plataformas digitales son indispensables en tu entorno laboral y por qué las elegiste?",
			"it": "Quali strumenti o piattaforme digitali sono indispensabili nel tuo lavoro e perché li hai scelti?",
			"pt": "Que ferramentas ou plataformas digitais são indispensáveis no seu trabalho e por que as escolheu?",
			"fr": "Quels outils ou plateformes numériques sont indispensables dans votre travail et pourquoi les avez-vous choisis?",
			"de": "Welche digitalen Tools oder Plattformen sind in deinem Arbeitsumfeld unverzichtbar und warum hast du sie gewählt?",
			"ja": "あなたの仕事に欠かせないデジタルツールやプラットフォームは何ですか？なぜそれらを選びましたか？",
			"zh": "在你的工作中，哪些数字工具或平台是不可缺少的？为什么选择它们？",
			"ru": "Какие цифровые инструменты или платформы незаменимы в твоей работе и почему ты их выбрал?",
			"ro": "Ce instrumente sau platforme digitale sunt indispensabile în munca ta și de ce le-ai ales?",
		},
		"marketing": {
			"es": "¿Cómo definirías una estrategia de marketing auténtica en un mundo saturado de contenido? ¿Qué es lo que realmente conecta?",
			"it": "Come definiresti una strategia di marketing autentica in un mondo saturo di contenuti? Cosa crea davvero connessione?",
			"pt": "Como você definiria uma estratégia de marketing autêntica num mundo saturado de conteúdo? O que realmente conecta?",
			"fr": "Comment définiriez-vous une stratégie marketing authentique dans un monde saturé de contenu? Qu'est-ce qui crée vraiment de la connexion?",
			"de": "Wie würdest du eine authentische Marketingstrategie in einer von Inhalten gesättigten Welt definieren? Was schafft echte Verbindung?",
			"ja": "コンテンツが溢れる世界で、本物のマーケティング戦略とはどのようなものでしょうか？本当に人々とつながるものは何ですか？",
			"zh": "在内容泛滥的世界中，你如何定义真正的营销策略？什么才能真正建立连接？",
			"ru": "Как ты определил бы подлинную маркетинговую стратегию в перегруженном контентом мире? Что создаёт настоящую связь?",
			"ro": "Cum ai defini o strategie de marketing autentică într-o lume saturată de conținut? Ce creează cu adevărat conexiune?",
		},
		"finance": {
			"es": "¿Cómo equilibras tus decisiones financieras con tus valores personales y metas de largo plazo?",
			"it": "Come bilanci le tue decisioni finanziarie con i tuoi valori personali e obiettivi a lungo termine?",
			"pt": "Como você equilibra suas decisões financeiras com seus valores pessoais e metas de longo prazo?",
			"fr": "Comment équilibrez-vous vos décisions financières avec vos valeurs personnelles et objectifs à long terme?",
			"de": "Wie balancierst du deine finanziellen Entscheidungen mit deinen persönlichen Werten und langfristigen Zielen?",
			"ja": "財務上の決断を個人的な価値観や長期的な目標とどのようにバランスを取っていますか？",
			"zh": "你如何将财务决策与个人价值观和长期目标相平衡？",
			"ru": "Как ты балансируешь финансовые решения со своими личными ценностями и долгосрочными целями?",
			"ro": "Cum echilibrezi deciziile tale financiare cu valorile personale și obiectivele pe termen lung?",
		},
	}

	// Build the message from greeting + question
	g := greetings[langCode]
	if g == "" {
		g = "Hello! I'm your language tutor."
	}

	var q string
	if level <= 2 {
		if qs, ok := beginnerQ[topicID]; ok {
			q = qs[langCode]
		}
		if q == "" {
			q = "What's your name?"
		}
	} else {
		if qs, ok := advancedQ[topicID]; ok {
			q = qs[langCode]
		}
		if q == "" {
			q = "What would you like to talk about today?"
		}
	}

	return g + " " + q
}

// agentBasePrompt is the static base prompt stored on the ElevenLabs agent.
// Per-session instructions are injected dynamically via conversation_config_override.
const agentBasePrompt = `You are LinguaAI, an expert 1-on-1 language tutor. At the start of each conversation you will receive a detailed system prompt with specific instructions about the student's language, proficiency level, learning mode, and topic. Follow those instructions precisely for the entire session.

Core rules that always apply:
- This is a VOICE conversation. Keep every response short and natural — 1 to 3 sentences maximum.
- Never use markdown, asterisks, bullet points, numbered lists, or any formatting. Speak as you would out loud.
- MANDATORY: Every single response without exception must end with either a direct question, a request to repeat a phrase, a prompt to try something, or an invitation to respond. Never end a response with a statement — always hand the turn back to the student.
- Corrections must be brief and woven naturally into your reply — never stop to lecture.
- Adapt your language complexity precisely to the student's level.`
