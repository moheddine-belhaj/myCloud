-- name: InsertRecoveryCodes :exec
-- Inserts several codes in one statement.
INSERT INTO recovery_codes (user_id, code_hash)
SELECT @user_id, unnest(@code_hashes::bytea[]);

-- name: DeleteRecoveryCodes :exec
DELETE FROM recovery_codes
WHERE user_id = @user_id;

-- name: UseRecoveryCode :execrows
-- Marks one unused code as used. 0 rows: wrong or already used.
UPDATE recovery_codes
SET used_at = now()
WHERE user_id = @user_id
  AND code_hash = @code_hash
  AND used_at IS NULL;

-- name: CountUnusedRecoveryCodes :one
SELECT count(*)
FROM recovery_codes
WHERE user_id = @user_id
  AND used_at IS NULL;
