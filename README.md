# Nexus SuperApp — Production-Ready Release

Location-first, safety-first social + dating + marketplace hybrid for
Myanmar, migrated from the provided HTML prototype to a real stack.

- **Mobile** — Flutter (Riverpod), replicating the prototype UI 1:1 with
  centralized color tokens (amber `#F59E0B` main, rose dating, emerald
  market), light + dark themes, and loading / error / empty states on every
  screen.
- **API** — Go (stdlib `net/http` REST). All business rules live server-side.
- **Database** — PostgreSQL 16 + PostGIS with **two strictly separated
  schemas** (`social`, `dating`) owned by different DB roles, plus Row Level
  Security on dating tables.
- **Redis** — OTP store and whisper message TTL store.

## Quick start (local, no Docker)

```bash
# 1. Bootstrap PostgreSQL (roles + database) and Redis, apply migrations
bash scripts/bootstrap.sh

# 2. Configure and run the API
cp .env.example .env         # dev defaults work out of the box
cd backend && go run ./cmd/api   # listens on :8080

# 3. Run the Flutter app (Android emulator reaches the API via 10.0.2.2)
cd mobile && flutter pub get && flutter run
```

## Quick start (Docker Compose)

```bash
docker compose up --build
# API:  http://localhost:8080/api/v1/health
# Web:  http://localhost:3000
```

The `db` service creates the two API roles from `deploy/init-roles.sql` on
first start; a one-shot `migrate` service then applies `migrations/*.sql`
before the API starts (the API waits on `service_completed_successfully`).
Redis runs with AOF so OTPs and in-flight whispers survive a restart.

## Configuration

Every setting comes from the environment — see `.env.example`. Required:
`DATABASE_URL`, `DATABASE_DATING_URL`, `JWT_SECRET`, `PAYMENT_SIGNING_KEY`.
Generate secrets with `openssl rand -hex 32`. `OTP_DEBUG=1` (dev only)
returns OTP codes in API responses (the login screen shows the code) and
enables unsigned dev coin top-ups; production builds must set it to `0`.

When `APP_ENV=development` the API also answers CORS preflights for
`localhost` / `127.0.0.1` / `[::1]` origins, so `flutter run -d chrome
--web-port 8090` can call it directly. The middleware is not installed in
staging or production; those deployments serve the app from the same origin.

`APP_ENV` (default `development`) controls startup validation. In `staging`
and `production` the API refuses to boot when:

- `JWT_SECRET` or `PAYMENT_SIGNING_KEY` is shorter than 32 characters,
  is a known placeholder (`change_me`, `dev-only`, …), or the two are equal;
- `OTP_DEBUG=1`;
- a database URL still uses the `dev_social` / `dev_dating` passwords;
- Facebook/TikTok is configured but `SOCIAL_REDIRECT_BASE` is not `https://`;
- `SMS_PROVIDER` / `PAYMENT_PROVIDER` is not `http`, or its endpoint, key,
  sender id / return URL is missing or not `https://` (the `log` stubs would
  either lock users out of OTP or create unpayable intents).

## Production hardening

```bash
cp .env.production.example .env.production   # fill every value
docker compose --env-file .env.production -f docker-compose.prod.yml up -d --build
```

- Only `caddy` publishes ports (80/443). PostgreSQL, Redis, the API and the
  web bundle stay on the compose network; Caddy terminates TLS and routes
  `/api/*` to the API and everything else to the Flutter bundle.
- `deploy/init-roles.sh` creates `nexus_social`/`nexus_dating` with the
  env-supplied passwords on first initialization; the one-shot `migrate`
  service applies schema + grants before the API starts.
- One image serves both entrypoints: the `api` and `migrate` binaries plus
  `/app/migrations`, non-root, with CA certificates for provider HTTPS.
- Backups: `scripts/backup.sh` runs `pg_dump -Fc` inside the db container
  (or directly via `DATABASE_URL`), optionally uploads to S3 and prunes
  local dumps older than `BACKUP_RETENTION_DAYS`. Restore order is in the
  script header: stop api → `pg_restore --clean --if-exists` → run
  `migrate` → start api.
