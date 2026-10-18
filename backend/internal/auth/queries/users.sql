-- name: CreateUser :one
INSERT INTO users (email, password_hash, role)
VALUES (@email, @password_hash, @role)
RETURNING id, email, role, created_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, role, created_at
FROM users
WHERE email = @email;

-- name: GetUserByID :one
SELECT id, email, role, created_at
FROM users
WHERE id = @id;