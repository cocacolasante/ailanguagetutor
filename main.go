package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/database"
	"github.com/ailanguagetutor/handlers"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	database.MigrateFromJSON(ctx, pool)

	userStore    := store.NewUserStore(pool)
	sessionStore := store.NewSessionStore()
	contextStore := store.NewContextStore(pool)
	historyStore := store.NewConversationHistoryStore(pool)
	profileStore := store.NewStudentProfileStore(pool)

	billingHandler      := handlers.NewBillingHandler(cfg, userStore)
	authHandler         := handlers.NewAuthHandler(cfg, userStore, billingHandler)
	convHandler         := handlers.NewConversationHandler(cfg, sessionStore, contextStore, userStore, historyStore, profileStore)
	ttsHandler          := handlers.NewTTSHandler(cfg)
	adminHandler        := handlers.NewAdminHandler(userStore, billingHandler)
	gamificationHandler := handlers.NewGamificationHandler(userStore, historyStore)
	agentHandler        := handlers.NewAgentHandler(cfg, sessionStore, profileStore)
	vocabHandler        := handlers.NewVocabHandler(cfg, userStore, profileStore)

	auth := middleware.NewAuthMiddleware(cfg)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(chimw.Logger)

	// ── Static files ──────────────────────────────────────────────────────────
	fs := http.FileServer(http.Dir("./static"))
	r.Handle("/css/*",   fs)
	r.Handle("/js/*",    fs)
	r.Handle("/fonts/*", fs)

	// Serve HTML pages
	r.Get("/",                       serveFile("./static/index.html"))
	r.Get("/dashboard.html",         serveFile("./static/dashboard.html"))
	r.Get("/conversation.html",      serveFile("./static/conversation.html"))
	r.Get("/summary.html",           serveFile("./static/summary.html"))
	r.Get("/leaderboard.html",       serveFile("./static/leaderboard.html"))
	r.Get("/admin.html",             serveFile("./static/admin.html"))
	r.Get("/profile.html",           serveFile("./static/profile.html"))
	r.Get("/checkout-complete.html", serveFile("./static/checkout-complete.html"))
	r.Get("/vocab.html",             serveFile("./static/vocab.html"))

	// ── Auth (public) ─────────────────────────────────────────────────────────
	r.Post("/api/auth/register",    authHandler.Register)
	r.Post("/api/auth/login",       authHandler.Login)
	r.Get("/api/auth/verify-email", authHandler.VerifyEmail)

	// ── Auth (protected) ──────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Post("/api/auth/logout",          authHandler.Logout)
		r.Get("/api/auth/me",               authHandler.Me)
		r.Patch("/api/user/preferences",    authHandler.UpdatePreferences)
	})

	// ── Meta (public) ─────────────────────────────────────────────────────────
	r.Get("/api/languages",    handlers.GetLanguages)
	r.Get("/api/topics",       handlers.GetTopics)
	r.Get("/api/personalities", handlers.GetPersonalities)

	// ── Billing webhook + verify-checkout (no auth) ────────────────────────────
	r.Post("/api/billing/webhook",        billingHandler.Webhook)
	r.Get("/api/billing/verify-checkout", billingHandler.VerifyCheckout)

	// ── Protected routes ──────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)

		// Billing
		r.Get("/api/billing/status",    billingHandler.Status)
		r.Post("/api/billing/checkout", billingHandler.CreateCheckoutSession)
		r.Post("/api/billing/cancel",   billingHandler.Cancel)
		r.Post("/api/billing/portal",   billingHandler.CreatePortalSession)

		// Conversation (legacy SSE flow)
		r.Post("/api/conversation/start",              convHandler.Start)
		r.Post("/api/conversation/message",            convHandler.Message)
		r.Post("/api/conversation/translate",          convHandler.Translate)
		r.Post("/api/conversation/end",                convHandler.End)
		r.Get("/api/conversation/history/{sessionId}", convHandler.History)

		// ElevenLabs Agent flow
		r.Post("/api/conversation/agent-url", agentHandler.GetConversationURL)

		// TTS
		r.Post("/api/tts", ttsHandler.Convert)

		// Vocab builder
		r.Post("/api/vocab/session",  vocabHandler.Session)
		r.Post("/api/vocab/check",    vocabHandler.Check)
		r.Post("/api/vocab/complete", vocabHandler.Complete)

		// Gamification
		r.Get("/api/user/stats",              gamificationHandler.Stats)
		r.Get("/api/conversation/records",    gamificationHandler.Records)
		r.Get("/api/conversation/records/{id}", gamificationHandler.GetRecord)
		r.Get("/api/badges",                  gamificationHandler.Badges)
	})

	// Leaderboard (public — no auth required)
	r.Get("/api/leaderboard", gamificationHandler.Leaderboard)

	// ── Admin (protected) ─────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Get("/api/admin/users",                     adminHandler.ListUsers)
		r.Patch("/api/admin/users/{id}/approval",     adminHandler.SetApproval)
		r.Patch("/api/admin/users/{id}/subscription", adminHandler.SetSubscription)
		// One-time setup: creates the ElevenLabs Conversational AI agent
		r.Post("/api/admin/setup-agent", agentHandler.SetupAgent)
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("AI Language Tutor running → http://localhost:%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func serveFile(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.ServeFile(w, r, path)
	}
}
