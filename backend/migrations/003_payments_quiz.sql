ALTER TABLE quiz_sessions ADD COLUMN answered INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS quiz_answers (
    session_id TEXT NOT NULL REFERENCES quiz_sessions(id) ON DELETE CASCADE,
    card_id    TEXT NOT NULL,
    answer     TEXT NOT NULL,
    correct    INTEGER NOT NULL CHECK (correct IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (session_id, card_id)
);
CREATE INDEX IF NOT EXISTS idx_quiz_answers_session ON quiz_answers(session_id);

CREATE TABLE IF NOT EXISTS payments (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider     TEXT NOT NULL DEFAULT 'yookassa',
    external_id  TEXT NOT NULL,
    amount_cents INTEGER NOT NULL,
    currency     TEXT NOT NULL DEFAULT 'RUB',
    status       TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (provider, external_id)
);
CREATE INDEX IF NOT EXISTS idx_payments_user ON payments(user_id);
