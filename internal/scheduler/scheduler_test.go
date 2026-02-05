package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"

	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/products"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// MockProductCollector implements ProductCollector for testing
type MockProductCollector struct {
	products      []repo.ListUniqueProductCodesToCollectRow
	listErr       error
	saveCrawlErr  error
	statusChanges map[string]*products.LifecycleStatusChange // map[productCode]change
	savedResults  []savedCrawlResult
	mu            sync.Mutex
	// Optional custom save function for advanced test scenarios
	customSaveFunc func(ctx context.Context, job products.CrawlerJob, data *products.CrawledData) (*products.LifecycleStatusChange, error)
}

type savedCrawlResult struct {
	Job  products.CrawlerJob
	Data *products.CrawledData
}

func (m *MockProductCollector) ListUniqueProductsToCollect(ctx context.Context) ([]repo.ListUniqueProductCodesToCollectRow, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.products, nil
}

func (m *MockProductCollector) SaveCrawlResult(ctx context.Context, job products.CrawlerJob, data *products.CrawledData) (*products.LifecycleStatusChange, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use custom function if provided
	if m.customSaveFunc != nil {
		return m.customSaveFunc(ctx, job, data)
	}

	if m.saveCrawlErr != nil {
		return nil, m.saveCrawlErr
	}

	m.savedResults = append(m.savedResults, savedCrawlResult{Job: job, Data: data})

	if m.statusChanges != nil {
		if change, ok := m.statusChanges[job.ProductCode]; ok {
			return change, nil
		}
	}
	return nil, nil
}

// MockEmail implements email.Email for testing
type MockEmail struct {
	sentEmails []SentEmail
	sendErr    error
	mu         sync.Mutex
}

type SentEmail struct {
	Subject    string
	Text       string
	HTML       string
	Recipients []string
}

func (m *MockEmail) Send(subject, text, html string, recipients []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}

	m.sentEmails = append(m.sentEmails, SentEmail{
		Subject:    subject,
		Text:       text,
		HTML:       html,
		Recipients: recipients,
	})
	return nil
}

// MockBatchSubmitter implements BatchSubmitter for testing
type MockBatchSubmitter struct {
	results   map[string]products.WorkerResult // map[productCode]result
	submitErr error
}

func (m *MockBatchSubmitter) SubmitBatch(ctx context.Context, jobs []products.CrawlerJob) (<-chan products.WorkerResult, error) {
	if m.submitErr != nil {
		return nil, m.submitErr
	}

	ch := make(chan products.WorkerResult, len(jobs))

	go func() {
		defer close(ch)
		for _, job := range jobs {
			if result, ok := m.results[job.ProductCode]; ok {
				result.Job = job
				ch <- result
			} else {
				// Default: successful crawl with some data
				ch <- products.WorkerResult{
					Job: job,
					Data: &products.CrawledData{
						Description: "Test Product",
						Status:      "Active",
					},
				}
			}
		}
	}()

	return ch, nil
}

