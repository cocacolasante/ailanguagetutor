package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
)

type TTSHandler struct {
	cfg    *config.Config
	client *http.Client
}

func NewTTSHandler(cfg *config.Config) *TTSHandler {
	return &TTSHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type ttsRequest struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

type elevenLabsBody struct {
	Text          string        `json:"text"`
	ModelID       string        `json:"model_id"`
	VoiceSettings voiceSettings `json:"voice_settings"`
}

type voiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	UseSpeakerBoost bool    `json:"use_speaker_boost"`
}

func (h *TTSHandler) Convert(w http.ResponseWriter, r *http.Request) {
	// Verify auth (context value set by middleware)
	_ = r.Context().Value(middleware.UserIDKey)

	var req ttsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}

	voiceID := h.voiceFor(req.Language)
	elBody := elevenLabsBody{
		Text:    req.Text,
		ModelID: h.cfg.ElevenLabsModel,
		VoiceSettings: voiceSettings{
			Stability:       0.5,
			SimilarityBoost: 0.75,
			Style:           0.0,
			UseSpeakerBoost: true,
		},
	}

	bodyBytes, err := json.Marshal(elBody)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s/stream", voiceID)
	elReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, `{"error":"failed to build TTS request"}`, http.StatusInternalServerError)
		return
	}
	elReq.Header.Set("Content-Type", "application/json")
	elReq.Header.Set("xi-api-key", h.cfg.ElevenLabsAPIKey)
	elReq.Header.Set("Accept", "audio/mpeg")

	resp, err := h.client.Do(elReq)
	if err != nil {
		log.Printf("ElevenLabs request error: %v", err)
		http.Error(w, `{"error":"TTS service error"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("ElevenLabs error %d: %s", resp.StatusCode, string(body))
		http.Error(w, `{"error":"TTS service unavailable"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Transfer-Encoding", "chunked")
	_, _ = io.Copy(w, resp.Body)
}

func (h *TTSHandler) voiceFor(lang string) string {
	switch lang {
	case "it":
		return h.cfg.ElevenLabsVoiceIT
	case "es":
		return h.cfg.ElevenLabsVoiceES
	case "pt":
		return h.cfg.ElevenLabsVoicePT
	case "fr":
		return h.cfg.ElevenLabsVoiceFR
	case "de":
		return h.cfg.ElevenLabsVoiceDE
	case "ja":
		return h.cfg.ElevenLabsVoiceJA
	case "ru":
		return h.cfg.ElevenLabsVoiceRU
	case "ro":
		return h.cfg.ElevenLabsVoiceRO
	case "zh":
		return h.cfg.ElevenLabsVoiceZH
	default:
		return h.cfg.ElevenLabsVoiceIT
	}
}
