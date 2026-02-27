package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	userStore *store.UserStore
	billing   *BillingHandler
}

func NewAdminHandler(us *store.UserStore, bh *BillingHandler) *AdminHandler {
	return &AdminHandler{userStore: us, billing: bh}
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
	}
	if !allowed[body.Status] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	var trialEndsAt *time.Time
	if body.Status == store.SubTrialing {
		t := time.Now().Add(7 * 24 * time.Hour)
		trialEndsAt = &t
	}

	// When revoking (suspending), cancel the Stripe subscription without
	// changing the local status — SetSubscriptionStatus below sets it to "suspended".
	if body.Status == store.SubSuspended {
		if u, err := h.userStore.GetByID(targetID); err == nil {
			if cancelErr := h.billing.cancelStripeOnly(u); cancelErr != nil {
				log.Printf("admin: stripe cancel error for user %s: %v", targetID, cancelErr)
				// Continue with local suspend even if Stripe cancel fails.
			}
		}
	}

	if err := h.userStore.SetSubscriptionStatus(targetID, body.Status, trialEndsAt); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}
