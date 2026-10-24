package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mikhail/anitaqlet/backend/internal/models"
)

const PrivacyPolicyVersion = "1.1"

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) UpsertFromOAuth(p models.Profile) (*models.User, bool, error) {
	existing, err := s.FindByOAuth(p.Provider, p.OAuthID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if err := s.updateProfile(existing.ID, p); err != nil {
			return nil, false, err
		}
		u, err := s.GetByID(existing.ID)
		return u, false, err
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	email := nullStr(p.Email)
	name := nullStr(p.DisplayName)
	avatar := nullStr(p.AvatarURL)

	_, err = s.db.Exec(`
		INSERT INTO users (id, oauth_provider, oauth_id, email, display_name, avatar_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.Provider, p.OAuthID, email, name, avatar, now, now,
	)
	if err != nil {
		return nil, false, fmt.Errorf("insert user: %w", err)
	}
	u, err := s.GetByID(id)
	return u, true, err
}

func (s *UserStore) updateProfile(id string, p models.Profile) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		UPDATE users SET
			email = COALESCE(?, email),
			display_name = COALESCE(?, display_name),
			avatar_url = COALESCE(?, avatar_url),
			updated_at = ?,
			deleted_at = NULL
		WHERE id = ?`,
		nullStr(p.Email), nullStr(p.DisplayName), nullStr(p.AvatarURL), now, id,
	)
	return err
}

func (s *UserStore) FindByOAuth(provider, oauthID string) (*models.User, error) {
	return s.scanUser(s.db.QueryRow(`
		SELECT id, oauth_provider, oauth_id, email, display_name, avatar_url,
			consent_personal_data, consent_at, privacy_policy_version,
			is_premium, premium_until, created_at, updated_at
		FROM users
		WHERE oauth_provider = ? AND oauth_id = ? AND deleted_at IS NULL`,
		provider, oauthID,
	))
}

func (s *UserStore) GetByID(id string) (*models.User, error) {
	return s.scanUser(s.db.QueryRow(`
		SELECT id, oauth_provider, oauth_id, email, display_name, avatar_url,
			consent_personal_data, consent_at, privacy_policy_version,
			is_premium, premium_until, created_at, updated_at
		FROM users
		WHERE id = ? AND deleted_at IS NULL`, id,
	))
}

func (s *UserStore) GrantConsent(userID, ip, ua string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE users SET
			consent_personal_data = 1,
			consent_at = ?,
			privacy_policy_version = ?,
			updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		now, PrivacyPolicyVersion, now, userID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO consent_logs (id, user_id, consent_type, granted, policy_version, ip_address, user_agent)
		VALUES (?, ?, 'personal_data', 1, ?, ?, ?)`,
		uuid.NewString(), userID, PrivacyPolicyVersion, nullStr(ip), nullStr(ua),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserStore) ActivatePremium(userID string, days int) error {
	var current sql.NullString
	err := s.db.QueryRow(`
		SELECT premium_until FROM users WHERE id = ? AND deleted_at IS NULL`, userID,
	).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	base := time.Now().UTC()
	if current.Valid {
		if t, err := time.Parse(time.RFC3339Nano, current.String); err == nil && t.After(base) {
			base = t
		}
	}
	until := base.Add(time.Duration(days) * 24 * time.Hour)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		UPDATE users SET is_premium = 1, premium_until = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		until.Format(time.RFC3339Nano), now, userID,
	)
	return err
}

func (s *UserStore) scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	var email, name, avatar, consentAt, policy, premiumUntil sql.NullString
	var consent, premium int
	err := row.Scan(
		&u.ID, &u.OAuthProvider, &u.OAuthID,
		&email, &name, &avatar,
		&consent, &consentAt, &policy,
		&premium, &premiumUntil,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Email = strPtr(email)
	u.DisplayName = strPtr(name)
	u.AvatarURL = strPtr(avatar)
	u.ConsentPersonalData = consent == 1
	u.ConsentAt = strPtr(consentAt)
	u.PrivacyPolicyVersion = strPtr(policy)
	u.IsPremium = premium == 1
	u.PremiumUntil = strPtr(premiumUntil)
	return &u, nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}