- Passwords are interpolated into URLs, so generate URL-safe values
  (`openssl rand -hex 32`). Role passwords are created only on the first
  initialization of an empty `pgdata` volume; rotating them afterwards needs
  `ALTER ROLE` (or a fresh volume plus restore).

Docker builds and `docker compose config` cannot be executed on the
maintainer's Windows machine (no daemon); the images are built and the stack
is validated in CI / on the deployment host.

## TLS with Caddy (deployment runbook)

Caddy is the only public entrypoint. It obtains and renews Let's Encrypt
certificates automatically, redirects HTTP to HTTPS, sets HSTS and other
security headers, and routes by path:

| Path | Upstream | Notes |
|---|---|---|
| `/api/*` | `api:8080` | Go REST API, including `/api/v1/health` |
| `/*` | `web:80` | Flutter web bundle (nginx) |

Same-origin routing means the web app calls `https://$DOMAIN/api/v1/...`
with no CORS setup. Caddy **overwrites** `X-Forwarded-For` with the direct
client IP, so the API rate limiter sees real clients and a spoofed header
cannot bypass it (if a CDN is added in front, switch to the CDN's real-IP
scheme).

**Prerequisites**

1. DNS: an `A` (and `AAAA` if IPv6) record for `DOMAIN` (bare hostname, no
   scheme) points at the host. ACME issuance fails until it resolves.
2. Firewall: inbound TCP 80, TCP 443 and UDP 443 (HTTP/3) open; everything
   else (5432, 6379, 8080) stays closed.
3. `.env.production`: `DOMAIN`, `ACME_EMAIL`, and both
   `NEXUS_PUBLIC_API_URL` / `SOCIAL_REDIRECT_BASE` set to
   `https://$DOMAIN[/api/v1/auth/social]`. The web build bakes
   `NEXUS_PUBLIC_API_URL` in at image build time, so changing it requires
   `--build`.

**First deploy**

Every value guarded with `:?` in `docker-compose.prod.yml` must be non-empty
before `up`, or compose aborts. Fill:

- `DOMAIN` (bare hostname), `ACME_EMAIL`, `NEXUS_PUBLIC_API_URL`
- `POSTGRES_PASSWORD`, `NEXUS_SOCIAL_PASSWORD`, `NEXUS_DATING_PASSWORD`
- `JWT_SECRET` and `PAYMENT_SIGNING_KEY` — two different values, ≥32 chars
- `SMS_API_URL`, `SMS_API_KEY`, `SMS_SENDER_ID` (https endpoint)
- `PAYMENT_API_URL`, `PAYMENT_API_KEY`, `PAYMENT_RETURN_URL` (https)
- Sentry DSNs are optional; leave empty to disable

Redis sits on the private compose network and has no password; it must not
be published. Generate each secret independently with `openssl rand -hex 32`.

```bash
cp .env.production.example .env.production   # fill every value above
docker compose --env-file .env.production -f docker-compose.prod.yml up -d --build
docker compose --env-file .env.production -f docker-compose.prod.yml logs -f caddy
```

**Verify**

```bash
curl -sI "https://$DOMAIN/" | head -5              # 200 + HSTS
curl -s  "https://$DOMAIN/api/v1/health"           # {"status":"ok"...}
curl -sI "http://$DOMAIN/" | head -1               # 308 -> https
```

**Certificate lifecycle**

- Certificates and ACME account data live in the `caddy_data` volume.
  Never delete it during normal operation; back it up or accept reissuance.
- Renewal is automatic (Caddy renews before expiry); no cron job needed.
- To rehearse against Let's Encrypt staging before going live, add
  `acme_ca https://acme-staging-v02.api.letsencrypt.org/directory` to the
  global block in `deploy/Caddyfile`, deploy, verify, then remove it and
  restart Caddy.
- If port 80 is blocked by the provider, Caddy cannot use HTTP-01; switch
  to DNS-01 with the provider's Caddy plugin (requires a custom image).

**Troubleshooting**

- `caddy` exits with an ACME error: check DNS, port 80 reachability, and
  that `ACME_EMAIL` is valid.
- 502 on `/api/*`: inspect `docker compose ... logs api`; the API refuses to
  start on unsafe config (see `APP_ENV` rules above).
