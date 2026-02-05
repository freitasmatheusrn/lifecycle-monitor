-- name: CreateArea :one
INSERT INTO areas (name, description)
VALUES ($1, $2)
RETURNING *;

-- name: FindAreaByID :one
SELECT * FROM areas WHERE id = $1;

-- name: FindAreaByName :one
SELECT * FROM areas WHERE name = $1;

-- name: ListAreas :many
SELECT * FROM areas ORDER BY name ASC;

-- name: UpdateArea :one
UPDATE areas
SET
    name = COALESCE(sqlc.narg('name'), name),
    description = COALESCE(sqlc.narg('description'), description)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteArea :exec
DELETE FROM areas WHERE id = $1;

-- name: CountProductsByArea :one
SELECT COUNT(*) FROM products WHERE area_id = $1;
