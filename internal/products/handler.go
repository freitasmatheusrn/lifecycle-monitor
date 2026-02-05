package products

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/user"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/htmx"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/parser"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// CreateProduct handles POST /products
// Creates a new product manually
func (h *Handler) CreateProduct(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	var input CreateProductInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar dados")
	}

	if input.Code == "" {
		return rest.NewBadRequestError("codigo do produto e obrigatorio")
	}

	result, apiErr := h.service.CreateProduct(c.Request().Context(), input)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusCreated, result)
}

// UpdateProduct handles PUT /products/:id
// Updates an existing product
func (h *Handler) UpdateProduct(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	productID := c.Param("id")
	if productID == "" {
		return rest.NewBadRequestError("id do produto e obrigatorio")
	}

	pgUUID, err := parser.PgUUIDFromString(productID)
	if err != nil {
		return rest.NewBadRequestError("id do produto invalido")
	}

	var input UpdateProductInput
	if err := c.Bind(&input); err != nil {
		return rest.NewUnprocessableEntity("erro ao processar dados")
	}

	result, apiErr := h.service.UpdateProduct(c.Request().Context(), pgUUID, input)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

// DeleteProduct handles DELETE /products/:id
// Deletes a product from the system
func (h *Handler) DeleteProduct(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	productID := c.Param("id")
	if productID == "" {
		return rest.NewBadRequestError("id do produto e obrigatorio")
	}

	pgUUID, err := parser.PgUUIDFromString(productID)
	if err != nil {
		return rest.NewBadRequestError("id do produto invalido")
	}

	apiErr := h.service.DeleteProduct(c.Request().Context(), pgUUID)
	if apiErr != nil {
		return apiErr
	}

	// Show success toast and return empty response for HTMX to remove the row
	htmx.TriggerToast(c, "success", "Produto removido com sucesso")
	return c.HTML(http.StatusOK, "")
}

// ListProducts handles GET /products
// Returns paginated list of all products with optional search and area filter
func (h *Handler) ListProducts(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	var input ListProductsInput
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

	result, apiErr := h.service.ListProducts(c.Request().Context(), input)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

// GetProduct handles GET /products/:id
// Returns a single product with its latest snapshot
func (h *Handler) GetProduct(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	productID := c.Param("id")
	if productID == "" {
		return rest.NewBadRequestError("id do produto e obrigatorio")
	}

	pgUUID, err := parser.PgUUIDFromString(productID)
	if err != nil {
		return rest.NewBadRequestError("id do produto invalido")
	}

	result, apiErr := h.service.GetProduct(c.Request().Context(), pgUUID)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

// GetProductSnapshots handles GET /products/:id/snapshots
// Returns all historical snapshots for a product
func (h *Handler) GetProductSnapshots(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	productID := c.Param("id")
	if productID == "" {
		return rest.NewBadRequestError("id do produto e obrigatorio")
	}

	pgUUID, err := parser.PgUUIDFromString(productID)
	if err != nil {
		return rest.NewBadRequestError("id do produto invalido")
	}

	result, apiErr := h.service.GetProductSnapshots(c.Request().Context(), pgUUID)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusOK, result)
}

// AddProductsSSE handles GET /products/add-stream
// Adds products with real-time progress updates via Server-Sent Events
func (h *Handler) AddProductsSSE(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	// Get codes from query params
	codes := c.QueryParams()["codes"]
	if len(codes) == 0 {
		return rest.NewBadRequestError("ao menos um codigo de produto e necessario")
	}

	// Get optional area_id from query
	var areaID pgtype.UUID
	areaIDStr := c.QueryParam("area_id")
	if areaIDStr != "" {
		parsedUUID, err := parser.PgUUIDFromString(areaIDStr)
		if err == nil {
			areaID = parsedUUID
		}
	}

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	// Create a channel for progress events
	eventChan := make(chan ProgressEvent, 10)
	done := make(chan struct{})

	// Progress callback that sends events to the channel
	onProgress := func(event ProgressEvent) {
		select {
		case eventChan <- event:
		case <-done:
		}
	}

	// Run product addition in a goroutine
	go func() {
		defer close(eventChan)
		input := AddProductsInput{Codes: codes, AreaID: areaID}
		h.service.AddProductsWithProgress(c.Request().Context(), input, onProgress)
	}()

	// Stream events to client
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return rest.NewInternalServerError("streaming nao suportado")
	}

	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// Channel closed, we're done
				return nil
			}

			// Serialize event to JSON
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			// Write SSE format
			fmt.Fprintf(c.Response(), "event: %s\n", event.Type)
			fmt.Fprintf(c.Response(), "data: %s\n\n", data)
			flusher.Flush()

			// If complete, close the connection
			if event.Type == ProgressEventComplete {
				close(done)
				return nil
			}

		case <-c.Request().Context().Done():
			// Client disconnected
			close(done)
			return nil
		}
	}
}

// ImportSpreadsheet handles POST /products/import
// Imports products from an Excel spreadsheet
func (h *Handler) ImportSpreadsheet(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		return rest.NewBadRequestError("arquivo nao fornecido")
	}

	// Get area_id from form
	var areaID pgtype.UUID
	areaIDStr := c.FormValue("area_id")
	if areaIDStr != "" {
		parsedUUID, err := parser.PgUUIDFromString(areaIDStr)
		if err == nil {
			areaID = parsedUUID
		}
	}

	src, err := file.Open()
	if err != nil {
		return rest.NewInternalServerError("erro ao abrir arquivo")
	}
	defer src.Close()

	result, apiErr := h.service.ImportFromSpreadsheet(c.Request().Context(), src, areaID)
	if apiErr != nil {
		return apiErr
	}

	return c.JSON(http.StatusCreated, result)
}

// ImportSpreadsheetSSE handles POST /products/import-stream
// Imports products from an Excel spreadsheet with real-time progress updates via SSE
func (h *Handler) ImportSpreadsheetSSE(c echo.Context) error {
	_, err := user.GetCurrentUser(c)
	if err != nil {
		return rest.NewUnauthorizedRequestError("usuario nao autenticado")
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		return rest.NewBadRequestError("arquivo nao fornecido")
	}

	// Get area_id from form
	var areaID pgtype.UUID
	areaIDStr := c.FormValue("area_id")
	if areaIDStr != "" {
		parsedUUID, err := parser.PgUUIDFromString(areaIDStr)
		if err == nil {
			areaID = parsedUUID
		}
	}

	src, err := file.Open()
	if err != nil {
		return rest.NewInternalServerError("erro ao abrir arquivo")
	}
	defer src.Close()

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	// Create a channel for progress events
	eventChan := make(chan ImportProgressEvent, 10)
	done := make(chan struct{})

	// Progress callback that sends events to the channel
	onProgress := func(event ImportProgressEvent) {
		select {
		case eventChan <- event:
		case <-done:
		}
	}

	// Run import in a goroutine
	go func() {
		defer close(eventChan)
		h.service.ImportFromSpreadsheetWithProgress(c.Request().Context(), src, areaID, onProgress)
	}()

	// Stream events to client
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return rest.NewInternalServerError("streaming nao suportado")
	}

	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return nil
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			fmt.Fprintf(c.Response(), "event: %s\n", event.Type)
			fmt.Fprintf(c.Response(), "data: %s\n\n", data)
			flusher.Flush()

			if event.Type == ImportEventComplete {
				close(done)
				return nil
			}

		case <-c.Request().Context().Done():
			close(done)
			return nil
		}
	}
}
