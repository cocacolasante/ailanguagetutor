package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AdminHandler struct {
	cfg       *config.Config
	userStore *store.UserStore
	billing   *BillingHandler
}

func NewAdminHandler(cfg *config.Config, us *store.UserStore, bh *BillingHandler) *AdminHandler {
	return &AdminHandler{cfg: cfg, userStore: us, billing: bh}
}

// requireAdmin checks the caller is the admin user; returns false and writes 403 if not.
func (h *AdminHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	u, err := h.userStore.GetByID(userID)
	if err != nil || !u.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return false
	}
	return true
}

// GET /api/admin/users
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	users := h.userStore.ListAll()

	// Sort: admin first, then by email
	sort.Slice(users, func(i, j int) bool {
		if users[i].IsAdmin != users[j].IsAdmin {
			return users[i].IsAdmin
		}
		return users[i].Email < users[j].Email
	})

	type adminUserDTO struct {
		ID                 string  `json:"id"`
		Email              string  `json:"email"`
		Username           string  `json:"username"`
		IsAdmin            bool    `json:"is_admin"`
		Approved           bool    `json:"approved"`
		EmailVerified      bool    `json:"email_verified"`
		SubscriptionStatus string  `json:"subscription_status"`
		TrialEndsAt        *string `json:"trial_ends_at,omitempty"`
		CreatedAt          string  `json:"created_at"`
	}

	out := make([]adminUserDTO, 0, len(users))
	for _, u := range users {
		dto := adminUserDTO{
			ID:                 u.ID,
			Email:              u.Email,
			Username:           u.Username,
			IsAdmin:            u.IsAdmin,
			Approved:           u.Approved,
			EmailVerified:      u.EmailVerified,
			SubscriptionStatus: u.SubscriptionStatus,
			CreatedAt:          u.CreatedAt.Format("Jan 2, 2006"),
		}
		if u.TrialEndsAt != nil {
			s := u.TrialEndsAt.Format("Jan 2, 2006")
			dto.TrialEndsAt = &s
		}
		out = append(out, dto)
	}

	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// PATCH /api/admin/users/{id}/approval  (legacy — kept for backward compat)
func (h *AdminHandler) SetApproval(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	targetID := chi.URLParam(r, "id")
	callerID := r.Context().Value(middleware.UserIDKey).(string)
	if targetID == callerID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot change your own approval status"})
		return
	}

	var body struct {
		Approved bool `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.userStore.SetApproved(targetID, body.Approved); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"approved": body.Approved})
}

// PATCH /api/admin/users/{id}/subscription
// body: { "status": "free" | "trialing" | "suspended" | "cancelled" }
func (h *AdminHandler) SetSubscription(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	targetID := chi.URLParam(r, "id")
	callerID := r.Context().Value(middleware.UserIDKey).(string)
	if targetID == callerID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot change your own subscription status"})
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	allowed := map[string]bool{
		store.SubFree: true, store.SubTrialing: true,
		store.SubSuspended: true, store.SubCancelled: true, store.SubActive: true,
		store.SubBetaTrial: true,
	}
	if !allowed[body.Status] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	var trialEndsAt *time.Time
	if body.Status == store.SubTrialing {
		t := time.Now().Add(7 * 24 * time.Hour)
		trialEndsAt = &t
	} else if body.Status == store.SubBetaTrial {
		t := time.Now().Add(30 * 24 * time.Hour)
		trialEndsAt = &t
	}

	// When revoking (suspending) or granting free access, cancel any active
	// Stripe subscription without changing local status — SetSubscriptionStatus
	// below sets the final local state.
	if body.Status == store.SubSuspended || body.Status == store.SubFree {
		if u, err := h.userStore.GetByID(targetID); err == nil {
			if cancelErr := h.billing.cancelStripeOnly(u); cancelErr != nil {
				log.Printf("admin: stripe cancel error for user %s: %v", targetID, cancelErr)
				// Continue even if Stripe cancel fails.
			}
		}
	}

	if err := h.userStore.SetSubscriptionStatus(targetID, body.Status, trialEndsAt); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

// POST /api/admin/invite-user
// Creates an account for a beta tester, bypassing Stripe entirely.
// Sends an invite email containing a 48-hour password-set link.
func (h *AdminHandler) InviteUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and username are required"})
		return
	}

	// Random temp password — user will overwrite it via the invite link.
	tempPassword := uuid.New().String()

	u, err := h.userStore.Create(req.Email, req.Username, tempPassword, "beta_trial")
	if err != nil {
		if err == store.ErrUserExists {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create account"})
		}
		return
	}

	// Mark email verified — no email-verification flow for admin-invited users.
	_ = h.userStore.SetEmailVerified(u.ID)

	// Set beta_trial with 30-day window.
	trialEndsAt := time.Now().Add(30 * 24 * time.Hour)
	if err := h.userStore.SetSubscriptionStatus(u.ID, store.SubBetaTrial, &trialEndsAt); err != nil {
		h.userStore.Delete(u.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to configure trial"})
		return
	}

	// Generate a 48-hour password-set token so the user can choose their password.
	resetToken := uuid.New().String()
	resetExpiry := time.Now().Add(48 * time.Hour)
	if err := h.userStore.SetPasswordResetToken(req.Email, resetToken, resetExpiry); err != nil {
		h.userStore.Delete(u.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create invite token"})
		return
	}

	resetURL := h.cfg.AppBaseURL + "/reset-password.html?token=" + resetToken
	if err := sendBetaInviteEmail(h.cfg, req.Email, req.Username, resetURL, trialEndsAt); err != nil {
		log.Printf("admin invite: email error for %s: %v", req.Email, err)
		h.userStore.Delete(u.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to send invite email. Please check SMTP config."})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"message":       "Invite sent successfully.",
		"email":         req.Email,
		"trial_ends_at": trialEndsAt.Format("Jan 2, 2006"),
	})
}
