package scheduler

import (
	"context"
	"fmt"
	"time"

	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/email"
	"github.com/freitasmatheusrn/lifecycle-monitor/internal/products"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// ProductCollector defines the interface for fetching and saving product data
type ProductCollector interface {
	ListUniqueProductsToCollect(ctx context.Context) ([]repo.ListUniqueProductCodesToCollectRow, error)
	SaveCrawlResult(ctx context.Context, job products.CrawlerJob, data *products.CrawledData) (*products.LifecycleStatusChange, error)
}

// BatchSubmitter defines the interface for submitting batch crawl jobs
type BatchSubmitter interface {
	SubmitBatch(ctx context.Context, jobs []products.CrawlerJob) (<-chan products.WorkerResult, error)
}

type Scheduler struct {
	cron            *cron.Cron
	workerPool      BatchSubmitter
	service         ProductCollector
	logger          *zap.Logger
	email           email.Email
	alertRecipients []string
}

func NewScheduler(workerPool BatchSubmitter, service ProductCollector, logger *zap.Logger, e email.Email, alertRecipients []string) *Scheduler {
	return &Scheduler{
		cron:            cron.New(cron.WithSeconds()),
		workerPool:      workerPool,
		service:         service,
		logger:          logger,
		email:           e,
		alertRecipients: alertRecipients,
	}
}

// Start initializes the scheduler with the lifecycle update job
// cronExpr uses 6 fields: seconds, minutes, hours, day of month, month, day of week
// Example: "0 0 3 * * *" runs at 3:00 AM every day
func (s *Scheduler) Start(cronExpr string) error {
	_, err := s.cron.AddFunc(cronExpr, s.runLifecycleUpdateJob)
	if err != nil {
		return err
	}

	s.cron.Start()
	s.logger.Info("scheduler started", zap.String("cron_expression", cronExpr))

	return nil
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() context.Context {
	s.logger.Info("stopping scheduler")
	return s.cron.Stop()
}

// runLifecycleUpdateJob fetches all unique product codes and submits crawling jobs
func (s *Scheduler) runLifecycleUpdateJob() {
	s.logger.Info("starting lifecycle update job")
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Get all unique product codes to collect
	productsToCollect, err := s.service.ListUniqueProductsToCollect(ctx)
	if err != nil {
		s.notifyError("failed to list products to collect", err)
		return
	}

	if len(productsToCollect) == 0 {
		s.logger.Info("no products to collect")
		return
	}

	s.logger.Info("found products to collect",
		zap.Int("count", len(productsToCollect)),
	)

	// Create crawler jobs
	jobs := make([]products.CrawlerJob, 0, len(productsToCollect))
	for _, p := range productsToCollect {
		jobs = append(jobs, products.CrawlerJob{
			ProductID:   p.ID,
			ProductCode: p.Code,
			ProductURL:  p.Url,
		})
	}

	// Submit batch and wait for results
	resultsChan, err := s.workerPool.SubmitBatch(ctx, jobs)
	if err != nil {
		s.notifyError("failed to submit batch", err)
		return
	}

	// Process results and collect lifecycle status changes
	var successCount, errorCount int
	var statusChanges []products.LifecycleStatusChange

	for result := range resultsChan {
		if result.Error != nil {
			errorCount++
			s.logger.Warn("crawl failed",
				zap.String("code", result.Job.ProductCode),
				zap.Error(result.Error),
			)
			continue
		}

		// Save snapshot and update lifecycle status
		change, err := s.service.SaveCrawlResult(ctx, result.Job, result.Data)
		if err != nil {
			errorCount++
			s.logger.Error("failed to save crawl result",
				zap.String("code", result.Job.ProductCode),
				zap.Error(err),
			)
			continue
		}

		// Collect status change if lifecycle status changed
		if change != nil {
			statusChanges = append(statusChanges, *change)
			s.logger.Info("lifecycle status changed",
				zap.String("code", change.ProductCode),
				zap.String("old_status", change.OldStatus),
				zap.String("new_status", change.NewStatus),
			)
		}

		successCount++
		s.logger.Debug("crawl succeeded",
			zap.String("code", result.Job.ProductCode),
			zap.String("status", result.Data.Status),
		)
	}

	duration := time.Since(startTime)
	s.logger.Info("lifecycle update job completed",
		zap.Int("total", len(productsToCollect)),
		zap.Int("success", successCount),
		zap.Int("errors", errorCount),
		zap.Int("status_changes", len(statusChanges)),
		zap.Duration("duration", duration),
	)

	// Send email notification if there are lifecycle status changes
	if len(statusChanges) > 0 {
		s.sendStatusChangeEmail(statusChanges)
	}
}

// RunNow executes the lifecycle update job immediately (for manual triggers)
func (s *Scheduler) RunNow() {
	go s.runLifecycleUpdateJob()
}

// sendStatusChangeEmail sends an email notification with all lifecycle status changes
func (s *Scheduler) sendStatusChangeEmail(changes []products.LifecycleStatusChange) {
	if len(changes) == 0 {
		return
	}

	subject := "Mudança de Lifecycle de equipamentos detectada"

	// Build plain text version
	var textBuilder string
	textBuilder = "Os seguintes equipamentos mudaram o  lifecycle status:\n\n"
	for _, change := range changes {
		textBuilder += "Product: " + change.ProductCode + "\n"
		textBuilder += "  Old Status: " + change.OldStatus + "\n"
		textBuilder += "  New Status: " + change.NewStatus + "\n\n"
	}

	// Build HTML version
	var htmlBuilder string
	htmlBuilder = `<!DOCTYPE html>
<html>
<head>
	<style>
		body { font-family: Arial, sans-serif; }
		table { border-collapse: collapse; width: 100%; margin-top: 20px; }
		th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
		th { background-color: #4CAF50; color: white; }
		tr:nth-child(even) { background-color: #f2f2f2; }
		h2 { color: #333; }
	</style>
</head>
<body>
	<h2>Mudanças no Lifecycle Detectadas</h2>
	<p>Os seguintes produtos tiveram mudanças no lifecycle:</p>
	<table>
		<tr>
			<th>Product Code</th>
			<th>Status Antigo</th>
			<th>Novo Status</th>
		</tr>`

	for _, change := range changes {
		htmlBuilder += "<tr>"
		htmlBuilder += "<td>" + change.ProductCode + "</td>"
		htmlBuilder += "<td>" + change.OldStatus + "</td>"
		htmlBuilder += "<td>" + change.NewStatus + "</td>"
		htmlBuilder += "</tr>"
	}

	htmlBuilder += `
	</table>
</body>
</html>`

	// TODO: Configure recipient email addresses
	recipients := []string{
		"bruno.rc@outlook.com.br",
		"freitasmatheusrn@gmail.com",
	}

	if len(recipients) == 0 {
		s.logger.Warn("no email recipients configured, skipping status change notification",
			zap.Int("changes_count", len(changes)),
		)
		return
	}

	if err := s.email.Send(subject, textBuilder, htmlBuilder, recipients); err != nil {
		s.logger.Error("failed to send status change email",
			zap.Error(err),
			zap.Int("changes_count", len(changes)),
		)
		return
	}

	s.logger.Info("status change email sent successfully",
		zap.Int("changes_count", len(changes)),
		zap.Int("recipients_count", len(recipients)),
	)
}

// notifyError logs the error and sends an email notification to alert recipients
func (s *Scheduler) notifyError(context string, err error) {
	s.logger.Error(context, zap.Error(err))

	if len(s.alertRecipients) == 0 {
		return
	}

	subject := "⚠️ Erro no Scheduler - " + context
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	textBody := fmt.Sprintf("Contexto: %s\nErro: %v\nHorário: %s", context, err, timestamp)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<style>
		body { font-family: Arial, sans-serif; }
		.error-box { background-color: #ffebee; border-left: 4px solid #f44336; padding: 16px; margin: 20px 0; }
		.label { font-weight: bold; color: #333; }
		.value { color: #666; }
	</style>
</head>
<body>
	<h2 style="color: #f44336;">⚠️ Erro no Scheduler</h2>
	<div class="error-box">
		<p><span class="label">Contexto:</span> <span class="value">%s</span></p>
		<p><span class="label">Erro:</span> <span class="value">%v</span></p>
		<p><span class="label">Horário:</span> <span class="value">%s</span></p>
	</div>
</body>
</html>`, context, err, timestamp)

	if sendErr := s.email.Send(subject, textBody, htmlBody, s.alertRecipients); sendErr != nil {
		s.logger.Error("failed to send error notification email",
			zap.Error(sendErr),
			zap.String("original_error_context", context),
		)
	}
}
