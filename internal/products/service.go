package products

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/freitasmatheusrn/lifecycle-monitor/internal/database"
	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	"github.com/freitasmatheusrn/lifecycle-monitor/pkg/rest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

type Service interface {
	CreateProduct(ctx context.Context, input CreateProductInput) (*ProductOutput, *rest.ApiErr)
	UpdateProduct(ctx context.Context, productID pgtype.UUID, input UpdateProductInput) (*ProductOutput, *rest.ApiErr)
	DeleteProduct(ctx context.Context, productID pgtype.UUID) *rest.ApiErr
	ListProducts(ctx context.Context, input ListProductsInput) (*PaginatedProductsOutput, *rest.ApiErr)
	GetProduct(ctx context.Context, productID pgtype.UUID) (*ProductWithSnapshotOutput, *rest.ApiErr)
	GetProductSnapshots(ctx context.Context, productID pgtype.UUID) ([]SnapshotOutput, *rest.ApiErr)
	AddProductsWithProgress(ctx context.Context, input AddProductsInput, onProgress ProgressCallback) (*AddProductsResult, *rest.ApiErr)
	ImportFromSpreadsheet(ctx context.Context, file io.Reader, areaID pgtype.UUID) (*ImportResult, *rest.ApiErr)
	ImportFromSpreadsheetWithProgress(ctx context.Context, file io.Reader, areaID pgtype.UUID, onProgress ImportProgressCallback) (*ImportResult, *rest.ApiErr)
	ExportToSpreadsheet(ctx context.Context) (*bytes.Buffer, *rest.ApiErr)
}

type svc struct {
	repo       repo.Querier
	workerPool *WorkerPool
	baseURL    string
	logger     *zap.Logger
}

func NewService(repo repo.Querier, workerPool *WorkerPool, baseURL string, logger *zap.Logger) *svc {
	return &svc{
		repo:       repo,
		workerPool: workerPool,
		baseURL:    baseURL,
		logger:     logger,
	}
}

func (s *svc) CreateProduct(ctx context.Context, input CreateProductInput) (*ProductOutput, *rest.ApiErr) {
	productURL := fmt.Sprintf("%s/%s", s.baseURL, input.Code)

	product, err := s.repo.CreateProduct(ctx, repo.CreateProductParams{
		Code:             input.Code,
		Url:              productURL,
		AreaID:           input.AreaID,
		Description:      toPgText(input.Description),
		ManufacturerCode: toPgText(input.ManufacturerCode),
		Quantity:         toPgInt4(input.Quantity),
		SapCode:          toPgText(input.SAPCode),
		Observations:     toPgText(input.Observations),
		MinQuantity:      toPgInt4(input.MinQuantity),
		MaxQuantity:      toPgInt4(input.MaxQuantity),
		InventoryStatus:  toPgText(input.InventoryStatus),
	})

	if err != nil {
		return nil, s.handleDBError(err)
	}

	return toProductOutput(product), nil
}

func (s *svc) UpdateProduct(ctx context.Context, productID pgtype.UUID, input UpdateProductInput) (*ProductOutput, *rest.ApiErr) {
	// Check if product exists
	_, err := s.repo.FindProductByID(ctx, productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rest.NewNotFoundError("produto nao encontrado")
		}
		return nil, s.handleDBError(err)
	}

	params := repo.UpdateProductParams{ID: productID}

	if input.Code != nil {
		params.Code = pgtype.Text{String: *input.Code, Valid: true}
	}
	if input.AreaID != nil {
		params.AreaID = *input.AreaID
	}
	if input.Description != nil {
		params.Description = pgtype.Text{String: *input.Description, Valid: true}
	}
	if input.ManufacturerCode != nil {
		params.ManufacturerCode = pgtype.Text{String: *input.ManufacturerCode, Valid: true}
	}
	if input.Quantity != nil {
		params.Quantity = pgtype.Int4{Int32: int32(*input.Quantity), Valid: true}
	}
	if input.SAPCode != nil {
		params.SapCode = pgtype.Text{String: *input.SAPCode, Valid: true}
	}
	if input.Observations != nil {
		params.Observations = pgtype.Text{String: *input.Observations, Valid: true}
	}
	if input.MinQuantity != nil {
		params.MinQuantity = pgtype.Int4{Int32: int32(*input.MinQuantity), Valid: true}
	}
	if input.MaxQuantity != nil {
		params.MaxQuantity = pgtype.Int4{Int32: int32(*input.MaxQuantity), Valid: true}
	}
	if input.InventoryStatus != nil {
		params.InventoryStatus = pgtype.Text{String: *input.InventoryStatus, Valid: true}
	}

	product, err := s.repo.UpdateProduct(ctx, params)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	return toProductOutputFromModel(product), nil
}

func (s *svc) DeleteProduct(ctx context.Context, productID pgtype.UUID) *rest.ApiErr {
	// Check if product exists
	_, err := s.repo.FindProductByID(ctx, productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rest.NewNotFoundError("produto nao encontrado")
		}
		return s.handleDBError(err)
	}

	err = s.repo.DeleteProduct(ctx, productID)
	if err != nil {
		return s.handleDBError(err)
	}

	return nil
}

