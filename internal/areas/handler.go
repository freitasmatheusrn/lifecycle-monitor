package areas

import (
	"net/http"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/user"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/parser"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// CreateArea handles POST /areas
func (h *Handler) CreateArea(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	var input CreateAreaInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar dados")
	}

	if input.Name == "" {
		return rest.NewBadRequestError("nome da area e obrigatorio")
	}

	result, apiErr := h.service.CreateArea(c.Request().Context(), input)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusCreated, result)
}

// ListAreas handles GET /areas
func (h *Handler) ListAreas(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	result, apiErr := h.service.ListAreas(c.Request().Context())
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

// GetArea handles GET /areas/:id
func (h *Handler) GetArea(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	areaID := c.Param("id")
	if areaID == "" {
		return rest.NewBadRequestError("id da area e obrigatorio")
	}

	pgUUID, err := parser.PgUUIDFromString(areaID)
	if err != nil {
		return rest.NewBadRequestError("id da area invalido")
	}

	result, apiErr := h.service.GetArea(c.Request().Context(), pgUUID)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

// UpdateArea handles PUT /areas/:id
func (h *Handler) UpdateArea(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	areaID := c.Param("id")
	if areaID == "" {
		return rest.NewBadRequestError("id da area e obrigatorio")
	}

	pgUUID, err := parser.PgUUIDFromString(areaID)
	if err != nil {
		return rest.NewBadRequestError("id da area invalido")
	}

	var input UpdateAreaInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar dados")
	}

	result, apiErr := h.service.UpdateArea(c.Request().Context(), pgUUID, input)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

// DeleteArea handles DELETE /areas/:id
func (h *Handler) DeleteArea(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	areaID := c.Param("id")
	if areaID == "" {
		return rest.NewBadRequestError("id da area e obrigatorio")
	}

	pgUUID, err := parser.PgUUIDFromString(areaID)
	if err != nil {
		return rest.NewBadRequestError("id da area invalido")
	}

	apiErr := h.service.DeleteArea(c.Request().Context(), pgUUID)
	if apiErr != nil {
		return apiErr
	}

	return c.NoContent(http.StatusNoContent)
}
