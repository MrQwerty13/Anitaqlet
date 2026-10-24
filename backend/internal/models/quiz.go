package models

type QuizSession struct {
	ID         string  `json:"id"`
	UserID     string  `json:"-"`
	DeckID     string  `json:"deck_id"`
	TotalCards int     `json:"total_cards"`
	Answered   int     `json:"answered"`
	Correct    int     `json:"correct"`
	FinishedAt *string `json:"finished_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type QuizCard struct {
	ID       string  `json:"id"`
	Term     string  `json:"term"`
	TermLang *string `json:"term_lang,omitempty"`
}

type QuizAnswer struct {
	SessionID string `json:"session_id"`
	CardID    string `json:"card_id"`
	Answer    string `json:"answer"`
	Correct   bool   `json:"correct"`
}
