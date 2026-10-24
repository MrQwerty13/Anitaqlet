package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mikhail/anitaqlet/backend/internal/models"
	"golang.org/x/oauth2"
)

func (s *OAuthService) fetchProfile(ctx context.Context, provider string, tok *oauth2.Token) (*models.Profile, error) {
	switch provider {
	case ProviderYandex:
		return fetchYandex(ctx, tok.AccessToken)
	case ProviderVK:
		return fetchVK(ctx, tok)
	case ProviderGitHub:
		return fetchGitHub(ctx, tok.AccessToken)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func fetchYandex(ctx context.Context, accessToken string) (*models.Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://login.yandex.ru/info?format=json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)
	body, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var info struct {
		ID              string `json:"id"`
		Login           string `json:"login"`
		DisplayName     string `json:"display_name"`
		RealName        string `json:"real_name"`
		DefaultEmail    string `json:"default_email"`
		DefaultAvatarID string `json:"default_avatar_id"`
		IsAvatarEmpty   bool   `json:"is_avatar_empty"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	if info.ID == "" {
		return nil, fmt.Errorf("yandex: empty id")
	}
	name := info.DisplayName
	if name == "" {
		name = info.RealName
	}
	if name == "" {
		name = info.Login
	}
	avatar := ""
	if !info.IsAvatarEmpty && info.DefaultAvatarID != "" {
		avatar = "https://avatars.yandex.net/get-yapic/" + info.DefaultAvatarID + "/islands-200"
	}
	return &models.Profile{
		Provider:    ProviderYandex,
		OAuthID:     info.ID,
		Email:       info.DefaultEmail,
		DisplayName: name,
		AvatarURL:   avatar,
	}, nil
}

func fetchVK(ctx context.Context, tok *oauth2.Token) (*models.Profile, error) {
	// VK puts user_id (and sometimes email) into the token response extras.
	userID := ""
	if v := tok.Extra("user_id"); v != nil {
		switch t := v.(type) {
		case float64:
			userID = strconv.FormatInt(int64(t), 10)
		case json.Number:
			userID = t.String()
		case string:
			userID = t
		default:
			userID = fmt.Sprint(t)
		}
	}
	email := ""
	if v := tok.Extra("email"); v != nil {
		email, _ = v.(string)
	}

	q := url.Values{}
	q.Set("access_token", tok.AccessToken)
	q.Set("v", "5.199")
	q.Set("fields", "photo_200")
	if userID != "" {
		q.Set("user_ids", userID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.vk.com/method/users.get?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	body, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Response []struct {
			ID        int    `json:"id"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Photo200  string `json:"photo_200"`
		} `json:"response"`
		Error *struct {
			ErrorMsg string `json:"error_msg"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("vk: %s", resp.Error.ErrorMsg)
	}
	if len(resp.Response) == 0 {
		return nil, fmt.Errorf("vk: empty profile")
	}
	u := resp.Response[0]
	id := strconv.Itoa(u.ID)
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	return &models.Profile{
		Provider:    ProviderVK,
		OAuthID:     id,
		Email:       email,
		DisplayName: name,
		AvatarURL:   u.Photo200,
	}, nil
}

func fetchGitHub(ctx context.Context, accessToken string) (*models.Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	body, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var info struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	if info.ID == 0 {
		return nil, fmt.Errorf("github: empty id")
	}
	email := info.Email
	if email == "" {
		email, _ = fetchGitHubPrimaryEmail(ctx, accessToken)
	}
	name := info.Name
	if name == "" {
		name = info.Login
	}
	return &models.Profile{
		Provider:    ProviderGitHub,
		OAuthID:     strconv.FormatInt(info.ID, 10),
		Email:       email,
		DisplayName: name,
		AvatarURL:   info.AvatarURL,
	}, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	body, err := doJSON(req)
	if err != nil {
		return "", err
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	return "", nil
}

func doJSON(req *http.Request) ([]byte, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
