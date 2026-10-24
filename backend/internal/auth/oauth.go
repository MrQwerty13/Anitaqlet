package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/config"
	"github.com/mikhail/anitaqlet/backend/internal/store"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

const (
	ProviderYandex = "yandex"
	ProviderVK     = "vk"
	ProviderGitHub = "github"
)

type OAuthService struct {
	cfg      config.Config
	configs  map[string]*oauth2.Config
	users    *store.UserStore
	sessions *store.SessionStore
}

func NewOAuthService(cfg config.Config, users *store.UserStore, sessions *store.SessionStore) *OAuthService {
	base := cfg.OAuthRedirectBase
	s := &OAuthService{
		cfg:      cfg,
		configs:  make(map[string]*oauth2.Config),
		users:    users,
		sessions: sessions,
	}

	s.configs[ProviderYandex] = &oauth2.Config{
		ClientID:     cfg.YandexClientID,
		ClientSecret: cfg.YandexClientSecret,
		RedirectURL:  base + "/auth/callback/yandex",
		Scopes:       []string{"login:email", "login:info", "login:avatar"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://oauth.yandex.ru/authorize",
			TokenURL: "https://oauth.yandex.ru/token",
		},
	}

	s.configs[ProviderVK] = &oauth2.Config{
		ClientID:     cfg.VKClientID,
		ClientSecret: cfg.VKClientSecret,
		RedirectURL:  base + "/auth/callback/vk",
		Scopes:       []string{"email"},
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://oauth.vk.com/authorize",
			TokenURL:  "https://oauth.vk.com/access_token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	s.configs[ProviderGitHub] = &oauth2.Config{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		RedirectURL:  base + "/auth/callback/github",
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	return s
}

func (s *OAuthService) configFor(provider string) (*oauth2.Config, error) {
	c, ok := s.configs[provider]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
	return c, nil
}

func (s *OAuthService) BeginRedirect(c *gin.Context, provider string) {
	cfg, err := s.configFor(provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cfg.ClientID == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":    "oauth not configured",
			"provider": provider,
			"hint":     "set client id/secret env vars",
		})
		return
	}

	nonce, err := randomNonce(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "state generation failed"})
		return
	}
	if err := s.setStateCookie(c, provider, nonce); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "state cookie failed"})
		return
	}

	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOnline}
	if provider == ProviderVK {
		opts = append(opts, oauth2.SetAuthURLParam("display", "page"))
	}
	c.Redirect(http.StatusFound, cfg.AuthCodeURL(nonce, opts...))
}

func (s *OAuthService) HandleCallback(c *gin.Context, provider string) {
	cfg, err := s.configFor(provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if errMsg := c.Query("error"); errMsg != "" {
		c.Redirect(http.StatusFound, "/?auth_error="+url.QueryEscape(errMsg))
		return
	}

	state := c.Query("state")
	if err := s.consumeState(c, provider, state); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid oauth state", "detail": err.Error()})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code"})
		return
	}

	ctx := c.Request.Context()
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "token exchange failed", "detail": err.Error()})
		return
	}

	profile, err := s.fetchProfile(ctx, provider, tok)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "profile fetch failed", "detail": err.Error()})
		return
	}

	user, _, err := s.users.UpsertFromOAuth(*profile)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user upsert failed", "detail": err.Error()})
		return
	}

	token, expires, err := s.sessions.Create(user.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session create failed"})
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     store.SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
	})

	if !user.ConsentPersonalData {
		c.Redirect(http.StatusFound, "/?consent=1")
		return
	}
	c.Redirect(http.StatusFound, "/")
}

func randomNonce(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