func (s *svc) ListProducts(ctx context.Context, input ListProductsInput) (*PaginatedProductsOutput, *rest.ApiErr) {
	// Set defaults
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = 10
	}
	if input.PageSize > 100 {
		input.PageSize = 100
	}

	offset := (input.Page - 1) * input.PageSize

	// Get lifecycle status counts
	statusCounts, err := s.repo.CountProductsByLifecycleStatus(ctx)
	if err != nil {
		s.logger.Warn("failed to get lifecycle status counts", zap.Error(err))
	}

	var total int64
	var products []ProductWithSnapshotOutput

	hasAreaFilter := input.AreaID.Valid
	hasSearch := input.Search != ""

	if hasAreaFilter && hasSearch {
		// Search with area filter - unique products
		searchRows, searchErr := s.repo.SearchUniqueProductsByAreaPaginated(ctx, repo.SearchUniqueProductsByAreaPaginatedParams{
			AreaID: input.AreaID,
			Search: input.Search,
			Limit:  int32(input.PageSize),
			Offset: int32(offset),
		})
		if searchErr != nil {
			return nil, s.handleDBError(searchErr)
		}

		total, err = s.repo.CountUniqueProductsByAreaAndSearch(ctx, repo.CountUniqueProductsByAreaAndSearchParams{
			AreaID: input.AreaID,
			Search: input.Search,
		})
		if err != nil {
			return nil, s.handleDBError(err)
		}

		products = make([]ProductWithSnapshotOutput, 0, len(searchRows))
		for _, r := range searchRows {
			products = append(products, uniqueProductRowToOutput(r))
		}
	} else if hasAreaFilter {
		// Filter by area only - unique products
		areaRows, areaErr := s.repo.ListUniqueProductsByAreaPaginated(ctx, repo.ListUniqueProductsByAreaPaginatedParams{
			AreaID: input.AreaID,
			Limit:  int32(input.PageSize),
			Offset: int32(offset),
		})
		if areaErr != nil {
			return nil, s.handleDBError(areaErr)
		}

		total, err = s.repo.CountUniqueProductsInArea(ctx, input.AreaID)
		if err != nil {
			return nil, s.handleDBError(err)
		}

		products = make([]ProductWithSnapshotOutput, 0, len(areaRows))
		for _, r := range areaRows {
			products = append(products, uniqueProductByAreaRowToOutput(r))
		}
	} else if hasSearch {
		// Search only (no area filter) - unique products
		searchRows, searchErr := s.repo.SearchUniqueProductsPaginated(ctx, repo.SearchUniqueProductsPaginatedParams{
			Search: input.Search,
			Limit:  int32(input.PageSize),
			Offset: int32(offset),
		})
		if searchErr != nil {
			return nil, s.handleDBError(searchErr)
		}

		total, err = s.repo.CountUniqueProductsBySearch(ctx, input.Search)
		if err != nil {
			return nil, s.handleDBError(err)
		}

		products = make([]ProductWithSnapshotOutput, 0, len(searchRows))
		for _, r := range searchRows {
			products = append(products, searchUniqueProductRowToOutput(r))
		}
	} else {
		// No filters - list all unique products
		productRows, listErr := s.repo.ListUniqueProductsPaginated(ctx, repo.ListUniqueProductsPaginatedParams{
			Limit:  int32(input.PageSize),
			Offset: int32(offset),
		})
		if listErr != nil {
			return nil, s.handleDBError(listErr)
		}

		total, err = s.repo.CountUniqueProducts(ctx)
		if err != nil {
			return nil, s.handleDBError(err)
		}

		products = make([]ProductWithSnapshotOutput, 0, len(productRows))
		for _, r := range productRows {
			products = append(products, listUniqueProductRowToOutput(r))
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(input.PageSize)))

	result := &PaginatedProductsOutput{
		Products:          products,
		Total:             total,
		Page:              input.Page,
		PageSize:          input.PageSize,
		TotalPages:        totalPages,
		ActiveCount:       statusCounts.ActiveCount,
		DiscontinuedCount: statusCounts.DiscontinuedCount,
		PhaseOutCount:     statusCounts.PhaseOutCount,
	}

	return result, nil
}

func (s *svc) GetProduct(ctx context.Context, productID pgtype.UUID) (*ProductWithSnapshotOutput, *rest.ApiErr) {
	product, err := s.repo.FindProductByID(ctx, productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rest.NewNotFoundError("produto nao encontrado")
		}
		return nil, s.handleDBError(err)
	}

	productOutput := ProductOutput{
		ID:               product.ID,
		Code:             product.Code,
		URL:              product.Url,
		AreaID:           product.AreaID,
		AreaName:         product.AreaName.String,
		Description:      product.Description.String,
		ManufacturerCode: product.ManufacturerCode.String,
		Quantity:         int(product.Quantity.Int32),
		ReplacementURL:   product.ReplacementUrl.String,
		SAPCode:          product.SapCode.String,
		Observations:     product.Observations.String,
		MinQuantity:      int(product.MinQuantity.Int32),
		MaxQuantity:      int(product.MaxQuantity.Int32),
		InventoryStatus:  product.InventoryStatus.String,
		LifeCycleStatus:  product.LifecycleStatus.String,
		CreatedAt:        product.CreatedAt.Time,
	}

	var snapshotOutput *SnapshotOutput
	snapshot, err := s.repo.GetLatestSnapshot(ctx, productID)
	if err == nil {
		snapshotOutput = &SnapshotOutput{
			ID:          snapshot.ID,
			ProductID:   snapshot.ProductID,
			Description: snapshot.Description,
			Status:      snapshot.Status.String,
			CollectedAt: snapshot.CollectedAt.Time,
		}
	}

	return &ProductWithSnapshotOutput{
		Product:        productOutput,
		LatestSnapshot: snapshotOutput,
	}, nil
}

func (s *svc) GetProductSnapshots(ctx context.Context, productID pgtype.UUID) ([]SnapshotOutput, *rest.ApiErr) {
	snapshots, err := s.repo.ListProductSnapshots(ctx, productID)
	if err != nil {
		return nil, s.handleDBError(err)
	}

	result := make([]SnapshotOutput, 0, len(snapshots))
	for _, snap := range snapshots {
		result = append(result, SnapshotOutput{
			ID:          snap.ID,
			ProductID:   snap.ProductID,
			Description: snap.Description,
			Status:      snap.Status.String,
			CollectedAt: snap.CollectedAt.Time,
		})
	}

	return result, nil
}

