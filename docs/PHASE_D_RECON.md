# Phase D Recon — Crypto / Attestation / DSR current state

Recon only — no code changed. Date: 2026-11-07.
Code references are against main @ de2f499.

## 1. Env-based secrets inventory

Every secret the backend reads from the environment (source:
`backend/internal/config/config.go`, all via `os.Getenv` in `FromEnv`):

| Env var | Used for | Sensitivity / notes |
|---|---|---|
| `JWT_SECRET` | access/refresh token signing (HS256) | 32+ chars enforced outside development; **static, no rotation** |
| `PAYMENT_SIGNING_KEY` | HMAC for coin top-up webhook verification | 32+ chars enforced; **static, no rotation** |
| `DATABASE_URL` / `DATABASE_DATING_URL` | Postgres role passwords embedded in URL | dev-role markers rejected outside development |
| `SMS_API_KEY` | SMS aggregator auth | required when `SMS_PROVIDER=http` |
| `PAYMENT_API_KEY` | payment provider auth | required when `PAYMENT_PROVIDER=http` |
| `FACEBOOK_APP_SECRET` | FB OAuth | optional |
| `TIKTOK_CLIENT_KEY` / `TIKTOK_CLIENT_SECRET` | TikTok OAuth | optional |
| `TELEGRAM_BOT_TOKEN` | Telegram widget login | optional |
| `EMAIL_API_KEY` | transactional email (DSR flows) | required when `EMAIL_PROVIDER=http` |
| `CRYPTO_LOCAL_KEY` | base64 32-byte local master key | **development only**; empty → ephemeral random key (lost on restart) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | static IAM creds for AWS KMS | required when `CRYPTO_PROVIDER=awskms` |
| `KMS_KEY_ID` | AWS KMS CMK ARN | awskms |
| `GCP_KMS_KEY_RESOURCE` | GCP KMS key path | gcpkms |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | GCP SA key JSON (**dual use**: GCP KMS + Play Integrity) | gcpkms + attestation |
| `GOOGLE_PLAY_PACKAGE_NAME` | Play Integrity package binding | attestation |
| `OTEL_EXPORTER_OTLP_ENDPOINT` + `OTEL_EXPORTER_OTLP_INSECURE` | tracing endpoint (Phase E) | non-secret; listed for completeness |

## 2. Envelope encryption (`backend/internal/crypto/`)

| Question | Answer |
|---|---|
| Algorithm | **AES-256-GCM** envelope: fresh random 256-bit DEK per payload; DEK wrapped by a master key; blob JSON `{v, kid, dek, nonce, ct}` (crypto.go:22) |
| Key source | `CRYPTO_PROVIDER`: `local` (env key / ephemeral) · `awskms` (KMS Encrypt/Decrypt via KMS_KEY_ID) · `gcpkms` (GCP KMS via SA JSON token source). Selected in `NewMasterKey` (masterkey.go:113) |
| Rotation | **Primitive exists, automation does not.** `Service.Rewrap` (crypto.go:126) re-wraps DEKs without touching payloads and is unit-tested (`TestRotationRewrap`), but no script/cron/job calls it. Manual monthly procedure documented in `evidence/kms-setup.md` |
| What's actually encrypted | **Only DSR export blobs** (`backend/internal/api/dsr.go:190` encrypt, `:388` decrypt). Phone numbers, identities, posts, market data are plaintext at rest; whispers are Redis-only by design (never touch the DB) |

## 3. Device attestation

| Layer | State |
|---|---|
| Backend verifier | `backend/internal/attest/` — Play Integrity via Google API (SA auth, nonce match, verdict check), `log` stub for development, App Attest returns 501/ErrUnavailable |
| Endpoints | `POST /api/v1/auth/attest/nonce`, `POST /api/v1/auth/attest` (server-side single-use nonce, record stored 30 days in Redis) |
| Config gate | `ATTESTATION_PROVIDER=playintegrity` + package + SA JSON **required outside development** (config.go Validate) |
| Enforcement | **Advisory only.** `s.Attest.Verify` is called solely from the attest handler; nothing requires an attestation record before other endpoints answer |
| Mobile | **Zero integration.** No attestation/integrity/Play-Integrity references anywhere under `mobile/` — the client never calls the endpoints |

## 4. DSR endpoints (`backend/internal/api/dsr.go`)

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/dsr/export` | start async export job (202) |
| `GET /api/v1/dsr/export/{jobID}` | download encrypted export blob |
| `GET /api/v1/dsr/status` | poll job status |
| `DELETE /api/v1/dsr/delete` | request deletion (202, email confirm token) |
| `POST /api/v1/dsr/delete/confirm` | confirm + execute anonymization |

Jobs live in Redis (30-day TTL = SLA). Export payloads are the only
envelope-encrypted data in the system. Design: `docs/DSR_DESIGN.md`.

## 5. Gaps list

1. **Attestation is not enforced** — verification records exist but no endpoint gates on them; a non-attested client works everywhere.
2. **Mobile never attests** — server endpoints are clientless; Android Play Integrity / iOS App Attest SDK integration missing on the app side.
3. **Encryption scope is minimal** — envelope crypto covers DSR exports only; PII at rest (phone numbers, real names, identity links) is unencrypted. Whisper privacy rests on the Redis-only + short-TTL design, not crypto.
   - **Accepted risk (2026-11-07, user decision): location data stays plaintext PostGIS.** Rationale: (a) presence rows are ephemeral — radar only reads `updated_at > now() - interval '15 minutes'`; (b) exact coordinates never leave the server — clients receive coarse distance buckets (500m/1km/2km) only; (c) radar results pass the k≥3 k-anonymity gate, so a single user's point cannot be isolated; (d) encrypting `geog` would break the GiST-backed `ST_DWithin`/`ST_Distance` query path and require a blind-index rewrite of the radar fanout for no exposure that isn't already suppressed by (a)–(c). Revisit only if threat model changes to "DBA with table-level read access" as an in-scope actor.
4. **No automated key rotation** — `Rewrap` exists but nothing schedules it; monthly rewrap is a documented manual procedure (`evidence/kms-setup.md`), not code.
5. **JWT + payment HMAC keys stay env-static** — deliberate (documented in kms-setup.md), but rotation means rolling restarts with no grace window for old tokens/signatures.
6. **GCP SA JSON dual use** — one service account key feeds both GCP KMS and Play Integrity; blast radius spans crypto + attestation if leaked.
7. **AWS KMS auth uses long-lived IAM keys** (`AWS_ACCESS_KEY_ID`/`SECRET`) rather than an instance role/IRSA — key exposure risk in env dumps.
