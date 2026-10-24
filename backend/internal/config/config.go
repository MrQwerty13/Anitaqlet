package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultSessionSecret = "dev-change-me"

type Config struct {
	Addr          string
	DBPath        string
	FrontendURL   string
	SessionSecret string

	AppEnv       string // development | production
	CookieSecure bool
	LogLevel     string

	AllowedOrigins []string

	YandexClientID     string
	YandexClientSecret string
	VKClientID         string
	VKClientSecret     string
	GitHubClientID     string
	GitHubClientSecret string

	OAuthRedirectBase string // e.g. http://localhost:8080

	LibreTranslateURL string
	LibreTranslateKey string

	YooKassaShopID    string
	YooKassaSecretKey string
	YooKassaAPIBase   string
	YooKassaReturnURL string
	PremiumPriceRUB   int
}

func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func (c Config) PaymentsEnabled() bool {
	return c.YooKassaShopID != "" && c.YooKassaSecretKey != ""
}

func (c Config) TranslateEnabled() bool {
	return c.LibreTranslateURL != ""
}

func Load() (Config, error) {
	frontendURL := getenv("FRONTEND_URL", "http://localhost:8080")
	appEnv := getenv("APP_ENV", "development")

	cfg := Config{
		Addr:          getenv("ADDR", ":8080"),
		DBPath:        getenv("DB_PATH", "/data/anitaqlet.db"),
		FrontendURL:   frontendURL,
		SessionSecret: getenv("SESSION_SECRET", defaultSessionSecret),

		AppEnv:   appEnv,
		LogLevel: getenv("LOG_LEVEL", "info"),

		YandexClientID:     os.Getenv("YANDEX_CLIENT_ID"),
		YandexClientSecret: os.Getenv("YANDEX_CLIENT_SECRET"),
		VKClientID:         os.Getenv("VK_CLIENT_ID"),
		VKClientSecret:     os.Getenv("VK_CLIENT_SECRET"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),

		OAuthRedirectBase: getenv("OAUTH_REDIRECT_BASE", "http://localhost:8080"),

		LibreTranslateURL: strings.TrimRight(os.Getenv("LIBRETRANSLATE_URL"), "/"),
		LibreTranslateKey: os.Getenv("LIBRETRANSLATE_API_KEY"),

		YooKassaShopID:    os.Getenv("YOOKASSA_SHOP_ID"),
		YooKassaSecretKey: os.Getenv("YOOKASSA_SECRET_KEY"),
		YooKassaAPIBase:   strings.TrimRight(getenv("YOOKASSA_API_BASE", "https://api.yookassa.ru"), "/"),
		YooKassaReturnURL: getenv("YOOKASSA_RETURN_URL", frontendURL+"/?paid=1"),
		PremiumPriceRUB:   getenvInt("PREMIUM_PRICE_RUB", 99),
	}

	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		cfg.CookieSecure = v == "true" || v == "1"
	} else {
		cfg.CookieSecure = strings.HasPrefix(frontendURL, "https://")
	}

	cfg.AllowedOrigins = parseOrigins(os.Getenv("ALLOWED_ORIGINS"))
	if len(cfg.AllowedOrigins) == 0 {
		if o := originOf(frontendURL); o != "" {
			cfg.AllowedOrigins = []string{o}
		}
	}

	if cfg.IsProduction() {
		if cfg.SessionSecret == "" || cfg.SessionSecret == defaultSessionSecret {
			return Config{}, fmt.Errorf("SESSION_SECRET must be set to a non-default value when APP_ENV=production (got default %q)", defaultSessionSecret)
		}
	}
	return cfg, nil
}

func parseOrigins(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.Trim(strings.TrimSpace(part), "/")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func originOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
