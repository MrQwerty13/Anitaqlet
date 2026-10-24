package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/auth"
	"github.com/mikhail/anitaqlet/backend/internal/config"
	"github.com/mikhail/anitaqlet/backend/internal/db"
	"github.com/mikhail/anitaqlet/backend/internal/handlers"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
	"github.com/mikhail/anitaqlet/backend/internal/payments"
	"github.com/mikhail/anitaqlet/backend/internal/store"
	"github.com/mikhail/anitaqlet/backend/internal/translate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config: " + err.Error())
		os.Exit(1)
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	sqlDB, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Error("db: " + err.Error())
		os.Exit(1)
	}
	defer sqlDB.Close()

	migrationsDir := getenv("MIGRATIONS_DIR", findMigrationsDir())
	if err := db.Migrate(sqlDB, migrationsDir); err != nil {
		log.Error("migrate: " + err.Error())
		os.Exit(1)
	}
	log.Info("sqlite ready", slog.String("path", cfg.DBPath))

	users := store.NewUserStore(sqlDB)
	sessions := store.NewSessionStore(sqlDB)
	decks := store.NewDeckStore(sqlDB)
	cards := store.NewCardStore(sqlDB)
	quizzes := store.NewQuizStore(sqlDB)
	payStore := store.NewPaymentStore(sqlDB)
	_ = sessions.DeleteExpired()

	features := gin.H{
		"translate": cfg.TranslateEnabled(),
		"payments":  cfg.PaymentsEnabled(),
	}
	oauth := auth.NewOAuthService(cfg, users, sessions)
	authH := handlers.NewAuthHandler(oauth, users, sessions, cfg.CookieSecure, features)
	mw := middleware.NewAuth(users, sessions)

	translator := translate.NewLibreTranslate(cfg.LibreTranslateURL, cfg.LibreTranslateKey)
	var gateway payments.Gateway
	if cfg.PaymentsEnabled() {
		gateway = payments.NewYooKassa(cfg.YooKassaShopID, cfg.YooKassaSecretKey, cfg.YooKassaAPIBase)
	}
	apiH := handlers.NewAPI(decks, cards, quizzes, users, payStore, translator, gateway, cfg)
	payH := handlers.NewPaymentsHandler(gateway, payStore, users, cfg)

	authLimiter := middleware.NewRateLimiter(10, time.Minute, log)
	callbackLimiter := middleware.NewRateLimiter(30, time.Minute, log)
	translateLimiter := middleware.NewRateLimiter(30, time.Minute, log)
	paymentsLimiter := middleware.NewRateLimiter(10, time.Minute, log)
	webhookLimiter := middleware.NewRateLimiter(60, time.Minute, log)

	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logger(log))
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(mw.Optional())

	r.GET("/health", handlers.Health(sqlDB))

	authGroup := r.Group("/auth")
	{
		authGroup.GET("/yandex", middleware.RateLimit(authLimiter, "auth-begin"), authH.BeginYandex)
		authGroup.GET("/vk", middleware.RateLimit(authLimiter, "auth-begin"), authH.BeginVK)
		authGroup.GET("/github", middleware.RateLimit(authLimiter, "auth-begin"), authH.BeginGitHub)

		authGroup.GET("/callback/yandex", middleware.RateLimit(callbackLimiter, "auth-callback"), authH.CallbackYandex)
		authGroup.GET("/callback/vk", middleware.RateLimit(callbackLimiter, "auth-callback"), authH.CallbackVK)
		authGroup.GET("/callback/github", middleware.RateLimit(callbackLimiter, "auth-callback"), authH.CallbackGitHub)

		authGroup.GET("/me", authH.Me)
		authGroup.POST("/logout", authH.Logout)
		authGroup.POST("/consent", mw.Required(), authH.Consent)
	}

	api := r.Group("/api")
	{
		consented := mw.ConsentRequired()
		api.GET("/decks", consented, apiH.ListDecks)
		api.POST("/decks", consented, apiH.CreateDeck)
		api.GET("/decks/:id", consented, apiH.GetDeck)
		api.DELETE("/decks/:id", consented, apiH.DeleteDeck)
		api.GET("/decks/:id/cards", consented, apiH.ListCards)
		api.POST("/decks/:id/cards", consented, apiH.CreateCard)
		api.PATCH("/cards/:id", consented, apiH.UpdateCard)
		api.DELETE("/cards/:id", consented, apiH.DeleteCard)
		api.POST("/translate", consented, middleware.RateLimit(translateLimiter, "translate"), apiH.Translate)

		api.POST("/quiz/sessions", consented, apiH.StartQuiz)
		api.POST("/quiz/sessions/:id/answers", consented, apiH.AnswerQuiz)
		api.POST("/quiz/sessions/:id/finish", consented, apiH.FinishQuiz)

		api.POST("/payments/create", consented, middleware.RateLimit(paymentsLimiter, "payments"), payH.Create)
		api.POST("/payments/yookassa/webhook", middleware.RateLimit(webhookLimiter, "webhook"), payH.YooKassaWebhook)
	}

	frontendDir := getenv("FRONTEND_DIR", "../frontend")
	if st, err := os.Stat(frontendDir); err == nil && st.IsDir() {
		r.Static("/static", filepath.Join(frontendDir, "css"))
		r.Static("/js", filepath.Join(frontendDir, "js"))
		r.StaticFile("/", filepath.Join(frontendDir, "index.html"))
		r.StaticFile("/privacy.html", filepath.Join(frontendDir, "privacy.html"))
		r.StaticFile("/sw.js", filepath.Join(frontendDir, "js", "sw.js"))
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", slog.String("addr", cfg.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server: " + err.Error())
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown: " + err.Error())
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
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
