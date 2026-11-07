# Nexus Architecture

## Service topology

```
┌──────────────┐   REST/JSON    ┌──────────────┐
│ Flutter app  │ ─────────────▶ │  Go API      │ ──▶ PostgreSQL 16 + PostGIS
│ (Riverpod)   │                │  (net/http)  │ ──▶ Redis 7 (OTP, whisper TTL)
└──────────────┘                └──────────────┘
```

One Go binary (`cmd/api`) serves every route; two pgx pools isolate the
social and dating database roles (see below). JWT HS256 access/refresh
tokens authenticate requests; fixed-window rate limiting runs per-IP and
per-user in-process.

## Database isolation model

| Concern | Mechanism |
|---|---|
| Schema separation | `social.*` vs `dating.*` schemas |
| Role separation | `nexus_social` has no privileges on `dating.*`; `nexus_dating` has none on social tables — only the pair-only `social.friend_ids` and `social.blocked_ids` views |
| Row Level Security | `dating.dating_profiles` policy requires `current_setting('app.dating_access') = 'service'` |
| Session hygiene | The API sets the RLS flag transaction-local (`set_config(..., true)`) inside `store.DatingConn`, so pooled connections are always clean |
| Feed purity | The social API's SQL never joins dating tables; tests assert the feed JSON cannot contain dating fields |

## Whisper (self-destruct chat)

1. `POST /api/v1/whisper` requires a mutual match (checked in the dating
   schema), then writes the body to Redis under `whisper:<uuid>` with
   `WHISPER_TTL_SECONDS` (default 5). PostgreSQL never stores the body.
2. `GET /api/v1/whisper/{id}` reads the payload; only the recipient may
   read; the read (or any rejected access) immediately deletes the key.
3. TTL expiry is the backstop — verified by a 5.1 s sleep test.

## Coins ledger

Append-only `ledger.coin_transactions`; balances are `SUM(amount)`.
Top-ups carry an HMAC-SHA256 payment confirmation over
`user_id:amount:request_id` and are idempotent by `request_id`. The
provider flow (007) creates a server-priced intent and credits it from a
signed webhook (see Provider integrations). Purchases
acquire `pg_advisory_xact_lock(user_id)`, re-read the product
`FOR UPDATE`, verify the balance, write debit+credit rows, insert the
order and deactivate the listing — all in one transaction, so two
concurrent buyers can never both succeed on one listing (tested).

## Radar privacy

Presence upserts store a PostGIS geography point. The query filters to a
configurable radius (clamped 100–5000 m), excludes friends (via the
minimal view) and hidden profiles, and applies **k-anonymity (k=3)**:
fewer than three candidates degrades to an empty result. Clients receive
only stable pseudonyms (`Bee-XXXX`), coarse distance buckets
(500m/1km/2km) and opaque node ids — never coordinates, names or raw ids.

## Frontend structure

`lib/theme.dart` centralizes the prototype's color tokens; `lib/api.dart`
is the single REST client; `lib/state.dart` holds Riverpod providers;
`lib/widgets.dart` renders the shared loading/error/empty states; the five
screens in `lib/screens.dart` mirror the prototype views 1:1, and
`lib/shell.dart` provides the header (township selector, coins badge) and
bottom navigation.

## Social login architecture (005)

- `internal/socialprov` owns all provider I/O: Facebook token exchange +
  `/me` + picture resolution, TikTok token exchange (PKCE) + `/user/info/`,
  Telegram widget HMAC validation. Base URLs are injectable so the test
  suite runs every flow against httptest fakes.
- `identity.social_identities` is the single home of provider real
  identity. Schema grants: `nexus_social` full, `nexus_dating` explicit
  REVOKE — mirroring the existing social/dating split.
- `handleSocialCallback` / `handleTelegramLogin` accept an optional Bearer
  token: with one, the flow LINKS the identity to the authenticated
  account; without one it signs up/logs in. A one-time state (Redis, 10 min
  TTL) binds provider + PKCE verifier, preventing CSRF and replay.
- The unlink guard refuses to remove the only login of an account without
  a verified phone, so accounts can never become unreachable.

## Safety layer (006)

- **18+ gate** — `social.users.birth_date` is the only input. Every dating
  route (profile GET/PUT, radar, swipe, whisper send/read) calls
  `requireAdult`, which computes the age in SQL and returns 403 otherwise.
  `PUT /api/v1/me/birthdate` persists only adult dates: an under-18 date is
  rejected with `{adult:false, stored:false}` and is not retained. The
  dating role cannot read `social.users`, so the birth date never reaches
  the dating schema or any dating payload; `/me` exposes only the boolean
  `adult`.
