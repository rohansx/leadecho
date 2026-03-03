-- name: ListUsersByExternalID :many
SELECT * FROM users
WHERE clerk_user_id = @clerk_user_id;

-- name: GetUser :one
SELECT * FROM users WHERE id = @id;

-- name: FindUserByEmail :one
SELECT * FROM users WHERE email = @email LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (clerk_user_id, workspace_id, email, name, avatar_url, role, password_hash)
VALUES (@clerk_user_id, @workspace_id, @email, @name, @avatar_url, @role, @password_hash)
RETURNING *;
