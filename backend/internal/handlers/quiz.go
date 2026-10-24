package handlers

import (
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
)

type startQuizRequest struct {
	DeckID string `json:"deck_id"`
}

func (h *API) StartQuiz(c *gin.Context) {
	u := middleware.UserFrom(c)
	var req startQuizRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.DeckID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deck_id required"})
		return
	}
	deck, err := h.decks.Get(u.ID, req.DeckID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deck get failed"})
		return
	}
	if deck == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
		return
	}
	quizCards, err := h.cards.TermsForQuiz(deck.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cards load failed"})
		return
	}
	if len(quizCards) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deck is empty"})
		return
	}
	for i := len(quizCards) - 1; i > 0; i-- {
		j := rand.IntN(i + 1)
		quizCards[i], quizCards[j] = quizCards[j], quizCards[i]
	}
	session, err := h.quizzes.CreateSession(u.ID, deck.ID, len(quizCards))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "quiz session failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session": session,
		"cards":   quizCards,
	})
}

type answerRequest struct {
	CardID string `json:"card_id"`
	Answer string `json:"answer"`
}

func (h *API) AnswerQuiz(c *gin.Context) {
	u := middleware.UserFrom(c)
	session, err := h.quizzes.GetSession(u.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session get failed"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if session.FinishedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "session already finished"})
		return
	}
	var req answerRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.CardID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "card_id required"})
		return
	}
	req.Answer = strings.TrimSpace(req.Answer)
	card, err := h.cards.GetForDeck(session.DeckID, req.CardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "card get failed"})
		return
	}
	if card == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
		return
	}
	correct := answersMatch(req.Answer, card.Translation) || answersMatch(req.Answer, card.Term)
	if err := h.quizzes.RecordAnswer(session.ID, card.ID, req.Answer, correct); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "answer save failed"})
		return
	}
	updated, err := h.quizzes.GetSession(u.ID, session.ID)
	if err != nil || updated == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session reload failed"})
		return
	}
	if updated.Answered >= updated.TotalCards && updated.FinishedAt == nil {
		_ = h.quizzes.FinishSession(u.ID, session.ID)
		updated, _ = h.quizzes.GetSession(u.ID, session.ID)
	}
	c.JSON(http.StatusOK, gin.H{
		"correct":     correct,
		"translation": card.Translation,
		"session":     updated,
	})
}

func (h *API) FinishQuiz(c *gin.Context) {
	u := middleware.UserFrom(c)
	session, err := h.quizzes.GetSession(u.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session get failed"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if err := h.quizzes.FinishSession(u.ID, session.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "finish failed"})
		return
	}
	updated, err := h.quizzes.GetSession(u.ID, session.ID)
	if err != nil || updated == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session reload failed"})
		return
	}
	percent := 0
	if updated.TotalCards > 0 {
		percent = updated.Correct * 100 / updated.TotalCards
	}
	c.JSON(http.StatusOK, gin.H{"session": updated, "percent": percent})
}

func answersMatch(given, want string) bool {
	return normalizeAnswer(given) == normalizeAnswer(want)
}

func normalizeAnswer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	return s
}
