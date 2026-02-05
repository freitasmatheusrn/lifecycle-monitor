-- name: CreateSnapshot :one
INSERT INTO product_snapshots (product_id, description, status, raw_html)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: FindSnapshotByID :one
SELECT * FROM product_snapshots WHERE id = $1;

-- name: ListProductSnapshots :many
SELECT * FROM product_snapshots
WHERE product_id = $1
ORDER BY collected_at DESC;

-- name: GetLatestSnapshot :one
SELECT * FROM product_snapshots
WHERE product_id = $1
ORDER BY collected_at DESC
LIMIT 1;

-- name: ListSnapshotsByDateRange :many
SELECT * FROM product_snapshots
WHERE product_id = $1
AND collected_at BETWEEN $2 AND $3
ORDER BY collected_at DESC;

-- name: DeleteOldSnapshots :exec
DELETE FROM product_snapshots
WHERE product_id = $1
AND collected_at < $2;

-- name: CountProductSnapshots :one
SELECT COUNT(*) FROM product_snapshots WHERE product_id = $1;

-- name: GetSnapshotStatusHistory :many
SELECT id, status, collected_at
FROM product_snapshots
WHERE product_id = $1
ORDER BY collected_at ASC;