- 502 on `/`: the `web` image has not finished building/serving; check
  `docker compose ... ps web`.
- Rotating `DOMAIN` later means a new certificate; keep the old DNS record
  until the new one is issued if you need zero downtime.

## Error monitoring (Sentry)

Reporting is optional and **off by default**: with an empty DSN no SDK is
initialised and every capture is a no-op.

**Backend** (`SENTRY_DSN`, `SENTRY_ENVIRONMENT`, `SENTRY_RELEASE`,
`SENTRY_TRACES_SAMPLE_RATE`):

- Recovers handler panics (returns a JSON 500) and captures every
  `serverError` (5xx) with `http.method`, `http.path` and `handler` tags.
- Health-check failures are deliberately not sent (they page the platform,
  not a developer).
- Never sends request bodies, headers, cookies, query strings or user ids;
  free-text values are scrubbed for e-mail addresses and long digit runs
  (phone numbers, payment references) before leaving the process.
- An invalid DSN fails startup instead of silently dropping events.

**Flutter** (web is wired through `SENTRY_WEB_DSN`; app builds take
dart-defines):

```bash
flutter run \
  --dart-define=SENTRY_DSN=https://key@o0.ingest.sentry.io/1 \
  --dart-define=SENTRY_ENVIRONMENT=staging \
  --dart-define=SENTRY_RELEASE=$(git rev-parse --short HEAD)
```

The app disables `sendDefaultPii`, screenshots and tracing, and strips
`request`/`user` in `beforeSend`.

Set `SENTRY_RELEASE` to the same git SHA for both projects so backend and
web events line up. `SENTRY_TRACES_SAMPLE_RATE` stays `0` unless you
explicitly want performance traces. Enable Sentry's server-side data
scrubbing as defense-in-depth; the client-side scrubbing above is the first
line.

**Verify:** start the API with a DSN (log line `sentry reporting enabled`)
and induce a 500 — events should arrive with the request path but no query
string, body or identity. Panics are captured automatically.