func (s *svc) AddProductsWithProgress(ctx context.Context, input AddProductsInput, onProgress ProgressCallback) (*AddProductsResult, *rest.ApiErr) {
	result := &AddProductsResult{
		Added:    make([]ProductOutput, 0),
		Existing: make([]ProductOutput, 0),
		Failed:   make([]FailedProduct, 0),
	}

	total := len(input.Codes)
	processed := 0

	// Send start event
	if onProgress != nil {
		onProgress(ProgressEvent{
			Type:  ProgressEventStart,
			Index: 0,
			Total: total,
		})
	}

	// Fase 1: Verificar quais produtos já existem e quais precisam de crawl
	type pendingCrawl struct {
		code string
		url  string
	}
	var toCrawl []pendingCrawl

	for _, code := range input.Codes {
		existingProduct, err := s.repo.FindProductByCode(ctx, code)
		if err == nil {
			// Product exists
			result.Existing = append(result.Existing, rowToProductOutputFromFindByCode(existingProduct))
			processed++
			if onProgress != nil {
				onProgress(ProgressEvent{
					Type:    ProgressEventSuccess,
					Code:    code,
					Index:   processed,
					Total:   total,
					Message: "produto ja existe",
				})
			}
			continue
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			result.Failed = append(result.Failed, FailedProduct{
				Code:   code,
				Reason: "erro ao verificar produto",
			})
			processed++
			if onProgress != nil {
				onProgress(ProgressEvent{
					Type:    ProgressEventError,
					Code:    code,
					Index:   processed,
					Total:   total,
					Message: "erro ao verificar produto",
				})
			}
			continue
		}

		// Produto precisa de crawl
		toCrawl = append(toCrawl, pendingCrawl{
			code: code,
			url:  fmt.Sprintf("%s/%s", s.baseURL, code),
		})
	}

	// Fase 2: Se há produtos para crawl, submeter em batch
	if len(toCrawl) > 0 {
		// Preparar jobs
		jobs := make([]CrawlerJob, len(toCrawl))
		for i, p := range toCrawl {
			jobs[i] = CrawlerJob{
				ProductCode: p.code,
				ProductURL:  p.url,
			}
		}

		// Submeter batch
		resultsChan, err := s.workerPool.SubmitBatch(ctx, jobs)
		if err != nil {
			s.logger.Error("failed to submit batch", zap.Error(err))
			// Marcar todos como falha
			for _, p := range toCrawl {
				result.Failed = append(result.Failed, FailedProduct{
					Code:   p.code,
					Reason: "falha ao submeter job",
				})
				processed++
				if onProgress != nil {
					onProgress(ProgressEvent{
						Type:    ProgressEventError,
						Code:    p.code,
						Index:   processed,
						Total:   total,
						Message: "falha ao submeter job",
					})
				}
			}
		} else {
			// Fase 3: Processar resultados conforme chegam
			for crawlResult := range resultsChan {
				code := crawlResult.Job.ProductCode
				productURL := crawlResult.Job.ProductURL

				if crawlResult.Error != nil {
					s.logger.Warn("failed to crawl product",
						zap.String("code", code),
						zap.Error(crawlResult.Error),
					)
					result.Failed = append(result.Failed, FailedProduct{
						Code:   code,
						Reason: "falha ao coletar dados do produto",
					})
					processed++
					if onProgress != nil {
						onProgress(ProgressEvent{
							Type:    ProgressEventError,
							Code:    code,
							Index:   processed,
							Total:   total,
							Message: "falha ao coletar dados do produto",
						})
					}
					continue
				}

				crawledData := crawlResult.Data

				// Create product in database
				newProduct, err := s.repo.CreateProduct(ctx, repo.CreateProductParams{
					Code:        code,
					Url:         productURL,
					AreaID:      input.AreaID,
					Description: toPgText(crawledData.Description),
				})

				if err != nil {
					if pgErr, ok := err.(*pgconn.PgError); ok {
						if pgErr.Code == "23505" { // Unique violation - race condition
							existingProduct, fetchErr := s.repo.FindProductByCode(ctx, code)
							if fetchErr == nil {
								result.Existing = append(result.Existing, rowToProductOutputFromFindByCode(existingProduct))
								processed++
								if onProgress != nil {
									onProgress(ProgressEvent{
										Type:    ProgressEventSuccess,
										Code:    code,
										Index:   processed,
										Total:   total,
										Message: "produto ja existe",
									})
								}
								continue
							}
						}
					}
					result.Failed = append(result.Failed, FailedProduct{
						Code:   code,
						Reason: "falha ao criar produto",
					})
					processed++
					if onProgress != nil {
						onProgress(ProgressEvent{
							Type:    ProgressEventError,
							Code:    code,
							Index:   processed,
							Total:   total,
							Message: "falha ao criar produto",
						})
					}
					continue
				}

				// Create initial snapshot with crawled data
				var status pgtype.Text
				if crawledData.Status != "" {
					status = pgtype.Text{String: crawledData.Status, Valid: true}
				}

				var rawHTML pgtype.Text
				if crawledData.RawHTML != "" {
					rawHTML = pgtype.Text{String: crawledData.RawHTML, Valid: true}
				}

				_, snapshotErr := s.repo.CreateSnapshot(ctx, repo.CreateSnapshotParams{
					ProductID:   newProduct.ID,
					Description: crawledData.Description,
					Status:      status,
					RawHtml:     rawHTML,
				})

				if snapshotErr != nil {
					s.logger.Warn("failed to create snapshot",
						zap.String("code", code),
						zap.Error(snapshotErr),
					)
				}

				// Update product lifecycle status and replacement URL if applicable
				if crawledData.Status != "" || crawledData.ReplacementCode != "" {
					var lifecycleStatus pgtype.Text
					if crawledData.Status != "" {
						lifecycleStatus = pgtype.Text{String: crawledData.Status, Valid: true}
					}

					var replacementURL pgtype.Text
					if crawledData.ReplacementCode != "" {
						replacementURL = pgtype.Text{String: crawledData.ReplacementCode, Valid: true}
					}

					err = s.repo.UpdateProductLifecycleStatus(ctx, repo.UpdateProductLifecycleStatusParams{
						Code:            code,
						LifecycleStatus: lifecycleStatus,
						ReplacementUrl:  replacementURL,
					})
					if err != nil {
						s.logger.Warn("failed to update product lifecycle status",
							zap.String("code", code),
							zap.Error(err),
						)
					}
				}

				result.Added = append(result.Added, *toProductOutputFromModel(newProduct))
				processed++
				if onProgress != nil {
					onProgress(ProgressEvent{
						Type:    ProgressEventSuccess,
						Code:    code,
						Index:   processed,
						Total:   total,
						Message: "produto adicionado com sucesso",
					})
				}
			}
		}
	}

	// Send complete event
	if onProgress != nil {
		onProgress(ProgressEvent{
			Type:   ProgressEventComplete,
			Index:  total,
			Total:  total,
			Result: result,
		})
	}

	return result, nil
}

