-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at, user_agent)
VALUES (@token_hash, @user_id, @expires_at, @user_agent);

-- name: GetSessionUser :one
-- Returns the user behind a valid (not expired) session, and whether 2FA is on.
SELECT u.id, u.email, u.role, u.created_at, s.expires_at AS session_expires_at,
       (t.enabled_at IS NOT NULL)::boolean AS totp_enabled
FROM sessions s
JOIN users u ON u.id = s.user_id
LEFT JOIN user_totp t ON t.user_id = u.id
WHERE s.token_hash = @token_hash
  AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = @token_hash;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= now();

-- name: DeleteOtherSessions :execrows
-- Ends every session of a user except the one making the request.
DELETE FROM sessions
WHERE user_id = @user_id
  AND token_hash <> @keep_token_hash;