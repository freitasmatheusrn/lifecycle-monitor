-- +goose Up
-- +goose StatementBegin
CREATE TABLE product_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    status TEXT,
    raw_html TEXT,
    collected_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_product_snapshots_product_id ON product_snapshots(product_id);
CREATE INDEX idx_product_snapshots_collected_at ON product_snapshots(collected_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS product_snapshots;
-- +goose StatementEnd
