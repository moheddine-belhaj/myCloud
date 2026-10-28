-- +goose Up

-- One TOTP secret per user (M3). The secret is encrypted with AES-256-GCM by
-- internal/auth/keyring, bound to the user ID. The key lives in Vault, never
-- here, so this table alone can't produce codes.
CREATE TABLE user_totp (
    user_id    uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    secret_enc bytea       NOT NULL,              -- version | key ID | nonce | ciphertext
    created_at timestamptz NOT NULL DEFAULT now(),
    enabled_at timestamptz,                       -- NULL until the first code is confirmed
    last_step  bigint      NOT NULL DEFAULT 0     -- last accepted 30 s step: a code works once
);

-- Single-use recovery codes for a lost phone. Only the SHA-256 is stored.
CREATE TABLE recovery_codes (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  bytea       NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz
);
CREATE INDEX recovery_codes_user_id_idx ON recovery_codes (user_id);

-- +goose Down
DROP TABLE recovery_codes;
DROP TABLE user_totp;
