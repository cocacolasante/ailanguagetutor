package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrSessionNotFound    = errors.New("session not found")
)

// ── Models ────────────────────────────────────────────────────────────────────

const AdminEmail = "anthony@csuitecode.com"

const (
	SubTrialing  = "trialing"
	SubActive    = "active"
	SubPastDue   = "past_due"
	SubCancelled = "cancelled"
	SubFree      = "free"      // admin-granted free access
	SubSuspended = "suspended" // admin-revoked
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	IsAdmin      bool      `json:"is_admin"`
	Approved     bool      `json:"approved"`
	CreatedAt    time.Time `json:"created_at"`

	EmailVerified    bool   `json:"email_verified"`
	EmailVerifyToken string `json:"email_verify_token,omitempty"`
	PendingPlan      string `json:"pending_plan,omitempty"`

	StripeCustomerID     string     `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string     `json:"stripe_subscription_id,omitempty"`
	SubscriptionStatus   string     `json:"subscription_status,omitempty"`
	TrialEndsAt          *time.Time `json:"trial_ends_at,omitempty"`

	// Gamification
	Streak            int            `json:"streak"`
	LastActivityDate  string         `json:"last_activity_date,omitempty"` // YYYY-MM-DD
	TotalFP           int            `json:"total_fp"`
	LanguageFP        map[string]int `json:"language_fp,omitempty"`
	LanguageLevel     map[string]int `json:"language_level,omitempty"`
	Achievements      []string       `json:"achievements,omitempty"`
	ConversationCount int            `json:"conversation_count"`

	// Learning preferences (set on profile page)
	PrefLanguage    string `json:"pref_language,omitempty"`
	PrefLevel       int    `json:"pref_level,omitempty"`
	PrefPersonality string `json:"pref_personality,omitempty"`

	// Password reset
	PasswordResetToken  string     `json:"password_reset_token,omitempty"`
	PasswordResetExpiry *time.Time `json:"password_reset_expires_at,omitempty"`
}

// HasFullAccess returns true when the user can use all levels.
func (u *User) HasFullAccess() bool {
	return u.SubscriptionStatus == SubActive ||
		u.SubscriptionStatus == SubFree ||
		u.SubscriptionStatus == SubPastDue
}

// HasAnyAccess returns true when the user can log in.
func (u *User) HasAnyAccess() bool {
	return u.Approved && (u.SubscriptionStatus == SubTrialing ||
		u.SubscriptionStatus == SubActive ||
		u.SubscriptionStatus == SubFree ||
		u.SubscriptionStatus == SubPastDue ||
		u.SubscriptionStatus == SubCancelled)
}

// HasConversationAccess returns true when the user can start conversations.
func (u *User) HasConversationAccess() bool {
	switch u.SubscriptionStatus {
	case SubActive, SubFree, SubPastDue, SubTrialing:
		return true
	case SubCancelled:
		return u.TrialEndsAt != nil && time.Now().Before(*u.TrialEndsAt)
	default:
		return false
	}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Session struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Language    string    `json:"language"`
	Topic       string    `json:"topic"`
	Level       int       `json:"level"`
	Personality string    `json:"personality,omitempty"`
	Messages    []Message `json:"messages"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ── Gamification ──────────────────────────────────────────────────────────────

type LeaderboardEntry struct {
	Rank     int    `json:"rank"`
	Username string `json:"username"`
	TotalFP  int    `json:"total_fp"`
	Streak   int    `json:"streak"`
}

// BadgeInfo describes an achievement badge.
type BadgeInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	Desc string `json:"desc"`
}

