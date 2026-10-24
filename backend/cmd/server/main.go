package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/auth"
	"github.com/mikhail/anitaqlet/backend/internal/config"
	"github.com/mikhail/anitaqlet/backend/internal/db"
	"github.com/mikhail/anitaqlet/backend/internal/handlers"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
	"github.com/mikhail/anitaqlet/backend/internal/store"
)

func main() {
	cfg := config.Load()

	sqlDB, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer sqlDB.Close()

	migrationsDir := getenv("MIGRATIONS_DIR", findMigrationsDir())
	if err := db.Migrate(sqlDB, migrationsDir); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("sqlite ready: %s", cfg.DBPath)

	users := store.NewUserStore(sqlDB)
	sessions := store.NewSessionStore(sqlDB)
	_ = sessions.DeleteExpired()

	oauth := auth.NewOAuthService(cfg, users, sessions)
	authH := handlers.NewAuthHandler(oauth, users, sessions)
	mw := middleware.NewAuth(users, sessions)

	r := gin.Default()
	r.Use(mw.Optional())

	r.GET("/health", handlers.Health)

	authGroup := r.Group("/auth")
	{
		authGroup.GET("/yandex", authH.BeginYandex)
		authGroup.GET("/vk", authH.BeginVK)
		authGroup.GET("/github", authH.BeginGitHub)

		authGroup.GET("/callback/yandex", authH.CallbackYandex)
		authGroup.GET("/callback/vk", authH.CallbackVK)
		authGroup.GET("/callback/github", authH.CallbackGitHub)

		authGroup.GET("/me", authH.Me)
		authGroup.POST("/logout", authH.Logout)
		authGroup.POST("/consent", mw.Required(), authH.Consent)
	}

	frontendDir := getenv("FRONTEND_DIR", "../frontend")
	if st, err := os.Stat(frontendDir); err == nil && st.IsDir() {
		r.Static("/static", filepath.Join(frontendDir, "css"))
		r.Static("/js", filepath.Join(frontendDir, "js"))
		r.StaticFile("/", filepath.Join(frontendDir, "index.html"))
		r.StaticFile("/privacy.html", filepath.Join(frontendDir, "privacy.html"))
	}

	log.Printf("listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func findMigrationsDir() string {
	candidates := []string{
		"migrations",
		"backend/migrations",
		"/app/migrations",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "migrations"
}
