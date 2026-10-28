# totp

Time-based one-time passwords (RFC 6238), the 6-digit codes of authenticator apps.

- HMAC-SHA1, 6 digits, a new code every 30 s, ±1 step accepted for clock drift.
- 20-byte random secrets, shown to the user as base32 and as an `otpauth://` URI (QR code).
- `Validate` refuses steps at or before the last used one, so a code can't be replayed.
  The caller stores that step (`user_totp.last_step`).
- Tests use the RFC 6238 Appendix B vectors.
