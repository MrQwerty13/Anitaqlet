package store_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/handlers"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
	"github.com/mikhail/anitaqlet/backend/internal/models"
	"github.com/mikhail/anitaqlet/backend/internal/store"
	_ "modernc.org/sqlite"
)

func setup(t *testing.T) (*store.UserStore, *store.SessionStore) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	root, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_init.sql", "002_sessions.sql"} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	return store.NewUserStore(db), store.NewSessionStore(db)
}

func TestUpsertConsentSessionMe(t *testing.T) {
	users, sessions := setup(t)

	u, created, err := users.UpsertFromOAuth(models.Profile{
		Provider:    "github",
		OAuthID:     "42",
		Email:       "a@b.c",
		DisplayName: "Ada",
		AvatarURL:   "https://example.com/a.png",
	})
	if err != nil || !created || u == nil {
		t.Fatalf("upsert: created=%v err=%v user=%v", created, err, u)
	}
	if u.ConsentPersonalData {
		t.Fatal("expected no consent yet")
	}

	token, _, err := sessions.Create(u.ID, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := middleware.NewAuth(users, sessions)
	h := handlers.NewAuthHandler(nil, users, sessions)
	r.Use(mw.Optional())
	r.GET("/auth/me", h.Me)
	r.POST("/auth/consent", mw.Required(), h.Consent)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: store.SessionCookie, Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("me status %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"needs_consent":true`) {
		t.Fatalf("expected needs_consent: %s", w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/auth/consent", strings.NewReader(`{"accepted":true}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: store.SessionCookie, Value: token})
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("consent status %d body %s", w2.Code, w2.Body.String())
	}

	u2, err := users.GetByID(u.ID)
	if err != nil || u2 == nil || !u2.ConsentPersonalData {
		t.Fatalf("consent not saved: %+v err=%v", u2, err)
	}
}
