package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const stateCookie = "anitaqlet_oauth_state"
const stateTTL = 10 * time.Minute

type oauthState struct {
	Provider string `json:"p"`
	Nonce    string `json:"n"`
	Exp      int64  `json:"e"`
}

func (s *OAuthService) setStateCookie(c *gin.Context, provider, nonce string) error {
	st := oauthState{
		Provider: provider,
		Nonce:    nonce,
		Exp:      time.Now().Add(stateTTL).Unix(),
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	sig := sign(s.cfg.SessionSecret, raw)
	val := base64.RawURLEncoding.EncodeToString(raw) + "." + sig
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     stateCookie,
		Value:    val,
		Path:     "/",
		MaxAge:   int(stateTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *OAuthService) consumeState(c *gin.Context, provider, returnedState string) error {
	cookie, err := c.Cookie(stateCookie)
	// clear cookie
	http.SetCookie(c.Writer, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	if err != nil {
		return fmt.Errorf("missing state cookie")
	}
	parts := strings.SplitN(cookie, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid state cookie")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("invalid state encoding")
	}
	if !hmac.Equal([]byte(parts[1]), []byte(sign(s.cfg.SessionSecret, raw))) {
		return fmt.Errorf("invalid state signature")
	}
	var st oauthState
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("invalid state payload")
	}
	if time.Now().Unix() > st.Exp {
		return fmt.Errorf("state expired")
	}
	if st.Provider != provider {
		return fmt.Errorf("provider mismatch")
	}
	if st.Nonce != returnedState {
		return fmt.Errorf("state mismatch")
	}
	return nil
}

func sign(secret string, raw []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
