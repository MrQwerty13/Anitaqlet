-- Anitaqlet schema (152-ФЗ: минимально необходимые ПДн + согласия + право на удаление)

PRAGMA foreign_keys = ON;

-- Пользователи (OAuth: yandex | vk | github)
CREATE TABLE IF NOT EXISTS users (
    id                TEXT PRIMARY KEY,
    oauth_provider    TEXT NOT NULL CHECK (oauth_provider IN ('yandex', 'vk', 'github')),
    oauth_id          TEXT NOT NULL,
    email             TEXT,
    display_name      TEXT,
    avatar_url        TEXT,

    -- Согласие на обработку персональных данных (ст. 9 152-ФЗ)
    consent_personal_data INTEGER NOT NULL DEFAULT 0 CHECK (consent_personal_data IN (0, 1)),
    consent_at            TEXT, -- ISO8601
    privacy_policy_version TEXT,

    -- Подписка
    is_premium        INTEGER NOT NULL DEFAULT 0 CHECK (is_premium IN (0, 1)),
    premium_until     TEXT, -- ISO8601, NULL если не премиум

    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at        TEXT, -- soft-delete / право на забвение

    UNIQUE (oauth_provider, oauth_id)
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

-- Журнал согласий (аудит для 152-ФЗ)
CREATE TABLE IF NOT EXISTS consent_logs (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    consent_type  TEXT NOT NULL, -- personal_data | marketing | ...
    granted       INTEGER NOT NULL CHECK (granted IN (0, 1)),
    policy_version TEXT,
    ip_address    TEXT,
    user_agent    TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_consent_logs_user ON consent_logs(user_id);

-- Стопки карточек
CREATE TABLE IF NOT EXISTS decks (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_decks_user ON decks(user_id) WHERE deleted_at IS NULL;

-- Карточки (слово — перевод)
CREATE TABLE IF NOT EXISTS cards (
    id              TEXT PRIMARY KEY,
    deck_id         TEXT NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    term            TEXT NOT NULL,      -- иностранное / изучаемое
    translation     TEXT NOT NULL,      -- родной язык / перевод
    term_lang       TEXT,
    translation_lang TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    deleted_at      TEXT
);

CREATE INDEX IF NOT EXISTS idx_cards_deck ON cards(deck_id) WHERE deleted_at IS NULL;

-- Сессии квизов (для истории результатов)
CREATE TABLE IF NOT EXISTS quiz_sessions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deck_id      TEXT NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    total_cards  INTEGER NOT NULL DEFAULT 0,
    correct      INTEGER NOT NULL DEFAULT 0,
    finished_at  TEXT,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_quiz_sessions_user ON quiz_sessions(user_id);
