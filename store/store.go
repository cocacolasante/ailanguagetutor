package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
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

// SubscriptionStatus values
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
	Rank      int    `json:"rank"`
	Username  string `json:"username"`
	TotalFP   int    `json:"total_fp"`
	Streak    int    `json:"streak"`
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

// ── User Store ────────────────────────────────────────────────────────────────

type UserStore struct {
	mu       sync.RWMutex
	byEmail  map[string]*User
	byID     map[string]*User
	filePath string
}

func NewUserStore() *UserStore {
	us := &UserStore{
		byEmail:  make(map[string]*User),
		byID:     make(map[string]*User),
		filePath: "data/users.json",
	}
	us.load()
	return us
}

func (us *UserStore) Create(email, username, password, pendingPlan string) (*User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	if _, exists := us.byEmail[email]; exists {
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

	u := &User{
		ID:           uuid.New().String(),
		Email:        email,
		Username:     username,
		PasswordHash: string(hash),
		IsAdmin:            isAdmin,
		Approved:           isAdmin,
		EmailVerified:      isAdmin,
		EmailVerifyToken:   verifyToken,
		PendingPlan:        pendingPlan,
		SubscriptionStatus: func() string {
			if isAdmin {
				return SubFree
			}
			return ""
		}(),
		CreatedAt:    time.Now(),
		LanguageFP:   make(map[string]int),
		LanguageLevel: make(map[string]int),
	}
	us.byEmail[email] = u
	us.byID[u.ID] = u
	us.save()
	return u, nil
}

func (us *UserStore) Authenticate(email, password string) (*User, error) {
	us.mu.RLock()
	u, exists := us.byEmail[email]
	us.mu.RUnlock()

	if !exists {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func (us *UserStore) GetByID(id string) (*User, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	u, exists := us.byID[id]
	if !exists {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (us *UserStore) ListAll() []*User {
	us.mu.RLock()
	defer us.mu.RUnlock()

	users := make([]*User, 0, len(us.byID))
	for _, u := range us.byID {
		users = append(users, u)
	}
	return users
}

func (us *UserStore) Delete(id string) {
	us.mu.Lock()
	defer us.mu.Unlock()

	u, exists := us.byID[id]
	if !exists {
		return
	}
	delete(us.byID, id)
	delete(us.byEmail, u.Email)
	us.save()
}

func (us *UserStore) SetApproved(id string, approved bool) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	u, exists := us.byID[id]
	if !exists {
		return ErrUserNotFound
	}
	u.Approved = approved
	us.save()
	return nil
}

func (us *UserStore) GetByEmailToken(token string) (*User, error) {
	if token == "" {
		return nil, ErrUserNotFound
	}
	us.mu.RLock()
	defer us.mu.RUnlock()

	for _, u := range us.byID {
		if u.EmailVerifyToken == token {
			return u, nil
		}
	}
	return nil, ErrUserNotFound
}

func (us *UserStore) SetEmailVerified(id string) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	u, exists := us.byID[id]
	if !exists {
		return ErrUserNotFound
	}
	u.EmailVerified = true
	u.EmailVerifyToken = ""
	us.save()
	return nil
}

func (us *UserStore) GetByStripeCustomerID(customerID string) (*User, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	for _, u := range us.byID {
		if u.StripeCustomerID == customerID {
			return u, nil
		}
	}
	return nil, ErrUserNotFound
}

// UpdateSubscription is called after Stripe checkout or webhook events.
func (us *UserStore) UpdateSubscription(userID, customerID, subscriptionID, status string, trialEndsAt *time.Time) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	u, exists := us.byID[userID]
	if !exists {
		return ErrUserNotFound
	}
	if customerID != "" {
		u.StripeCustomerID = customerID
	}
	if subscriptionID != "" {
		u.StripeSubscriptionID = subscriptionID
	}
	u.SubscriptionStatus = status
	u.TrialEndsAt = trialEndsAt
	if status != "" {
		switch status {
		case SubTrialing, SubActive, SubFree, SubPastDue, SubCancelled:
			u.Approved = true
			u.EmailVerified = true
		default:
			u.Approved = false
		}
	}
	us.save()
	return nil
}

// UpdatePreferences saves the user's language/level/personality preferences.
func (us *UserStore) UpdatePreferences(id, language string, level int, personality string) error {
	us.mu.Lock()
	defer us.mu.Unlock()
	u, exists := us.byID[id]
	if !exists {
		return ErrUserNotFound
	}
	u.PrefLanguage    = language
	u.PrefLevel       = level
	u.PrefPersonality = personality
	us.save()
	return nil
}

// SetSubscriptionStatus lets admins override subscription state.
func (us *UserStore) SetSubscriptionStatus(id, status string, trialEndsAt *time.Time) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	u, exists := us.byID[id]
	if !exists {
		return ErrUserNotFound
	}
	u.SubscriptionStatus = status
	u.TrialEndsAt = trialEndsAt
	switch status {
	case SubTrialing, SubActive, SubFree, SubPastDue, SubCancelled:
		u.Approved = true
	default:
		u.Approved = false
	}
	us.save()
	return nil
}

// UpdateActivity updates the user's streak, awards FP for a completed
// conversation, increments conversation count, and checks for new achievements.
// Returns the new streak, any newly earned badge IDs, and any error.
func (us *UserStore) UpdateActivity(id, language string, fp int) (newStreak int, newBadges []string, err error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	u, exists := us.byID[id]
	if !exists {
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

	us.save()
	return u.Streak, newBadges, nil
}

// GetLeaderboard returns the top N users sorted by TotalFP descending.
func (us *UserStore) GetLeaderboard(limit int) []LeaderboardEntry {
	us.mu.RLock()
	defer us.mu.RUnlock()

	var entries []LeaderboardEntry
	for _, u := range us.byID {
		if !u.IsAdmin {
			entries = append(entries, LeaderboardEntry{
				Username: u.Username,
				TotalFP:  u.TotalFP,
				Streak:   u.Streak,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TotalFP > entries[j].TotalFP
	})

	for i := range entries {
		entries[i].Rank = i + 1
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func (us *UserStore) load() {
	data, err := os.ReadFile(us.filePath)
	if err != nil {
		return
	}
	var users []*User
	if err := json.Unmarshal(data, &users); err != nil {
		return
	}
	modified := false
	for _, u := range users {
		// Migration: ensure admin is always correct
		if u.Email == AdminEmail {
			if !u.IsAdmin || !u.Approved || u.SubscriptionStatus != SubFree || !u.EmailVerified {
				u.IsAdmin = true
				u.Approved = true
				u.SubscriptionStatus = SubFree
				u.EmailVerified = true
				modified = true
			}
		}
		// Migration: auto-verify existing subscribed users
		if !u.EmailVerified && u.SubscriptionStatus != "" && u.SubscriptionStatus != SubSuspended {
			u.EmailVerified = true
			modified = true
		}
		// Migration: ensure gamification maps are initialized
		if u.LanguageFP == nil {
			u.LanguageFP = make(map[string]int)
		}
		if u.LanguageLevel == nil {
			u.LanguageLevel = make(map[string]int)
		}
		us.byEmail[u.Email] = u
		us.byID[u.ID] = u
	}
	if modified {
		us.save()
	}
}

func (us *UserStore) save() {
	_ = os.MkdirAll("data", 0755)
	users := make([]*User, 0, len(us.byID))
	for _, u := range us.byID {
		users = append(users, u)
	}
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(us.filePath, data, 0600)
}

// ── Session Store ─────────────────────────────────────────────────────────────

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
const maxContextMessages  = 30

type contextEntry struct {
	Sessions  [][]Message `json:"sessions"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type ContextStore struct {
	mu       sync.RWMutex
	data     map[string]contextEntry
	filePath string
}

func NewContextStore() *ContextStore {
	cs := &ContextStore{
		data:     make(map[string]contextEntry),
		filePath: "data/contexts.json",
	}
	cs.load()
	return cs
}

func contextKey(userID, language string, level int) string {
	return fmt.Sprintf("%s:%s:%d", userID, language, level)
}

func (cs *ContextStore) Save(userID, language string, level int, messages []Message) {
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

	cs.mu.Lock()
	defer cs.mu.Unlock()

	key := contextKey(userID, language, level)
	entry := cs.data[key]
	entry.Sessions = append(entry.Sessions, msgs)
	if len(entry.Sessions) > maxContextSessions {
		entry.Sessions = entry.Sessions[len(entry.Sessions)-maxContextSessions:]
	}
	entry.UpdatedAt = time.Now()
	cs.data[key] = entry
	cs.persist()
}

func (cs *ContextStore) Get(userID, language string, level int) []Message {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	entry, ok := cs.data[contextKey(userID, language, level)]
	if !ok || len(entry.Sessions) == 0 {
		return nil
	}
	var all []Message
	for _, session := range entry.Sessions {
		all = append(all, session...)
	}
	return all
}

func (cs *ContextStore) load() {
	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &cs.data)
}

func (cs *ContextStore) persist() {
	_ = os.MkdirAll("data", 0755)
	data, err := json.MarshalIndent(cs.data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(cs.filePath, data, 0600)
}

// ── Conversation History Store ────────────────────────────────────────────────

const maxHistoryPerUser = 10

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
	mu       sync.RWMutex
	records  map[string][]*ConversationRecord // userID → records (newest first)
	byID     map[string]*ConversationRecord
	filePath string
}

func NewConversationHistoryStore() *ConversationHistoryStore {
	hs := &ConversationHistoryStore{
		records:  make(map[string][]*ConversationRecord),
		byID:     make(map[string]*ConversationRecord),
		filePath: "data/conv_history.json",
	}
	hs.load()
	return hs
}

func (hs *ConversationHistoryStore) Save(record *ConversationRecord) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.byID[record.ID] = record
	recs := hs.records[record.UserID]
	// Prepend so newest is first
	recs = append([]*ConversationRecord{record}, recs...)
	if len(recs) > maxHistoryPerUser {
		// Remove the oldest from the byID index too
		old := recs[maxHistoryPerUser]
		delete(hs.byID, old.ID)
		recs = recs[:maxHistoryPerUser]
	}
	hs.records[record.UserID] = recs
	hs.persist()
}

func (hs *ConversationHistoryStore) GetForUser(userID string) []*ConversationRecord {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	recs := hs.records[userID]
	if recs == nil {
		return []*ConversationRecord{}
	}
	return recs
}

func (hs *ConversationHistoryStore) GetRecord(id string) (*ConversationRecord, error) {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	r, ok := hs.byID[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return r, nil
}

type historyPersist struct {
	Records []*ConversationRecord `json:"records"`
}

func (hs *ConversationHistoryStore) load() {
	data, err := os.ReadFile(hs.filePath)
	if err != nil {
		return
	}
	var p historyPersist
	if err := json.Unmarshal(data, &p); err != nil {
		return
	}
	for _, r := range p.Records {
		hs.byID[r.ID] = r
		hs.records[r.UserID] = append(hs.records[r.UserID], r)
	}
	// Records were stored newest-first per user
}

func (hs *ConversationHistoryStore) persist() {
	_ = os.MkdirAll("data", 0755)
	var all []*ConversationRecord
	for _, recs := range hs.records {
		all = append(all, recs...)
	}
	p := historyPersist{Records: all}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(hs.filePath, data, 0600)
}
