package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool, pings the DB, and runs auto-migration.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := autoMigrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	log.Println("database: connected and migrated")
	return pool, nil
}

func autoMigrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin BOOLEAN DEFAULT FALSE,
    approved BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    email_verified BOOLEAN DEFAULT FALSE,
    email_verify_token TEXT DEFAULT '',
    pending_plan TEXT DEFAULT '',
    stripe_customer_id TEXT DEFAULT '',
    stripe_subscription_id TEXT DEFAULT '',
    subscription_status TEXT DEFAULT '',
    trial_ends_at TIMESTAMPTZ,
    streak INT DEFAULT 0,
    last_activity_date TEXT DEFAULT '',
    total_fp INT DEFAULT 0,
    language_fp JSONB DEFAULT '{}',
    language_level JSONB DEFAULT '{}',
    achievements JSONB DEFAULT '[]',
    conversation_count INT DEFAULT 0,
    pref_language TEXT DEFAULT '',
    pref_level INT DEFAULT 0,
    pref_personality TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS conversation_history (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT,
    language TEXT,
    topic TEXT,
    topic_name TEXT,
    level INT,
    personality TEXT DEFAULT '',
    message_count INT DEFAULT 0,
    duration_secs INT DEFAULT 0,
    fp_earned INT DEFAULT 0,
    summary TEXT DEFAULT '',
    topics JSONB DEFAULT '[]',
    vocabulary JSONB DEFAULT '[]',
    corrections JSONB DEFAULT '[]',
    suggestions JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    ended_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS conversation_contexts (
    user_id TEXT NOT NULL,
    language TEXT NOT NULL,
    level INT NOT NULL,
    sessions JSONB DEFAULT '[]',
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, language, level)
);

CREATE TABLE IF NOT EXISTS student_profiles (
    user_id TEXT NOT NULL,
    language TEXT NOT NULL,
    name TEXT DEFAULT '',
    weak_areas JSONB DEFAULT '[]',
    strong_areas JSONB DEFAULT '[]',
    recent_topics JSONB DEFAULT '[]',
    recent_vocab JSONB DEFAULT '[]',
    next_suggestions JSONB DEFAULT '[]',
    session_count INT DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, language)
);
`)
	if err != nil {
		return err
	}

	// Add password reset columns to existing tables (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_reset_token TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_reset_expires_at TIMESTAMPTZ;
`)
	if err != nil {
		return err
	}

	// Add sentence builder column (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE student_profiles ADD COLUMN IF NOT EXISTS recent_sentences JSONB DEFAULT '[]';
`)
	if err != nil {
		return err
	}

	// Add list-cache index columns (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE student_profiles ADD COLUMN IF NOT EXISTS vocab_list_idx    JSONB DEFAULT '{}';
ALTER TABLE student_profiles ADD COLUMN IF NOT EXISTS sentence_list_idx JSONB DEFAULT '{}';
`)
	if err != nil {
		return err
	}

	// Add listening comprehension index column (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE student_profiles ADD COLUMN IF NOT EXISTS listening_list_idx JSONB DEFAULT '{}';
`)
	if err != nil {
		return err
	}

	// Writing Coach: cache index on student profiles (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE student_profiles ADD COLUMN IF NOT EXISTS writing_list_idx JSONB DEFAULT '{}';
`)
	if err != nil {
		return err
	}

	// Writing Coach: misspellings on conversation_history (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE conversation_history ADD COLUMN IF NOT EXISTS misspellings JSONB DEFAULT '[]';
`)
	if err != nil {
		return err
	}

	// Mistake tracking: separate weak vocab and weak grammar (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE student_profiles ADD COLUMN IF NOT EXISTS weak_vocab   JSONB DEFAULT '[]';
ALTER TABLE student_profiles ADD COLUMN IF NOT EXISTS weak_grammar JSONB DEFAULT '[]';
`)
	if err != nil {
		return err
	}

	// Password reset tokens moved to Redis — drop Postgres columns (idempotent)
	_, err = pool.Exec(ctx, `
ALTER TABLE users DROP COLUMN IF EXISTS password_reset_token;
ALTER TABLE users DROP COLUMN IF EXISTS password_reset_expires_at;
`)
	return err
}

