# Деплой Anitaqlet на свой сервер с доменом

Полный гайд: VPS + домен + HTTPS (Caddy, автоматический TLS) + Docker Compose. Приложение само отдаёт фронтенд и API на одном порту — наружу через reverse proxy на 80/443.

Примеры ниже для Ubuntu 22.04/24.04 LTS. Домен в примерах: `anitaqlet.example.com` — подставьте свой.

---

## 0. Что нужно

- VPS (1 vCPU / 1 ГБ RAM достаточно; нужен публичный IP)
- Домен и доступ к DNS-записям
- SSH-доступ к серверу (по ключу)
- OAuth-приложения провайдеров (Yandex, VK, GitHub) — понадобятся client id/secret

---

## 1. DNS

Создайте A-запись:

```
Type  Name    Value            TTL
A     @       <IP сервера>     300
```

(или `anitaqlet` для поддомена). Проверить: `dig +short anitaqlet.example.com` должно вернуть IP сервера. До выпуска сертификата записи propagation должен пройти — иначе Let's Encrypt не выпустит сертификат.

## 2. Подготовка сервера

```bash
ssh root@<IP сервера>

apt update && apt upgrade -y
apt install -y ufw git curl
```

**Firewall** (SSH оставляем, HTTP/HTTPS откроем; приложение наружу публиковать не будем):

```bash
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw enable
ufw status
```

**Docker:**

```bash
curl -fsSL https://get.docker.com | sh
usermod -aG docker $USER
```

Выйдите из SSH и зайдите снова (чтобы подхватилась группа docker). Проверка:

```bash
docker --version
docker compose version
```

**Рекомендуется** (SSH-безопасность): запретить вход по паролю и root-логин — в `/etc/ssh/sshd_config`:

```
PasswordAuthentication no
PermitRootLogin no
```

затем `systemctl restart sshd`. Убедитесь, что ключ уже добавлен (`~/.ssh/authorized_keys`), иначе отрежете себя.

## 3. Код и окружение

```bash
cd ~
git clone https://github.com/MrQwerty13/Anitaqlet.git
cd Anitaqlet
cp .env.example .env
```

### 3.1 `.env` на проде

```bash
SESSION_SECRET=$(openssl rand -hex 32)
```

`.env` (заполните своими значениями):

```bash
SESSION_SECRET=<сгенерированная строка выше>

YANDEX_CLIENT_ID=...
YANDEX_CLIENT_SECRET=...
VK_CLIENT_ID=...
VK_CLIENT_SECRET=...
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...

OAUTH_REDIRECT_BASE=https://anitaqlet.example.com
FRONTEND_URL=https://anitaqlet.example.com
```

`.env` в git не попадает (`.gitignore`) — так и оставляем.

### 3.2 OAuth-приложения провайдеров

Redirect URI у всех — `{домен}/auth/callback/{провайдер}`:

**GitHub** → Settings → Developer settings → OAuth Apps → New OAuth App:

| Поле | Значение |
| --- | --- |
| Homepage URL | `https://anitaqlet.example.com` |
| Authorization callback URL | `https://anitaqlet.example.com/auth/callback/github` |

**Yandex** → <https://oauth.yandex.ru> → «Создать OAuth-клиент» → платформа «Веб-сервисы»:

| Поле | Значение |
| --- | --- |
| Callback URI #1 | `https://anitaqlet.example.com/auth/callback/yandex` |

**VK** → <https://vk.com/dev> → «Создать приложение» (тип «Веб-приложение» или «Stand-alone»):

| Поле | Значение |
| --- | --- |
| HTTPS-редирект | `https://anitaqlet.example.com/auth/callback/vk` |

После создания OAuth-приложений впишите id/secret в `.env` и перезапустите контейнер (шаг 5).

## 4. Скрыть порт приложения наружу

Сейчас `docker-compose.yml` публикует `8080:8080` на все интерфейсы. За reverse proxy приложению это не нужно — замените в `docker-compose.yml`:

```yaml
    ports:
      - "127.0.0.1:8080:8080"
```

Приложение станет доступно только с самого сервера; наружу пойдёт только Caddy (80/443).

## 5. Поднять приложение

```bash
cd ~/Anitaqlet
docker compose up -d --build
docker compose ps            # backend должен быть Up/healthy
docker compose logs backend  # ожидаем: "sqlite ready" и "listening on :8080"
```

Проверка с сервера:

```bash
curl -s http://127.0.0.1:8080/health   # {"status":"ok"}
```

Миграции применяются автоматически при старте. SQLite лежит в named volume (`anitaqlet_sqlite_data` — точное имя: `docker volume ls`).

## 6. HTTPS и домен (Caddy)

Caddy сам получает и продлевает сертификат Let's Encrypt — ручное возня с certbot не нужна.

```bash
apt install -y caddy
```

Конфиг `/etc/caddy/Caddyfile` (полностью заменить содержимое):

