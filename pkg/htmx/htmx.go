package htmx

import (
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

const (
	HXRequest      = "HX-Request"
	HXTrigger      = "HX-Trigger"
	HXRedirect     = "HX-Redirect"
	HXReswap       = "HX-Reswap"
	HXRetarget     = "HX-Retarget"
	HXPushURL      = "HX-Push-Url"
	HXRefresh      = "HX-Refresh"
	HXReplaceURL   = "HX-Replace-Url"
	HXTriggerAfter = "HX-Trigger-After-Settle"
)

// IsHTMXRequest checks if the request is from HTMX
func IsHTMXRequest(c echo.Context) bool {
	return c.Request().Header.Get(HXRequest) == "true"
}

// IsBoosted checks if request came from hx-boost
func IsBoosted(c echo.Context) bool {
	return c.Request().Header.Get("HX-Boosted") == "true"
}

// Render renders a templ component to the response
func Render(c echo.Context, statusCode int, component templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(statusCode)
	return component.Render(c.Request().Context(), c.Response())
}

// Redirect performs HTMX-aware redirect
func Redirect(c echo.Context, url string) error {
	if IsHTMXRequest(c) {
		c.Response().Header().Set(HXRedirect, url)
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusFound, url)
}

// SetTrigger sets the HX-Trigger header
func SetTrigger(c echo.Context, trigger string) {
	c.Response().Header().Set(HXTrigger, trigger)
}

// SetReswap sets the HX-Reswap header
func SetReswap(c echo.Context, value string) {
	c.Response().Header().Set(HXReswap, value)
}

// SetRetarget sets the HX-Retarget header
func SetRetarget(c echo.Context, selector string) {
	c.Response().Header().Set(HXRetarget, selector)
}

// Refresh triggers a full page refresh
func Refresh(c echo.Context) error {
	c.Response().Header().Set(HXRefresh, "true")
	return c.NoContent(http.StatusOK)
}

// TriggerToast sends a toast notification via HX-Trigger header
// level can be: "success", "danger", "warning", "info"
func TriggerToast(c echo.Context, level, message string) {
	trigger := fmt.Sprintf(`{"makeToast": {"level": "%s", "message": "%s"}}`, level, message)
	c.Response().Header().Set(HXTrigger, trigger)
}

// TriggerToastAfterSettle sends a toast notification after HTMX settles the DOM
func TriggerToastAfterSettle(c echo.Context, level, message string) {
	trigger := fmt.Sprintf(`{"makeToast": {"level": "%s", "message": "%s"}}`, level, message)
	c.Response().Header().Set(HXTriggerAfter, trigger)
}

// RedirectWithToast performs a redirect and shows a toast on the target page
func RedirectWithToast(c echo.Context, url, level, message string) error {
	if IsHTMXRequest(c) {
		TriggerToastAfterSettle(c, level, message)
		c.Response().Header().Set(HXRedirect, url)
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusFound, url)
}
