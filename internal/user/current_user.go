package user

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

type CurrentUser struct {
	ID    pgtype.UUID
	Name  string
	Email string
}

func SetCurrentUser(c echo.Context, user CurrentUser) {
	c.Set("current_user", user)
}

func GetCurrentUser(c echo.Context) (CurrentUser, error) {
	u := c.Get("current_user")
	if u == nil {
		return CurrentUser{}, errors.New("user not authenticated")
	}

	currentUser, ok := u.(CurrentUser)
	if !ok {
		return CurrentUser{}, errors.New("invalid user context")
	}
	return currentUser, nil
}
