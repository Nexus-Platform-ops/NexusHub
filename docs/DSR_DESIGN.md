# DSR Design — Export & Erasure (GDPR / Myanmar PDPA)

Endpoints (all JWT-authenticated, base `/api/v1/dsr`):

| Endpoint | Purpose |
|---|---|
| `GET /dsr/export` | Start an export job (202, async) |
| `GET /dsr/export/{jobID}` | Download the completed export (JSON) |
| `GET /dsr/status` (`?job_id=`) | Poll job status |
| `DELETE /dsr/delete` | Request deletion (202, `awaiting_confirmation`) |
| `POST /dsr/delete/confirm` | Confirm with the single-use token (202, then async) |

## SLA and job model

Every job is a Redis hash-less JSON document at `dsr:job:<id>` with a
**30-day TTL equal to the SLA window** — a job cannot outlive the window it
must complete in. One in-flight job per user (a second request gets 409);
completed/failed jobs no longer block. Ownership is enforced on every load:
another user's job id returns 404, never 403, so job ids are not an oracle.

## Export

- `collectUserData` gathers every personal-data row across both schemas:
  account, profile, posts, comments, reactions, friendships, blocks, reports
  filed, chat threads/messages, coin ledger, market listings, linked
  identities, and the dating profile (via the flagged dating connection).
  Any query failure fails the job rather than shipping a partial export.
- The payload is **envelope-encrypted before storage** (fresh DEK per job,
  AES-256-GCM, DEK wrapped by the configured KMS master key — see
  `evidence/kms-setup.md`). The blob at `dsr:export:<id>` is ciphertext; a
  Redis dump alone yields nothing. Verified by `TestDSRExportFlow`, which
  asserts the stored blob does not contain seeded plaintext.
- Notification: email to the linked identity address when one exists; the
  authenticated download is the real delivery channel (email carries no link
  with auth material).
- Expiry: the blob shares the 30-day TTL; download after expiry returns 410.

## Deletion (anonymization, not removal)

Deletion is a **two-step confirmed flow**. `DELETE /dsr/delete` issues a
single-use 32-byte confirm token: sent by email to the linked identity
address, or — in development only, mirroring `dev_code` — returned as
`dev_confirm_token` when `OTP_DEBUG=1`. Only the hash (sha256) is stored in
the job; the API compares constant-time on confirm. The token never appears
in logs (the log email provider masks addresses and never prints bodies).

On confirm, `runDelete` anonymizes rather than hard-deletes:

| Data | Action |
|---|---|
| `social.users` | `phone` → `+deleted-<id>`, `display_name` → `Deleted user`, `phone_verified` → false, `birth_date` → NULL |
| `social.user_profiles` | `real_name`, `avatar_url` scrubbed |
| `identity.social_identities` | erased (provider linkage gone) |
| `dating.*` | profile, radar presence, and swipes erased (both directions) |
| Posts, comments, chat history, ledger, listings | **kept**, attributed to the anonymized account |

**Why anonymization:** content belongs to conversations with other users;
hard-deleting it would erase other people's data (their received messages,
their comment threads, their purchased orders). Scrubbing identity columns +
erasing provider linkage + deleting dating data removes the personal-data
surface while preserving platform integrity. This is the same posture as
major platforms' "delete my account" flows.

**Phone recycling:** `phone` becomes `+deleted-<id>`, which can never match
the phone regex — the number is releasable to a future registrant without
collision or account takeover.

## Residual items

- Chat messages are exported but only anonymized on delete (thread
  continuity). A future "also scrub my message bodies" option can be added
  as a flag on the delete request.
- The 30-day SLA is currently best-effort synchronous (jobs run in-process).
  If export volume grows, move `runExport`/`runDelete` to a queue worker —
  the job model already supports it.
