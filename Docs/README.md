# Anitaqlet

Anitaqlet is a self-hosted Quizlet analog — an MVP web app for learning foreign words (decks of term/translation cards, quiz mode, offline support). UI is in Russian; the project targets the Russian market and is designed around 152-ФЗ requirements (minimal personal data, explicit consent, right to deletion).

**Status: early MVP.** Auth and infrastructure are in place; decks/cards/quiz APIs are not implemented yet (schema exists). See [Roadmap](#roadmap).

## Stack

| Layer     | Choice                                                                  |
| --------- | ----------------------------------------------------------------------- |
| Backend   | Go 1.26, [gin](https://github.com/gin-gonic/gin)                        |
| Database  | SQLite via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (pure Go, `CGO_ENABLED=0`) |
| Auth      | OAuth2 (`golang.org/x/oauth2`) — **Yandex, VK, GitHub**                  |
| Frontend  | Vanilla HTML + CSS + JS, no frameworks or build step                    |
| Packaging | Docker (multi-stage) + docker-compose                                   |
| Migrations| Plain SQL files (`backend/migrations/*.sql`), applied at startup        |

## Project layout

```
Anitaqlet/
├── backend/
│   ├── cmd/server/main.go      # wiring: config, db, auth, routes, static frontend
│   ├── internal/
│   │   ├── auth/               # OAuth flows, state, profile fetching per provider
│   │   ├── config/             # env-based config
│   │   ├── db/                 # open + migrate
│   │   ├── handlers/           # auth, health handlers
│   │   ├── middleware/         # optional/required session auth
│   │   ├── models/             # user/profile types
│   │   └── store/              # users, sessions (with tests)
│   ├── migrations/             # 001_init.sql, 002_sessions.sql
│   └── Dockerfile
├── frontend/                   # served statically by the backend
│   ├── index.html              # login, profile, consent dialog
│   ├── privacy.html            # privacy policy (152-ФЗ)
│   ├── css/styles.css          # CSS variables, light/dark via prefers-color-scheme
│   └── js/app.js               # session state, consent flow, logout
├── docker-compose.yml
├── .env.example
└── Docs/                       # README, TODO, LICENSE
```

## Getting started

### Run with Docker

```bash
cp .env.example .env   # fill in OAuth credentials and SESSION_SECRET
docker compose up --build
```

The app is served at <http://localhost:8080> (backend and frontend on the same origin). SQLite is persisted in the `sqlite_data` volume.

### Run locally (development)

```bash
cd backend
DB_PATH=../data/anitaqlet.db FRONTEND_DIR=../frontend go run ./cmd/server
```

### Tests

```bash
cd backend
go test ./...
```

## Configuration

All configuration is environment variables (see `.env.example`):

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | Listen address |
| `DB_PATH` | `/data/anitaqlet.db` | SQLite file path |
| `MIGRATIONS_DIR` | `/app/migrations` | SQL migrations directory |
| `FRONTEND_DIR` | `/app/frontend` | Static frontend directory |
| `FRONTEND_URL` | `http://localhost:8080` | Public origin of the app |
| `OAUTH_REDIRECT_BASE` | `http://localhost:8080` | Base for OAuth redirect URIs |
| `SESSION_SECRET` | `dev-change-me` | Secret — **must be set in production** |
| `YANDEX_CLIENT_ID` / `YANDEX_CLIENT_SECRET` | — | Yandex OAuth app |
| `VK_CLIENT_ID` / `VK_CLIENT_SECRET` | — | VK OAuth app |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | — | GitHub OAuth app |

Register redirect URIs with each provider as `{OAUTH_REDIRECT_BASE}/auth/callback/{yandex|vk|github}`.

## HTTP API (implemented)

| Method | Route | Auth | Description |
| --- | --- | --- | --- |
| `GET` | `/health` | — | Liveness probe |
| `GET` | `/auth/yandex`, `/auth/vk`, `/auth/github` | — | Start OAuth flow |
| `GET` | `/auth/callback/{yandex\|vk\|github}` | — | OAuth callback; sets session cookie; may redirect to `/?consent=1` |
| `GET` | `/auth/me` | optional | Current user, `needs_consent`, `privacy_policy_version` |
| `POST` | `/auth/consent` | required | Record personal-data consent (152-ФЗ) |
| `POST` | `/auth/logout` | optional | Destroy session |
| `GET` | `/`, `/privacy.html`, `/static/*`, `/js/*` | — | Static frontend |

Session is a cookie backed by the `sessions` table (expired rows are purged at startup).

## Database

Migrations run automatically on startup.

- **001_init.sql** — `users` (OAuth identity, consent fields, `is_premium`/`premium_until`, soft-delete), `consent_logs` (consent audit trail), `decks`, `cards`, `quiz_sessions`
- **002_sessions.sql** — `sessions`

152-ФЗ-oriented choices: only the data needed from the OAuth provider (name, email, avatar), explicit consent flag + timestamp + policy version, an audit log of consents, and soft deletion (`deleted_at`) for the right to be forgotten.

## Frontend

Single-page shell with no build tooling:

- Guest state: login buttons for Yandex / VK / GitHub
- Signed-in state: profile (avatar, name, email), logout
- Consent dialog shown when the backend reports missing consent
- Light/dark theme follows `prefers-color-scheme`; accent color via `--accent` CSS variable

## Roadmap

### Done

- [x] Repo structure: `backend/`, `frontend/`, `docker-compose.yml`, `.env.example`
- [x] Go module, config, SQLite open + SQL migrations
- [x] OAuth (Yandex, VK, GitHub): start, callback, profile fetch, signed state
- [x] Session cookie auth + `middleware` (optional/required)
- [x] Consent flow (`/auth/consent`, consent dialog, privacy policy page)
- [x] Schema for users, consent logs, decks, cards, quiz sessions, premium flags
- [x] Docker multi-stage build (non-root user, `CGO_ENABLED=0`), compose volume for SQLite
- [x] Health endpoint, static frontend serving, store unit tests

### Planned (from `Docs/TODO.md`)

- [ ] Decks/cards CRUD API and UI (create/delete cards, group into decks)
- [ ] Auto-translation via LibreTranslate (self-hosted or public instance)
- [ ] Quiz mode: pick a deck → shuffled cards → answer → score
- [ ] Offline mode: Service Worker + IndexedDB cache, background sync
- [ ] Premium subscription: free tier limits (100 cards, 1 deck), 99₽/mo unlimited, enforced by backend middleware
- [ ] UI polish: accent color picker, full Apple-like minimalist styling

## License

MIT — see [LICENSE](LICENSE).