func (s *svc) ImportFromSpreadsheet(ctx context.Context, file io.Reader, areaID pgtype.UUID) (*ImportResult, *rest.ApiErr) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		s.logger.Error("failed to open spreadsheet", zap.Error(err))
		return nil, rest.NewBadRequestError("erro ao abrir planilha: " + err.Error())
	}
	defer f.Close()

	// Get active sheet name
	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		return nil, rest.NewBadRequestError("planilha vazia")
	}

	result := &ImportResult{
		Created:  0,
		Updated:  0,
		Failed:   0,
		Errors:   make([]ImportError, 0),
		Products: make([]ProductOutput, 0),
	}

	// Cache for area lookups to avoid repeated queries
	areaCache := make(map[string]pgtype.UUID)

	// Start from row 3 (1-indexed in excelize)
	// Columns: B=Área, C=Descrição, D=Código Fabricante, E=Qtd, F=Código SAP, G=Obs, H=MIN, I=MAX, J=STATUS
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, rest.NewBadRequestError("erro ao ler linhas da planilha")
	}

	for rowIdx := 2; rowIdx < len(rows); rowIdx++ { // Start from index 2 (row 3)
		row := rows[rowIdx]
		rowNum := rowIdx + 1 // Human-readable row number

		// Skip empty rows
		if len(row) < 2 {
			continue
		}

		// Get values from columns (B=1, C=2, D=3, E=4, F=5, G=6, H=7, I=8, J=9)
		getValue := func(col int) string {
			if col < len(row) {
				return strings.TrimSpace(row[col])
			}
			return ""
		}

		areaName := getValue(1)         // B
		description := getValue(2)      // C
		manufacturerCode := getValue(3) // D - main product code
		qtyStr := getValue(4)           // E
		sapCode := getValue(5)          // F
		observations := getValue(6)     // G
		minStr := getValue(7)           // H
		maxStr := getValue(8)           // I
		status := getValue(9)           // J

		// Skip rows without manufacturer code (main identifier)
		if manufacturerCode == "" {
			continue
		}

		// Parse quantity values
		qty, _ := strconv.Atoi(qtyStr)
		minQty, _ := strconv.Atoi(minStr)
		maxQty, _ := strconv.Atoi(maxStr)

		// Resolve area ID
		var productAreaID pgtype.UUID
		if areaID.Valid {
			// Use the area_id provided in the form
			productAreaID = areaID
		} else if areaName != "" {
			// Try to find area by name from cache or database
			if cachedAreaID, ok := areaCache[areaName]; ok {
				productAreaID = cachedAreaID
			} else {
				area, err := s.repo.FindAreaByName(ctx, areaName)
				if err == nil {
					productAreaID = area.ID
					areaCache[areaName] = area.ID
				} else if !errors.Is(err, pgx.ErrNoRows) {
					s.logger.Warn("failed to find area by name",
						zap.String("area", areaName),
						zap.Error(err),
					)
				}
			}
		}

		// Check if product already exists (by code AND area)
		existingProduct, err := s.repo.FindProductByCodeAndArea(ctx, repo.FindProductByCodeAndAreaParams{
			Code:   manufacturerCode,
			AreaID: productAreaID,
		})
		if err == nil {
			// Product exists in this area - update it
			updateParams := repo.UpdateProductParams{
				ID:               existingProduct.ID,
				Description:      toPgText(description),
				ManufacturerCode: toPgText(manufacturerCode),
				Quantity:         pgtype.Int4{Int32: int32(qty), Valid: true},
				SapCode:          toPgText(sapCode),
				Observations:     toPgText(observations),
				MinQuantity:      pgtype.Int4{Int32: int32(minQty), Valid: true},
				MaxQuantity:      pgtype.Int4{Int32: int32(maxQty), Valid: true},
				InventoryStatus:  toPgText(status),
			}

			updatedProduct, err := s.repo.UpdateProduct(ctx, updateParams)
			if err != nil {
				s.logger.Warn("failed to update product from spreadsheet",
					zap.String("code", manufacturerCode),
					zap.Int("row", rowNum),
					zap.Error(err),
				)
				result.Failed++
				result.Errors = append(result.Errors, ImportError{
					Row:    rowNum,
					Code:   manufacturerCode,
					Reason: "erro ao atualizar produto",
				})
				continue
			}

			result.Updated++
			result.Products = append(result.Products, *toProductOutputFromModel(updatedProduct))
		} else if errors.Is(err, pgx.ErrNoRows) {
			// Product doesn't exist - create it
			productURL := fmt.Sprintf("%s/%s", s.baseURL, manufacturerCode)

			newProduct, err := s.repo.CreateProduct(ctx, repo.CreateProductParams{
				Code:             manufacturerCode,
				Url:              productURL,
				AreaID:           productAreaID,
				Description:      toPgText(description),
				ManufacturerCode: toPgText(manufacturerCode),
				Quantity:         toPgInt4(qty),
				SapCode:          toPgText(sapCode),
				Observations:     toPgText(observations),
				MinQuantity:      toPgInt4(minQty),
				MaxQuantity:      toPgInt4(maxQty),
				InventoryStatus:  toPgText(status),
			})

			if err != nil {
				s.logger.Warn("failed to create product from spreadsheet",
					zap.String("code", manufacturerCode),
					zap.Int("row", rowNum),
					zap.Error(err),
				)
				result.Failed++
				result.Errors = append(result.Errors, ImportError{
					Row:    rowNum,
					Code:   manufacturerCode,
					Reason: "erro ao criar produto",
				})
				continue
			}

			result.Created++
			result.Products = append(result.Products, *toProductOutput(newProduct))
		} else {
			// Other database error
			s.logger.Warn("failed to check product existence",
				zap.String("code", manufacturerCode),
				zap.Int("row", rowNum),
				zap.Error(err),
			)
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:    rowNum,
				Code:   manufacturerCode,
				Reason: "erro ao verificar produto",
			})
		}
	}

	return result, nil
}

