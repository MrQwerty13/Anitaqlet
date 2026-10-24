package store

import (
	"database/sql"

	"github.com/google/uuid"
	"github.com/mikhail/anitaqlet/backend/internal/models"
)

type PaymentStore struct {
	db *sql.DB
}

func NewPaymentStore(db *sql.DB) *PaymentStore {
	return &PaymentStore{db: db}
}

func (s *PaymentStore) GetByExternalID(externalID string) (*models.Payment, error) {
	return s.scan(s.db.QueryRow(`
		SELECT id, user_id, provider, external_id, amount_cents, currency, status, created_at, updated_at
		FROM payments WHERE provider = 'yookassa' AND external_id = ?`,
		externalID,
	))
}

func (s *PaymentStore) UpsertSucceeded(userID, externalID string, amountCents int, currency string) (created bool, err error) {
	existing, err := s.GetByExternalID(externalID)
	if err != nil {
		return false, err
	}
	now := nowRFC3339()
	if existing != nil {
		if existing.Status == "succeeded" {
			return false, nil
		}
		_, err = s.db.Exec(`
			UPDATE payments SET status = 'succeeded', updated_at = ? WHERE id = ?`,
			now, existing.ID,
		)
		return err == nil, err
	}
	_, err = s.db.Exec(`
		INSERT INTO payments (id, user_id, provider, external_id, amount_cents, currency, status)
		VALUES (?, ?, 'yookassa', ?, ?, ?, 'succeeded')`,
		uuid.NewString(), userID, externalID, amountCents, currency,
	)
	return err == nil, err
}

func (s *PaymentStore) InsertPending(userID, externalID string, amountCents int, currency string) error {
	_, err := s.db.Exec(`
		INSERT INTO payments (id, user_id, provider, external_id, amount_cents, currency, status)
		VALUES (?, ?, 'yookassa', ?, ?, ?, 'pending')
		ON CONFLICT (provider, external_id) DO NOTHING`,
		uuid.NewString(), userID, externalID, amountCents, currency,
	)
	return err
}

func (s *PaymentStore) scan(row *sql.Row) (*models.Payment, error) {
	var p models.Payment
	err := row.Scan(&p.ID, &p.UserID, &p.Provider, &p.ExternalID, &p.AmountCents, &p.Currency, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
