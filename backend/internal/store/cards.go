package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mikhail/anitaqlet/backend/internal/models"
)

type CardStore struct {
	db *sql.DB
}

func NewCardStore(db *sql.DB) *CardStore {
	return &CardStore{db: db}
}

func (s *CardStore) Create(deckID, term, translation, termLang, translationLang, requestedID string) (*models.Card, error) {
	if requestedID != "" {
		if existing, err := s.GetAny(requestedID); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}
	id := requestedID
	if id == "" {
		id = uuid.NewString()
	}
	_, err := s.db.Exec(`
		INSERT INTO cards (id, deck_id, term, translation, term_lang, translation_lang)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, deckID, term, translation, nullStr(termLang), nullStr(translationLang),
	)
	if err != nil {
		return nil, fmt.Errorf("insert card: %w", err)
	}
	return s.GetAny(id)
}

func (s *CardStore) ListByDeck(deckID string) ([]models.Card, error) {
	rows, err := s.db.Query(`
		SELECT id, deck_id, term, translation, term_lang, translation_lang, created_at, updated_at
		FROM cards
		WHERE deck_id = ? AND deleted_at IS NULL
		ORDER BY created_at ASC`,
		deckID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *CardStore) Get(userID, cardID string) (*models.Card, error) {
	var c models.Card
	var termLang, translationLang sql.NullString
	err := s.db.QueryRow(`
		SELECT c.id, c.deck_id, c.term, c.translation, c.term_lang, c.translation_lang, c.created_at, c.updated_at
		FROM cards c
		JOIN decks d ON d.id = c.deck_id
		WHERE c.id = ? AND d.user_id = ? AND c.deleted_at IS NULL AND d.deleted_at IS NULL`,
		cardID, userID,
	).Scan(&c.ID, &c.DeckID, &c.Term, &c.Translation, &termLang, &translationLang, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.TermLang = strPtr(termLang)
	c.TranslationLang = strPtr(translationLang)
	return &c, nil
}

func (s *CardStore) GetAny(cardID string) (*models.Card, error) {
	var c models.Card
	var termLang, translationLang sql.NullString
	err := s.db.QueryRow(`
		SELECT id, deck_id, term, translation, term_lang, translation_lang, created_at, updated_at
		FROM cards WHERE id = ? AND deleted_at IS NULL`,
		cardID,
	).Scan(&c.ID, &c.DeckID, &c.Term, &c.Translation, &termLang, &translationLang, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.TermLang = strPtr(termLang)
	c.TranslationLang = strPtr(translationLang)
	return &c, nil
}

type CardUpdate struct {
	Term            *string
	Translation     *string
	TermLang        *string
	TranslationLang *string
}

func (s *CardStore) Update(userID, cardID string, upd CardUpdate) (*models.Card, error) {
	now := nowRFC3339()
	_, err := s.db.Exec(`
		UPDATE cards SET
			term = COALESCE(?, term),
			translation = COALESCE(?, translation),
			term_lang = COALESCE(?, term_lang),
			translation_lang = COALESCE(?, translation_lang),
			updated_at = ?
		FROM decks
		WHERE cards.id = ? AND cards.deck_id = decks.id AND decks.user_id = ? AND cards.deleted_at IS NULL`,
		nullable(upd.Term), nullable(upd.Translation), nullable(upd.TermLang), nullable(upd.TranslationLang),
		now, cardID, userID,
	)
	if err != nil {
		return nil, err
	}
	return s.Get(userID, cardID)
}

func (s *CardStore) Delete(userID, cardID string) error {
	now := nowRFC3339()
	_, err := s.db.Exec(`
		UPDATE cards SET deleted_at = ?, updated_at = ?
		FROM decks
		WHERE cards.id = ? AND cards.deck_id = decks.id AND decks.user_id = ? AND cards.deleted_at IS NULL`,
		now, now, cardID, userID,
	)
	return err
}

func (s *CardStore) CountByDeck(deckID string) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM cards WHERE deck_id = ? AND deleted_at IS NULL`, deckID,
	).Scan(&n)
	return n, err
}

func (s *CardStore) CountByUser(userID string) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM cards c
		JOIN decks d ON d.id = c.deck_id
		WHERE d.user_id = ? AND c.deleted_at IS NULL AND d.deleted_at IS NULL`,
		userID,
	).Scan(&n)
	return n, err
}

func (s *CardStore) TermsForQuiz(deckID string) ([]models.QuizCard, error) {
	rows, err := s.db.Query(`
		SELECT id, term, term_lang FROM cards
		WHERE deck_id = ? AND deleted_at IS NULL`,
		deckID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.QuizCard
	for rows.Next() {
		var q models.QuizCard
		var termLang sql.NullString
		if err := rows.Scan(&q.ID, &q.Term, &termLang); err != nil {
			return nil, err
		}
		q.TermLang = strPtr(termLang)
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *CardStore) GetForDeck(deckID, cardID string) (*models.Card, error) {
	var c models.Card
	var termLang, translationLang sql.NullString
	err := s.db.QueryRow(`
		SELECT id, deck_id, term, translation, term_lang, translation_lang, created_at, updated_at
		FROM cards WHERE id = ? AND deck_id = ? AND deleted_at IS NULL`,
		cardID, deckID,
	).Scan(&c.ID, &c.DeckID, &c.Term, &c.Translation, &termLang, &translationLang, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.TermLang = strPtr(termLang)
	c.TranslationLang = strPtr(translationLang)
	return &c, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanCard(row rowScanner) (*models.Card, error) {
	var c models.Card
	var termLang, translationLang sql.NullString
	if err := row.Scan(&c.ID, &c.DeckID, &c.Term, &c.Translation, &termLang, &translationLang, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.TermLang = strPtr(termLang)
	c.TranslationLang = strPtr(translationLang)
	return &c, nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func nullable(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
