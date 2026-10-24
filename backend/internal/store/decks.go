package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/mikhail/anitaqlet/backend/internal/models"
)

type DeckStore struct {
	db *sql.DB
}

func NewDeckStore(db *sql.DB) *DeckStore {
	return &DeckStore{db: db}
}

func (s *DeckStore) Create(userID, title, description, requestedID string) (*models.Deck, error) {
	if requestedID != "" {
		if existing, err := s.Get(userID, requestedID); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}
	id := requestedID
	if id == "" {
		id = uuid.NewString()
	}
	var desc interface{}
	if description != "" {
		desc = description
	}
	_, err := s.db.Exec(`
		INSERT INTO decks (id, user_id, title, description)
		VALUES (?, ?, ?, ?)`,
		id, userID, title, desc,
	)
	if err != nil {
		return nil, fmt.Errorf("insert deck: %w", err)
	}
	return s.Get(userID, id)
}

func (s *DeckStore) ListByUser(userID string) ([]models.Deck, error) {
	rows, err := s.db.Query(`
		SELECT d.id, d.user_id, d.title, d.description,
			(SELECT COUNT(*) FROM cards c WHERE c.deck_id = d.id AND c.deleted_at IS NULL),
			d.created_at, d.updated_at
		FROM decks d
		WHERE d.user_id = ? AND d.deleted_at IS NULL
		ORDER BY d.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Deck
	for rows.Next() {
		var d models.Deck
		var desc sql.NullString
		if err := rows.Scan(&d.ID, &d.UserID, &d.Title, &desc, &d.CardCount, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Description = strPtr(desc)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *DeckStore) Get(userID, deckID string) (*models.Deck, error) {
	var d models.Deck
	var desc sql.NullString
	err := s.db.QueryRow(`
		SELECT d.id, d.user_id, d.title, d.description,
			(SELECT COUNT(*) FROM cards c WHERE c.deck_id = d.id AND c.deleted_at IS NULL),
			d.created_at, d.updated_at
		FROM decks d
		WHERE d.id = ? AND d.user_id = ? AND d.deleted_at IS NULL`,
		deckID, userID,
	).Scan(&d.ID, &d.UserID, &d.Title, &desc, &d.CardCount, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Description = strPtr(desc)
	return &d, nil
}

func (s *DeckStore) Delete(userID, deckID string) error {
	_, err := s.db.Exec(`
		UPDATE decks SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		nowRFC3339(), nowRFC3339(), deckID, userID,
	)
	return err
}

func (s *DeckStore) CountByUser(userID string) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM decks WHERE user_id = ? AND deleted_at IS NULL`, userID,
	).Scan(&n)
	return n, err
}
