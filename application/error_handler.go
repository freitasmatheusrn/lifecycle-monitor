package application

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/htmx"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/labstack/echo/v4"
)

func (a *Application) CustomErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	var code int
	var message string

	if apiErr, ok := err.(*rest.ApiErr); ok {
		code = apiErr.Code
		message = apiErr.Message
		log.Printf("code: %v, message: %s, causes: %v", apiErr.Code, apiErr.Message, apiErr.Causes)
	} else if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		switch he.Code {
		case http.StatusUnauthorized:
			message = he.Error()
		default:
			if msg, ok := he.Message.(string); ok {
				message = msg
			} else {
				message = http.StatusText(he.Code)
			}
		}
		log.Printf("code: %v, message: %s", code, message)
	} else {
		code = http.StatusInternalServerError
		message = "Erro interno do servidor"
		c.Logger().Error(err)
	}

	// HTMX request: return toast notification via HX-Trigger header
	if htmx.IsHTMXRequest(c) {
		if code == http.StatusUnauthorized {
			c.Response().Header().Set("HX-Redirect", "/login")
			c.NoContent(http.StatusOK)
			return
		}

		trigger := fmt.Sprintf(`{"makeToast": {"level": "danger", "message": "%s"}}`, message)
		c.Response().Header().Set("HX-Trigger", trigger)
		c.Response().Header().Set("HX-Reswap", "none")
		c.NoContent(code)
		return
	}

	// HTML request (browser): redirect to login on 401
	accept := c.Request().Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		if code == http.StatusUnauthorized {
			c.Redirect(http.StatusFound, "/login")
			return
		}
	}

	// JSON response for API clients
	apiErr := &rest.ApiErr{
		Message: message,
		Err:     http.StatusText(code),
		Code:    code,
	}
	c.JSON(code, apiErr)
}
