package auth

import (
	"net/http"

	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/auth"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/cookie"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

func AutoRefreshMiddleware(service Service, accessExp, refreshExp int, jwtSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			accessCookie, err := c.Cookie("access_token")
			if err != nil || accessCookie.Value == "" {
				return tryRefreshAndContinue(c, next, service, accessExp, refreshExp, jwtSecret)
			}

			token, err := jwt.ParseWithClaims(accessCookie.Value, &auth.JWTCustomClaims{}, func(t *jwt.Token) (interface{}, error) {
				return []byte(jwtSecret), nil
			})

			if err == nil && token.Valid {
				return next(c)
			}

			return tryRefreshAndContinue(c, next, service, accessExp, refreshExp, jwtSecret)
		}
	}
}

func tryRefreshAndContinue(c echo.Context, next echo.HandlerFunc, service Service, accessExp, refreshExp int, jwtSecret string) error {
	refreshCookie, err := c.Cookie("refresh_token")
	if err != nil || refreshCookie.Value == "" {
		return next(c)
	}

	tokens, apiErr := service.RefreshTokens(c.Request().Context(), refreshCookie.Value, c.Request().UserAgent(), c.RealIP())
	if apiErr != nil {
		cookie.ClearAuthCookies(c)
		return next(c)
	}

	cookie.SetAccessTokenCookie(c, tokens.AccessToken, accessExp)
	cookie.SetRefreshTokenCookie(c, tokens.RefreshToken, refreshExp)

	c.Request().AddCookie(&http.Cookie{
		Name:  "access_token",
		Value: tokens.AccessToken,
	})

	return next(c)
}
