# queries

Plain SQL that sqlc compiles into Go methods. Each query starts with a comment `-- name: CreateUser :one`:

- `:one` returns one row, `:exec` nothing, `:execrows` the affected row count.
- `@param` and `sqlc.narg(param)` (nullable) become typed function arguments.

Files: `users.sql` (create, get by email/id), `invites.sql` (create, claim: atomic "mark used if unused and unexpired"), `sessions.sql` (create, lookup user by token hash, delete, delete all of a user's sessions, purge expired), `totp.sql` (encrypted seed, enable, last used step), `recovery_codes.sql` (insert, delete, use one, count unused), `mfa_challenges.sql` (create, count an attempt, delete, purge expired).

`CountMFAAttempt` is one `UPDATE ... RETURNING`: the attempt is counted and checked in a single statement, so parallel requests cannot exceed the limit.

Parameters are always bound by pgx, never concatenated into SQL strings.