```
anitaqlet.example.com {
    encode gzip
    reverse_proxy 127.0.0.1:8080
}
```

```bash
systemctl reload caddy     # или systemctl restart caddy
```

Проверка из браузера: `https://anitaqlet.example.com` — должна открыться страница входа с сертификатом (Let's Encrypt выпускается в течение ~1–2 минут после reload). Первый запрос может занять до минуты, пока идёт issuance.

Если за сервером NAT/другой хостинг поддерживает ACME — убедитесь, что порты 80 и 443 действительно доступны снаружи.

### Альтернатива: nginx + certbot

```bash
apt install -y nginx
# /etc/nginx/sites-available/anitaqlet:
#   server {
#       server_name anitaqlet.example.com;
#       location / {
#           proxy_pass http://127.0.0.1:8080;
#           proxy_set_header Host $host;
#           proxy_set_header X-Real-IP $remote_addr;
#           proxy_set_header X-Forwarded-Proto $scheme;
#       }
#   }
ln -s /etc/nginx/sites-available/anitaqlet /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
apt install -y certbot python3-certbot-nginx
certbot --nginx -d anitaqlet.example.com
```

## 7. Бэкапы (SQLite)

БД — один файл в volume. Дамп делаем через официальный sqlite3-образ (в образе приложения sqlite3-CLI нет):

Скрипт `~/backup-anitaqlet.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

VOL="anitaqlet_sqlite_data"          # уточните: docker volume ls
DIR="$HOME/backups/anitaqlet"
KEEP_DAYS=14

mkdir -p "$DIR"
docker run --rm \
  -v "${VOL}:/data:ro" \
  -v "${DIR}:/backup" \
  keinos/sqlite3:latest \
  sqlite3 /data/anitaqlet.db ".backup /backup/anitaqlet-$(date +%F).db"

find "$DIR" -name 'anitaqlet-*.db' -mtime +"$KEEP_DAYS" -delete
```

```bash
chmod +x ~/backup-anitaqlet.sh
crontab -e   # добавить строку:
# 0 3 * * * /home/<user>/backup-anitaqlet.sh
```

**Восстановление**: `docker compose stop backend`, скопировать бэкап в volume (`docker run --rm -v anitaqlet_sqlite_data:/data -v $HOME/backups/anitaqlet:/b alpine cp /b/anitaqlet-<дата>.db /data/anitaqlet.db`), `docker compose start backend`.

Копии лежат на самом сервере — параллельно складывайте их на внешнее хранилище (S3 и т.п.).

## 8. Обновления

```bash
cd ~/Anitaqlet
git pull
docker compose up -d --build
docker compose logs -f backend   # убедиться, что поднялся
```

Миграции применятся сами. Откат — `git reset --hard <коммит>` + `docker compose up -d --build`.

## 9. Эксплуатация

```bash
docker compose ps                 # статус
docker compose logs -f backend    # логи в реальном времени
docker compose restart backend    # рестарт
docker compose down               # остановить всё (volume сохраняется)
journalctl -u caddy -f            # логи Caddy
```

Health-чек для мониторинга/uptime-сервисов: `GET https://anitaqlet.example.com/health`.

## 10. Troubleshooting

| Симптом | Что смотреть |
| --- | --- |
| Сайт не открывается по домену | `dig +short домен`; `ufw status`; `journalctl -u caddy -f` (частая причина — DNS ещё не применился или 80/443 закрыты) |
| `502 Bad Gateway` | `docker compose ps` (backend Up?), `docker compose logs backend`; слушает ли на 127.0.0.1:8080 |
| Ошибка входа через OAuth / redirect mismatch | В `OAUTH_REDIRECT_BASE` и в настройках OAuth-приложения провайдера должен стоять точный HTTPS-домен и путь `/auth/callback/...`. Проверьте `.env` и перезапустили ли контейнер после правок |
| Caddy не выпускает сертификат | Домен должен резолвиться в IP сервера и быть доступен на 80 снаружи; смотрите `journalctl -u caddy` |
| Backend падает при старте | `docker compose logs backend` — обычно ошибка миграций или недоступность volume |
| Удостовериться, что БД на месте | `docker volume ls | grep anitaqlet`; размер: `docker run --rm -v anitaqlet_sqlite_data:/data alpine ls -lh /data` |

## Чек-лист

- [ ] DNS A-запись указывает на сервер, `dig` подтверждает
- [ ] UFW: 22/80/443 открыты, 8080 не торчит наружу
- [ ] Docker + compose установлены, пользователь в группе `docker`
- [ ] `.env`: сильный `SESSION_SECRET`, OAuth id/secret, `OAUTH_REDIRECT_BASE=https://домен`
- [ ] OAuth-приятия провайдеров созданы с callback `https://домен/auth/callback/...`
- [ ] `docker compose up -d --build` → `/health` отвечает
- [ ] Caddy отдаёт HTTPS по домену, вход через OAuth работает end-to-end
- [ ] Бэкап-скрипт в crontab, тестовый restore проверен
