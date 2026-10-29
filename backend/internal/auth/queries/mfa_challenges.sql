-- name: CreateMFAChallenge :exec
INSERT INTO mfa_challenges (token_hash, user_id, expires_at, user_agent)
VALUES (@token_hash, @user_id, @expires_at, @user_agent);

-- name: CountMFAAttempt :one
-- Counts one code attempt before the code is checked, in a single statement,
-- so parallel requests can't get more than max_attempts tries. Returns no row
-- when the challenge is unknown, expired or out of attempts.
UPDATE mfa_challenges
SET attempts = attempts + 1
WHERE token_hash = @token_hash
  AND expires_at > now()
  AND attempts < @max_attempts::integer
RETURNING user_id, attempts, user_agent;

-- name: DeleteMFAChallenge :exec
DELETE FROM mfa_challenges
WHERE token_hash = @token_hash;

-- name: DeleteExpiredMFAChallenges :execrows
DELETE FROM mfa_challenges
WHERE expires_at <= now();
