-- +goose Up

-- Accounts. Emails are stored lowercase so "Mo@x.com" and "mo@x.com" are one account.
CREATE TABLE users (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text        NOT NULL,
    password_hash text        NOT NULL,                 -- argon2id, PHC string format
    role          text        NOT NULL DEFAULT 'user',
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_lowercase CHECK (email = lower(email)),
    CONSTRAINT users_role_valid      CHECK (role IN ('user', 'admin'))
);
CREATE UNIQUE INDEX users_email_key ON users (email);

-- Single-use registration codes. Only the SHA-256 of the code is stored.
CREATE TABLE invites (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash  bytea       NOT NULL UNIQUE,
    created_by uuid        REFERENCES users (id) ON DELETE SET NULL,  -- NULL when created by CLI
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    used_by    uuid        REFERENCES users (id) ON DELETE SET NULL,
    -- used_by is only set on a used invite (it becomes NULL again if that user is deleted)
    CONSTRAINT invites_used_by_requires_used_at CHECK (used_by IS NULL OR used_at IS NOT NULL)
);

-- Login sessions. Only the SHA-256 of the cookie token is stored.
CREATE TABLE sessions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash bytea       NOT NULL UNIQUE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    user_agent text        NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_id_idx    ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE invites;
DROP TABLE users;