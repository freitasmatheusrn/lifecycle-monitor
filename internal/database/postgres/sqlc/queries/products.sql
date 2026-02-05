-- name: CreateProduct :one
INSERT INTO products (
    code, url, area_id, description, manufacturer_code,
    quantity, sap_code, observations, min_quantity, max_quantity, inventory_status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: FindProductByID :one
SELECT p.*, a.name as area_name
FROM products p
LEFT JOIN areas a ON p.area_id = a.id
WHERE p.id = $1;

-- name: FindProductByCode :one
SELECT p.*, a.name as area_name
FROM products p
LEFT JOIN areas a ON p.area_id = a.id
WHERE p.code = $1;

-- name: FindProductByCodeAndArea :one
SELECT p.*, a.name as area_name
FROM products p
LEFT JOIN areas a ON p.area_id = a.id
WHERE p.code = $1 AND (p.area_id = $2 OR ($2 IS NULL AND p.area_id IS NULL));

-- name: ListProducts :many
SELECT * FROM (
    SELECT p.*, a.name as area_name
    FROM products p
    LEFT JOIN areas a ON p.area_id = a.id
    ORDER BY p.code, p.created_at DESC
) sub
ORDER BY lifecycle_status DESC, created_at DESC;

-- name: ListProductsPaginated :many
SELECT * FROM (
    SELECT p.*, a.name as area_name
    FROM products p
    LEFT JOIN areas a ON p.area_id = a.id
    ORDER BY p.code, p.created_at DESC
) sub
ORDER BY lifecycle_status DESC, created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListProductsByArea :many
SELECT p.*, a.name as area_name
FROM products p
LEFT JOIN areas a ON p.area_id = a.id
WHERE p.area_id = $1
ORDER BY p.lifecycle_status DESC, p.created_at DESC;

-- name: ListProductsByAreaPaginated :many
SELECT p.*, a.name as area_name
FROM products p
LEFT JOIN areas a ON p.area_id = a.id
WHERE p.area_id = $1
ORDER BY p.lifecycle_status DESC, p.created_at DESC
LIMIT $2 OFFSET $3;

-- name: SearchProducts :many
SELECT p.*, a.name as area_name
FROM products p
LEFT JOIN areas a ON p.area_id = a.id
WHERE p.code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%'
ORDER BY p.lifecycle_status DESC, p.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: SearchProductsByArea :many
SELECT p.*, a.name as area_name
FROM products p
LEFT JOIN areas a ON p.area_id = a.id
WHERE p.area_id = sqlc.arg('area_id')
  AND (p.code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%')
ORDER BY p.lifecycle_status DESC, p.created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountProducts :one
SELECT COUNT(*) FROM products;

-- name: CountProductsInArea :one
SELECT COUNT(*) FROM products WHERE area_id = $1;

-- name: CountProductsBySearch :one
SELECT COUNT(*)
FROM products p
WHERE p.code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%';

-- name: CountProductsByAreaAndSearch :one
SELECT COUNT(*)
FROM products p
WHERE p.area_id = sqlc.arg('area_id')
  AND (p.code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%');

-- name: DeleteProduct :exec
DELETE FROM products WHERE id = $1;

-- name: UpdateProduct :one
UPDATE products
SET
    code = COALESCE(sqlc.narg('code'), code),
    url = COALESCE(sqlc.narg('url'), url),
    area_id = COALESCE(sqlc.narg('area_id'), area_id),
    description = COALESCE(sqlc.narg('description'), description),
    manufacturer_code = COALESCE(sqlc.narg('manufacturer_code'), manufacturer_code),
    quantity = COALESCE(sqlc.narg('quantity'), quantity),
    sap_code = COALESCE(sqlc.narg('sap_code'), sap_code),
    observations = COALESCE(sqlc.narg('observations'), observations),
    min_quantity = COALESCE(sqlc.narg('min_quantity'), min_quantity),
    max_quantity = COALESCE(sqlc.narg('max_quantity'), max_quantity),
    inventory_status = COALESCE(sqlc.narg('inventory_status'), inventory_status)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListAllProductsToCollect :many
SELECT p.id, p.code, p.url, p.created_at
FROM products p
ORDER BY p.created_at ASC;

-- name: UpdateProductLifecycleStatus :exec
UPDATE products
SET
    lifecycle_status = COALESCE(sqlc.narg('lifecycle_status'), lifecycle_status),
    replacement_url = COALESCE(sqlc.narg('replacement_url'), replacement_url)
WHERE code = sqlc.arg('code');

-- name: CountProductsByLifecycleStatus :one
SELECT
    COUNT(*) FILTER (WHERE lifecycle_status = 'Active Product') AS active_count,
    COUNT(*) FILTER (
        WHERE lifecycle_status IN (
            'Prod. Cancellation',
            'End Prod.Lifecycl.',
            'Prod. Discont.'
        )
    ) AS discontinued_count,
    COUNT(*) FILTER (WHERE lifecycle_status = 'Phase Out Announce') AS phase_out_count
FROM (
    SELECT DISTINCT code, lifecycle_status
    FROM products
) p;

-- name: ListUniqueProductsPaginated :many
WITH product_aggregates AS (
    SELECT
        p.code,
        MIN(p.description) as description,
        MIN(p.lifecycle_status) as lifecycle_status,
        MIN(p.replacement_url) as replacement_url,
        MIN(p.url) as url,
        MIN(p.manufacturer_code) as manufacturer_code,
        MIN(p.sap_code) as sap_code,
        SUM(COALESCE(p.quantity, 0))::INTEGER as total_quantity,
        MIN(p.created_at) as created_at,
        json_agg(
            json_build_object(
                'area_id', p.area_id,
                'area_name', a.name,
                'quantity', COALESCE(p.quantity, 0)
            ) ORDER BY a.name
        ) FILTER (WHERE p.area_id IS NOT NULL) as quantity_by_area
    FROM products p
    LEFT JOIN areas a ON p.area_id = a.id
    GROUP BY p.code
)
SELECT * FROM product_aggregates
ORDER BY lifecycle_status DESC, created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountUniqueProducts :one
SELECT COUNT(DISTINCT code) FROM products;

-- name: SearchUniqueProductsPaginated :many
WITH product_aggregates AS (
    SELECT
        p.code,
        MIN(p.description) as description,
        MIN(p.lifecycle_status) as lifecycle_status,
        MIN(p.replacement_url) as replacement_url,
        MIN(p.url) as url,
        MIN(p.manufacturer_code) as manufacturer_code,
        MIN(p.sap_code) as sap_code,
        SUM(COALESCE(p.quantity, 0))::INTEGER as total_quantity,
        MIN(p.created_at) as created_at,
        json_agg(
            json_build_object(
                'area_id', p.area_id,
                'area_name', a.name,
                'quantity', COALESCE(p.quantity, 0)
            ) ORDER BY a.name
        ) FILTER (WHERE p.area_id IS NOT NULL) as quantity_by_area
    FROM products p
    LEFT JOIN areas a ON p.area_id = a.id
    GROUP BY p.code
)
SELECT * FROM product_aggregates
WHERE code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%'
ORDER BY lifecycle_status DESC, created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountUniqueProductsBySearch :one
SELECT COUNT(DISTINCT code)
FROM products p
WHERE p.code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%';

-- name: ListUniqueProductsByAreaPaginated :many
WITH product_aggregates AS (
    SELECT
        p.code,
        MIN(p.description) as description,
        MIN(p.lifecycle_status) as lifecycle_status,
        MIN(p.replacement_url) as replacement_url,
        MIN(p.url) as url,
        MIN(p.manufacturer_code) as manufacturer_code,
        MIN(p.sap_code) as sap_code,
        SUM(COALESCE(p.quantity, 0))::INTEGER as total_quantity,
        MIN(p.created_at) as created_at,
        json_agg(
            json_build_object(
                'area_id', p2.area_id,
                'area_name', a.name,
                'quantity', COALESCE(p2.quantity, 0)
            ) ORDER BY a.name
        ) FILTER (WHERE p2.area_id IS NOT NULL) as quantity_by_area
    FROM products p
    INNER JOIN products p2 ON p.code = p2.code
    LEFT JOIN areas a ON p2.area_id = a.id
    WHERE p.area_id = sqlc.arg('area_id')
    GROUP BY p.code
)
SELECT * FROM product_aggregates
ORDER BY lifecycle_status DESC, created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountUniqueProductsInArea :one
SELECT COUNT(DISTINCT code) FROM products WHERE area_id = $1;

-- name: SearchUniqueProductsByAreaPaginated :many
WITH product_aggregates AS (
    SELECT
        p.code,
        MIN(p.description) as description,
        MIN(p.lifecycle_status) as lifecycle_status,
        MIN(p.replacement_url) as replacement_url,
        MIN(p.url) as url,
        MIN(p.manufacturer_code) as manufacturer_code,
        MIN(p.sap_code) as sap_code,
        SUM(COALESCE(p.quantity, 0))::INTEGER as total_quantity,
        MIN(p.created_at) as created_at,
        json_agg(
            json_build_object(
                'area_id', p2.area_id,
                'area_name', a.name,
                'quantity', COALESCE(p2.quantity, 0)
            ) ORDER BY a.name
        ) FILTER (WHERE p2.area_id IS NOT NULL) as quantity_by_area
    FROM products p
    INNER JOIN products p2 ON p.code = p2.code
    LEFT JOIN areas a ON p2.area_id = a.id
    WHERE p.area_id = sqlc.arg('area_id')
    GROUP BY p.code
)
SELECT * FROM product_aggregates
WHERE code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%'
ORDER BY lifecycle_status DESC, created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountUniqueProductsByAreaAndSearch :one
SELECT COUNT(DISTINCT p.code)
FROM products p
WHERE p.area_id = sqlc.arg('area_id')
  AND (p.code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.description ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.sap_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.manufacturer_code ILIKE '%' || sqlc.arg('search')::text || '%'
   OR p.lifecycle_status ILIKE '%' || sqlc.arg('search')::text || '%');

-- name: ListUniqueProductCodesToCollect :many
SELECT DISTINCT ON (p.code) p.id, p.code, p.url
FROM products p
ORDER BY p.code, p.created_at ASC;

