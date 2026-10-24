package models

type Payment struct {
	ID          string `json:"id"`
	UserID      string `json:"-"`
	Provider    string `json:"provider"`
	ExternalID  string `json:"external_id"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
