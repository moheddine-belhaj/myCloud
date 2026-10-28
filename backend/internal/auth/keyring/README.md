# keyring

Encrypts small secrets at rest with **AES-256-GCM**. M3 uses it for TOTP seeds.

- The key comes from Vault (`secret/mycloud/<env>/auth-totp`, key `key`) through External Secrets Operator
  into the env var `TOTP_KEY` (base64, 32 bytes). It never touches Git or the database.
- `auth` refuses to start without a valid key.
- Blob layout in one `bytea` column: `version (1) | key ID (1) | nonce (12) | ciphertext + tag`.
- **Associated data**: callers pass the user ID, so a sealed seed can't be copied to another user's row.
- **Key ID**: rows remember which key sealed them. A rotation adds key 2 and keeps key 1 for reading old rows.
