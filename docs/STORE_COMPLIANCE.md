# Store Compliance Checklist — Play Store & App Store

Companion to the privacy posture enforced in code (Phase D). Fill each form
field with the values below; every claim is backed by a doc or test.

> Spec note: this file is a fourth deliverable beyond the three named
> outputs (`PENTEST_SCOPE.md`, `DSR_DESIGN.md`, `kms-setup.md`) — the
> batch's item 5 ("Store compliance checklist") needs a home.

## 1. Google Play — Data Safety form

| Data type | Collected? | Shared? | Encrypted in transit? | Deletable? | Source of truth |
|---|---|---|---|---|---|
| Phone number | Yes (account + OTP) | No | Yes (TLS) | Yes (DSR delete anonymizes) | `docs/DSR_DESIGN.md` |
| Name / profile info | Yes (display name, optional real name) | No | Yes | Yes | DSR design |
| User IDs | Yes | No | Yes | Yes | DSR design |
| Photos / avatar | Optional | No | Yes | Yes | DSR design |
| Precise location | **No** — radar buckets distances server-side; raw coordinates never leave the device except to the API for the search itself | — | Yes | n/a | `docs/PENTEST_SCOPE.md` §1 |
| Approximate location | Yes (derived buckets only) | No | Yes | n/a | PENTEST_SCOPE §1 |
| Contacts | No | — | — | — | friend graph is in-app only |
| SMS | No (OTP via SMS aggregator, never read) | — | — | — | sms provider logs masked |
| Financial info | Yes (payment intents for coin top-ups) | Payment provider only | Yes | Ledger kept (anonymized) | DSR design |
| Messages (chat) | Yes | Matched users only | Yes | Anonymized on delete | DSR design |
| App activity / diagnostics | Crash logs via Sentry (no PII attached — `internal/observability`) | Sentry | Yes | n/a | observability package |

- **Security practices to declare:** data encrypted in transit (TLS,
  ATS/OkHttp defaults, zero ATS exceptions — iOS `Info.plist`); deletion
  request mechanism in-app and via DSR endpoints; independent security
  review (pentest scope doc, harness results).

## 2. Apple App Store — Privacy Nutrition Labels

| Category | Answer |
|---|---|
| Contact info (phone, name) | Linked to identity; used for account + OTP |
| User content (posts, chat, photos) | Linked to identity; app functionality |
| Location | **Not collected** for tracking; precise location is processed transiently for the radar query and never stored raw (PostGIS stores device-supplied coordinates — see note below) |
| Identifiers | User ID |
| Purchases | In-app coin top-ups via payment provider |
| Crash data | Sentry, no PII |
| Data used for tracking | **None** — no third-party ads, no cross-app tracking |

**Location nuance for review:** the dating radar stores a geography point
(`dating.radar_presence`, 15-minute freshness window) so nearby matching
works. That is *collected* location under Apple's definition — declare
"Location → coarse/precise, linked to user, app functionality", and cite the
k-anonymity + bucket-only exposure controls from PENTEST_SCOPE §1. There is
no background collection (while-using only, `NSLocationWhenInUseUsageDescription`).

## 3. Age rating (17+)

Justification for both stores:

- **Dating component** with adult (18+) gating via birth-date attestation
  (`/api/v1/me/birthdate`, minor rejection tested) → mature themes.
- **User-generated content** (posts, comments, chat) with community
  moderation (`social.reports`, moderation schema) but unmoderated
  user-to-user contact → 17+ per UGC guidelines.
- **In-app purchases** (coins) — not a rating driver but disclosed.
- No gambling, no graphic violence, no sexual content by default; radar
  hides friends and blocked users and degrades below k=3 (privacy, not
  content, but worth noting in the review notes).

Recommended Play rating: **Teen (16+)** with the dating disclaimer, or Mature
17+ to match App Store; recommend 17+ on both for consistency.

## 4. Pre-launch gates (must close before store submission)

1. Release keystore (Phase C flagged debug signing — generate upload key,
   configure Play App Signing).
2. Play Integrity enforced beyond "advisory": gate radar/whisper on a valid
   attestation record (endpoint records today; gating is a policy flag).
3. App Attest for iOS (currently 501 — Apple will ask for platform parity).
4. Live SMS aggregator credentials (4-network Myanmar test — currently
   blocked, see project memory).
5. Real email provider for DSR confirmation (`EMAIL_PROVIDER=http`).
