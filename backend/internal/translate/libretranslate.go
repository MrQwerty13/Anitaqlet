package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Result struct {
	Text string `json:"text"`
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewLibreTranslate(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.baseURL != ""
}

func (c *Client) Translate(ctx context.Context, text, source, target string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("translation service is not configured")
	}
	body, err := json.Marshal(map[string]string{
		"q":       text,
		"source":  source,
		"target":  target,
		"format":  "text",
		"api_key": c.apiKey,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/translate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("translate service returned status %d", resp.StatusCode)
	}
	var out struct {
		TranslatedText string `json:"translatedText"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.TranslatedText == "" {
		return "", fmt.Errorf("empty translation")
	}
	return out.TranslatedText, nil
}
