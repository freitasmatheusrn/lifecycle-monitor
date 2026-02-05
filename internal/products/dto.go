package products

import (
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Input DTOs

type CreateProductInput struct {
	Code             string      `json:"code" form:"code" validate:"required"`
	AreaID           pgtype.UUID `json:"area_id" form:"area_id"`
	Description      string      `json:"description" form:"description"`
	ManufacturerCode string      `json:"manufacturer_code" form:"manufacturer_code"`
	Quantity         int         `json:"quantity" form:"quantity"`
	SAPCode          string      `json:"sap_code" form:"sap_code"`
	Observations     string      `json:"observations" form:"observations"`
	MinQuantity      int         `json:"min_quantity" form:"min_quantity"`
	MaxQuantity      int         `json:"max_quantity" form:"max_quantity"`
	InventoryStatus  string      `json:"inventory_status" form:"inventory_status"`
}

type UpdateProductInput struct {
	Code             *string      `json:"code" form:"code"`
	AreaID           *pgtype.UUID `json:"area_id" form:"area_id"`
	Description      *string      `json:"description" form:"description"`
	ManufacturerCode *string      `json:"manufacturer_code" form:"manufacturer_code"`
	Quantity         *int         `json:"quantity" form:"quantity"`
	SAPCode          *string      `json:"sap_code" form:"sap_code"`
	Observations     *string      `json:"observations" form:"observations"`
	MinQuantity      *int         `json:"min_quantity" form:"min_quantity"`
	MaxQuantity      *int         `json:"max_quantity" form:"max_quantity"`
	InventoryStatus  *string      `json:"inventory_status" form:"inventory_status"`
}

type AddProductsInput struct {
	Codes  []string    `json:"codes" form:"codes" validate:"required,min=1"`
	AreaID pgtype.UUID `json:"area_id" form:"area_id"`
}

type ListProductsInput struct {
	Page     int         `query:"page"`
	PageSize int         `query:"page_size"`
	Search   string      `query:"search"`
	AreaID   pgtype.UUID `query:"area_id"`
}

// Output DTOs

type ProductOutput struct {
	ID               pgtype.UUID `json:"id"`
	Code             string      `json:"code"`
	URL              string      `json:"url"`
	AreaID           pgtype.UUID `json:"area_id,omitempty"`
	AreaName         string      `json:"area_name,omitempty"`
	Description      string      `json:"description,omitempty"`
	ManufacturerCode string      `json:"manufacturer_code,omitempty"`
	Quantity         int         `json:"quantity"`
	ReplacementURL   string      `json:"replacement_url,omitempty"`
	SAPCode          string      `json:"sap_code,omitempty"`
	Observations     string      `json:"observations,omitempty"`
	MinQuantity      int         `json:"min_quantity"`
	MaxQuantity      int         `json:"max_quantity"`
	InventoryStatus  string      `json:"inventory_status,omitempty"`
	LifeCycleStatus  string      `json:"lifecycle_status,omitempty"`
	CreatedAt        time.Time   `json:"created_at"`
	TotalQuantity    int         `json:"total_quantity"`
	QuantityByArea   []AreaQuantity `json:"quantity_by_area,omitempty"`
}

type AreaQuantity struct {
	AreaID   string `json:"area_id"`
	AreaName string `json:"area_name"`
	Quantity int    `json:"quantity"`
}

type SnapshotOutput struct {
	ID          pgtype.UUID `json:"id"`
	ProductID   pgtype.UUID `json:"product_id"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	CollectedAt time.Time   `json:"collected_at"`
}

type ProductWithSnapshotOutput struct {
	Product        ProductOutput   `json:"product"`
	LatestSnapshot *SnapshotOutput `json:"latest_snapshot,omitempty"`
}

type PaginatedProductsOutput struct {
	Products          []ProductWithSnapshotOutput `json:"products"`
	Total             int64                       `json:"total"`
	Page              int                         `json:"page"`
	PageSize          int                         `json:"page_size"`
	TotalPages        int                         `json:"total_pages"`
	ActiveCount       int64                       `json:"active_count"`
	DiscontinuedCount int64                       `json:"discontinued_count"`
	PhaseOutCount     int64                       `json:"phase_out_count"`
}

type AddProductsResult struct {
	Added    []ProductOutput `json:"added"`
	Existing []ProductOutput `json:"existing"`
	Failed   []FailedProduct `json:"failed,omitempty"`
}

type FailedProduct struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// Import DTOs

type ImportInput struct {
	File   io.Reader
	AreaID pgtype.UUID
}

type ImportResult struct {
	Created  int             `json:"created"`
	Updated  int             `json:"updated"`
	Failed   int             `json:"failed"`
	Errors   []ImportError   `json:"errors,omitempty"`
	Products []ProductOutput `json:"products,omitempty"`
}

type ImportError struct {
	Row    int    `json:"row"`
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason"`
}

// SSE Progress Event Types
type ProgressEventType string

const (
	ProgressEventStart      ProgressEventType = "start"
	ProgressEventProcessing ProgressEventType = "processing"
	ProgressEventSuccess    ProgressEventType = "success"
	ProgressEventError      ProgressEventType = "error"
	ProgressEventComplete   ProgressEventType = "complete"
)

// ProgressEvent represents an SSE event for product addition progress
type ProgressEvent struct {
	Type    ProgressEventType  `json:"type"`
	Code    string             `json:"code,omitempty"`
	Index   int                `json:"index"`
	Total   int                `json:"total"`
	Message string             `json:"message,omitempty"`
	Result  *AddProductsResult `json:"result,omitempty"`
}

// ProgressCallback is called for each progress update during product addition
type ProgressCallback func(event ProgressEvent)

// Import Progress Event Types
type ImportProgressEventType string

const (
	ImportEventImportStart      ImportProgressEventType = "import_start"
	ImportEventImportProcessing ImportProgressEventType = "import_processing"
	ImportEventImportSuccess    ImportProgressEventType = "import_success"
	ImportEventImportError      ImportProgressEventType = "import_error"
	ImportEventCrawlStart       ImportProgressEventType = "crawl_start"
	ImportEventCrawlProcessing  ImportProgressEventType = "crawl_processing"
	ImportEventCrawlSuccess     ImportProgressEventType = "crawl_success"
	ImportEventCrawlError       ImportProgressEventType = "crawl_error"
	ImportEventComplete         ImportProgressEventType = "complete"
)

// ImportProgressEvent represents an SSE event for import progress
type ImportProgressEvent struct {
	Type         ImportProgressEventType `json:"type"`
	Phase        string                  `json:"phase"` // "import" or "crawl"
	Code         string                  `json:"code,omitempty"`
	Row          int                     `json:"row,omitempty"`
	Index        int                     `json:"index"`
	Total        int                     `json:"total"`
	Message      string                  `json:"message,omitempty"`
	ImportResult *ImportResult           `json:"import_result,omitempty"`
}

// ImportProgressCallback is called for each progress update during import
type ImportProgressCallback func(event ImportProgressEvent)

// Crawler DTOs

type CrawlerJob struct {
	ProductID   pgtype.UUID
	ProductCode string
	ProductURL  string
	jobID       string // ID interno para jobs síncronos (SubmitAndWait)
}

type CrawledData struct {
	Description     string
	Status          string
	ReplacementCode string
	RawHTML         string
}

// LifecycleStatusChange represents a change in product lifecycle status
type LifecycleStatusChange struct {
	ProductCode string
	OldStatus   string
	NewStatus   string
}