// MigrateFromJSON imports existing JSON data into Postgres on first run (no-op if data exists).
func MigrateFromJSON(ctx context.Context, pool *pgxpool.Pool) {
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil || count > 0 {
		return
	}

	usersData, err := os.ReadFile("data/users.json")
	if err != nil {
		return
	}

	type jsonUser struct {
		ID                   string         `json:"id"`
		Email                string         `json:"email"`
		Username             string         `json:"username"`
		PasswordHash         string         `json:"password_hash"`
		IsAdmin              bool           `json:"is_admin"`
		Approved             bool           `json:"approved"`
		CreatedAt            time.Time      `json:"created_at"`
		EmailVerified        bool           `json:"email_verified"`
		EmailVerifyToken     string         `json:"email_verify_token"`
		PendingPlan          string         `json:"pending_plan"`
		StripeCustomerID     string         `json:"stripe_customer_id"`
		StripeSubscriptionID string         `json:"stripe_subscription_id"`
		SubscriptionStatus   string         `json:"subscription_status"`
		TrialEndsAt          *time.Time     `json:"trial_ends_at"`
		Streak               int            `json:"streak"`
		LastActivityDate     string         `json:"last_activity_date"`
		TotalFP              int            `json:"total_fp"`
		LanguageFP           map[string]int `json:"language_fp"`
		LanguageLevel        map[string]int `json:"language_level"`
		Achievements         []string       `json:"achievements"`
		ConversationCount    int            `json:"conversation_count"`
		PrefLanguage         string         `json:"pref_language"`
		PrefLevel            int            `json:"pref_level"`
		PrefPersonality      string         `json:"pref_personality"`
	}

	var users []jsonUser
	if err := json.Unmarshal(usersData, &users); err != nil {
		log.Printf("migrate: failed to parse users.json: %v", err)
		return
	}

	userCount := 0
	for _, u := range users {
		if u.LanguageFP == nil {
			u.LanguageFP = map[string]int{}
		}
		if u.LanguageLevel == nil {
			u.LanguageLevel = map[string]int{}
		}
		if u.Achievements == nil {
			u.Achievements = []string{}
		}
		langFP, _ := json.Marshal(u.LanguageFP)
		langLevel, _ := json.Marshal(u.LanguageLevel)
		achievements, _ := json.Marshal(u.Achievements)

		_, err := pool.Exec(ctx, `
INSERT INTO users (id, email, username, password_hash, is_admin, approved, created_at,
    email_verified, email_verify_token, pending_plan,
    stripe_customer_id, stripe_subscription_id, subscription_status, trial_ends_at,
    streak, last_activity_date, total_fp, language_fp, language_level, achievements,
    conversation_count, pref_language, pref_level, pref_personality)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
ON CONFLICT (id) DO NOTHING`,
			u.ID, u.Email, u.Username, u.PasswordHash, u.IsAdmin, u.Approved, u.CreatedAt,
			u.EmailVerified, u.EmailVerifyToken, u.PendingPlan,
			u.StripeCustomerID, u.StripeSubscriptionID, u.SubscriptionStatus, u.TrialEndsAt,
			u.Streak, u.LastActivityDate, u.TotalFP, langFP, langLevel, achievements,
			u.ConversationCount, u.PrefLanguage, u.PrefLevel, u.PrefPersonality,
		)
		if err != nil {
			log.Printf("migrate: user %s: %v", u.Email, err)
			continue
		}
		userCount++
	}
	log.Printf("migrate: imported %d users from data/users.json", userCount)

	histData, err := os.ReadFile("data/conv_history.json")
	if err == nil {
		type jsonRecord struct {
			ID           string    `json:"id"`
			UserID       string    `json:"user_id"`
			SessionID    string    `json:"session_id"`
			Language     string    `json:"language"`
			Topic        string    `json:"topic"`
			TopicName    string    `json:"topic_name"`
			Level        int       `json:"level"`
			Personality  string    `json:"personality"`
			MessageCount int       `json:"message_count"`
			DurationSecs int       `json:"duration_secs"`
			FPEarned     int       `json:"fp_earned"`
			Summary      string    `json:"summary"`
			Topics       []string  `json:"topics_discussed"`
			Vocabulary   []string  `json:"vocabulary_learned"`
			Corrections  []string  `json:"grammar_corrections"`
			Suggestions  []string  `json:"suggested_next_lessons"`
			CreatedAt    time.Time `json:"created_at"`
			EndedAt      time.Time `json:"ended_at"`
		}
		var hp struct {
			Records []jsonRecord `json:"records"`
		}
		if err := json.Unmarshal(histData, &hp); err == nil {
			histCount := 0
			for _, r := range hp.Records {
				if r.Topics == nil {
					r.Topics = []string{}
				}
				if r.Vocabulary == nil {
					r.Vocabulary = []string{}
				}
				if r.Corrections == nil {
					r.Corrections = []string{}
				}
				if r.Suggestions == nil {
					r.Suggestions = []string{}
				}
				topics, _ := json.Marshal(r.Topics)
				vocab, _ := json.Marshal(r.Vocabulary)
				corrections, _ := json.Marshal(r.Corrections)
				suggestions, _ := json.Marshal(r.Suggestions)
				_, err := pool.Exec(ctx, `
INSERT INTO conversation_history (id, user_id, session_id, language, topic, topic_name, level,
    personality, message_count, duration_secs, fp_earned, summary,
    topics, vocabulary, corrections, suggestions, created_at, ended_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
ON CONFLICT (id) DO NOTHING`,
					r.ID, r.UserID, r.SessionID, r.Language, r.Topic, r.TopicName, r.Level,
					r.Personality, r.MessageCount, r.DurationSecs, r.FPEarned, r.Summary,
					topics, vocab, corrections, suggestions, r.CreatedAt, r.EndedAt,
				)
				if err == nil {
					histCount++
				}
			}
			log.Printf("migrate: imported %d conversation records", histCount)
		}
	}

	ctxData, err := os.ReadFile("data/contexts.json")
	if err == nil {
		type contextEntry struct {
			Sessions  []json.RawMessage `json:"sessions"`
			UpdatedAt time.Time         `json:"updated_at"`
		}
		var contexts map[string]contextEntry
		if err := json.Unmarshal(ctxData, &contexts); err == nil {
			ctxCount := 0
			for key, entry := range contexts {
				userID, language, level, err := splitContextKey(key)
				if err != nil {
					continue
				}
				sessions, _ := json.Marshal(entry.Sessions)
				_, err = pool.Exec(ctx, `
INSERT INTO conversation_contexts (user_id, language, level, sessions, updated_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (user_id, language, level) DO NOTHING`,
					userID, language, level, sessions, entry.UpdatedAt,
				)
				if err == nil {
					ctxCount++
				}
			}
			log.Printf("migrate: imported %d context entries", ctxCount)
		}
	}
}

func splitContextKey(key string) (userID, language string, level int, err error) {
	// key format: "userID:language:level" — find last two colons
	last := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == ':' {
			last = i
			break
		}
	}
	if last < 0 {
		return "", "", 0, fmt.Errorf("invalid key")
	}
	second := -1
	for i := last - 1; i >= 0; i-- {
		if key[i] == ':' {
			second = i
			break
		}
	}
	if second < 0 {
		return "", "", 0, fmt.Errorf("invalid key")
	}
	userID = key[:second]
	language = key[second+1 : last]
	_, err = fmt.Sscanf(key[last+1:], "%d", &level)
	return
}
