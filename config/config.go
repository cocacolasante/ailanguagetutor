package config

import (
	"log"
	"os"
)

type Config struct {
	Port string

	JWTSecret string

	IONOSAPIKey  string
	IONOSBaseURL string
	IONOSModel   string

	ElevenLabsAPIKey  string
	ElevenLabsAgentID string
	ElevenLabsVoiceIT string
	ElevenLabsVoiceES string
	ElevenLabsVoicePT string
	ElevenLabsVoiceFR string
	ElevenLabsVoiceDE string
	ElevenLabsVoiceJA string
	ElevenLabsVoiceRU string
	ElevenLabsVoiceRO string
	ElevenLabsVoiceZH string
	ElevenLabsModel   string

	StripeSecretKey     string
	StripeWebhookSecret string
	StripePriceID       string
	AppBaseURL          string

	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	EmailFrom    string
}

func Load() *Config {
	return &Config{
		Port:      getEnv("PORT", "8080"),
		JWTSecret: getEnvRequired("JWT_SECRET"),

		IONOSAPIKey:  getEnvRequired("IONOS_API_KEY"),
		IONOSBaseURL: getEnv("IONOS_BASE_URL", "https://openai.inference.de-txl.ionos.com/v1"),
		IONOSModel:   getEnv("IONOS_MODEL", "mistral-small-24b"),

		ElevenLabsAPIKey:  getEnvRequired("ELEVENLABS_API_KEY"),
		ElevenLabsAgentID: getEnv("ELEVENLABS_AGENT_ID", ""),
		// Default to ElevenLabs multilingual voice (Rachel) — override with native voices
		ElevenLabsVoiceIT: getEnv("ELEVENLABS_VOICE_IT", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceES: getEnv("ELEVENLABS_VOICE_ES", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoicePT: getEnv("ELEVENLABS_VOICE_PT", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceFR: getEnv("ELEVENLABS_VOICE_FR", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceDE: getEnv("ELEVENLABS_VOICE_DE", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceJA: getEnv("ELEVENLABS_VOICE_JA", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceRU: getEnv("ELEVENLABS_VOICE_RU", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceRO: getEnv("ELEVENLABS_VOICE_RO", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceZH: getEnv("ELEVENLABS_VOICE_ZH", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsModel:   getEnv("ELEVENLABS_MODEL", "eleven_multilingual_v2"),

		StripeSecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePriceID:       getEnv("STRIPE_PRICE_ID", ""),
		AppBaseURL:          getEnv("APP_BASE_URL", "http://localhost:8080"),

		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getEnv("SMTP_PORT", "587"),
		SMTPUsername: getEnv("SMTP_USERNAME", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		EmailFrom:    getEnv("EMAIL_FROM", ""),
	}
}

// VoiceForLanguage returns the configured ElevenLabs voice ID for a language code.
func (c *Config) VoiceForLanguage(langCode string) string {
	switch langCode {
	case "it":
		return c.ElevenLabsVoiceIT
	case "es":
		return c.ElevenLabsVoiceES
	case "pt":
		return c.ElevenLabsVoicePT
	case "fr":
		return c.ElevenLabsVoiceFR
	case "de":
		return c.ElevenLabsVoiceDE
	case "ja":
		return c.ElevenLabsVoiceJA
	case "ru":
		return c.ElevenLabsVoiceRU
	case "ro":
		return c.ElevenLabsVoiceRO
	case "zh":
		return c.ElevenLabsVoiceZH
	default:
		return "21m00Tcm4TlvDq8ikWAM" // Rachel multilingual fallback
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return v
}