func (s *svc) ImportFromSpreadsheetWithProgress(ctx context.Context, file io.Reader, areaID pgtype.UUID, onProgress ImportProgressCallback) (*ImportResult, *rest.ApiErr) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		s.logger.Error("failed to open spreadsheet", zap.Error(err))
		return nil, rest.NewBadRequestError("erro ao abrir planilha: " + err.Error())
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		return nil, rest.NewBadRequestError("planilha vazia")
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, rest.NewBadRequestError("erro ao ler linhas da planilha")
	}

	result := &ImportResult{
		Created:  0,
		Updated:  0,
		Failed:   0,
		Errors:   make([]ImportError, 0),
		Products: make([]ProductOutput, 0),
	}

	// Count valid rows (rows with manufacturer code starting from row 4)
	validRows := make([]int, 0)
	for rowIdx := 3; rowIdx < len(rows); rowIdx++ {
		row := rows[rowIdx]
		if len(row) > 3 && strings.TrimSpace(row[3]) != "" {
			validRows = append(validRows, rowIdx)
		}
	}

	totalImport := len(validRows)
	if totalImport == 0 {
		return result, nil
	}

	// Send import start event
	if onProgress != nil {
		onProgress(ImportProgressEvent{
			Type:  ImportEventImportStart,
			Phase: "import",
			Index: 0,
			Total: totalImport,
		})
	}

	areaCache := make(map[string]pgtype.UUID)
	newProductCodes := make([]string, 0)          // Codes that need crawling
	newProductIDs := make(map[string]pgtype.UUID) // Map code to product ID

	for i, rowIdx := range validRows {
		row := rows[rowIdx]
		rowNum := rowIdx + 1

		getValue := func(col int) string {
			if col < len(row) {
				return strings.TrimSpace(row[col])
			}
			return ""
		}
		
		areaName := getValue(1)
		description := getValue(2)
		manufacturerCode := getValue(3)
		qtyStr := getValue(4)
		sapCode := getValue(5)
		observations := getValue(6)
		minStr := getValue(7)
		maxStr := getValue(8)
		status := getValue(9)

		// Send processing event
		if onProgress != nil {
			onProgress(ImportProgressEvent{
				Type:  ImportEventImportProcessing,
				Phase: "import",
				Code:  manufacturerCode,
				Row:   rowNum,
				Index: i,
				Total: totalImport,
			})
		}

		qty, _ := strconv.Atoi(qtyStr)
		minQty, _ := strconv.Atoi(minStr)
		maxQty, _ := strconv.Atoi(maxStr)

		var productAreaID pgtype.UUID
		if areaID.Valid {
			productAreaID = areaID
		} else if areaName != "" {
			if cachedAreaID, ok := areaCache[areaName]; ok {
				productAreaID = cachedAreaID
			} else {
				area, err := s.repo.FindAreaByName(ctx, areaName)
				if err == nil {
					productAreaID = area.ID
					areaCache[areaName] = area.ID
				} else if !errors.Is(err, pgx.ErrNoRows) {
					s.logger.Warn("failed to find area by name",
						zap.String("area", areaName),
						zap.Error(err),
					)
				}
			}
		}

		existingProduct, err := s.repo.FindProductByCodeAndArea(ctx, repo.FindProductByCodeAndAreaParams{
			Code:   manufacturerCode,
			AreaID: productAreaID,
		})

		if err == nil {
			// Product exists - update it
			updateParams := repo.UpdateProductParams{
				ID:               existingProduct.ID,
				Description:      toPgText(description),
				ManufacturerCode: toPgText(manufacturerCode),
				Quantity:         pgtype.Int4{Int32: int32(qty), Valid: true},
				SapCode:          toPgText(sapCode),
				Observations:     toPgText(observations),
				MinQuantity:      pgtype.Int4{Int32: int32(minQty), Valid: true},
				MaxQuantity:      pgtype.Int4{Int32: int32(maxQty), Valid: true},
				InventoryStatus:  toPgText(status),
			}

			updatedProduct, err := s.repo.UpdateProduct(ctx, updateParams)
			if err != nil {
				s.logger.Warn("failed to update product from spreadsheet",
					zap.String("code", manufacturerCode),
					zap.Int("row", rowNum),
					zap.Error(err),
				)
				result.Failed++
				result.Errors = append(result.Errors, ImportError{
					Row:    rowNum,
					Code:   manufacturerCode,
					Reason: "erro ao atualizar produto",
				})
				if onProgress != nil {
					onProgress(ImportProgressEvent{
						Type:    ImportEventImportError,
						Phase:   "import",
						Code:    manufacturerCode,
						Row:     rowNum,
						Index:   i,
						Total:   totalImport,
						Message: "erro ao atualizar produto",
					})
				}
				continue
			}

			result.Updated++
			result.Products = append(result.Products, *toProductOutputFromModel(updatedProduct))
			if onProgress != nil {
				onProgress(ImportProgressEvent{
					Type:    ImportEventImportSuccess,
					Phase:   "import",
					Code:    manufacturerCode,
					Row:     rowNum,
					Index:   i,
					Total:   totalImport,
					Message: "produto atualizado",
				})
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			// Product doesn't exist - create it
			productURL := fmt.Sprintf("%s/%s", s.baseURL, manufacturerCode)

			newProduct, err := s.repo.CreateProduct(ctx, repo.CreateProductParams{
				Code:             manufacturerCode,
				Url:              productURL,
				AreaID:           productAreaID,
				Description:      toPgText(description),
				ManufacturerCode: toPgText(manufacturerCode),
				Quantity:         toPgInt4(qty),
				SapCode:          toPgText(sapCode),
				Observations:     toPgText(observations),
				MinQuantity:      toPgInt4(minQty),
				MaxQuantity:      toPgInt4(maxQty),
				InventoryStatus:  toPgText(status),
			})

			if err != nil {
				s.logger.Warn("failed to create product from spreadsheet",
					zap.String("code", manufacturerCode),
					zap.Int("row", rowNum),
					zap.Error(err),
				)
				result.Failed++
				result.Errors = append(result.Errors, ImportError{
					Row:    rowNum,
					Code:   manufacturerCode,
					Reason: "erro ao criar produto",
				})
				if onProgress != nil {
					onProgress(ImportProgressEvent{
						Type:    ImportEventImportError,
						Phase:   "import",
						Code:    manufacturerCode,
						Row:     rowNum,
						Index:   i,
						Total:   totalImport,
						Message: "erro ao criar produto",
					})
				}
				continue
			}

			result.Created++
			result.Products = append(result.Products, *toProductOutput(newProduct))

			// Track new products for crawling (only unique codes)
			if _, exists := newProductIDs[manufacturerCode]; !exists {
				newProductCodes = append(newProductCodes, manufacturerCode)
				newProductIDs[manufacturerCode] = newProduct.ID
			}

			if onProgress != nil {
				onProgress(ImportProgressEvent{
					Type:    ImportEventImportSuccess,
					Phase:   "import",
					Code:    manufacturerCode,
					Row:     rowNum,
					Index:   i,
					Total:   totalImport,
					Message: "produto criado",
				})
			}
		} else {
			s.logger.Warn("failed to check product existence",
				zap.String("code", manufacturerCode),
				zap.Int("row", rowNum),
				zap.Error(err),
			)
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:    rowNum,
				Code:   manufacturerCode,
				Reason: "erro ao verificar produto",
			})
			if onProgress != nil {
				onProgress(ImportProgressEvent{
					Type:    ImportEventImportError,
					Phase:   "import",
					Code:    manufacturerCode,
					Row:     rowNum,
					Index:   i,
					Total:   totalImport,
					Message: "erro ao verificar produto",
				})
			}
		}
	}

	// Phase 2: Crawl new products (em paralelo)
	totalCrawl := len(newProductCodes)
	if totalCrawl > 0 {
		if onProgress != nil {
			onProgress(ImportProgressEvent{
				Type:  ImportEventCrawlStart,
				Phase: "crawl",
				Index: 0,
				Total: totalCrawl,
			})
		}

		// Preparar jobs
		jobs := make([]CrawlerJob, len(newProductCodes))
		for i, code := range newProductCodes {
			jobs[i] = CrawlerJob{
				ProductID:   newProductIDs[code],
				ProductCode: code,
				ProductURL:  fmt.Sprintf("%s/%s", s.baseURL, code),
			}
		}

		// Submeter batch
		resultsChan, err := s.workerPool.SubmitBatch(ctx, jobs)
		if err != nil {
			s.logger.Error("failed to submit crawl batch", zap.Error(err))
			// Notificar erro para todos
			for i, code := range newProductCodes {
				if onProgress != nil {
					onProgress(ImportProgressEvent{
						Type:    ImportEventCrawlError,
						Phase:   "crawl",
						Code:    code,
						Index:   i,
						Total:   totalCrawl,
						Message: "falha ao submeter job",
					})
				}
			}
		} else {
			// Processar resultados conforme chegam
			crawlProcessed := 0
			for crawlResult := range resultsChan {
				code := crawlResult.Job.ProductCode
				productID := crawlResult.Job.ProductID

				if crawlResult.Error != nil {
					s.logger.Warn("failed to crawl product",
						zap.String("code", code),
						zap.Error(crawlResult.Error),
					)
					crawlProcessed++
					if onProgress != nil {
						onProgress(ImportProgressEvent{
							Type:    ImportEventCrawlError,
							Phase:   "crawl",
							Code:    code,
							Index:   crawlProcessed,
							Total:   totalCrawl,
							Message: "falha ao coletar dados",
						})
					}
					continue
				}

				crawledData := crawlResult.Data

				// Create snapshot
				var status pgtype.Text
				if crawledData.Status != "" {
					status = pgtype.Text{String: crawledData.Status, Valid: true}
				}

				_, snapshotErr := s.repo.CreateSnapshot(ctx, repo.CreateSnapshotParams{
					ProductID:   productID,
					Description: crawledData.Description,
					Status:      status,
				})

				if snapshotErr != nil {
					s.logger.Warn("failed to create snapshot",
						zap.String("code", code),
						zap.Error(snapshotErr),
					)
				}

				// Update product lifecycle status and replacement URL if applicable
				if crawledData.Status != "" || crawledData.ReplacementCode != "" {
					var lifecycleStatus pgtype.Text
					if crawledData.Status != "" {
						lifecycleStatus = pgtype.Text{String: crawledData.Status, Valid: true}
					}

					var replacementURL pgtype.Text
					if crawledData.ReplacementCode != "" {
						replacementURL = pgtype.Text{String: crawledData.ReplacementCode, Valid: true}
					}

					err := s.repo.UpdateProductLifecycleStatus(ctx, repo.UpdateProductLifecycleStatusParams{
						Code:            code,
						LifecycleStatus: lifecycleStatus,
						ReplacementUrl:  replacementURL,
					})
					if err != nil {
						s.logger.Warn("failed to update product lifecycle status",
							zap.String("code", code),
							zap.Error(err),
						)
					}
				}

				crawlProcessed++
				if onProgress != nil {
					onProgress(ImportProgressEvent{
						Type:    ImportEventCrawlSuccess,
						Phase:   "crawl",
						Code:    code,
						Index:   crawlProcessed,
						Total:   totalCrawl,
						Message: "dados coletados",
					})
				}
			}
		}
	}

	// Send complete event
	if onProgress != nil {
		onProgress(ImportProgressEvent{
			Type:         ImportEventComplete,
			Phase:        "complete",
			Index:        totalImport,
			Total:        totalImport,
			ImportResult: result,
		})
	}

	return result, nil
}

