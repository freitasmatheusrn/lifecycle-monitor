-- +goose Up
-- +goose StatementBegin
CREATE TABLE areas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_areas_name ON areas(name);

-- Insert initial areas
INSERT INTO areas (name) VALUES
    ('Quimicos'),
    ('Cutter'),
    ('Bailing'),
    ('Turbo Bomba'),
    ('Ozonio'),
    ('Centrifuga'),
    ('Plataformas'),
    ('HVAC');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS areas;
-- +goose StatementEnd
