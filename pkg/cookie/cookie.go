package cookie

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func New(name string, value string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   maxAge,
	}
}

func SetAccessTokenCookie(c echo.Context, token string, expSeconds int) {
	expires := time.Now().Add(time.Duration(expSeconds) * time.Second)
	cookie := New("access_token", token, expires, expSeconds)
	c.SetCookie(cookie)
}

func SetRefreshTokenCookie(c echo.Context, token string, expSeconds int) {
	expires := time.Now().Add(time.Duration(expSeconds) * time.Second)
	cookie := New("refresh_token", token, expires, expSeconds)
	c.SetCookie(cookie)
}

func ClearAuthCookies(c echo.Context) {
	// Clear access token
	c.SetCookie(&http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	// Clear refresh token
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// SetFlashToast sets a flash toast message that will be displayed on the next page load
// The cookie is readable by JavaScript so the toast.js can consume it
func SetFlashToast(c echo.Context, level, message string) {
	c.SetCookie(&http.Cookie{
		Name:     "flash_toast",
		Value:    level + "|" + message,
		Path:     "/",
		HttpOnly: false, // Must be readable by JavaScript
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60, // 1 minute expiry
	})
}
