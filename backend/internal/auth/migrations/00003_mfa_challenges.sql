-- +goose Up

-- Half-finished logins (M3): the password was right, the 2FA code is still
-- missing. The browser holds the token in the __Host-mfa cookie; only its
-- SHA-256 is stored. A challenge lives 5 minutes and allows 5 code attempts.
CREATE TABLE mfa_challenges (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash bytea       NOT NULL UNIQUE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    attempts   integer     NOT NULL DEFAULT 0,
    user_agent text        NOT NULL DEFAULT ''
);
CREATE INDEX mfa_challenges_expires_at_idx ON mfa_challenges (expires_at);

-- +goose Down
DROP TABLE mfa_challenges;
