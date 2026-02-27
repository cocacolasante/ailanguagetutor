package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/ailanguagetutor/config"
	"github.com/ailanguagetutor/middleware"
	"github.com/ailanguagetutor/store"
	"github.com/golang-jwt/jwt/v5"
	stripe "github.com/stripe/stripe-go/v76"
	checkoutsession "github.com/stripe/stripe-go/v76/checkout/session"
	portalsession "github.com/stripe/stripe-go/v76/billingportal/session"
	stripeCustomer "github.com/stripe/stripe-go/v76/customer"
	stripesub "github.com/stripe/stripe-go/v76/subscription"
	"github.com/stripe/stripe-go/v76/webhook"
)

type BillingHandler struct {
	cfg       *config.Config
	userStore *store.UserStore
}

func NewBillingHandler(cfg *config.Config, us *store.UserStore) *BillingHandler {
	stripe.Key = cfg.StripeSecretKey
	return &BillingHandler{cfg: cfg, userStore: us}
}

// CreateCheckoutSession creates a Stripe Checkout session for an existing user
// who has not yet set up a subscription (e.g. they abandoned the original checkout).
// POST /api/billing/checkout  body: { "plan": "trial" | "immediate" }
func (h *BillingHandler) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	u, err := h.userStore.GetByID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	var req struct {
		Plan string `json:"plan"` // "trial" or "immediate"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Plan != "trial" && req.Plan != "immediate") {
		req.Plan = "trial"
	}

	checkoutURL, err := h.createCheckout(u, req.Plan)
	if err != nil {
		log.Printf("stripe checkout error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create checkout session"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"checkout_url": checkoutURL})
}

// createCheckout is the shared helper used at registration and on-demand.
func (h *BillingHandler) createCheckout(u *store.User, plan string) (string, error) {
	// Ensure the user has a Stripe customer
	customerID := u.StripeCustomerID
	if customerID == "" {
		c, err := stripeCustomer.New(&stripe.CustomerParams{
			Email: stripe.String(u.Email),
			Name:  stripe.String(u.Username),
			Metadata: map[string]string{
				"user_id": u.ID,
			},
		})
		if err != nil {
			return "", err
		}
		customerID = c.ID
		// Persist immediately so we can look up by customer ID in webhooks
		_ = h.userStore.UpdateSubscription(u.ID, customerID, "", "", nil)
	}

	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		Mode:     stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(h.cfg.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
		ClientReferenceID: stripe.String(u.ID),
		SuccessURL:        stripe.String(h.cfg.AppBaseURL + "/checkout-complete.html?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:         stripe.String(h.cfg.AppBaseURL + "/?checkout=cancelled"),
	}

	if plan == "trial" {
		params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
			TrialPeriodDays: stripe.Int64(7),
		}
	}

	s, err := checkoutsession.New(params)
	if err != nil {
		return "", err
	}
	return s.URL, nil
}

// VerifyCheckout is called after Stripe redirects back (public — session_id is the auth token).
// GET /api/billing/verify-checkout?session_id=...
func (h *BillingHandler) VerifyCheckout(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session_id"})
		return
	}

	// Expand subscription so Status and TrialEnd are populated
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("subscription")
	s, err := checkoutsession.Get(sessionID, params)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid checkout session"})
		return
	}

	// The user is identified by client_reference_id set at checkout creation
	userID := s.ClientReferenceID
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no user associated with session"})
		return
	}

	if s.Subscription == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no subscription in session"})
		return
	}

	subStatus := string(s.Subscription.Status)
	var trialEndsAt *time.Time
	if s.Subscription.TrialEnd > 0 {
		t := time.Unix(s.Subscription.TrialEnd, 0)
		trialEndsAt = &t
	}

	custID := ""
	if s.Customer != nil {
		custID = s.Customer.ID
	}

	mappedStatus := mapStripeStatus(subStatus)
	if err := h.userStore.UpdateSubscription(
		userID,
		custID,
		s.Subscription.ID,
		mappedStatus,
		trialEndsAt,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update subscription"})
		return
	}

	u, _ := h.userStore.GetByID(userID)

	// Issue a login token so the checkout-complete page can skip the login step
	token, _ := issueToken(h.cfg.JWTSecret, u.ID)

	writeJSON(w, http.StatusOK, map[string]any{
		"subscription_status": u.SubscriptionStatus,
		"trial_ends_at":       u.TrialEndsAt,
		"token":               token,
		"user":                toDTO(u),
	})
}

