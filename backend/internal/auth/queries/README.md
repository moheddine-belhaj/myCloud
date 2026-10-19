# queries

Plain SQL that sqlc compiles into Go methods. Each query starts with a comment `-- name: CreateUser :one`:

- `:one` returns one row, `:exec` nothing, `:execrows` the affected row count.
- `@param` and `sqlc.narg(param)` (nullable) become typed function arguments.

Files: `users.sql` (create, get by email/id), `invites.sql` (create, claim: atomic "mark used if unused and unexpired"), `sessions.sql` (create, lookup user by token hash, delete, purge expired).

Parameters are always bound by pgx, never concatenated into SQL strings.
