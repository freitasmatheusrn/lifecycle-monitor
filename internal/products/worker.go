package products

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	repo "github.com/freitasmatheusrn/lifecycle-monitor/internal/database/postgres/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

type WorkerPool struct {
	crawler     *Crawler
	repo        repo.Querier
	logger      *zap.Logger
	jobs        chan CrawlerJob
	results     chan WorkerResult
	numWorkers  int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	isRunning   bool
	mu          sync.Mutex
	syncResults map[string]chan WorkerResult // canais para jobs síncronos
	syncMu      sync.RWMutex                 // mutex para syncResults
}

type WorkerResult struct {
	Job   CrawlerJob
	Data  *CrawledData
	Error error
}

type WorkerPoolConfig struct {
	NumWorkers int
	QueueSize  int
}

func NewWorkerPool(crawler *Crawler, repo repo.Querier, logger *zap.Logger, config WorkerPoolConfig) *WorkerPool {
	if config.NumWorkers <= 0 {
		config.NumWorkers = 3
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		crawler:     crawler,
		repo:        repo,
		logger:      logger,
		jobs:        make(chan CrawlerJob, config.QueueSize),
		results:     make(chan WorkerResult, config.QueueSize),
		numWorkers:  config.NumWorkers,
		ctx:         ctx,
		cancel:      cancel,
		syncResults: make(map[string]chan WorkerResult),
	}
}

func (wp *WorkerPool) Start() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.isRunning {
		return nil
	}

	if err := wp.crawler.Start(); err != nil {
		return fmt.Errorf("failed to start crawler: %w", err)
	}

	// Start workers
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	// Start result processor
	wp.wg.Add(1)
	go wp.processResults()

	wp.isRunning = true
	wp.logger.Info("worker pool started", zap.Int("workers", wp.numWorkers))

	return nil
}

func (wp *WorkerPool) Stop() error {
	wp.mu.Lock()
	if !wp.isRunning {
		wp.mu.Unlock()
		return nil
	}
	wp.mu.Unlock()

	wp.cancel()
	close(wp.jobs)
	wp.wg.Wait()
	close(wp.results)

	if err := wp.crawler.Stop(); err != nil {
		return fmt.Errorf("failed to stop crawler: %w", err)
	}

	wp.mu.Lock()
	wp.isRunning = false
	wp.mu.Unlock()

	wp.logger.Info("worker pool stopped")
	return nil
}

func (wp *WorkerPool) Submit(job CrawlerJob) error {
	wp.mu.Lock()
	if !wp.isRunning {
		wp.mu.Unlock()
		return fmt.Errorf("worker pool is not running")
	}
	wp.mu.Unlock()

	select {
	case wp.jobs <- job:
		wp.logger.Debug("job submitted", zap.String("code", job.ProductCode))
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is shutting down")
	default:
		return fmt.Errorf("job queue is full")
	}
}

