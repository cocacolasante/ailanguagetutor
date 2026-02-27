package handlers

import (
	"encoding/json"
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
	SubscriptionStatus string     `json:"subscription_status"`
	TrialEndsAt        *time.Time `json:"trial_ends_at,omitempty"`
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
		SubscriptionStatus: u.SubscriptionStatus,
		TrialEndsAt:        u.TrialEndsAt,
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

	u, err := h.store.Create(req.Email, req.Username, req.Password)
	if err != nil {
		if err == store.ErrUserExists {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create account"})
		}
		return
	}

	// Admin gets a token immediately, no Stripe needed
	if u.IsAdmin {
		token, err := h.generateToken(u.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
			return
		}
		writeJSON(w, http.StatusCreated, authResponse{Token: token, User: toDTO(u)})
		return
	}

	// Everyone else goes through Stripe Checkout
	checkoutURL, err := h.billing.createCheckout(u, req.Plan)
	if err != nil {
		h.store.Delete(u.ID) // roll back so the email isn't permanently locked out
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "payment system unavailable — please try again"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"checkout_url": checkoutURL})
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

func (h *AuthHandler) generateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.cfg.JWTSecret))
}
