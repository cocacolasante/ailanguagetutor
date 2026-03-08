package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	cfg     *config.Config
	store   *store.UserStore
	billing *BillingHandler
}

func NewAuthHandler(cfg *config.Config, s *store.UserStore, b *BillingHandler) *AuthHandler {
	return &AuthHandler{cfg: cfg, store: s, billing: b}
}

type registerRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	Plan     string `json:"plan"` // "trial" or "immediate"
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userDTO struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	Username           string     `json:"username"`
	IsAdmin            bool       `json:"is_admin"`
	Approved           bool       `json:"approved"`
	EmailVerified      bool       `json:"email_verified"`
	SubscriptionStatus string     `json:"subscription_status"`
	TrialEndsAt        *time.Time `json:"trial_ends_at,omitempty"`
	PrefLanguage       string     `json:"pref_language"`
	PrefLevel          int        `json:"pref_level"`
	PrefPersonality    string     `json:"pref_personality"`
}

type authResponse struct {
	Token string  `json:"token"`
	User  userDTO `json:"user"`
}

func toDTO(u *store.User) userDTO {
	return userDTO{
		ID:                 u.ID,
		Email:              u.Email,
		Username:           u.Username,
		IsAdmin:            u.IsAdmin,
		Approved:           u.Approved,
		EmailVerified:      u.EmailVerified,
		SubscriptionStatus: u.SubscriptionStatus,
		TrialEndsAt:        u.TrialEndsAt,
		PrefLanguage:       u.PrefLanguage,
		PrefLevel:          u.PrefLevel,
		PrefPersonality:    u.PrefPersonality,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Email == "" || req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email, username, and password are required"})
		return
	}
	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}
	if req.Plan != "immediate" {
		req.Plan = "trial"
	}

	u, err := h.store.Create(req.Email, req.Username, req.Password, req.Plan)
	if err != nil {
		if err == store.ErrUserExists {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create account"})
		}
		return
	}

	// Admin gets a token immediately, no email verification needed
	if u.IsAdmin {
		token, err := h.generateToken(u.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
			return
		}
		writeJSON(w, http.StatusCreated, authResponse{Token: token, User: toDTO(u)})
		return
	}

	// Send verification email; clicking the link will redirect to Stripe Checkout
	verifyURL := h.cfg.AppBaseURL + "/api/auth/verify-email?token=" + u.EmailVerifyToken
	if err := sendVerificationEmail(h.cfg, u.Email, u.Username, verifyURL); err != nil {
		log.Printf("register: email send error for %s: %v", u.Email, err)
		h.store.Delete(u.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to send verification email. Please try again."})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"message": "Please check your email to verify your account.",
	})
}

// VerifyEmail handles the email verification link. On success it redirects to
// Stripe Checkout so the user can set up their subscription.
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/?error=invalid_token", http.StatusSeeOther)
		return
	}

	u, err := h.store.GetByEmailToken(token)
	if err != nil {
		http.Redirect(w, r, "/?error=invalid_token", http.StatusSeeOther)
		return
	}

	if err := h.store.SetEmailVerified(u.ID); err != nil {
		http.Redirect(w, r, "/?error=server_error", http.StatusSeeOther)
		return
	}

	// Re-fetch to pick up the cleared token + verified flag
	u, _ = h.store.GetByID(u.ID)

	// Create the Stripe Checkout session using the plan the user chose at signup
	plan := u.PendingPlan
	if plan != "immediate" {
		plan = "trial"
	}
	checkoutURL, err := h.billing.createCheckout(u, plan)
	if err != nil {
		log.Printf("verify-email: stripe checkout error for user %s: %v", u.ID, err)
		// Stripe not configured or unavailable — redirect to login with a notice
		http.Redirect(w, r, "/?verified=true", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	u, err := h.store.Authenticate(req.Email, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	if !u.EmailVerified {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":  "Please verify your email address before signing in. Check your inbox for a verification link.",
			"status": "email_unverified",
		})
		return
	}

	if !u.HasAnyAccess() {
		// Give a specific message + checkout URL when no subscription yet
		resp := map[string]string{"status": u.SubscriptionStatus}
		switch u.SubscriptionStatus {
		case "":
			checkoutURL, _ := h.billing.createCheckout(u, "trial")
			resp["error"] = "Please complete your subscription setup to sign in."
			resp["checkout_url"] = checkoutURL
		case store.SubSuspended:
			resp["error"] = "Your account has been suspended. Please contact support."
		case store.SubCancelled:
			resp["error"] = "Your subscription has been cancelled. Please resubscribe to continue."
		default:
			resp["error"] = "Account access denied."
		}
		writeJSON(w, http.StatusForbidden, resp)
		return
	}

	token, err := h.generateToken(u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token, User: toDTO(u)})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	u, err := h.store.GetByID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, toDTO(u))
}

func (h *AuthHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	var req struct {
		Language    string `json:"language"`
		Level       int    `json:"level"`
		Personality string `json:"personality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if err := h.store.UpdatePreferences(userID, req.Language, req.Level, req.Personality); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save preferences"})
		return
	}
	u, _ := h.store.GetByID(userID)
	writeJSON(w, http.StatusOK, toDTO(u))
}

func (h *AuthHandler) generateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.cfg.JWTSecret))
}
