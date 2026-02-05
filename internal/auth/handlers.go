package auth

import (
	"net/http"

	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/auth"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/cookie"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/htmx"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	service         Service
	accessTokenExp  int
	refreshTokenExp int
}

func NewHandler(service Service, accessExp, refreshExp int) *Handler {
	return &Handler{
		service:         service,
		accessTokenExp:  accessExp,
		refreshTokenExp: refreshExp,
	}
}

func (h *Handler) Signup(c echo.Context) error {
	var input SignupInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar dados")
	}

	result, apiErr := h.service.Signup(c.Request().Context(), input, c.Request().UserAgent(), c.RealIP())
	if apiErr != nil {
		return apiErr
	}

	h.setTokenCookies(c, result.Tokens)

	if htmx.IsHTMXRequest(c) {
		cookie.SetFlashToast(c, "success", "Conta criada com sucesso!")
		return htmx.Redirect(c, "/")
	}
	return c.JSON(http.StatusCreated, result.User)
}

func (h *Handler) Signin(c echo.Context) error {
	var credentials SigninInput
	if err := c.Bind(&credentials); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar dados")
	}

	result, apiErr := h.service.Signin(c.Request().Context(), credentials, c.Request().UserAgent(), c.RealIP())
	if apiErr != nil {
		return apiErr
	}

	h.setTokenCookies(c, result.Tokens)

	if htmx.IsHTMXRequest(c) {
		cookie.SetFlashToast(c, "success", "Login realizado com sucesso!")
		return htmx.Redirect(c, "/")
	}
	return c.JSON(http.StatusAccepted, map[string]any{
		"user":         result.User,
		"access_token": result.Tokens.AccessToken,
	})
}

func (h *Handler) Refresh(c echo.Context) error {
	refreshCookie, err := c.Cookie("refresh_token")
	if err != nil {
		return rest.NewUnauthorizedRequestError("refresh token não encontrado")
	}

	tokens, apiErr := h.service.RefreshTokens(c.Request().Context(), refreshCookie.Value, c.Request().UserAgent(), c.RealIP())
	if apiErr != nil {
		cookie.ClearAuthCookies(c)
		return apiErr
	}

	h.setTokenCookies(c, tokens)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "tokens atualizados com sucesso",
	})
}

func (h *Handler) Logout(c echo.Context) error {
	refreshCookie, err := c.Cookie("refresh_token")
	if err == nil {
		_ = h.service.Logout(c.Request().Context(), refreshCookie.Value)
	}

	cookie.ClearAuthCookies(c)

	if htmx.IsHTMXRequest(c) {
		cookie.SetFlashToast(c, "info", "Você saiu da sua conta")
		return htmx.Redirect(c, "/login")
	}
	return c.JSON(http.StatusOK, map[string]string{
		"message": "logout realizado com sucesso",
	})
}

func (h *Handler) LogoutAll(c echo.Context) error {
	userID, apiErr := auth.GetUserID(c)
	if apiErr != nil {
		return apiErr
	}

	if err := h.service.LogoutAll(c.Request().Context(), userID); err != nil {
		return rest.NewInternalServerError("erro ao revogar tokens")
	}

	cookie.ClearAuthCookies(c)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "logout de todas as sessões realizado com sucesso",
	})
}

func (h *Handler) setTokenCookies(c echo.Context, tokens *TokenPair) {
	cookie.SetAccessTokenCookie(c, tokens.AccessToken, h.accessTokenExp)
	cookie.SetRefreshTokenCookie(c, tokens.RefreshToken, h.refreshTokenExp)
}
