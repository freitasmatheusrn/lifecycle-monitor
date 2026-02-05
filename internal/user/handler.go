package user

import (
	"net/http"

	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetMe(c echo.Context) error {
	currentUser, err := GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuário não autenticado")
	}

	result, apiErr := h.service.FindByID(c.Request().Context(), currentUser.ID)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

func (h *Handler) Update(c echo.Context) error {
	var input UpdateUserInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar dados")
	}

	currentUser, err := GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuário não autenticado")
	}

	result, apiErr := h.service.UpdateUser(c.Request().Context(), currentUser.ID, input)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}
