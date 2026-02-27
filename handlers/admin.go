package handlers

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	userStore *store.UserStore
}

func NewAdminHandler(us *store.UserStore) *AdminHandler {
	return &AdminHandler{userStore: us}
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
		ID        string `json:"id"`
		Email     string `json:"email"`
		Username  string `json:"username"`
		IsAdmin   bool   `json:"is_admin"`
		Approved  bool   `json:"approved"`
		CreatedAt string `json:"created_at"`
	}

	out := make([]adminUserDTO, 0, len(users))
	for _, u := range users {
		out = append(out, adminUserDTO{
			ID:        u.ID,
			Email:     u.Email,
			Username:  u.Username,
			IsAdmin:   u.IsAdmin,
			Approved:  u.Approved,
			CreatedAt: u.CreatedAt.Format("Jan 2, 2006"),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// PATCH /api/admin/users/{id}/approval
func (h *AdminHandler) SetApproval(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}

	targetID := chi.URLParam(r, "id")

	var body struct {
		Approved bool `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Prevent revoking the admin's own access
	callerID := r.Context().Value(middleware.UserIDKey).(string)
	if targetID == callerID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot change your own approval status"})
		return
	}

	if err := h.userStore.SetApproved(targetID, body.Approved); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"approved": body.Approved})
}
