# 12 · M3: Two-factor authentication (TOTP)

Goal: login needs a password plus a 6-digit code from an authenticator app; TOTP secrets are encrypted with a key from Vault.

## PR 1: key and plumbing
1. Generate a 32-byte encryption key per environment directly into Vault: `secret/mycloud/<env>/auth-totp` (stdin, never printed).
2. Extend the Helm chart `service`: an optional `ExternalSecret` and `secretEnv` (environment variables from a Secret).
3. Set the auth values for every environment: `TOTP_KEY` from Secret `auth-totp`.
4. Load the key at start-up; fail fast if it's missing or not 32 bytes.
5. Add `internal/auth/crypto`: AES-256-GCM seal/open with a key ID for future rotation, plus tests.

Done when: auth runs in dev, qa and prod with the key loaded, and Vault and cluster hashes match.

## PR 2: TOTP core and enrollment
1. Migration `00002`: table `user_totp` (encrypted secret, key ID, `enabled_at`, `last_step`) and table `recovery_codes` (SHA-256 hashes, `used_at`).
2. Add `internal/auth/totp` (RFC 6238: SHA-1, 6 digits, 30 s, ±1 step), tested against the RFC test vectors.
3. Block reuse: a code's time step is accepted only once per user.
4. `POST /api/auth/2fa/setup`: create a pending secret, return the `otpauth://` URI and the secret.
5. `POST /api/auth/2fa/enable {code}`: activate 2FA, return 10 recovery codes once, end the user's other sessions.
6. `POST /api/auth/2fa/disable {password, code}`.
7. Show `totpEnabled` in `GET /api/auth/me`.

Done when: enrollment works with a real authenticator app in dev; a wrong code gets 400; the same code used twice is refused.

## PR 3: login with 2FA
1. Migration `00003`: table `mfa_challenges` (token hash, user, 5-minute expiry, attempt counter).
2. `POST /api/auth/login`: when 2FA is on, return `mfaRequired` and a `__Host-mfa` cookie instead of a session.
3. `POST /api/auth/login/totp {code}`: accept a TOTP code or a recovery code, then issue `__Host-session`; the challenge dies after 5 wrong attempts.
4. `POST /api/auth/2fa/recovery-codes`: regenerate the codes (needs a valid TOTP code).
5. CLI `reset-2fa --email` for a lost phone and lost recovery codes.
6. Make 2FA mandatory for admins.
7. Log 2FA events with `slog` (enabled, disabled, failed, recovery code used), never codes or secrets.
8. Update the Postman collection and `Troubleshooting/auth_service.md`; promote to qa, then prod.

Done when: login with 2FA works end to end in all three environments, a recovery code works once, and the 6th wrong attempt is refused.

Result: accounts are protected by a second factor; a stolen password or a database dump alone is not enough to log in.