## API surface

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/health` | GET | DB + Redis readiness |
| `/api/v1/auth/register` / `verify` / `login` / `refresh` | POST | Phone-OTP auth, JWT pair |
| `/api/v1/me`, `/api/v1/profile` | GET/PUT | user_profiles (main account; `/me` carries the `adult` flag) |
| `/api/v1/me/birthdate` | PUT | 18+ verification (under-18 dates are not stored) |
| `/api/v1/reports` | POST | Abuse report queue (user/post/product/whisper) |
| `/api/v1/blocks`, `/blocks/{nodeID}` | POST/GET/DELETE | Block list; blocks hide both users from radar and lock whisper |
| `/api/v1/posts`, `/posts/{id}`, `/posts/{id}/reactions`, `/posts/{id}/comments` | POST/GET | Social feed |
| `/api/v1/dating/profile` | GET/PUT | dating_profiles (isolated schema) |
| `/api/v1/radar` | GET | k-anonymous nearby nodes (k=3) |
| `/api/v1/dating/swipe` | POST | like/pass, mutual match |
| `/api/v1/whisper`, `/api/v1/whisper/{id}` | POST/GET | Redis-TTL self-destruct chat (5s, read-once) |
| `/api/v1/coins/balance` / `topup` / `topup/dev` / `ledger` | GET/POST | Server-side ledger; topup requires HMAC signature |
| `/api/v1/coins/intents`, `/coins/intents/{id}` | POST/GET | Server-priced top-up intent + hosted checkout URL; poll after payment |
| `/api/v1/payments/webhook` | POST | Provider callback (HMAC + timestamp), credits the ledger once |
| `/api/v1/market/products`, `/api/v1/market/orders` | GET/POST | Listings + atomic coin purchase |
| `/api/v1/chat/threads…` | POST/GET | Unified chat (personal vs business) |

## Database isolation model (why dating data cannot leak)

1. **Schema separation** — `social.*` and `dating.*` are separate schemas.
2. **Role separation** — the API opens two pools: `nexus_social` holds zero
   privileges on `dating.*`; `nexus_dating` holds zero privileges on social
   tables and sees only the `social.friend_ids` view (user_id pairs).
3. **Row Level Security** — `dating.dating_profiles` enforces
   `current_setting('app.dating_access') = 'service'`; the API sets this
   flag inside a transaction only for authenticated requests, so a leaked
   role credential still reads nothing.
4. **Verified by tests** — `TestProfileIsolation` asserts the social role
   gets `permission denied`, an unflagged dating connection sees zero rows
   and cannot INSERT, and the social feed JSON never contains dating data.

## Safety, 18+ gate and moderation

Dating, Radar and Whisper are adults-only. `PUT /api/v1/me/birthdate`
accepts a `YYYY-MM-DD` date; the server stores it only when the account is
18+ and every dating route rejects unverified accounts with 403. The birth
date lives in `social.users`, which the dating DB role cannot read — the
isolation contract is unchanged.

`POST /api/v1/blocks` takes an opaque `node_id`. Blocks are symmetric for
visibility: the blocked account disappears from both users' radar, swipes
against it return 404, and whisper returns 403 even if an old mutual match
row still exists. `GET /api/v1/blocks` lists opaque node ids only.
`POST /api/v1/reports` records user/post/product/whisper reports in
`social.reports` for moderation review (no operator UI yet).

## Whisper auto-delete

Bodies are stored **only in Redis** (`whisper:<id>`, TTL 5s, configurable
via `WHISPER_TTL_SECONDS`). Reading uses GET semantics + explicit delete:
first successful read destroys the message; TTL is the backstop; a
non-recipient read attempt is rejected and also destroys the payload.
Nothing is persisted in PostgreSQL.

## Nexus Coins integrity

Balances are always derived from an append-only ledger
(`ledger.coin_transactions`). Top-ups require an HMAC-SHA256 payment
confirmation over `user_id:amount:request_id`; `request_id` makes retries
idempotent (no double credit). Purchases run inside a transaction guarded
by `pg_advisory_xact_lock`, debit the buyer, credit the seller and
deactivate the listing atomically.

## SMS and payment providers

Outbound integrations use provider-agnostic adapters (`internal/sms`,
`internal/payments`) with injectable endpoints, so the vendor can change
without touching money or auth logic. Both default to the development `log`
provider; `staging`/`production` require the `http` provider with https
endpoints and keys (see Configuration).

**SMS wire contract** — `POST $SMS_API_URL` with
`Authorization: Bearer $SMS_API_KEY` and JSON:

```json
{"to": "+959...", "sender_id": "Nexus",
 "message": "Nexus verification code: 123456 (valid 5 minutes). Do not share this code."}
