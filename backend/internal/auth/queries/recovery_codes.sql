-- name: InsertRecoveryCodes :exec
-- Inserts several codes in one statement.
INSERT INTO recovery_codes (user_id, code_hash)
SELECT @user_id, unnest(@code_hashes::bytea[]);

-- name: DeleteRecoveryCodes :exec
DELETE FROM recovery_codes
WHERE user_id = @user_id;