var AllBadges = []BadgeInfo{
	{ID: "streak_3", Name: "3-Day Streak", Icon: "🔥", Desc: "Practice 3 days in a row"},
	{ID: "streak_7", Name: "Week Warrior", Icon: "🔥", Desc: "Practice 7 days in a row"},
	{ID: "streak_30", Name: "Monthly Master", Icon: "🌟", Desc: "Practice 30 days in a row"},
	{ID: "streak_100", Name: "Century Streak", Icon: "💎", Desc: "Practice 100 days in a row"},
	{ID: "first_conv", Name: "First Steps", Icon: "👶", Desc: "Complete your first conversation"},
	{ID: "conv_10", Name: "Getting Started", Icon: "📖", Desc: "Complete 10 conversations"},
	{ID: "conv_50", Name: "Dedicated Learner", Icon: "🎓", Desc: "Complete 50 conversations"},
	{ID: "conv_100", Name: "Language Champion", Icon: "🏆", Desc: "Complete 100 conversations"},
	{ID: "fp_100", Name: "FP Collector", Icon: "⭐", Desc: "Earn 100 Fluency Points"},
	{ID: "fp_500", Name: "FP Enthusiast", Icon: "🌟", Desc: "Earn 500 Fluency Points"},
	{ID: "fp_1000", Name: "FP Expert", Icon: "💫", Desc: "Earn 1,000 Fluency Points"},
	{ID: "fp_5000", Name: "FP Legend", Icon: "✨", Desc: "Earn 5,000 Fluency Points"},
	{ID: "lang_level_5", Name: "Intermediate", Icon: "📈", Desc: "Reach language level 5"},
	{ID: "lang_level_10", Name: "Advanced", Icon: "🎯", Desc: "Reach language level 10"},
	{ID: "lang_level_20", Name: "Master", Icon: "👑", Desc: "Reach language level 20"},
}

// checkAchievements evaluates which new badges a user has earned and appends
// them to u.Achievements. Returns the list of newly earned badge IDs.
func checkAchievements(u *User) []string {
	existing := map[string]bool{}
	for _, a := range u.Achievements {
		existing[a] = true
	}

	var newBadges []string

	award := func(id string) {
		if !existing[id] {
			u.Achievements = append(u.Achievements, id)
			existing[id] = true
			newBadges = append(newBadges, id)
		}
	}

	// Streak badges
	if u.Streak >= 3 {
		award("streak_3")
	}
	if u.Streak >= 7 {
		award("streak_7")
	}
	if u.Streak >= 30 {
		award("streak_30")
	}
	if u.Streak >= 100 {
		award("streak_100")
	}

	// Conversation count badges
	if u.ConversationCount >= 1 {
		award("first_conv")
	}
	if u.ConversationCount >= 10 {
		award("conv_10")
	}
	if u.ConversationCount >= 50 {
		award("conv_50")
	}
	if u.ConversationCount >= 100 {
		award("conv_100")
	}

	// FP badges
	if u.TotalFP >= 100 {
		award("fp_100")
	}
	if u.TotalFP >= 500 {
		award("fp_500")
	}
	if u.TotalFP >= 1000 {
		award("fp_1000")
	}
	if u.TotalFP >= 5000 {
		award("fp_5000")
	}

	// Language level badges (max across all languages)
	maxLangLevel := 0
	for _, lv := range u.LanguageLevel {
		if lv > maxLangLevel {
			maxLangLevel = lv
		}
	}
	if maxLangLevel >= 5 {
		award("lang_level_5")
	}
	if maxLangLevel >= 10 {
		award("lang_level_10")
	}
	if maxLangLevel >= 20 {
		award("lang_level_20")
	}

	return newBadges
}

// ── JSONB helpers ─────────────────────────────────────────────────────────────

func scanJSONB(data []byte, dest any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, dest)
}

// ── User Store ────────────────────────────────────────────────────────────────

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