```

Any 2xx is a successful send. A failure returns 502 and deletes the stored
OTP so the user's retry gets a fresh code. The code is never written to logs.

**Payment flow** (KBZPay / Wave Money / PSP aggregator):

1. Client calls `POST /api/v1/coins/intents` with only a `package_id`:

   | Package | Coins | Price |
   |---|---|---|
   | `coins_100` | 100 | 2,000 MMK |
   | `coins_500` | 500 | 9,000 MMK |
   | `coins_1200` | 1,200 | 20,000 MMK |
   | `coins_2500` | 2,500 | 40,000 MMK |

2. The API creates a 30-minute intent and calls `POST $PAYMENT_API_URL`
   with `Authorization: Bearer $PAYMENT_API_KEY`:

   ```json
   {"intent_id": "…", "reference": "…", "amount_mmk": 9000,
    "currency": "MMK", "return_url": "https://app.example.com/wallet"}
   ```

   It expects a 2xx `{"id": "provider-charge-id", "payment_url": "https://…"}`.
3. The client opens `payment_url`. When the payment completes the provider
   calls `POST /api/v1/payments/webhook` with:

   - `X-Payment-Timestamp`: unix seconds
   - `X-Payment-Signature`: hex HMAC-SHA256 of `<timestamp>.<raw body>` using
     `PAYMENT_SIGNING_KEY`
   - body: `{"intent_id": "…", "provider_ref": "…", "status": "paid"|"failed", "amount_mmk": 9000}`

4. The API rejects signatures outside a ±5 minute window, then checks the
   intent's price, provider ref and expiry, marks it paid and inserts the
   `ledger.coin_transactions` row (`request_id = intent:<id>`) in one
   transaction. Duplicate callbacks return 200 with `duplicate: true`; a
   different amount never credits.
5. `GET /api/v1/coins/intents/{id}` returns the status so the client can
   poll after returning from checkout.

Clients never choose coins or prices for provider top-ups. The legacy signed
`/coins/topup` callback remains for manual/partner confirmations. Vendor
wire formats are not hard-coded: new providers implement the small `Sender`
or `Provider` interface and are covered by httptest-based tests.

## Tests & gates

```bash
cd backend && go test ./...            # unit + integration (real PG + Redis)
cd backend && go test ./internal/... -coverpkg=./internal/... -coverprofile=coverage.out
cd mobile  && flutter test             # widget + API client tests
```

Gate evidence lives in `evidence/verification.json`; the final gate table in
`docs/READINESS.md`. See `evidence/asset-integration.md` for the logo
integration chain.

## Project layout

```
backend/          Go REST API (cmd/api, cmd/migrate, internal/{api,auth,config,ratelimit,store,socialprov,sms,payments})
migrations/       PostgreSQL schema + RLS + grants (001..007)
mobile/           Flutter app (lib/{theme,api,state,widgets,screens,shell,main}.dart)
deploy/           container init: dev roles SQL, prod roles script, Caddyfile (TLS edge)
scripts/          bootstrap / migrate / backup / assets / evidence helpers
.github/workflows CI: tests, coverage gate, docker builds, compose/Caddyfile validation
evidence/         verification.json, asset-integration.md
docs/             ARCHITECTURE.md, READINESS.md
Dockerfile.api    Go API + one-shot migrate runner image
Dockerfile.web    Flutter web build served by nginx
docker-compose.yml      dev stack (published ports, dev secrets)
docker-compose.prod.yml production stack (env secrets, Caddy-only published ports)
build_zip.sh      produces nexus-production.zip
```

## Social Logins (Facebook / TikTok / Telegram)

Three OAuth2 / signed-payload social logins are built in:

| Provider | Flow | Backend endpoints |
|---|---|---|
| Facebook | OAuth2 authorization code (`public_profile`) | `GET /api/v1/auth/social/facebook/start`, `GET /api/v1/auth/social/facebook/callback` |
| TikTok | OAuth2 authorization code **with PKCE (S256)** | `GET /api/v1/auth/social/tiktok/start`, `GET /api/v1/auth/social/tiktok/callback` |
| Telegram | Login Widget payload verified with HMAC-SHA256 (secret = SHA256(bot token)) + 24h freshness | `POST /api/v1/auth/social/telegram/callback` |

Account management (Bearer auth): `GET /api/v1/auth/social/links`,
`DELETE /api/v1/auth/social/link/{provider}`.

### Identity isolation (critical)

Real identity data returned by the providers (real names, profile pictures,
e-mails) is stored **only** in the `identity` schema
(`identity.social_identities`, migration `005_social_logins.sql`). It is
never copied into `social.users`, `social.user_profiles`, the `dating`
schema, or any response that feeds the Dating Profile or Anonymous Radar
features:

- Accounts created through a social login get a generated pseudonymous
  display name (`Nexus-XXXXXXXX`), never the provider name.
- The `nexus_dating` DB role holds zero privileges on `identity` (enforced
  by the migration and verified by an isolation test that asserts the
  dating role's `SELECT` fails with permission denied).
- `GET /api/v1/auth/social/links` returns only provider / provider id /
  link time — never the real name or avatar.
- End-to-end tests (`backend/internal/api/social_login_test.go`) assert the
  provider real name and avatar URL appear in **no** dating-profile, radar,
  feed or `/me` response after a full social signup.

### Environment variables (all optional; endpoints answer 503 until set)

```
FACEBOOK_APP_ID / FACEBOOK_APP_SECRET
TIKTOK_CLIENT_KEY / TIKTOK_CLIENT_SECRET
TELEGRAM_BOT_TOKEN
SOCIAL_REDIRECT_BASE   # e.g. https://api.example.com/api/v1/auth/social
```

OAuth state is single-use and stored in Redis with a 10-minute TTL.