// issueToken generates a 7-day JWT for the given user ID.
func issueToken(secret, userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

// Cancel cancels the user's Stripe subscription, keeping trial access if applicable.
// POST /api/billing/cancel
func (h *BillingHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	u, err := h.userStore.GetByID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	if err := h.CancelUserSubscription(u); err != nil {
		log.Printf("cancel subscription error (user %s): %v", userID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to cancel subscription"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": store.SubCancelled})
}

// CancelUserSubscription cancels the Stripe subscription (if any) and marks the
// user as cancelled in the DB.  The TrialEndsAt date is preserved so that
// cancelled-during-trial users keep access until the trial expires.
func (h *BillingHandler) CancelUserSubscription(u *store.User) error {
	if err := h.cancelStripeOnly(u); err != nil {
		return err
	}
	// Keep TrialEndsAt intact so cancelled-during-trial users retain access.
	return h.userStore.UpdateSubscription(u.ID, "", "", store.SubCancelled, u.TrialEndsAt)
}

// cancelStripeOnly cancels the Stripe subscription without touching the local DB.
// Used by admin revoke so the caller can set its own final status (suspended).
func (h *BillingHandler) cancelStripeOnly(u *store.User) error {
	if u.StripeSubscriptionID == "" {
		return nil
	}
	if _, err := stripesub.Cancel(u.StripeSubscriptionID, nil); err != nil {
		stripeErr, ok := err.(*stripe.Error)
		if !ok || stripeErr.Code != stripe.ErrorCodeResourceMissing {
			return err
		}
	}
	return nil
}

// Status returns the current user's subscription status.
// GET /api/billing/status
func (h *BillingHandler) Status(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	u, err := h.userStore.GetByID(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"subscription_status":     u.SubscriptionStatus,
		"trial_ends_at":           u.TrialEndsAt,
		"has_full_access":         u.HasFullAccess(),
		"has_conversation_access": u.HasConversationAccess(),
	})
}

// CreatePortalSession creates a Stripe Customer Portal session.
// POST /api/billing/portal
func (h *BillingHandler) CreatePortalSession(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(string)
	u, err := h.userStore.GetByID(userID)
	if err != nil || u.StripeCustomerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no billing account found"})
		return
	}

	ps, err := portalsession.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(u.StripeCustomerID),
		ReturnURL: stripe.String(h.cfg.AppBaseURL + "/profile.html"),
	})
	if err != nil {
		log.Printf("stripe portal error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create billing portal session"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"portal_url": ps.URL})
}

// Webhook handles Stripe event notifications.
// POST /api/billing/webhook  (no auth — verified by Stripe signature)
func (h *BillingHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), h.cfg.StripeWebhookSecret)
	if err != nil {
		log.Printf("stripe webhook signature error: %v", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		var s stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
			break
		}
		// s.Subscription may be just an ID at this point; expand it
		if s.Subscription != nil && s.ClientReferenceID != "" {
			sub := s.Subscription
			subStatus := mapStripeStatus(string(sub.Status))
			var trialEndsAt *time.Time
			if sub.TrialEnd > 0 {
				t := time.Unix(sub.TrialEnd, 0)
				trialEndsAt = &t
			}
			custID := ""
			if s.Customer != nil {
				custID = s.Customer.ID
			}
			_ = h.userStore.UpdateSubscription(s.ClientReferenceID, custID, sub.ID, subStatus, trialEndsAt)
		}

	case "customer.subscription.updated":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			break
		}
		u, err := h.userStore.GetByStripeCustomerID(sub.Customer.ID)
		if err != nil {
			break
		}
		subStatus := mapStripeStatus(string(sub.Status))
		var trialEndsAt *time.Time
		if sub.TrialEnd > 0 {
			t := time.Unix(sub.TrialEnd, 0)
			trialEndsAt = &t
		}
		_ = h.userStore.UpdateSubscription(u.ID, "", sub.ID, subStatus, trialEndsAt)

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			break
		}
		u, err := h.userStore.GetByStripeCustomerID(sub.Customer.ID)
		if err != nil {
			break
		}
		// Don't override a manual admin suspension with 'cancelled'.
		if u.SubscriptionStatus == store.SubSuspended {
			break
		}
		// Preserve any trial end date so cancelled-during-trial users keep access.
		trialEnd := u.TrialEndsAt
		if sub.TrialEnd > 0 {
			t := time.Unix(sub.TrialEnd, 0)
			trialEnd = &t
		}
		_ = h.userStore.UpdateSubscription(u.ID, "", sub.ID, store.SubCancelled, trialEnd)

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			break
		}
		if inv.Customer == nil {
			break
		}
		u, err := h.userStore.GetByStripeCustomerID(inv.Customer.ID)
		if err != nil {
			break
		}
		_ = h.userStore.UpdateSubscription(u.ID, "", "", store.SubPastDue, nil)

	case "invoice.payment_succeeded":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			break
		}
		if inv.Customer == nil {
			break
		}
		u, err := h.userStore.GetByStripeCustomerID(inv.Customer.ID)
		if err != nil {
			break
		}
		if u.SubscriptionStatus == store.SubPastDue {
			_ = h.userStore.UpdateSubscription(u.ID, "", "", store.SubActive, nil)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func mapStripeStatus(s string) string {
	switch s {
	case "trialing":
		return store.SubTrialing
	case "active":
		return store.SubActive
	case "past_due":
		return store.SubPastDue
	case "canceled", "cancelled":
		return store.SubCancelled
	default:
		return s
	}
}
