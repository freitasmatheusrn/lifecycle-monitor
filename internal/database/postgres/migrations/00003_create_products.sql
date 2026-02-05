-- +goose Up
-- +goose StatementBegin
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL,
    url TEXT NOT NULL,
    area_id UUID REFERENCES areas(id) ON DELETE SET NULL,
    description TEXT,
    manufacturer_code TEXT,
    quantity INTEGER DEFAULT 0,
    replacement_url TEXT,
    sap_code TEXT,
    observations TEXT,
    min_quantity INTEGER DEFAULT 0,
    max_quantity INTEGER DEFAULT 0,
    inventory_status TEXT,
    lifecycle_status TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_products_code ON products(code);
CREATE INDEX idx_products_area_id ON products(area_id);
CREATE INDEX idx_products_sap_code ON products(sap_code);
CREATE INDEX idx_products_manufacturer_code ON products(manufacturer_code);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS products;
-- +goose StatementEnd
