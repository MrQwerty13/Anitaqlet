package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/auth"
	"github.com/mikhail/anitaqlet/backend/internal/middleware"
	"github.com/mikhail/anitaqlet/backend/internal/store"
)

type AuthHandler struct {
	oauth    *auth.OAuthService
	users    *store.UserStore
	sessions *store.SessionStore
}

func NewAuthHandler(oauth *auth.OAuthService, users *store.UserStore, sessions *store.SessionStore) *AuthHandler {
	return &AuthHandler{oauth: oauth, users: users, sessions: sessions}
}

func (h *AuthHandler) BeginYandex(c *gin.Context) {
	h.oauth.BeginRedirect(c, auth.ProviderYandex)
}

func (h *AuthHandler) BeginVK(c *gin.Context) {
	h.oauth.BeginRedirect(c, auth.ProviderVK)
}

func (h *AuthHandler) BeginGitHub(c *gin.Context) {
	h.oauth.BeginRedirect(c, auth.ProviderGitHub)
}

func (h *AuthHandler) CallbackYandex(c *gin.Context) {
	h.oauth.HandleCallback(c, auth.ProviderYandex)
}

func (h *AuthHandler) CallbackVK(c *gin.Context) {
	h.oauth.HandleCallback(c, auth.ProviderVK)
}

func (h *AuthHandler) CallbackGitHub(c *gin.Context) {
	h.oauth.HandleCallback(c, auth.ProviderGitHub)
}

func (h *AuthHandler) Me(c *gin.Context) {
	u := middleware.UserFrom(c)
	if u == nil {
		c.JSON(http.StatusOK, gin.H{"user": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user":                   u,
		"privacy_policy_version": store.PrivacyPolicyVersion,
		"needs_consent":          !u.ConsentPersonalData,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if token, err := c.Cookie(store.SessionCookie); err == nil && token != "" {
		_ = h.sessions.Delete(token)
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     store.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type consentRequest struct {
	Accepted bool `json:"accepted"`
}

func (h *AuthHandler) Consent(c *gin.Context) {
	u := middleware.UserFrom(c)
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req consentRequest
	if err := c.ShouldBindJSON(&req); err != nil || !req.Accepted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accepted must be true"})
		return
	}
	if err := h.users.GrantConsent(u.ID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "consent save failed"})
		return
	}
	updated, err := h.users.GetByID(u.ID)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "user": updated})
}
