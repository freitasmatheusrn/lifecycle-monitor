package auth

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type SignupInput struct {
	Name     string `json:"name" form:"name"`
	Email    string `json:"email" form:"email"`
	Password string `json:"password" form:"password"`
}

type SignupOutput struct {
	ID    pgtype.UUID `json:"id"`
	Name  string      `json:"name"`
	Email string      `json:"email"`
}

type SigninInput struct {
	Email    string `json:"email" form:"email"`
	Password string `json:"password" form:"password"`
}

type SigninOutput struct {
	ID    pgtype.UUID `json:"id"`
	Name  string      `json:"name"`
	Email string      `json:"email"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type SignupResult struct {
	User   SignupOutput
	Tokens *TokenPair
}

type SigninResult struct {
	User   SigninOutput
	Tokens *TokenPair
}