- **Blocks** — `social.blocks(blocker_id, blocked_id)` is symmetric for
  visibility: radar excludes blocked ids in both directions, swipe treats a
  blocked target as non-existent (404), and whisper returns 403 even when a
  mutual match row exists. Blocking also deletes any existing match/swipes
  in the dating schema as hygiene (the explicit check is the enforcement).
  The dating role reads pairs through the `social.blocked_ids` view only.
- **Reports** — `social.reports` is an append-only queue (`status`:
  open/reviewing/actioned/dismissed). User reports resolve opaque node ids
  to numeric user ids; post/product/whisper reports store the raw target
  string. There is no admin API yet — moderation consumes the table
  directly until a separate operator surface exists.

## Deployment hardening

- **One image, two entrypoints** — `Dockerfile.api` builds `cmd/api` and
  `cmd/migrate`. In every compose stack the `migrate` service runs first and
  the API waits for `service_completed_successfully`, so a long-running
  process never mutates the schema and an old schema never serves traffic.
- **Fail-fast configuration** — `APP_ENV` selects strict validation at
  startup (`internal/config.Validate`): weak/placeholder/reused secrets,
  `OTP_DEBUG=1`, dev database passwords and non-HTTPS social redirects are
  fatal in staging/production. The process exits before opening pools.
- **Network posture** — the production compose file publishes only Caddy
  (80/443). PostgreSQL, Redis, the API and the static web bundle have no
  host ports. Caddy terminates TLS with automatic Let's Encrypt
  certificates, redirects HTTP to HTTPS, sets HSTS, routes `/api/*` to the
  API and everything else to the Flutter bundle, and overwrites
  `X-Forwarded-For` with the direct client IP so rate limiting cannot be
  bypassed with a spoofed header. Role passwords
  come from `.env.production` and are created by `deploy/init-roles.sh` on
  first initialization.
- **Durability** — Redis runs with AOF (`appendfsync everysec`), so OTP
  codes and unread whispers survive a restart within their TTL. PostgreSQL
  keeps standard WAL; `scripts/backup.sh` produces `pg_dump -Fc` archives
  (optionally off-site) and prunes by retention. Restore is stop-api →
  `pg_restore --clean --if-exists` → `migrate` → start-api.
- **Build context hygiene** — `.dockerignore` keeps `.env*`, git metadata,
  evidence, backups and mobile build output out of the image context.

## Provider integrations (007: SMS + payments)

- **SMS** — `internal/sms` defines `Sender.SendOTP`. The `log` sender is a
  development stub that logs a masked number and never the code; the `http`
  sender posts `{"to","sender_id","message"}` with a bearer key. The API
  calls it from `deliverOTP` for register and login, returns 502 on
  delivery failure and deletes the stored OTP so a retry is clean.
- **Payments** — `internal/payments` owns the provider adapter (`log` /
  `http`), the authoritative package price list and webhook signing. The API
  stores only an intent id, server-priced coins/MMK, provider reference and
  status in `ledger.payment_intents`; the client never sends a price. A
  provider failure marks the intent `failed` before returning 502.
- **Webhook** — `POST /api/v1/payments/webhook` verifies
  `hex(HMAC-SHA256(PAYMENT_SIGNING_KEY, "<timestamp>.<raw body>"))` with a
  ±5 minute window, then locks the intent row, re-checks amount, provider
  ref and expiry, flips it to `paid` and inserts the ledger row
  (`request_id = intent:<id>`) in the same transaction. A `failed` intent
  may still be credited by a later signed `paid` event (the provider is
  authoritative and the user must not lose money); `expired` intents are
  rejected. Unique `request_id` plus the status check make replays
  idempotent; the endpoint
  is not IP-limited so a provider callback is never dropped, and the HMAC is
  the sole authentication.
- **Failure isolation** — the ledger and intents stay in the social pool;
  no dating data is involved, and provider credentials never reach clients
  or logs.

## Observability (Sentry)

- **Backend** — `internal/observability.InitSentry` initialises sentry-go
  only when `SENTRY_DSN` is set; an invalid DSN aborts startup. The HTTP
  middleware in `api.logMiddleware` attaches request-scoped hubs (method +
  path tags, never the query string), recovers panics into a JSON 500, and
  `serverError` forwards the underlying exception exactly once per request.
  A status-only fallback reports 5xx responses that bypass `serverError`.
  Health-check failures are excluded. `BeforeSend` drops the request, user
  and server name and scrubs e-mail addresses and long digit runs.
- **Flutter** — `main.dart` calls `SentryFlutter.init` only when a DSN is
  compiled in (`SENTRY_DSN` dart-define; prod compose maps
  `SENTRY_WEB_DSN`). PII, screenshots and tracing are disabled and
  `request`/`user` are stripped in `beforeSend`.
- **Release correlation** — both sides read `SENTRY_RELEASE`, so the same
  git SHA can be passed to the API container and the web build.