func (s *svc) ExportToSpreadsheet(ctx context.Context) (*bytes.Buffer, *rest.ApiErr) {
	products, err := s.repo.ListProducts(ctx)
	if err != nil {
		s.logger.Error("failed to list products for export", zap.Error(err))
		return nil, rest.NewInternalServerError("erro ao buscar produtos para exportacao")
	}

	f := excelize.NewFile()
	sheetName := "Sheet1"

	// Header style: green background, white bold text, thin borders
	headerStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:  true,
			Color: "#FFFFFF",
			Size:  11,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#548235"},
			Pattern: 1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "#000000", Style: 1},
			{Type: "top", Color: "#000000", Style: 1},
			{Type: "bottom", Color: "#000000", Style: 1},
			{Type: "right", Color: "#000000", Style: 1},
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})

	// Data style: thin borders
	dataStyleID, _ := f.NewStyle(&excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Color: "#000000", Style: 1},
			{Type: "top", Color: "#000000", Style: 1},
			{Type: "bottom", Color: "#000000", Style: 1},
			{Type: "right", Color: "#000000", Style: 1},
		},
		Alignment: &excelize.Alignment{
			Vertical: "center",
		},
	})

	// Header row matching the import layout (columns B-J)
	type header struct {
		col   string
		value string
	}
	headers := []header{
		{"B", "Área"},
		{"C", "Descrição"},
		{"D", "Código Fabricante"},
		{"E", "Qtd"},
		{"F", "Código SAP"},
		{"G", "Obs."},
		{"H", "MIN"},
		{"I", "MAX"},
		{"J", "STATUS"},
	}

	// Track max column widths for auto-sizing
	colMaxWidth := make(map[string]float64)
	for _, h := range headers {
		f.SetCellValue(sheetName, h.col+"2", h.value)
		colMaxWidth[h.col] = float64(len([]rune(h.value)))
	}
	f.SetCellStyle(sheetName, "B2", "J2", headerStyleID)

	// Data starts at row 3 (matching import's rowIdx=2)
	for i, p := range products {
		row := strconv.Itoa(i + 3)

		trackWidth := func(col, value string) {
			f.SetCellValue(sheetName, col+row, value)
			if w := float64(len([]rune(value))); w > colMaxWidth[col] {
				colMaxWidth[col] = w
			}
		}

		trackWidth("B", p.AreaName.String)
		trackWidth("C", p.Description.String)
		trackWidth("D", p.Code)
		trackWidth("F", p.SapCode.String)
		trackWidth("G", computeExportObservations(p))
		trackWidth("J", p.InventoryStatus.String)

		if p.Quantity.Valid {
			v := int(p.Quantity.Int32)
			f.SetCellValue(sheetName, "E"+row, v)
			if w := float64(len(strconv.Itoa(v))); w > colMaxWidth["E"] {
				colMaxWidth["E"] = w
			}
		}

		if p.MinQuantity.Valid {
			v := int(p.MinQuantity.Int32)
			f.SetCellValue(sheetName, "H"+row, v)
			if w := float64(len(strconv.Itoa(v))); w > colMaxWidth["H"] {
				colMaxWidth["H"] = w
			}
		}

		if p.MaxQuantity.Valid {
			v := int(p.MaxQuantity.Int32)
			f.SetCellValue(sheetName, "I"+row, v)
			if w := float64(len(strconv.Itoa(v))); w > colMaxWidth["I"] {
				colMaxWidth["I"] = w
			}
		}

		f.SetCellStyle(sheetName, "B"+row, "J"+row, dataStyleID)
	}

	// Auto-fit column widths with padding
	for col, maxW := range colMaxWidth {
		width := maxW*1.2 + 4
		if width < 8 {
			width = 8
		}
		f.SetColWidth(sheetName, col, col, width)
	}

	// Add autofilter on header row
	lastRow := len(products) + 2
	f.AutoFilter(sheetName, fmt.Sprintf("B2:J%d", lastRow), nil)

	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		s.logger.Error("failed to write spreadsheet to buffer", zap.Error(err))
		return nil, rest.NewInternalServerError("erro ao gerar planilha")
	}
	f.Close()

	return buf, nil
}

