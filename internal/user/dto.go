package user

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type UserOutput struct {
	ID        pgtype.UUID `json:"id"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	CreatedAt time.Time   `json:"created_at"`
}

type UpdateUserInput struct {
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Password *string `json:"password"`
}
