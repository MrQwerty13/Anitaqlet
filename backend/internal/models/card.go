package models

type Card struct {
	ID              string  `json:"id"`
	DeckID          string  `json:"deck_id"`
	Term            string  `json:"term"`
	Translation     string  `json:"translation"`
	TermLang        *string `json:"term_lang,omitempty"`
	TranslationLang *string `json:"translation_lang,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}
