package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	cfg       *config.Config
	store     *store.UserStore
	billing   *BillingHandler
	blocklist *store.TokenBlocklist
}

func NewAuthHandler(cfg *config.Config, s *store.UserStore, b *BillingHandler, bl *store.TokenBlocklist) *AuthHandler {
	return &AuthHandler{cfg: cfg, store: s, billing: b, blocklist: bl}
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
	tokenStr := extractRawToken(r)
	if tokenStr != "" {
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(h.cfg.JWTSecret), nil
		})
		if err == nil && token.Valid {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if expUnix, ok := claims["exp"].(float64); ok {
					exp := time.Unix(int64(expUnix), 0)
					_ = h.blocklist.Add(r.Context(), tokenStr, exp)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func extractRawToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if c, err := r.Cookie("token"); err == nil {
		return c.Value
	}
	return ""
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

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	// Always respond with success to avoid exposing whether the email exists
	u, err := h.store.GetByEmail(req.Email)
	if err == nil {
		token := uuid.New().String()
		expiry := time.Now().Add(time.Hour)
		if err := h.store.SetPasswordResetToken(u.Email, token, expiry); err != nil {
			log.Printf("forgot-password: set token error: %v", err)
		} else {
			resetURL := h.cfg.AppBaseURL + "/reset-password.html?token=" + token
			if err := sendPasswordResetEmail(h.cfg, u.Email, u.Username, resetURL); err != nil {
				log.Printf("forgot-password: email send error for %s: %v", u.Email, err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "If that email is registered, you'll receive a reset link shortly.",
	})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Token == "" || len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token and password (min 8 chars) are required"})
		return
	}

	u, err := h.store.GetByPasswordResetToken(req.Token)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid or expired reset link."})
		return
	}
	if u.PasswordResetExpiry == nil || time.Now().After(*u.PasswordResetExpiry) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "This reset link has expired. Please request a new one."})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
		return
	}
	if err := h.store.ResetPassword(u.ID, string(hash)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Password updated. You can now sign in."})
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
