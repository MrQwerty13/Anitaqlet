package models

type Deck struct {
	ID          string  `json:"id"`
	UserID      string  `json:"-"`
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	CardCount   int     `json:"card_count"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}