func TestRunLifecycleUpdateJob_WithStatusChange(t *testing.T) {
	logger := zap.NewNop()

	// Create mock products to collect
	product1ID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	product2ID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	product3ID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}

	mockCollector := &MockProductCollector{
		products: []repo.ListUniqueProductCodesToCollectRow{
			{ID: product1ID, Code: "PROD-001", Url: "http://example.com/PROD-001"},
			{ID: product2ID, Code: "PROD-002", Url: "http://example.com/PROD-002"},
			{ID: product3ID, Code: "PROD-003", Url: "http://example.com/PROD-003"},
		},
		// Configure status changes - PROD-002 will have a status change
		statusChanges: map[string]*products.LifecycleStatusChange{
			"PROD-002": {
				ProductCode: "PROD-002",
				OldStatus:   "Active",
				NewStatus:   "Discontinued",
			},
		},
	}

	mockEmail := &MockEmail{}

	// Create mock worker pool results
	mockWorkerPool := &MockBatchSubmitter{
		results: map[string]products.WorkerResult{
			"PROD-001": {
				Data: &products.CrawledData{
					Description: "Product 1",
					Status:      "Active",
				},
			},
			"PROD-002": {
				Data: &products.CrawledData{
					Description: "Product 2",
					Status:      "Discontinued", // This triggers status change
				},
			},
			"PROD-003": {
				Data: &products.CrawledData{
					Description: "Product 3",
					Status:      "Active",
				},
			},
		},
	}

	scheduler := &Scheduler{
		workerPool: mockWorkerPool,
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	// Run the job directly (not through cron)
	scheduler.runLifecycleUpdateJob()

	// Verify results
	mockCollector.mu.Lock()
	savedCount := len(mockCollector.savedResults)
	mockCollector.mu.Unlock()

	if savedCount != 3 {
		t.Errorf("expected 3 saved results, got %d", savedCount)
	}

	// Verify email was sent for status change
	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 1 {
		t.Errorf("expected 1 email sent, got %d", emailCount)
	}

	if emailCount > 0 {
		mockEmail.mu.Lock()
		email := mockEmail.sentEmails[0]
		mockEmail.mu.Unlock()

		if email.Subject != "Mudança de Lifecycle de equipamentos detectada" {
			t.Errorf("unexpected email subject: %s", email.Subject)
		}

		// Verify email contains the changed product
		if !containsString(email.Text, "PROD-002") {
			t.Errorf("email text should contain PROD-002")
		}
		if !containsString(email.Text, "Active") {
			t.Errorf("email text should contain old status 'Active'")
		}
		if !containsString(email.Text, "Discontinued") {
			t.Errorf("email text should contain new status 'Discontinued'")
		}
	}
}

