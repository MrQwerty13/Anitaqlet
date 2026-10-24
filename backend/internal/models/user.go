package models

import (
	"time"
)

type User struct {
	ID                   string  `json:"id"`
	OAuthProvider        string  `json:"oauth_provider"`
	OAuthID              string  `json:"-"`
	Email                *string `json:"email,omitempty"`
	DisplayName          *string `json:"display_name,omitempty"`
	AvatarURL            *string `json:"avatar_url,omitempty"`
	ConsentPersonalData  bool    `json:"consent_personal_data"`
	ConsentAt            *string `json:"consent_at,omitempty"`
	PrivacyPolicyVersion *string `json:"privacy_policy_version,omitempty"`
	IsPremium            bool    `json:"is_premium"`
	PremiumUntil         *string `json:"premium_until,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

func (u *User) PremiumActive() bool {
	if !u.IsPremium {
		return false
	}
	if u.PremiumUntil == nil {
		return true
	}
	t, err := time.Parse(time.RFC3339Nano, *u.PremiumUntil)
	if err != nil {
		return false
	}
	return time.Now().UTC().Before(t)
}

// Profile is data pulled from an OAuth provider.
type Profile struct {
	Provider    string
	OAuthID     string
	Email       string
	DisplayName string
	AvatarURL   string
}
