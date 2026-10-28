-- name: StartTOTPSetup :execrows
-- Stores a new pending secret. Repeating setup replaces a pending secret, but
-- never an enabled one: then no row changes and the caller reports a conflict.
INSERT INTO user_totp (user_id, secret_enc)
VALUES (@user_id, @secret_enc)
ON CONFLICT (user_id) DO UPDATE
SET secret_enc = EXCLUDED.secret_enc, created_at = now(), last_step = 0
WHERE user_totp.enabled_at IS NULL;

-- name: GetTOTP :one
SELECT secret_enc, enabled_at, last_step
FROM user_totp
WHERE user_id = @user_id;

-- name: EnableTOTP :execrows
-- Turns a pending secret on and records the step of the confirming code.
-- Returns 0 rows when it was enabled meanwhile or the step was already used.
UPDATE user_totp
SET enabled_at = now(), last_step = @step
WHERE user_id = @user_id
  AND enabled_at IS NULL
  AND last_step < @step;

-- name: UseTOTPStep :execrows
-- Records a used step. Two requests with the same code race here: only one
-- updates the row, the other gets 0 rows and is refused.
UPDATE user_totp
SET last_step = @step
WHERE user_id = @user_id
  AND enabled_at IS NOT NULL
  AND last_step < @step;

-- name: DeleteTOTP :exec
DELETE FROM user_totp
WHERE user_id = @user_id;
