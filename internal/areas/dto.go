package areas

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Input DTOs

type CreateAreaInput struct {
	Name        string `json:"name" form:"name" validate:"required"`
	Description string `json:"description" form:"description"`
}

type UpdateAreaInput struct {
	Name        *string `json:"name" form:"name"`
	Description *string `json:"description" form:"description"`
}

// Output DTOs

type AreaOutput struct {
	ID          pgtype.UUID `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

type AreaWithCountOutput struct {
	AreaOutput
	ProductCount int64 `json:"product_count"`
}
