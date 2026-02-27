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
	ElevenLabsVoiceIT string
	ElevenLabsVoiceES string
	ElevenLabsVoicePT string
	ElevenLabsModel   string

	StripeSecretKey     string
	StripeWebhookSecret string
	StripePriceID       string
	AppBaseURL          string
}

func Load() *Config {
	return &Config{
		Port:      getEnv("PORT", "8080"),
		JWTSecret: getEnvRequired("JWT_SECRET"),

		IONOSAPIKey:  getEnvRequired("IONOS_API_KEY"),
		IONOSBaseURL: getEnv("IONOS_BASE_URL", "https://openai.inference.de-txl.ionos.com/v1"),
		IONOSModel:   getEnv("IONOS_MODEL", "mistral-small-24b"),

		ElevenLabsAPIKey: getEnvRequired("ELEVENLABS_API_KEY"),
		// Default to ElevenLabs multilingual voice (Rachel)
		ElevenLabsVoiceIT: getEnv("ELEVENLABS_VOICE_IT", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoiceES: getEnv("ELEVENLABS_VOICE_ES", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsVoicePT: getEnv("ELEVENLABS_VOICE_PT", "21m00Tcm4TlvDq8ikWAM"),
		ElevenLabsModel:   getEnv("ELEVENLABS_MODEL", "eleven_multilingual_v2"),

		StripeSecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePriceID:       getEnv("STRIPE_PRICE_ID", ""),
		AppBaseURL:          getEnv("APP_BASE_URL", "http://localhost:8080"),
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