func computeExportObservations(p repo.ListProductsRow) string {
	if !p.LifecycleStatus.Valid {
		return ""
	}

	switch p.LifecycleStatus.String {
	case "Prod. Cancellation", "End Prod.Lifecycl.", "Prod. Discont.":
		return fmt.Sprintf("Produto obsoleto, substituir por %s", p.ReplacementUrl.String)
	case "Phase Out Announce":
		return "Phase out anunciado"
	default:
		return ""
	}
}

func (s *svc) handleDBError(err error) *rest.ApiErr {
	if errors.Is(err, pgx.ErrNoRows) {
		return rest.NewNotFoundError("recurso nao encontrado")
	}
	if pgErr, ok := err.(*pgconn.PgError); ok {
		return database.GetError(pgErr, pgErr.ConstraintName)
	}
	return rest.NewInternalServerError("erro interno do servidor")
}

// Helper functions

func toPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toPgInt4(i int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(i), Valid: true}
}

func toProductOutput(p repo.Product) *ProductOutput {
	return &ProductOutput{
		ID:               p.ID,
		Code:             p.Code,
		URL:              p.Url,
		AreaID:           p.AreaID,
		Description:      p.Description.String,
		ManufacturerCode: p.ManufacturerCode.String,
		Quantity:         int(p.Quantity.Int32),
		ReplacementURL:   p.ReplacementUrl.String,
		SAPCode:          p.SapCode.String,
		Observations:     p.Observations.String,
		MinQuantity:      int(p.MinQuantity.Int32),
		MaxQuantity:      int(p.MaxQuantity.Int32),
		InventoryStatus:  p.InventoryStatus.String,
		LifeCycleStatus:  p.LifecycleStatus.String,
		CreatedAt:        p.CreatedAt.Time,
	}
}

func toProductOutputFromModel(p repo.Product) *ProductOutput {
	return toProductOutput(p)
}

func rowToProductOutputFromFindByCode(row repo.FindProductByCodeRow) ProductOutput {
	return ProductOutput{
		ID:               row.ID,
		Code:             row.Code,
		URL:              row.Url,
		AreaID:           row.AreaID,
		AreaName:         row.AreaName.String,
		Description:      row.Description.String,
		ManufacturerCode: row.ManufacturerCode.String,
		Quantity:         int(row.Quantity.Int32),
		ReplacementURL:   row.ReplacementUrl.String,
		SAPCode:          row.SapCode.String,
		Observations:     row.Observations.String,
		MinQuantity:      int(row.MinQuantity.Int32),
		MaxQuantity:      int(row.MaxQuantity.Int32),
		InventoryStatus:  row.InventoryStatus.String,
		LifeCycleStatus:  row.LifecycleStatus.String,
		CreatedAt:        row.CreatedAt.Time,
	}
}

// Helper functions for unique product queries

