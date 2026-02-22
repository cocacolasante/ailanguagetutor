package store

import (
	"encoding/json"
	"errors"
	"os"
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

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Language  string    `json:"language"`
	Topic     string    `json:"topic"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

func (us *UserStore) Create(email, username, password string) (*User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	if _, exists := us.byEmail[email]; exists {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &User{
		ID:           uuid.New().String(),
		Email:        email,
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
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

func (us *UserStore) load() {
	data, err := os.ReadFile(us.filePath)
	if err != nil {
		return
	}
	var users []*User
	if err := json.Unmarshal(data, &users); err != nil {
		return
	}
	for _, u := range users {
		us.byEmail[u.Email] = u
		us.byID[u.ID] = u
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

func (ss *SessionStore) Create(userID, language, topic, systemPrompt string) *Session {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	s := &Session{
		ID:       uuid.New().String(),
		UserID:   userID,
		Language: language,
		Topic:    topic,
		Messages: []Message{{Role: "system", Content: systemPrompt}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
	// Return a copy without the system message
	var msgs []Message
	for _, m := range s.Messages {
		if m.Role != "system" {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}
