-- name: CreateInvite :one
INSERT INTO invites (code_hash, created_by, expires_at)
VALUES (@code_hash, sqlc.narg(created_by), @expires_at)
RETURNING id, expires_at;

-- name: ClaimInvite :one
-- Marks an invite as used. Returns no row (pgx.ErrNoRows) when the code is
-- unknown, already used or expired. Run it in the same transaction as
-- CreateUser and SetInviteUsedBy.
UPDATE invites
SET used_at = now(), used_by = @used_by
WHERE code_hash = @code_hash
  AND used_at IS NULL
  AND expires_at > now()
RETURNING id;

-- name: SetInviteUsedBy :exec
-- Links a claimed invite to the user it created. Register claims the invite
-- before inserting the user, so the user ID is only known afterwards.
UPDATE invites
SET used_by = @used_by
WHERE id = @id;