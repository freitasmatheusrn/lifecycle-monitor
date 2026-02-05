package auth

import (
	"time"

	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type JWTCustomClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

func NewClaims(userID, email string, tokenExp int) *JWTCustomClaims {
	return &JWTCustomClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Second * time.Duration(tokenExp))),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
}

func GenerateJWT(claims *JWTCustomClaims, jwtSecret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", err
	}
	return t, nil
}

func GetClaims(c echo.Context) (*JWTCustomClaims, *rest.ApiErr) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok {
		return nil, rest.NewUnauthorizedRequestError("token inválido")
	}

	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok {
		return nil, rest.NewUnauthorizedRequestError("claims inválidas")
	}
	return claims, nil
}

func GetUserID(c echo.Context) (string, *rest.ApiErr) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok {
		return "", rest.NewUnauthorizedRequestError("token inválido")
	}

	claims, ok := token.Claims.(*JWTCustomClaims)
	if !ok {
		return "", rest.NewUnauthorizedRequestError("claims inválidas")
	}
	return claims.UserID, nil
}