func (wp *WorkerPool) SubmitAndWait(ctx context.Context, job CrawlerJob) (*CrawledData, error) {
	// Gera um ID único para este job
	jobID := fmt.Sprintf("%s-%d", job.ProductCode, time.Now().UnixNano())
	job.jobID = jobID

	// Cria e registra o canal de resultado
	resultChan := make(chan WorkerResult, 1)
	wp.syncMu.Lock()
	wp.syncResults[jobID] = resultChan
	wp.syncMu.Unlock()

	// Garante limpeza do canal ao final
	defer func() {
		wp.syncMu.Lock()
		delete(wp.syncResults, jobID)
		wp.syncMu.Unlock()
	}()

	// Submete o job para o worker pool
	if err := wp.Submit(job); err != nil {
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}

	// Espera pelo resultado ou timeout/cancelamento
	select {
	case result := <-resultChan:
		if result.Error != nil {
			return nil, result.Error
		}
		return result.Data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubmitBatch submete múltiplos jobs e retorna um canal que emite resultados conforme cada job termina.
// O canal é fechado automaticamente quando todos os jobs forem processados.
// Os resultados podem chegar fora de ordem.
func (wp *WorkerPool) SubmitBatch(ctx context.Context, jobs []CrawlerJob) (<-chan WorkerResult, error) {
	if len(jobs) == 0 {
		ch := make(chan WorkerResult)
		close(ch)
		return ch, nil
	}

	// Canal interno para receber dos workers
	internalChan := make(chan WorkerResult, len(jobs))
	// Canal de saída para o chamador
	outputChan := make(chan WorkerResult, len(jobs))

	// Gera IDs e registra canais para cada job
	jobIDs := make([]string, len(jobs))
	for i := range jobs {
		jobID := fmt.Sprintf("%s-%d-%d", jobs[i].ProductCode, time.Now().UnixNano(), i)
		jobs[i].jobID = jobID
		jobIDs[i] = jobID

		wp.syncMu.Lock()
		wp.syncResults[jobID] = internalChan
		wp.syncMu.Unlock()
	}

	// Submete todos os jobs
	submitted := 0
	for _, job := range jobs {
		if err := wp.Submit(job); err != nil {
			// Limpa os jobs já registrados em caso de erro
			wp.syncMu.Lock()
			for _, id := range jobIDs[:submitted] {
				delete(wp.syncResults, id)
			}
			wp.syncMu.Unlock()
			close(outputChan)
			return nil, fmt.Errorf("failed to submit job %s: %w", job.ProductCode, err)
		}
		submitted++
	}

	// Goroutine para repassar resultados e limpar quando todos terminarem
	go func() {
		defer close(outputChan)
		defer func() {
			wp.syncMu.Lock()
			for _, id := range jobIDs {
				delete(wp.syncResults, id)
			}
			wp.syncMu.Unlock()
		}()

		received := 0
		for received < len(jobs) {
			select {
			case result := <-internalChan:
				outputChan <- result
				received++
			case <-ctx.Done():
				return
			}
		}
	}()

	return outputChan, nil
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	wp.logger.Debug("worker started", zap.Int("worker_id", id))

	for {
		select {
		case job, ok := <-wp.jobs:
			if !ok {
				wp.logger.Debug("worker stopping", zap.Int("worker_id", id))
				return
			}

			// Add random delay to avoid detection
			delay := time.Duration(rand.Intn(2000)+500) * time.Millisecond
			time.Sleep(delay)

			wp.logger.Debug("processing job",
				zap.Int("worker_id", id),
				zap.String("code", job.ProductCode),
			)

			data, err := wp.crawler.Collect(job.ProductCode)

			result := WorkerResult{
				Job:   job,
				Data:  data,
				Error: err,
			}

			// Se é um job síncrono (tem jobID), envia para o canal específico
			if job.jobID != "" {
				wp.syncMu.RLock()
				if ch, exists := wp.syncResults[job.jobID]; exists {
					ch <- result
				}
				wp.syncMu.RUnlock()
			} else {
				// Job assíncrono normal, envia para o canal de resultados
				wp.results <- result
			}

		case <-wp.ctx.Done():
			wp.logger.Debug("worker cancelled", zap.Int("worker_id", id))
			return
		}
	}
}

func (wp *WorkerPool) processResults() {
	defer wp.wg.Done()

	for {
		select {
		case result, ok := <-wp.results:
			if !ok {
				return
			}

			if result.Error != nil {
				wp.logger.Error("crawling failed",
					zap.String("code", result.Job.ProductCode),
					zap.Error(result.Error),
				)
				continue
			}

			// Save snapshot to database
			if err := wp.saveSnapshot(result.Job, result.Data); err != nil {
				wp.logger.Error("failed to save snapshot",
					zap.String("code", result.Job.ProductCode),
					zap.Error(err),
				)
				continue
			}

			wp.logger.Info("snapshot saved",
				zap.String("code", result.Job.ProductCode),
				zap.String("description", result.Data.Description),
				zap.String("status", result.Data.Status),
			)

		case <-wp.ctx.Done():
			// Drain remaining results
			for result := range wp.results {
				if result.Error == nil {
					_ = wp.saveSnapshot(result.Job, result.Data)
				}
			}
			return
		}
	}
}

func (wp *WorkerPool) saveSnapshot(job CrawlerJob, data *CrawledData) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var status pgtype.Text
	if data.Status != "" {
		status = pgtype.Text{String: data.Status, Valid: true}
	}

	var rawHTML pgtype.Text
	if data.RawHTML != "" {
		rawHTML = pgtype.Text{String: data.RawHTML, Valid: true}
	}

	// Create snapshot
	_, err := wp.repo.CreateSnapshot(ctx, repo.CreateSnapshotParams{
		ProductID:   job.ProductID,
		Description: data.Description,
		Status:      status,
		RawHtml:     rawHTML,
	})
	if err != nil {
		return err
	}

	// Update product lifecycle status and replacement URL if applicable
	if data.Status != "" || data.ReplacementCode != "" {
		var lifecycleStatus pgtype.Text
		if data.Status != "" {
			lifecycleStatus = pgtype.Text{String: data.Status, Valid: true}
		}

		var replacementURL pgtype.Text
		if data.ReplacementCode != "" {
			replacementURL = pgtype.Text{String: data.ReplacementCode, Valid: true}
		}

		err = wp.repo.UpdateProductLifecycleStatus(ctx, repo.UpdateProductLifecycleStatusParams{
			Code:            job.ProductCode,
			LifecycleStatus: lifecycleStatus,
			ReplacementUrl:  replacementURL,
		})
		if err != nil {
			wp.logger.Warn("failed to update product lifecycle status",
				zap.String("code", job.ProductCode),
				zap.Error(err),
			)
		}
	}

	return nil
}

// CollectAll fetches data for all products in the system
func (wp *WorkerPool) CollectAll(ctx context.Context) error {
	products, err := wp.repo.ListAllProductsToCollect(ctx)
	if err != nil {
		return fmt.Errorf("failed to list products to collect: %w", err)
	}

	for _, product := range products {
		job := CrawlerJob{
			ProductID:   product.ID,
			ProductCode: product.Code,
			ProductURL:  product.Url,
		}

		if err := wp.Submit(job); err != nil {
			wp.logger.Warn("failed to submit job",
				zap.String("code", product.Code),
				zap.Error(err),
			)
		}
	}

	return nil
}
