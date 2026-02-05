package pages

import (
	"net/http"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/products"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/user"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/views/pages"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/views/partials"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/htmx"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/parser"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	productService products.Service
}

func NewHandler(productService products.Service) *Handler {
	return &Handler{
		productService: productService,
	}
}

func (h *Handler) Login(c echo.Context) error {
	return htmx.Render(c, http.StatusOK, pages.Login())
}

func (h *Handler) Signup(c echo.Context) error {
	return htmx.Render(c, http.StatusOK, pages.Signup())
}

func (h *Handler) Index(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return htmx.Redirect(c, "/login")
	}

	var input products.ListProductsInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar parametros")
	}

	// Parse area_id from query if present
	areaIDStr := c.QueryParam("area_id")
	if areaIDStr != "" {
		areaUUID, err := parser.PgUUIDFromString(areaIDStr)
		if err == nil {
			input.AreaID = areaUUID
		}
	}

	result, apiErr := h.productService.ListProducts(c.Request().Context(), input)
	if apiErr != nil {
		return apiErr
	}

	search := c.QueryParam("search")

	if htmx.IsHTMXRequest(c) {
		return htmx.Render(c, http.StatusOK, partials.ProductsList(result))
	}
	return htmx.Render(c, http.StatusOK, pages.Index(result, search))
}

func (h *Handler) ProductForm(c echo.Context) error {
	return htmx.Render(c, http.StatusOK, partials.ProductForm())
}

func (h *Handler) ProductsList(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	var input products.ListProductsInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar parametros")
	}

	// Parse area_id from query if present
	areaIDStr := c.QueryParam("area_id")
	if areaIDStr != "" {
		areaUUID, err := parser.PgUUIDFromString(areaIDStr)
		if err == nil {
			input.AreaID = areaUUID
		}
	}

	result, apiErr := h.productService.ListProducts(c.Request().Context(), input)
	if apiErr != nil {
		return apiErr
	}

	return htmx.Render(c, http.StatusOK, partials.ProductsList(result))
}
