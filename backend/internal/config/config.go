package config

import (
	"os"
)

type Config struct {
	Addr          string
	DBPath        string
	FrontendURL   string
	SessionSecret string

	YandexClientID     string
	YandexClientSecret string
	VKClientID         string
	VKClientSecret     string
	GitHubClientID     string
	GitHubClientSecret string

	OAuthRedirectBase string // e.g. http://localhost:8080
}

func Load() Config {
	return Config{
		Addr:          getenv("ADDR", ":8080"),
		DBPath:        getenv("DB_PATH", "/data/anitaqlet.db"),
		FrontendURL:   getenv("FRONTEND_URL", "http://localhost:8080"),
		SessionSecret: getenv("SESSION_SECRET", "dev-change-me"),

		YandexClientID:     os.Getenv("YANDEX_CLIENT_ID"),
		YandexClientSecret: os.Getenv("YANDEX_CLIENT_SECRET"),
		VKClientID:         os.Getenv("VK_CLIENT_ID"),
		VKClientSecret:     os.Getenv("VK_CLIENT_SECRET"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),

		OAuthRedirectBase: getenv("OAUTH_REDIRECT_BASE", "http://localhost:8080"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