func parseQuantityByArea(data []byte) []AreaQuantity {
	if data == nil {
		return nil
	}
	var raw []struct {
		AreaID   *string `json:"area_id"`
		AreaName *string `json:"area_name"`
		Quantity int     `json:"quantity"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	result := make([]AreaQuantity, 0, len(raw))
	for _, r := range raw {
		areaID := ""
		if r.AreaID != nil {
			areaID = *r.AreaID
		}
		areaName := ""
		if r.AreaName != nil {
			areaName = *r.AreaName
		}
		result = append(result, AreaQuantity{
			AreaID:   areaID,
			AreaName: areaName,
			Quantity: r.Quantity,
		})
	}
	return result
}

func interfaceToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func interfaceToTime(v interface{}) time.Time {
	if v == nil {
		return time.Time{}
	}
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Time{}
}

func listUniqueProductRowToOutput(row repo.ListUniqueProductsPaginatedRow) ProductWithSnapshotOutput {
	return ProductWithSnapshotOutput{
		Product: ProductOutput{
			Code:             row.Code,
			Description:      interfaceToString(row.Description),
			LifeCycleStatus:  interfaceToString(row.LifecycleStatus),
			ReplacementURL:   interfaceToString(row.ReplacementUrl),
			URL:              interfaceToString(row.Url),
			ManufacturerCode: interfaceToString(row.ManufacturerCode),
			SAPCode:          interfaceToString(row.SapCode),
			TotalQuantity:    int(row.TotalQuantity),
			CreatedAt:        interfaceToTime(row.CreatedAt),
			QuantityByArea:   parseQuantityByArea(row.QuantityByArea),
		},
	}
}

func searchUniqueProductRowToOutput(row repo.SearchUniqueProductsPaginatedRow) ProductWithSnapshotOutput {
	return ProductWithSnapshotOutput{
		Product: ProductOutput{
			Code:             row.Code,
			Description:      interfaceToString(row.Description),
			LifeCycleStatus:  interfaceToString(row.LifecycleStatus),
			ReplacementURL:   interfaceToString(row.ReplacementUrl),
			URL:              interfaceToString(row.Url),
			ManufacturerCode: interfaceToString(row.ManufacturerCode),
			SAPCode:          interfaceToString(row.SapCode),
			TotalQuantity:    int(row.TotalQuantity),
			CreatedAt:        interfaceToTime(row.CreatedAt),
			QuantityByArea:   parseQuantityByArea(row.QuantityByArea),
		},
	}
}

func uniqueProductByAreaRowToOutput(row repo.ListUniqueProductsByAreaPaginatedRow) ProductWithSnapshotOutput {
	return ProductWithSnapshotOutput{
		Product: ProductOutput{
			Code:             row.Code,
			Description:      interfaceToString(row.Description),
			LifeCycleStatus:  interfaceToString(row.LifecycleStatus),
			ReplacementURL:   interfaceToString(row.ReplacementUrl),
			URL:              interfaceToString(row.Url),
			ManufacturerCode: interfaceToString(row.ManufacturerCode),
			SAPCode:          interfaceToString(row.SapCode),
			TotalQuantity:    int(row.TotalQuantity),
			CreatedAt:        interfaceToTime(row.CreatedAt),
			QuantityByArea:   parseQuantityByArea(row.QuantityByArea),
		},
	}
}

func uniqueProductRowToOutput(row repo.SearchUniqueProductsByAreaPaginatedRow) ProductWithSnapshotOutput {
	return ProductWithSnapshotOutput{
		Product: ProductOutput{
			Code:             row.Code,
			Description:      interfaceToString(row.Description),
			LifeCycleStatus:  interfaceToString(row.LifecycleStatus),
			ReplacementURL:   interfaceToString(row.ReplacementUrl),
			URL:              interfaceToString(row.Url),
			ManufacturerCode: interfaceToString(row.ManufacturerCode),
			SAPCode:          interfaceToString(row.SapCode),
			TotalQuantity:    int(row.TotalQuantity),
			CreatedAt:        interfaceToTime(row.CreatedAt),
			QuantityByArea:   parseQuantityByArea(row.QuantityByArea),
		},
	}
}

// ListUniqueProductsToCollect returns all unique products (by code) for scheduled crawling
func (s *svc) ListUniqueProductsToCollect(ctx context.Context) ([]repo.ListUniqueProductCodesToCollectRow, error) {
	return s.repo.ListUniqueProductCodesToCollect(ctx)
}

// SaveCrawlResult saves the crawl result (snapshot) and updates the product lifecycle status.
// Returns a LifecycleStatusChange if the lifecycle status changed, nil otherwise.
func (s *svc) SaveCrawlResult(ctx context.Context, job CrawlerJob, data *CrawledData) (*LifecycleStatusChange, error) {
	var status pgtype.Text
	if data.Status != "" {
		status = pgtype.Text{String: data.Status, Valid: true}
	}

	var rawHTML pgtype.Text
	if data.RawHTML != "" {
		rawHTML = pgtype.Text{String: data.RawHTML, Valid: true}
	}

	// Create snapshot
	_, err := s.repo.CreateSnapshot(ctx, repo.CreateSnapshotParams{
		ProductID:   job.ProductID,
		Description: data.Description,
		Status:      status,
		RawHtml:     rawHTML,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	var statusChange *LifecycleStatusChange

	// Update product lifecycle status and replacement URL if applicable
	if data.Status != "" || data.ReplacementCode != "" {
		// Get current lifecycle status before updating
		currentProduct, err := s.repo.FindProductByCode(ctx, job.ProductCode)
		if err != nil {
			s.logger.Warn("failed to get current product status",
				zap.String("code", job.ProductCode),
				zap.Error(err),
			)
		}

		oldStatus := ""
		if err == nil {
			oldStatus = currentProduct.LifecycleStatus.String
		}

		var lifecycleStatus pgtype.Text
		if data.Status != "" {
			lifecycleStatus = pgtype.Text{String: data.Status, Valid: true}
		}

		var replacementURL pgtype.Text
		if data.ReplacementCode != "" {
			replacementURL = pgtype.Text{String: data.ReplacementCode, Valid: true}
		}

		err = s.repo.UpdateProductLifecycleStatus(ctx, repo.UpdateProductLifecycleStatusParams{
			Code:            job.ProductCode,
			LifecycleStatus: lifecycleStatus,
			ReplacementUrl:  replacementURL,
		})
		if err != nil {
			s.logger.Warn("failed to update product lifecycle status",
				zap.String("code", job.ProductCode),
				zap.Error(err),
			)
		}

		// Check if lifecycle status changed
		if data.Status != "" && oldStatus != data.Status {
			statusChange = &LifecycleStatusChange{
				ProductCode: job.ProductCode,
				OldStatus:   oldStatus,
				NewStatus:   data.Status,
			}
		}
	}

	return statusChange, nil
}
