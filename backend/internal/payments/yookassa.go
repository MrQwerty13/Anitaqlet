package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GatewayPayment struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	Amount       Amount            `json:"amount"`
	Confirmation *Confirmation     `json:"confirmation,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type Amount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type Confirmation struct {
	Type        string `json:"type"`
	ReturnURL   string `json:"return_url,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

type Gateway interface {
	CreatePayment(ctx context.Context, amountRUB int, returnURL, description, userID, idempotenceKey string) (*GatewayPayment, error)
	GetPayment(ctx context.Context, id string) (*GatewayPayment, error)
}

type YooKassa struct {
	shopID  string
	secret  string
	baseURL string
	http    *http.Client
}

func NewYooKassa(shopID, secret, baseURL string) *YooKassa {
	return &YooKassa{
		shopID:  shopID,
		secret:  secret,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (y *YooKassa) CreatePayment(ctx context.Context, amountRUB int, returnURL, description, userID, idempotenceKey string) (*GatewayPayment, error) {
	payload := map[string]interface{}{
		"amount": map[string]string{
			"value":    fmt.Sprintf("%d.00", amountRUB),
			"currency": "RUB",
		},
		"confirmation": map[string]string{
			"type":       "redirect",
			"return_url": returnURL,
		},
		"capture":     true,
		"description": description,
		"metadata": map[string]string{
			"user_id": userID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, y.baseURL+"/v3/payments", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(y.shopID, y.secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotenceKey)

	resp, err := y.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yookassa create payment: status %d", resp.StatusCode)
	}
	var out GatewayPayment
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (y *YooKassa) GetPayment(ctx context.Context, id string) (*GatewayPayment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.baseURL+"/v3/payments/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(y.shopID, y.secret)

	resp, err := y.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yookassa get payment: status %d", resp.StatusCode)
	}
	var out GatewayPayment
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
