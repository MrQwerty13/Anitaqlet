package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikhail/anitaqlet/backend/internal/models"
	"github.com/mikhail/anitaqlet/backend/internal/store"
)

const ContextUserKey = "user"

type Auth struct {
	users    *store.UserStore
	sessions *store.SessionStore
}

func NewAuth(users *store.UserStore, sessions *store.SessionStore) *Auth {
	return &Auth{users: users, sessions: sessions}
}

// Optional loads the user into context when a valid session cookie exists.
func (a *Auth) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		if u := a.loadUser(c); u != nil {
			c.Set(ContextUserKey, u)
		}
		c.Next()
	}
}

// Required rejects unauthenticated requests.
func (a *Auth) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := a.loadUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(ContextUserKey, u)
		c.Next()
	}
}

// ConsentRequired requires login + personal-data consent (152-ФЗ).
func (a *Auth) ConsentRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := a.loadUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		outdated := u.PrivacyPolicyVersion == nil || *u.PrivacyPolicyVersion != store.PrivacyPolicyVersion
		if !u.ConsentPersonalData || outdated {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "consent_required",
				"message": "Требуется согласие на обработку персональных данных",
			})
			return
		}
		c.Set(ContextUserKey, u)
		c.Next()
	}
}

func (a *Auth) loadUser(c *gin.Context) *models.User {
	token, err := c.Cookie(store.SessionCookie)
	if err != nil || token == "" {
		return nil
	}
	userID, err := a.sessions.UserID(token)
	if err != nil || userID == "" {
		return nil
	}
	u, err := a.users.GetByID(userID)
	if err != nil || u == nil {
		return nil
	}
	return u
}

func UserFrom(c *gin.Context) *models.User {
	v, ok := c.Get(ContextUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*models.User)
	return u
}