func (us *UserStore) Create(email, username, password, pendingPlan string) (*User, error) {
	ctx := context.Background()

	// Check for existing email
	var exists bool
	err := us.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", email).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	isAdmin := email == AdminEmail
	verifyToken := ""
	if !isAdmin {
		verifyToken = uuid.New().String()
	}

	subStatus := ""
	if isAdmin {
		subStatus = SubFree
	}

	u := &User{
		ID:                 uuid.New().String(),
		Email:              email,
		Username:           username,
		PasswordHash:       string(hash),
		IsAdmin:            isAdmin,
		Approved:           isAdmin,
		EmailVerified:      isAdmin,
		EmailVerifyToken:   verifyToken,
		PendingPlan:        pendingPlan,
		SubscriptionStatus: subStatus,
		CreatedAt:          time.Now(),
		LanguageFP:         make(map[string]int),
		LanguageLevel:      make(map[string]int),
		Achievements:       []string{},
	}

	langFP, _ := json.Marshal(u.LanguageFP)
	langLevel, _ := json.Marshal(u.LanguageLevel)
	achievements, _ := json.Marshal(u.Achievements)

	_, err = us.pool.Exec(ctx, `
INSERT INTO users (id, email, username, password_hash, is_admin, approved, created_at,
    email_verified, email_verify_token, pending_plan,
    stripe_customer_id, stripe_subscription_id, subscription_status,
    streak, last_activity_date, total_fp, language_fp, language_level, achievements,
    conversation_count, pref_language, pref_level, pref_personality)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
		u.ID, u.Email, u.Username, u.PasswordHash, u.IsAdmin, u.Approved, u.CreatedAt,
		u.EmailVerified, u.EmailVerifyToken, u.PendingPlan,
		"", "", u.SubscriptionStatus,
		0, "", 0, langFP, langLevel, achievements,
		0, "", 0, "",
	)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (us *UserStore) Authenticate(email, password string) (*User, error) {
	u, err := us.GetByEmail(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func (us *UserStore) GetByEmail(email string) (*User, error) {
	ctx := context.Background()
	row := us.pool.QueryRow(ctx, "SELECT * FROM users WHERE email=$1", email)
	return scanUser(row)
}

func (us *UserStore) GetByID(id string) (*User, error) {
	ctx := context.Background()
	row := us.pool.QueryRow(ctx, "SELECT * FROM users WHERE id=$1", id)
	u, err := scanUser(row)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (us *UserStore) GetByEmailToken(token string) (*User, error) {
	if token == "" {
		return nil, ErrUserNotFound
	}
	ctx := context.Background()
	row := us.pool.QueryRow(ctx, "SELECT * FROM users WHERE email_verify_token=$1", token)
	u, err := scanUser(row)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (us *UserStore) GetByStripeCustomerID(customerID string) (*User, error) {
	ctx := context.Background()
	row := us.pool.QueryRow(ctx, "SELECT * FROM users WHERE stripe_customer_id=$1", customerID)
	u, err := scanUser(row)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (us *UserStore) ListAll() []*User {
	ctx := context.Background()
	rows, err := us.pool.Query(ctx, "SELECT * FROM users")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err == nil {
			users = append(users, u)
		}
	}
	return users
}

func (us *UserStore) Delete(id string) {
	ctx := context.Background()
	_, _ = us.pool.Exec(ctx, "DELETE FROM users WHERE id=$1", id)
}

func (us *UserStore) SetApproved(id string, approved bool) error {
	ctx := context.Background()
	tag, err := us.pool.Exec(ctx, "UPDATE users SET approved=$2 WHERE id=$1", id, approved)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (us *UserStore) SetEmailVerified(id string) error {
	ctx := context.Background()
	_, err := us.pool.Exec(ctx, "UPDATE users SET email_verified=TRUE, email_verify_token='' WHERE id=$1", id)
	return err
}

// UpdateSubscription is called after Stripe checkout or webhook events.
func (us *UserStore) UpdateSubscription(userID, customerID, subscriptionID, status string, trialEndsAt *time.Time) error {
	ctx := context.Background()
	approved := false
	emailVerified := false
	switch status {
	case SubTrialing, SubActive, SubFree, SubPastDue, SubCancelled:
		approved = true
		emailVerified = true
	}
	_, err := us.pool.Exec(ctx, `
UPDATE users SET
    stripe_customer_id = CASE WHEN $2 != '' THEN $2 ELSE stripe_customer_id END,
    stripe_subscription_id = CASE WHEN $3 != '' THEN $3 ELSE stripe_subscription_id END,
    subscription_status = $4,
    trial_ends_at = $5,
    approved = $6,
    email_verified = CASE WHEN $7 THEN TRUE ELSE email_verified END
WHERE id = $1`,
		userID, customerID, subscriptionID, status, trialEndsAt, approved, emailVerified,
	)
	return err
}

// UpdatePreferences saves the user's language/level/personality preferences.
func (us *UserStore) UpdatePreferences(id, language string, level int, personality string) error {
	ctx := context.Background()
	_, err := us.pool.Exec(ctx, `
UPDATE users SET pref_language=$2, pref_level=$3, pref_personality=$4 WHERE id=$1`,
		id, language, level, personality,
	)
	return err
}

// SetSubscriptionStatus lets admins override subscription state.
func (us *UserStore) SetSubscriptionStatus(id, status string, trialEndsAt *time.Time) error {
	ctx := context.Background()
	approved := false
	switch status {
	case SubTrialing, SubActive, SubFree, SubPastDue, SubCancelled:
		approved = true
	}
	_, err := us.pool.Exec(ctx, `
UPDATE users SET subscription_status=$2, trial_ends_at=$3, approved=$4 WHERE id=$1`,
		id, status, trialEndsAt, approved,
	)
	return err
}

// UpdateActivity updates the user's streak, awards FP for a completed
// conversation, increments conversation count, and checks for new achievements.
// Returns the new streak, any newly earned badge IDs, and any error.
func (us *UserStore) UpdateActivity(id, language string, fp int) (newStreak int, newBadges []string, err error) {
	ctx := context.Background()
	u, err := us.GetByID(id)
	if err != nil {
		return 0, nil, ErrUserNotFound
	}

	// Update streak
	today := time.Now().Format("2006-01-02")
	if u.LastActivityDate != today {
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		if u.LastActivityDate == yesterday {
			u.Streak++
		} else {
			u.Streak = 1
		}
		u.LastActivityDate = today
	}

	// Award FP
	if u.LanguageFP == nil {
		u.LanguageFP = make(map[string]int)
	}
	if u.LanguageLevel == nil {
		u.LanguageLevel = make(map[string]int)
	}
	u.TotalFP += fp
	u.LanguageFP[language] += fp
	// Language level: every 500 FP = 1 level, max 20
	u.LanguageLevel[language] = min(20, u.LanguageFP[language]/500+1)

	// Increment conversation count
	u.ConversationCount++

	// Check achievements
	newBadges = checkAchievements(u)

	langFP, _ := json.Marshal(u.LanguageFP)
	langLevel, _ := json.Marshal(u.LanguageLevel)
	achievements, _ := json.Marshal(u.Achievements)

	_, err = us.pool.Exec(ctx, `
UPDATE users SET streak=$2, last_activity_date=$3, total_fp=$4,
    language_fp=$5, language_level=$6, achievements=$7, conversation_count=$8
WHERE id=$1`,
		id, u.Streak, u.LastActivityDate, u.TotalFP,
		langFP, langLevel, achievements, u.ConversationCount,
	)
	if err != nil {
		return 0, nil, err
	}
	return u.Streak, newBadges, nil
}

// SetPasswordResetToken stores a reset token and expiry for the given email.
func (us *UserStore) SetPasswordResetToken(email, token string, expiry time.Time) error {
	ctx := context.Background()
	tag, err := us.pool.Exec(ctx,
		"UPDATE users SET password_reset_token=$2, password_reset_expires_at=$3 WHERE email=$1",
		email, token, expiry,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// GetByPasswordResetToken looks up a user by their reset token.
func (us *UserStore) GetByPasswordResetToken(token string) (*User, error) {
	if token == "" {
		return nil, ErrUserNotFound
	}
	ctx := context.Background()
	row := us.pool.QueryRow(ctx, "SELECT * FROM users WHERE password_reset_token=$1", token)
	u, err := scanUser(row)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// ResetPassword updates the user's password hash and clears the reset token.
func (us *UserStore) ResetPassword(id, newPasswordHash string) error {
	ctx := context.Background()
	_, err := us.pool.Exec(ctx,
		"UPDATE users SET password_hash=$2, password_reset_token='', password_reset_expires_at=NULL WHERE id=$1",
		id, newPasswordHash,
	)
	return err
}

// GetLeaderboard returns the top N users sorted by TotalFP descending.
func (us *UserStore) GetLeaderboard(limit int) []LeaderboardEntry {
	ctx := context.Background()
	rows, err := us.pool.Query(ctx, `
SELECT username, total_fp, streak FROM users
WHERE is_admin=FALSE ORDER BY total_fp DESC LIMIT $1`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var entries []LeaderboardEntry
	rank := 1
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(&e.Username, &e.TotalFP, &e.Streak); err == nil {
			e.Rank = rank
			rank++
			entries = append(entries, e)
		}
	}
	return entries
}

// scanUser reads a User from a pgx row (all columns in table order).
func scanUser(row pgx.Row) (*User, error) {
	var u User
	var langFP, langLevel, achievements []byte
	err := row.Scan(
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.Approved, &u.CreatedAt,
		&u.EmailVerified, &u.EmailVerifyToken, &u.PendingPlan,
		&u.StripeCustomerID, &u.StripeSubscriptionID, &u.SubscriptionStatus, &u.TrialEndsAt,
		&u.Streak, &u.LastActivityDate, &u.TotalFP, &langFP, &langLevel, &achievements,
		&u.ConversationCount, &u.PrefLanguage, &u.PrefLevel, &u.PrefPersonality,
		&u.PasswordResetToken, &u.PasswordResetExpiry,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.LanguageFP = make(map[string]int)
	u.LanguageLevel = make(map[string]int)
	_ = scanJSONB(langFP, &u.LanguageFP)
	_ = scanJSONB(langLevel, &u.LanguageLevel)
	if len(achievements) > 0 {
		_ = scanJSONB(achievements, &u.Achievements)
	}
	return &u, nil
}

// ── Session Store (in-memory, unchanged) ──────────────────────────────────────

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

func (ss *SessionStore) Create(userID, language, topic string, level int, personality, systemPrompt string) *Session {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s := &Session{
		ID:          uuid.New().String(),
		UserID:      userID,
		Language:    language,
		Topic:       topic,
		Level:       level,
		Personality: personality,
		Messages:    []Message{{Role: "system", Content: systemPrompt}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	ss.sessions[s.ID] = s
	return s
}

func (ss *SessionStore) Get(id string) (*Session, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	s, exists := ss.sessions[id]
	if !exists {
		return nil, ErrSessionNotFound
	}
	return s, nil
}

func (ss *SessionStore) AddMessage(id string, msg Message) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	s, exists := ss.sessions[id]
	if !exists {
		return ErrSessionNotFound
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
	return nil
}

func (ss *SessionStore) GetMessages(id string) ([]Message, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	s, exists := ss.sessions[id]
	if !exists {
		return nil, ErrSessionNotFound
	}
	var msgs []Message
	for _, m := range s.Messages {
		if m.Role != "system" {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

// ── Context Store ─────────────────────────────────────────────────────────────

const maxContextSessions = 5
const maxContextMessages = 30

type ContextStore struct {
	pool *pgxpool.Pool
}

func NewContextStore(pool *pgxpool.Pool) *ContextStore {
	return &ContextStore{pool: pool}
}

func (cs *ContextStore) Save(userID, language string, level int, messages []Message) {
	ctx := context.Background()

	var msgs []Message
	for _, m := range messages {
		if m.Role != "system" {
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		return
	}
	if len(msgs) > maxContextMessages {
		msgs = msgs[len(msgs)-maxContextMessages:]
	}

	// Read existing sessions
	var sessionsJSON []byte
	err := cs.pool.QueryRow(ctx,
		"SELECT sessions FROM conversation_contexts WHERE user_id=$1 AND language=$2 AND level=$3",
		userID, language, level,
	).Scan(&sessionsJSON)

	var sessions [][]Message
	if err == nil && len(sessionsJSON) > 0 {
		_ = json.Unmarshal(sessionsJSON, &sessions)
	}

	sessions = append(sessions, msgs)
	if len(sessions) > maxContextSessions {
		sessions = sessions[len(sessions)-maxContextSessions:]
	}

	newSessions, _ := json.Marshal(sessions)
	_, _ = cs.pool.Exec(ctx, `
INSERT INTO conversation_contexts (user_id, language, level, sessions, updated_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (user_id, language, level) DO UPDATE SET sessions=$4, updated_at=NOW()`,
		userID, language, level, newSessions,
	)
}

func (cs *ContextStore) Get(userID, language string, level int) []Message {
	ctx := context.Background()
	var sessionsJSON []byte
	err := cs.pool.QueryRow(ctx,
		"SELECT sessions FROM conversation_contexts WHERE user_id=$1 AND language=$2 AND level=$3",
		userID, language, level,
	).Scan(&sessionsJSON)
	if err != nil || len(sessionsJSON) == 0 {
		return nil
	}
	var sessions [][]Message
	if err := json.Unmarshal(sessionsJSON, &sessions); err != nil {
		return nil
	}
	var all []Message
	for _, s := range sessions {
		all = append(all, s...)
	}
	return all
}

// ── Conversation History Store ────────────────────────────────────────────────

type ConversationRecord struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	SessionID    string    `json:"session_id"`
	Language     string    `json:"language"`
	Topic        string    `json:"topic"`
	TopicName    string    `json:"topic_name"`
	Level        int       `json:"level"`
	Personality  string    `json:"personality,omitempty"`
	MessageCount int       `json:"message_count"`
	DurationSecs int       `json:"duration_secs"`
	FPEarned     int       `json:"fp_earned"`
	Summary      string    `json:"summary"`
	Topics       []string  `json:"topics_discussed,omitempty"`
	Vocabulary   []string  `json:"vocabulary_learned,omitempty"`
	Corrections  []string  `json:"grammar_corrections,omitempty"`
	Suggestions  []string  `json:"suggested_next_lessons,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	EndedAt      time.Time `json:"ended_at"`
}

type ConversationHistoryStore struct {
	pool *pgxpool.Pool
}

func NewConversationHistoryStore(pool *pgxpool.Pool) *ConversationHistoryStore {
	return &ConversationHistoryStore{pool: pool}
}

func (hs *ConversationHistoryStore) Save(record *ConversationRecord) {
	ctx := context.Background()
	topics, _ := json.Marshal(nilSafe(record.Topics))
	vocab, _ := json.Marshal(nilSafe(record.Vocabulary))
	corrections, _ := json.Marshal(nilSafe(record.Corrections))
	suggestions, _ := json.Marshal(nilSafe(record.Suggestions))

	_, _ = hs.pool.Exec(ctx, `
INSERT INTO conversation_history (id, user_id, session_id, language, topic, topic_name, level,
    personality, message_count, duration_secs, fp_earned, summary,
    topics, vocabulary, corrections, suggestions, created_at, ended_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
ON CONFLICT (id) DO NOTHING`,
		record.ID, record.UserID, record.SessionID, record.Language, record.Topic, record.TopicName,
		record.Level, record.Personality, record.MessageCount, record.DurationSecs, record.FPEarned,
		record.Summary, topics, vocab, corrections, suggestions, record.CreatedAt, record.EndedAt,
	)
}

func (hs *ConversationHistoryStore) GetForUser(userID string) []*ConversationRecord {
	ctx := context.Background()
	rows, err := hs.pool.Query(ctx, `
SELECT id, user_id, session_id, language, topic, topic_name, level, personality,
    message_count, duration_secs, fp_earned, summary,
    topics, vocabulary, corrections, suggestions, created_at, ended_at
FROM conversation_history WHERE user_id=$1 ORDER BY ended_at DESC LIMIT 10`, userID)
	if err != nil {
		return []*ConversationRecord{}
	}
	defer rows.Close()
	var records []*ConversationRecord
	for rows.Next() {
		r, err := scanRecord(rows)
		if err == nil {
			records = append(records, r)
		}
	}
	if records == nil {
		return []*ConversationRecord{}
	}
	return records
}

func (hs *ConversationHistoryStore) GetRecord(id string) (*ConversationRecord, error) {
	ctx := context.Background()
	row := hs.pool.QueryRow(ctx, `
SELECT id, user_id, session_id, language, topic, topic_name, level, personality,
    message_count, duration_secs, fp_earned, summary,
    topics, vocabulary, corrections, suggestions, created_at, ended_at
FROM conversation_history WHERE id=$1`, id)
	r, err := scanRecord(row)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return r, nil
}

func scanRecord(row pgx.Row) (*ConversationRecord, error) {
	var r ConversationRecord
	var topics, vocab, corrections, suggestions []byte
	err := row.Scan(
		&r.ID, &r.UserID, &r.SessionID, &r.Language, &r.Topic, &r.TopicName, &r.Level,
		&r.Personality, &r.MessageCount, &r.DurationSecs, &r.FPEarned, &r.Summary,
		&topics, &vocab, &corrections, &suggestions, &r.CreatedAt, &r.EndedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = scanJSONB(topics, &r.Topics)
	_ = scanJSONB(vocab, &r.Vocabulary)
	_ = scanJSONB(corrections, &r.Corrections)
	_ = scanJSONB(suggestions, &r.Suggestions)
	return &r, nil
}

func nilSafe(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ── Student Profile Store ─────────────────────────────────────────────────────

type StudentProfile struct {
	UserID           string         `json:"user_id"`
	Language         string         `json:"language"`
	Name             string         `json:"name"`
	WeakAreas        []string       `json:"weak_areas"`
	StrongAreas      []string       `json:"strong_areas"`
	RecentTopics     []string       `json:"recent_topics"`
	RecentVocab      []string       `json:"recent_vocab"`
	RecentSentences  []string       `json:"recent_sentences"`
	NextSuggestions  []string       `json:"next_suggestions"`
	SessionCount     int            `json:"session_count"`
	VocabListIdx     map[string]int `json:"vocab_list_idx"`     // pool key → next list index
	SentenceListIdx  map[string]int `json:"sentence_list_idx"`  // pool key → next list index
	ListeningListIdx map[string]int `json:"listening_list_idx"` // pool key → next list index
	UpdatedAt        time.Time      `json:"updated_at"`
}

type StudentProfileStore struct {
	pool *pgxpool.Pool
}

func NewStudentProfileStore(pool *pgxpool.Pool) *StudentProfileStore {
	return &StudentProfileStore{pool: pool}
}

func (s *StudentProfileStore) Get(ctx context.Context, userID, language string) (*StudentProfile, error) {
	var p StudentProfile
	var weakAreas, strongAreas, recentTopics, recentVocab, recentSentences, nextSuggestions []byte
	var vocabIdx, sentenceIdx, listeningIdx []byte
	err := s.pool.QueryRow(ctx, `
SELECT user_id, language, name, weak_areas, strong_areas, recent_topics, recent_vocab,
    recent_sentences, next_suggestions, session_count, updated_at,
    vocab_list_idx, sentence_list_idx, listening_list_idx
FROM student_profiles WHERE user_id=$1 AND language=$2`, userID, language).Scan(
		&p.UserID, &p.Language, &p.Name,
		&weakAreas, &strongAreas, &recentTopics, &recentVocab, &recentSentences, &nextSuggestions,
		&p.SessionCount, &p.UpdatedAt,
		&vocabIdx, &sentenceIdx, &listeningIdx,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = scanJSONB(weakAreas, &p.WeakAreas)
	_ = scanJSONB(strongAreas, &p.StrongAreas)
	_ = scanJSONB(recentTopics, &p.RecentTopics)
	_ = scanJSONB(recentVocab, &p.RecentVocab)
	_ = scanJSONB(recentSentences, &p.RecentSentences)
	_ = scanJSONB(nextSuggestions, &p.NextSuggestions)
	p.VocabListIdx = make(map[string]int)
	p.SentenceListIdx = make(map[string]int)
	p.ListeningListIdx = make(map[string]int)
	_ = scanJSONB(vocabIdx, &p.VocabListIdx)
	_ = scanJSONB(sentenceIdx, &p.SentenceListIdx)
	_ = scanJSONB(listeningIdx, &p.ListeningListIdx)
	return &p, nil
}

func (s *StudentProfileStore) Upsert(ctx context.Context, p *StudentProfile) error {
	weakAreas, _ := json.Marshal(nilSafe(p.WeakAreas))
	strongAreas, _ := json.Marshal(nilSafe(p.StrongAreas))
	recentTopics, _ := json.Marshal(nilSafe(p.RecentTopics))
	recentVocab, _ := json.Marshal(nilSafe(p.RecentVocab))
	recentSentences, _ := json.Marshal(nilSafe(p.RecentSentences))
	nextSuggestions, _ := json.Marshal(nilSafe(p.NextSuggestions))
	vocabListIdx, _ := json.Marshal(nilSafeMap(p.VocabListIdx))
	sentenceListIdx, _ := json.Marshal(nilSafeMap(p.SentenceListIdx))
	listeningListIdx, _ := json.Marshal(nilSafeMap(p.ListeningListIdx))

	_, err := s.pool.Exec(ctx, `
INSERT INTO student_profiles (user_id, language, name, weak_areas, strong_areas, recent_topics,
    recent_vocab, recent_sentences, next_suggestions, session_count, vocab_list_idx, sentence_list_idx, listening_list_idx, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW())
ON CONFLICT (user_id, language) DO UPDATE SET
    name=$3, weak_areas=$4, strong_areas=$5, recent_topics=$6,
    recent_vocab=$7, recent_sentences=$8, next_suggestions=$9, session_count=$10,
    vocab_list_idx=$11, sentence_list_idx=$12, listening_list_idx=$13, updated_at=NOW()`,
		p.UserID, p.Language, p.Name, weakAreas, strongAreas, recentTopics,
		recentVocab, recentSentences, nextSuggestions, p.SessionCount,
		vocabListIdx, sentenceListIdx, listeningListIdx,
	)
	return err
}

// nilSafeMap returns an empty map if m is nil.
func nilSafeMap(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

