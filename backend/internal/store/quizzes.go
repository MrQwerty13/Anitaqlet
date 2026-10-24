package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/mikhail/anitaqlet/backend/internal/models"
)

type QuizStore struct {
	db *sql.DB
}

func NewQuizStore(db *sql.DB) *QuizStore {
	return &QuizStore{db: db}
}

func (s *QuizStore) CreateSession(userID, deckID string, total int) (*models.QuizSession, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(`
		INSERT INTO quiz_sessions (id, user_id, deck_id, total_cards)
		VALUES (?, ?, ?, ?)`,
		id, userID, deckID, total,
	)
	if err != nil {
		return nil, fmt.Errorf("insert quiz session: %w", err)
	}
	return s.GetSession(userID, id)
}

func (s *QuizStore) GetSession(userID, sessionID string) (*models.QuizSession, error) {
	var q models.QuizSession
	var finished sql.NullString
	err := s.db.QueryRow(`
		SELECT id, user_id, deck_id, total_cards, answered, correct, finished_at, created_at
		FROM quiz_sessions WHERE id = ? AND user_id = ?`,
		sessionID, userID,
	).Scan(&q.ID, &q.UserID, &q.DeckID, &q.TotalCards, &q.Answered, &q.Correct, &finished, &q.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q.FinishedAt = strPtr(finished)
	return &q, nil
}

func (s *QuizStore) RecordAnswer(sessionID, cardID, answer string, correct bool) error {
	correctInt := 0
	if correct {
		correctInt = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO quiz_answers (session_id, card_id, answer, correct)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (session_id, card_id) DO NOTHING`,
		sessionID, cardID, answer, correctInt,
	)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		UPDATE quiz_sessions SET
			answered = (SELECT COUNT(*) FROM quiz_answers WHERE session_id = ?),
			correct = (SELECT COUNT(*) FROM quiz_answers WHERE session_id = ? AND correct = 1)
		WHERE id = ?`,
		sessionID, sessionID, sessionID,
	)
	return err
}

func (s *QuizStore) FinishSession(userID, sessionID string) error {
	now := nowRFC3339()
	_, err := s.db.Exec(`
		UPDATE quiz_sessions SET finished_at = ?
		WHERE id = ? AND user_id = ? AND finished_at IS NULL`,
		now, sessionID, userID,
	)
	return err
}
