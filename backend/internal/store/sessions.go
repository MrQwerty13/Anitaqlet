package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const SessionTTL = 30 * 24 * time.Hour
const SessionCookie = "anitaqlet_session"

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) Create(userID, ip, ua string) (token string, expiresAt time.Time, err error) {
	token, err = randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().UTC().Add(SessionTTL)
	_, err = s.db.Exec(`
		INSERT INTO sessions (id, user_id, expires_at, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?)`,
		token, userID, expiresAt.Format(time.RFC3339Nano), nullStr(ip), nullStr(ua),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return token, expiresAt, nil
}

func (s *SessionStore) UserID(token string) (string, error) {
	var userID string
	var expires string
	err := s.db.QueryRow(`
		SELECT user_id, expires_at FROM sessions WHERE id = ?`, token,
	).Scan(&userID, &expires)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	t, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		t, err = time.Parse(time.RFC3339, expires)
		if err != nil {
			return "", err
		}
	}
	if time.Now().UTC().After(t) {
		_ = s.Delete(token)
		return "", nil
	}
	return userID, nil
}

func (s *SessionStore) Delete(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, token)
	return err
}

func (s *SessionStore) DeleteExpired() error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now)
	return err
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
