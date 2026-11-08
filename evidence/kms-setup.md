# KMS Envelope Encryption — Setup & Rotation Runbook

Phase D replaces the env-based encryption key with envelope encryption
backed by a cloud KMS. Code: `backend/internal/crypto/` (service + AWS KMS
and GCP KMS adapters), `backend/internal/gauth/` (service-account JWT for
GCP). No AWS/GCP SDKs are used — SigV4 and the SA JWT bearer are
hand-rolled to keep dependencies minimal; both are covered by unit tests
(`crypto_test.go`, `token_test.go`).

## Model

- Each payload is encrypted with a **fresh random 256-bit DEK** (AES-256-GCM).
- The DEK is **wrapped by the master key** (KMS Encrypt/Decrypt).
- The stored blob is self-describing JSON: `{v, kid, dek, nonce, ct}` — all
  base64. `kid` identifies the wrapping key so stale blobs are detectable.
- **Rotation is rewrap, not re-encrypt:** a new master key unwraps the DEK
  and re-wraps it; the payload ciphertext is untouched. Implemented as
  `crypto.Service.Rewrap`.

## Environment wiring (see `backend/.env.example`)

Development:

```
CRYPTO_PROVIDER=local
CRYPTO_LOCAL_KEY=<base64 32-byte key>
```

Production (AWS KMS, recommended — mirrors the ap-southeast-1 region choice):

```
CRYPTO_PROVIDER=awskms
AWS_REGION=ap-southeast-1
AWS_ACCESS_KEY_ID=<IAM user or role creds with kms:Encrypt/kms:Decrypt on the key>
AWS_SECRET_ACCESS_KEY=<secret — never committed; verify by name/length only>
KMS_KEY_ID=arn:aws:kms:ap-southeast-1:<account>:key/<uuid>
```

GCP KMS alternative:

```
CRYPTO_PROVIDER=gcpkms
GCP_KMS_KEY_RESOURCE=projects/<p>/locations/<l>/keyRings/<r>/cryptoKeys/<k>
GCP_SERVICE_ACCOUNT_JSON=<service account with roles/cloudkms.cryptoKeyEncrypterDecrypter>
```

**Validate enforces this outside development:** `CRYPTO_PROVIDER=local` is
refused in staging/production, so an existing `.env.production` without
these values will fail startup at `config.Validate` until the KMS variables
are set. This is intentional (fail closed) but is a deployment-blocking
change for the next prod rollout — flagged for whoever cuts that release.

## Setup checklist (AWS KMS path)

1. Create a symmetric CMK in the project Region (ap-southeast-1). Note the ARN.
2. Create an IAM policy allowing `kms:Encrypt`, `kms:Decrypt`,
   `kms:ReEncrypt*` (for rewrap) scoped to that ARN.
3. Attach to the instance role (ECS task role / EC2 instance profile) or
   create an IAM user with static keys if the host is outside AWS.
4. Set the four env vars above; start the API; confirm startup passes
   `config.Validate` and `crypto.New` (a bad ARN fails fast at first use —
   there is no eager health check against KMS today).
5. Encrypt a probe payload via the DSR export flow (or a unit test against
   the real key in staging) and confirm decrypt round-trips.

## Monthly rotation (rewrap procedure)

1. Create the new CMK (or a new key version — same ARN works; rewrap only
   needs a `MasterKey` whose `KeyID` differs).
2. Deploy the API with the new `KMS_KEY_ID`. **Old blobs still decrypt** as
   long as the old key remains enabled and the IAM role retains decrypt on
   it — `kid` mismatch surfaces a clear error only when the old key is gone.
3. Run the rewrap pass over stored envelopes (DSR export blobs in Redis;
   any future encrypted columns):

   ```go
   next, _ := crypto.NewMasterKey("awskms", cfg.NextKMSConfig)
   for _, env := range storedEnvelopes {
       out, _ := svc.Rewrap(ctx, env, next) // payload untouched; kid updated
       store(out)
   }
   ```

4. After every blob reports the new `kid`, revoke decrypt on the old key
   version (scheduled key deletion with a 7–30 day window as a safety net).

Rotation cadence: monthly per spec. Because rotation is rewrap-only, the
data volume re-encrypted is proportional to stored envelope count (today:
DSR exports, 30-day TTL), not to database size.

## What intentionally stays env-based

JWT signing secret and the payment-callback HMAC key remain environment
secrets, not KMS-managed: they are *signing* keys with per-request HMAC
verification, not encryption-at-rest keys, and rotating them invalidates
live sessions/callbacks. If store-level envelope encryption of columns is
added later, reuse this same `crypto.Service` and add the column DEKs under
the same master key.