func TestRunLifecycleUpdateJob_NoStatusChange(t *testing.T) {
	logger := zap.NewNop()

	product1ID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	mockCollector := &MockProductCollector{
		products: []repo.ListUniqueProductCodesToCollectRow{
			{ID: product1ID, Code: "PROD-001", Url: "http://example.com/PROD-001"},
		},
		// No status changes configured - SaveCrawlResult will return nil
		statusChanges: nil,
	}

	mockEmail := &MockEmail{}

	mockWorkerPool := &MockBatchSubmitter{
		results: map[string]products.WorkerResult{
			"PROD-001": {
				Data: &products.CrawledData{
					Description: "Product 1",
					Status:      "Active",
				},
			},
		},
	}

	scheduler := &Scheduler{
		workerPool: mockWorkerPool,
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	scheduler.runLifecycleUpdateJob()

	// Verify no email was sent when there are no status changes
	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 0 {
		t.Errorf("expected 0 emails sent when no status changes, got %d", emailCount)
	}
}

func TestRunLifecycleUpdateJob_MultipleStatusChanges(t *testing.T) {
	logger := zap.NewNop()

	product1ID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	product2ID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	mockCollector := &MockProductCollector{
		products: []repo.ListUniqueProductCodesToCollectRow{
			{ID: product1ID, Code: "PROD-001", Url: "http://example.com/PROD-001"},
			{ID: product2ID, Code: "PROD-002", Url: "http://example.com/PROD-002"},
		},
		statusChanges: map[string]*products.LifecycleStatusChange{
			"PROD-001": {
				ProductCode: "PROD-001",
				OldStatus:   "Active",
				NewStatus:   "Phase Out",
			},
			"PROD-002": {
				ProductCode: "PROD-002",
				OldStatus:   "Phase Out",
				NewStatus:   "Discontinued",
			},
		},
	}

	mockEmail := &MockEmail{}

	mockWorkerPool := &MockBatchSubmitter{
		results: map[string]products.WorkerResult{
			"PROD-001": {
				Data: &products.CrawledData{
					Description: "Product 1",
					Status:      "Phase Out",
				},
			},
			"PROD-002": {
				Data: &products.CrawledData{
					Description: "Product 2",
					Status:      "Discontinued",
				},
			},
		},
	}

	scheduler := &Scheduler{
		workerPool: mockWorkerPool,
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	scheduler.runLifecycleUpdateJob()

	// Verify email was sent with both status changes
	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	var email SentEmail
	if emailCount > 0 {
		email = mockEmail.sentEmails[0]
	}
	mockEmail.mu.Unlock()

	if emailCount != 1 {
		t.Errorf("expected 1 email sent for multiple changes, got %d", emailCount)
	}

	// Verify both products are mentioned in the email
	if !containsString(email.Text, "PROD-001") {
		t.Errorf("email should contain PROD-001")
	}
	if !containsString(email.Text, "PROD-002") {
		t.Errorf("email should contain PROD-002")
	}
}

func TestRunLifecycleUpdateJob_CrawlError(t *testing.T) {
	logger := zap.NewNop()

	product1ID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	product2ID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	mockCollector := &MockProductCollector{
		products: []repo.ListUniqueProductCodesToCollectRow{
			{ID: product1ID, Code: "PROD-001", Url: "http://example.com/PROD-001"},
			{ID: product2ID, Code: "PROD-002", Url: "http://example.com/PROD-002"},
		},
		statusChanges: map[string]*products.LifecycleStatusChange{
			"PROD-002": {
				ProductCode: "PROD-002",
				OldStatus:   "Active",
				NewStatus:   "Discontinued",
			},
		},
	}

	mockEmail := &MockEmail{}

	// PROD-001 will fail during crawl
	mockWorkerPool := &MockBatchSubmitter{
		results: map[string]products.WorkerResult{
			"PROD-001": {
				Error: context.DeadlineExceeded,
			},
			"PROD-002": {
				Data: &products.CrawledData{
					Description: "Product 2",
					Status:      "Discontinued",
				},
			},
		},
	}

	scheduler := &Scheduler{
		workerPool: mockWorkerPool,
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	scheduler.runLifecycleUpdateJob()

	// Verify only successful crawl was saved
	mockCollector.mu.Lock()
	savedCount := len(mockCollector.savedResults)
	mockCollector.mu.Unlock()

	if savedCount != 1 {
		t.Errorf("expected 1 saved result (only successful crawl), got %d", savedCount)
	}

	// Verify email was still sent for the successful product
	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 1 {
		t.Errorf("expected 1 email sent, got %d", emailCount)
	}
}

func TestRunLifecycleUpdateJob_NoProducts(t *testing.T) {
	logger := zap.NewNop()

	mockCollector := &MockProductCollector{
		products: []repo.ListUniqueProductCodesToCollectRow{},
	}

	mockEmail := &MockEmail{}

	scheduler := &Scheduler{
		workerPool: &MockBatchSubmitter{},
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	scheduler.runLifecycleUpdateJob()

	// Verify no email sent when no products
	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 0 {
		t.Errorf("expected 0 emails when no products, got %d", emailCount)
	}
}

func TestRunLifecycleUpdateJob_ListProductsError(t *testing.T) {
	logger := zap.NewNop()

	mockCollector := &MockProductCollector{
		listErr: errors.New("database connection failed"),
	}

	mockEmail := &MockEmail{}

	scheduler := &Scheduler{
		workerPool: &MockBatchSubmitter{},
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	// Should not panic and should not send email
	scheduler.runLifecycleUpdateJob()

	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 0 {
		t.Errorf("expected 0 emails when list products fails, got %d", emailCount)
	}
}

func TestRunLifecycleUpdateJob_SubmitBatchError(t *testing.T) {
	logger := zap.NewNop()

	product1ID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	mockCollector := &MockProductCollector{
		products: []repo.ListUniqueProductCodesToCollectRow{
			{ID: product1ID, Code: "PROD-001", Url: "http://example.com/PROD-001"},
		},
	}

	mockEmail := &MockEmail{}

	mockWorkerPool := &MockBatchSubmitter{
		submitErr: errors.New("worker pool is not running"),
	}

	scheduler := &Scheduler{
		workerPool: mockWorkerPool,
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	// Should not panic and should not send email
	scheduler.runLifecycleUpdateJob()

	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 0 {
		t.Errorf("expected 0 emails when submit batch fails, got %d", emailCount)
	}
}

func TestRunLifecycleUpdateJob_SaveCrawlResultError(t *testing.T) {
	logger := zap.NewNop()

	product1ID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	product2ID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	// Create a collector that fails on PROD-001 but succeeds on PROD-002
	mockCollector := &MockProductCollector{
		products: []repo.ListUniqueProductCodesToCollectRow{
			{ID: product1ID, Code: "PROD-001", Url: "http://example.com/PROD-001"},
			{ID: product2ID, Code: "PROD-002", Url: "http://example.com/PROD-002"},
		},
		customSaveFunc: func(ctx context.Context, job products.CrawlerJob, data *products.CrawledData) (*products.LifecycleStatusChange, error) {
			if job.ProductCode == "PROD-001" {
				return nil, errors.New("database error")
			}
			// PROD-002 succeeds with status change
			return &products.LifecycleStatusChange{
				ProductCode: "PROD-002",
				OldStatus:   "Active",
				NewStatus:   "Discontinued",
			}, nil
		},
	}

	mockEmail := &MockEmail{}

	mockWorkerPool := &MockBatchSubmitter{
		results: map[string]products.WorkerResult{
			"PROD-001": {
				Data: &products.CrawledData{
					Description: "Product 1",
					Status:      "Active",
				},
			},
			"PROD-002": {
				Data: &products.CrawledData{
					Description: "Product 2",
					Status:      "Discontinued",
				},
			},
		},
	}

	scheduler := &Scheduler{
		workerPool: mockWorkerPool,
		service:    mockCollector,
		logger:     logger,
		email:      mockEmail,
	}

	scheduler.runLifecycleUpdateJob()

	// Email should still be sent for the successful save
	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 1 {
		t.Errorf("expected 1 email sent for successful save, got %d", emailCount)
	}
}

func TestSendStatusChangeEmail_HTMLContent(t *testing.T) {
	logger := zap.NewNop()
	mockEmail := &MockEmail{}

	scheduler := &Scheduler{
		logger: logger,
		email:  mockEmail,
	}

	changes := []products.LifecycleStatusChange{
		{ProductCode: "PROD-001", OldStatus: "Active", NewStatus: "Phase Out"},
		{ProductCode: "PROD-002", OldStatus: "Phase Out", NewStatus: "Discontinued"},
	}

	scheduler.sendStatusChangeEmail(changes)

	mockEmail.mu.Lock()
	defer mockEmail.mu.Unlock()

	if len(mockEmail.sentEmails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mockEmail.sentEmails))
	}

	email := mockEmail.sentEmails[0]

	// Verify HTML contains table structure
	if !containsString(email.HTML, "<table>") {
		t.Error("HTML should contain table element")
	}
	if !containsString(email.HTML, "PROD-001") {
		t.Error("HTML should contain PROD-001")
	}
	if !containsString(email.HTML, "PROD-002") {
		t.Error("HTML should contain PROD-002")
	}

	// Verify text version
	if !containsString(email.Text, "Product: PROD-001") {
		t.Error("Text should contain 'Product: PROD-001'")
	}
}

func TestSendStatusChangeEmail_EmptyChanges(t *testing.T) {
	logger := zap.NewNop()
	mockEmail := &MockEmail{}

	scheduler := &Scheduler{
		logger: logger,
		email:  mockEmail,
	}

	// Should not send email for empty changes
	scheduler.sendStatusChangeEmail([]products.LifecycleStatusChange{})

	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 0 {
		t.Errorf("expected 0 emails for empty changes, got %d", emailCount)
	}
}

func TestSendStatusChangeEmail_EmailError(t *testing.T) {
	logger := zap.NewNop()
	mockEmail := &MockEmail{
		sendErr: errors.New("SMTP connection failed"),
	}

	scheduler := &Scheduler{
		logger: logger,
		email:  mockEmail,
	}

	changes := []products.LifecycleStatusChange{
		{ProductCode: "PROD-001", OldStatus: "Active", NewStatus: "Discontinued"},
	}

	// Should not panic even if email fails
	scheduler.sendStatusChangeEmail(changes)

	// Email was attempted but failed
	mockEmail.mu.Lock()
	emailCount := len(mockEmail.sentEmails)
	mockEmail.mu.Unlock()

	if emailCount != 0 {
		t.Errorf("expected 0 successful emails when send fails, got %d", emailCount)
	}
}

// containsString checks if s contains substr
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
