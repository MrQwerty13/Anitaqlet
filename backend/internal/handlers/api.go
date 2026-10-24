package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mikhail/anitaqlet/backend/internal/config"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
	"github.com/mikhail/anitaqlet/backend/internal/models"
	"github.com/mikhail/anitaqlet/backend/internal/payments"
	"github.com/mikhail/anitaqlet/backend/internal/store"
	"github.com/mikhail/anitaqlet/backend/internal/translate"
)

const (
	freeDeckLimit = 1
	freeCardLimit = 100
)

type API struct {
	decks      *store.DeckStore
	cards      *store.CardStore
	quizzes    *store.QuizStore
	users      *store.UserStore
	payments   *store.PaymentStore
	translator *translate.Client
	gateway    payments.Gateway
	cfg        config.Config
}

func NewAPI(decks *store.DeckStore, cards *store.CardStore, quizzes *store.QuizStore, users *store.UserStore, payStore *store.PaymentStore, translator *translate.Client, gateway payments.Gateway, cfg config.Config) *API {
	return &API{
		decks:      decks,
		cards:      cards,
		quizzes:    quizzes,
		users:      users,
		payments:   payStore,
		translator: translator,
		gateway:    gateway,
		cfg:        cfg,
	}
}

func (h *API) limitsPayload(u *models.User, deckCount, cardCount int) gin.H {
	premium := u.PremiumActive()
	maxDecks := deckCount
	if premium {
		maxDecks = -1
	} else {
		maxDecks = freeDeckLimit
	}
	maxCards := cardCount
	if premium {
		maxCards = -1
	} else {
		maxCards = freeCardLimit
	}
	return gin.H{
		"premium": premium,
		"decks":   gin.H{"used": deckCount, "max": maxDecks},
		"cards":   gin.H{"used": cardCount, "max": maxCards},
	}
}

type deckRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ID          string `json:"id"`
}

func (h *API) ListDecks(c *gin.Context) {
	u := middleware.UserFrom(c)
	decks, err := h.decks.ListByUser(u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decks list failed"})
		return
	}
	cardCount, err := h.cards.CountByUser(u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "card count failed"})
		return
	}
	if decks == nil {
		decks = []models.Deck{}
	}
	c.JSON(http.StatusOK, gin.H{
		"decks":  decks,
		"limits": h.limitsPayload(u, len(decks), cardCount),
	})
}

func (h *API) CreateDeck(c *gin.Context) {
	u := middleware.UserFrom(c)
	var req deckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || len(req.Title) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title must be 1-200 characters"})
		return
	}
	if len(req.Description) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "description too long"})
		return
	}
	if req.ID != "" {
		if _, err := uuid.Parse(req.ID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
	}
	if !u.PremiumActive() {
		count, err := h.decks.CountByUser(u.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "deck count failed"})
			return
		}
		if count >= freeDeckLimit {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "deck_limit",
				"message": "На бесплатном тарифе доступна 1 колода. Перейдите на премиум для безлимита.",
			})
			return
		}
	}
	deck, err := h.decks.Create(u.ID, req.Title, req.Description, req.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deck create failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"deck": deck})
}

func (h *API) GetDeck(c *gin.Context) {
	u := middleware.UserFrom(c)
	deck, err := h.decks.Get(u.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deck get failed"})
		return
	}
	if deck == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deck": deck})
}

func (h *API) DeleteDeck(c *gin.Context) {
	u := middleware.UserFrom(c)
	deck, err := h.decks.Get(u.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deck get failed"})
		return
	}
	if deck == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
		return
	}
	if err := h.decks.Delete(u.ID, deck.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deck delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type cardRequest struct {
	Term            string `json:"term"`
	Translation     string `json:"translation"`
	TermLang        string `json:"term_lang"`
	TranslationLang string `json:"translation_lang"`
	ID              string `json:"id"`
}

func (h *API) ListCards(c *gin.Context) {
	u := middleware.UserFrom(c)
	deck, err := h.decks.Get(u.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deck get failed"})
		return
	}
	if deck == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
		return
	}
	cards, err := h.cards.ListByDeck(deck.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cards list failed"})
		return
	}
	if cards == nil {
		cards = []models.Card{}
	}
	c.JSON(http.StatusOK, gin.H{"cards": cards})
}

func (h *API) CreateCard(c *gin.Context) {
	u := middleware.UserFrom(c)
	deck, err := h.decks.Get(u.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deck get failed"})
		return
	}
	if deck == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
		return
	}
	var req cardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	req.Term = strings.TrimSpace(req.Term)
	req.Translation = strings.TrimSpace(req.Translation)
	if req.Term == "" || len(req.Term) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "term must be 1-500 characters"})
		return
	}
	if req.Translation == "" || len(req.Translation) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "translation must be 1-500 characters"})
		return
	}
	if req.ID != "" {
		if _, err := uuid.Parse(req.ID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
	}
	if !u.PremiumActive() {
		count, err := h.cards.CountByUser(u.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "card count failed"})
			return
		}
		if count >= freeCardLimit {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "card_limit",
				"message": "На бесплатном тарифе доступно до 100 карточек. Перейдите на премиум для безлимита.",
			})
			return
		}
	}
	card, err := h.cards.Create(deck.ID, req.Term, req.Translation, req.TermLang, req.TranslationLang, req.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "card create failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"card": card})
}

type cardUpdateRequest struct {
	Term            *string `json:"term"`
	Translation     *string `json:"translation"`
	TermLang        *string `json:"term_lang"`
	TranslationLang *string `json:"translation_lang"`
}

func (h *API) UpdateCard(c *gin.Context) {
	u := middleware.UserFrom(c)
	var req cardUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if req.Term != nil {
		t := strings.TrimSpace(*req.Term)
		if t == "" || len(t) > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "term must be 1-500 characters"})
			return
		}
		req.Term = &t
	}
	if req.Translation != nil {
		t := strings.TrimSpace(*req.Translation)
		if t == "" || len(t) > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "translation must be 1-500 characters"})
			return
		}
		req.Translation = &t
	}
	existing, err := h.cards.Get(u.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "card get failed"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
		return
	}
	card, err := h.cards.Update(u.ID, existing.ID, store.CardUpdate{
		Term:            req.Term,
		Translation:     req.Translation,
		TermLang:        req.TermLang,
		TranslationLang: req.TranslationLang,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "card update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"card": card})
}

func (h *API) DeleteCard(c *gin.Context) {
	u := middleware.UserFrom(c)
	if err := h.cards.Delete(u.ID, c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "card delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type translateRequest struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func (h *API) Translate(c *gin.Context) {
	if !h.translator.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "translation_disabled"})
		return
	}
	var req translateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" || len(req.Text) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text must be 1-500 characters"})
		return
	}
	if req.Source == "" {
		req.Source = "auto"
	}
	if req.Target == "" {
		req.Target = "ru"
	}
	out, err := h.translator.Translate(c.Request.Context(), req.Text, req.Source, req.Target)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "translation_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"translation": out})
}
