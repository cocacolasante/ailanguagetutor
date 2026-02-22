package main

import (
	"log"
	"net/http"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/handlers"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

	userStore    := store.NewUserStore()
	sessionStore := store.NewSessionStore()

	authHandler := handlers.NewAuthHandler(cfg, userStore)
	convHandler := handlers.NewConversationHandler(cfg, sessionStore)
	ttsHandler  := handlers.NewTTSHandler(cfg)

	auth := middleware.NewAuthMiddleware(cfg)

	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(chimw.Logger)

	// ── Static files ──────────────────────────────────────────────────────────
	fs := http.FileServer(http.Dir("./static"))
	r.Handle("/css/*",    fs)
	r.Handle("/js/*",     fs)
	r.Handle("/fonts/*",  fs)

	// Serve HTML pages
	r.Get("/",                   serveFile("./static/index.html"))
	r.Get("/dashboard.html",     serveFile("./static/dashboard.html"))
	r.Get("/conversation.html",  serveFile("./static/conversation.html"))

	// ── Auth (public) ─────────────────────────────────────────────────────────
	r.Post("/api/auth/register", authHandler.Register)
	r.Post("/api/auth/login",    authHandler.Login)

	// ── Auth (protected) ──────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Post("/api/auth/logout", authHandler.Logout)
		r.Get("/api/auth/me",      authHandler.Me)
	})

	// ── Meta (public) ─────────────────────────────────────────────────────────
	r.Get("/api/languages", handlers.GetLanguages)
	r.Get("/api/topics",    handlers.GetTopics)

	// ── Conversation + TTS (protected) ────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)
		r.Post("/api/conversation/start",              convHandler.Start)
		r.Post("/api/conversation/message",            convHandler.Message)
		r.Get("/api/conversation/history/{sessionId}", convHandler.History)
		r.Post("/api/tts",                             ttsHandler.Convert)
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
		http.ServeFile(w, r, path)
	}
}
