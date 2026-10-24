package handlers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/config"
	"github.com/mikhail/anitaqlet/backend/internal/handlers"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
	"github.com/mikhail/anitaqlet/backend/internal/models"
	"github.com/mikhail/anitaqlet/backend/internal/store"
	"github.com/mikhail/anitaqlet/backend/internal/translate"
	_ "modernc.org/sqlite"
)

type testEnv struct {
	router   *gin.Engine
	users    *store.UserStore
	sessions *store.SessionStore
	db       *sql.DB
	token    string
	userID   string
}

func (e *testEnv) payStore() *store.PaymentStore {
	return store.NewPaymentStore(e.db)
}

func setupRouter(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	root, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".sql" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
	}

	users := store.NewUserStore(db)
	sessions := store.NewSessionStore(db)
	decks := store.NewDeckStore(db)
	cards := store.NewCardStore(db)
	quizzes := store.NewQuizStore(db)
	payStore := store.NewPaymentStore(db)
	translator := translate.NewLibreTranslate("", "")
	cfg := config.Config{PremiumPriceRUB: 99, YooKassaReturnURL: "http://localhost/?paid=1"}

	api := handlers.NewAPI(decks, cards, quizzes, users, payStore, translator, nil, cfg)
	payH := handlers.NewPaymentsHandler(nil, payStore, users, cfg)
	mw := middleware.NewAuth(users, sessions)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw.Optional())
	r.GET("/health", handlers.Health(db))
	consented := mw.ConsentRequired()
	r.GET("/api/decks", consented, api.ListDecks)
	r.POST("/api/decks", consented, api.CreateDeck)
	r.GET("/api/decks/:id", consented, api.GetDeck)
	r.DELETE("/api/decks/:id", consented, api.DeleteDeck)
	r.GET("/api/decks/:id/cards", consented, api.ListCards)
	r.POST("/api/decks/:id/cards", consented, api.CreateCard)
	r.PATCH("/cards/:id", consented, api.UpdateCard)
	r.DELETE("/cards/:id", consented, api.DeleteCard)
	r.POST("/api/translate", consented, api.Translate)
	r.POST("/api/quiz/sessions", consented, api.StartQuiz)
	r.POST("/api/quiz/sessions/:id/answers", consented, api.AnswerQuiz)
	r.POST("/api/quiz/sessions/:id/finish", consented, api.FinishQuiz)
	r.POST("/api/payments/create", consented, payH.Create)
	r.POST("/api/payments/yookassa/webhook", payH.YooKassaWebhook)

	u, created, err := users.UpsertFromOAuth(models.Profile{Provider: "github", OAuthID: "1", Email: "a@b.c", DisplayName: "A"})
	if err != nil || !created {
		t.Fatalf("user upsert: %v", err)
	}
	if err := users.GrantConsent(u.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	token, _, err := sessions.Create(u.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return &testEnv{router: r, users: users, sessions: sessions, db: db, token: token, userID: u.ID}
}

func (e *testEnv) otherToken(t *testing.T) string {
	t.Helper()
	other, _, err := e.users.UpsertFromOAuth(models.Profile{Provider: "github", OAuthID: "other", Email: "x@y.z", DisplayName: "X"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.users.GrantConsent(other.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	token, _, err := e.sessions.Create(other.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func doJSON(t *testing.T, e *testEnv, method, path, token, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: store.SessionCookie, Value: token})
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	var out map[string]interface{}
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("bad json (%d): %s", w.Code, w.Body.String())
		}
	}
	return w.Code, out
}

func TestDeckCardQuizFlow(t *testing.T) {
	e := setupRouter(t)

	code, body := doJSON(t, e, http.MethodPost, "/api/decks", e.token, `{"title":"English basics"}`)
	if code != http.StatusCreated {
		t.Fatalf("create deck: %d %v", code, body)
	}
	deckID := body["deck"].(map[string]interface{})["id"].(string)

	code, body = doJSON(t, e, http.MethodPost, "/api/decks", e.token, `{"title":"Second"}`)
	if code != http.StatusForbidden || body["error"] != "deck_limit" {
		t.Fatalf("expected deck_limit for free tier, got %d %v", code, body)
	}

	code, body = doJSON(t, e, http.MethodPost, "/api/decks/"+deckID+"/cards", e.token,
		`{"term":"cat","translation":"кот","term_lang":"en","translation_lang":"ru"}`)
	if code != http.StatusCreated {
		t.Fatalf("create card: %d %v", code, body)
	}
	cardID := body["card"].(map[string]interface{})["id"].(string)

	code, _ = doJSON(t, e, http.MethodPost, "/api/decks/"+deckID+"/cards", e.token,
		`{"term":"dog","translation":"собака"}`)
	if code != http.StatusCreated {
		t.Fatalf("create card 2: %d", code)
	}

	code, body = doJSON(t, e, http.MethodPatch, "/cards/"+cardID, e.token, `{"translation":"кошка"}`)
	if code != http.StatusOK {
		t.Fatalf("update card: %d %v", code, body)
	}

	code, body = doJSON(t, e, http.MethodGet, "/api/decks/"+deckID+"/cards", e.token, "")
	if code != http.StatusOK {
		t.Fatalf("list cards: %d %v", code, body)
	}
	if got := len(body["cards"].([]interface{})); got != 2 {
		t.Fatalf("expected 2 cards, got %d", got)
	}

	code, body = doJSON(t, e, http.MethodPost, "/api/quiz/sessions", e.token, `{"deck_id":"`+deckID+`"}`)
	if code != http.StatusOK {
		t.Fatalf("start quiz: %d %v", code, body)
	}
	sessionID := body["session"].(map[string]interface{})["id"].(string)
	quizCards := body["cards"].([]interface{})
	if len(quizCards) != 2 {
		t.Fatalf("expected 2 quiz cards, got %d", len(quizCards))
	}

	for _, qc := range quizCards {
		card := qc.(map[string]interface{})
		answer := "dog"
		if card["term"] == "cat" {
			answer = "кошка"
		}
		payload, _ := json.Marshal(map[string]string{"card_id": card["id"].(string), "answer": answer})
		code, body = doJSON(t, e, http.MethodPost, "/api/quiz/sessions/"+sessionID+"/answers", e.token, string(payload))
		if code != http.StatusOK {
			t.Fatalf("answer: %d %v", code, body)
		}
		if body["correct"] != true {
			t.Fatalf("expected correct answer, got %v", body)
		}
	}

	code, body = doJSON(t, e, http.MethodPost, "/api/quiz/sessions/"+sessionID+"/finish", e.token, "")
	if code != http.StatusOK {
		t.Fatalf("finish quiz: %d %v", code, body)
	}
	if body["percent"].(float64) != 100 {
		t.Fatalf("expected 100%%, got %v", body["percent"])
	}

	code, _ = doJSON(t, e, http.MethodDelete, "/api/decks/"+deckID, e.token, "")
	if code != http.StatusOK {
		t.Fatalf("delete deck: %d", code)
	}
	code, body = doJSON(t, e, http.MethodGet, "/api/decks/"+deckID, e.token, "")
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d %v", code, body)
	}
}

func TestFreeCardLimit(t *testing.T) {
	e := setupRouter(t)

	code, body := doJSON(t, e, http.MethodPost, "/api/decks", e.token, `{"title":"Deck"}`)
	if code != http.StatusCreated {
		t.Fatalf("create deck: %d %v", code, body)
	}
	deckID := body["deck"].(map[string]interface{})["id"].(string)

	for i := 0; i < 100; i++ {
		payload := fmt.Sprintf(`{"term":"term%d","translation":"перевод%d"}`, i, i)
		code, body = doJSON(t, e, http.MethodPost, "/api/decks/"+deckID+"/cards", e.token, payload)
		if code != http.StatusCreated {
			t.Fatalf("card %d: %d %v", i, code, body)
		}
	}
	code, body = doJSON(t, e, http.MethodPost, "/api/decks/"+deckID+"/cards", e.token,
		`{"term":"overflow","translation":"перевод"}`)
	if code != http.StatusForbidden || body["error"] != "card_limit" {
		t.Fatalf("expected card_limit, got %d %v", code, body)
	}
}

func TestOwnership(t *testing.T) {
	e := setupRouter(t)

	code, body := doJSON(t, e, http.MethodPost, "/api/decks", e.token, `{"title":"Mine"}`)
	if code != http.StatusCreated {
		t.Fatalf("create deck: %d %v", code, body)
	}
	deckID := body["deck"].(map[string]interface{})["id"].(string)

	other := e.otherToken(t)
	code, body = doJSON(t, e, http.MethodGet, "/api/decks/"+deckID, other, "")
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign deck, got %d %v", code, body)
	}
}

func TestPaymentsDisabled(t *testing.T) {
	e := setupRouter(t)
	code, body := doJSON(t, e, http.MethodPost, "/api/payments/create", e.token, "")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when payments disabled, got %d %v", code, body)
	}
}
