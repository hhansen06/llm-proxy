package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"llm-proxy/backend/internal/config"
	"llm-proxy/backend/internal/observability"
)

type WorkerSyncer struct {
	db          *sql.DB
	httpClient  *http.Client
	interval    time.Duration
	probeTimeout time.Duration
	metrics     *observability.Metrics
}

type workerRow struct {
	ID     int64
	BaseURL string
	APIKey string
}

func NewWorkerSyncer(cfg config.Config, db *sql.DB, metrics *observability.Metrics) *WorkerSyncer {
	interval := time.Duration(cfg.WorkerSyncIntervalSec) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	probeTimeout := time.Duration(cfg.WorkerProbeTimeoutSec) * time.Second
	if probeTimeout <= 0 {
		probeTimeout = 10 * time.Second
	}

	return &WorkerSyncer{
		db: db,
		httpClient: &http.Client{Timeout: probeTimeout},
		interval: interval,
		probeTimeout: probeTimeout,
		metrics: metrics,
	}
}

func (s *WorkerSyncer) Start(ctx context.Context) {
	s.SyncOnce(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SyncOnce(ctx)
		}
	}
}

func (s *WorkerSyncer) SyncOnce(ctx context.Context) {
	start := time.Now()
	defer s.metrics.ObserveWorkerSyncRun(time.Since(start))

	workers, err := s.listWorkers(ctx)
	if err != nil {
		s.metrics.IncWorkerSyncError()
		return
	}

	for _, worker := range workers {
		probeCtx, cancel := context.WithTimeout(ctx, s.probeTimeout)
		models, latencyMS, probeErr := s.discoverModels(probeCtx, worker.BaseURL, worker.APIKey)
		cancel()

		if probeErr != nil {
			s.metrics.IncWorkerSyncError()
			s.markWorkerDegraded(ctx, worker.ID)
			continue
		}

		s.markWorkerActive(ctx, worker.ID, latencyMS)
		s.replaceWorkerModels(ctx, worker.ID, models)
	}

	active, degraded, inactive, countErr := s.workerStatusCounts(ctx)
	if countErr != nil {
		s.metrics.IncWorkerSyncError()
		return
	}
	s.metrics.SetWorkerStatusCounts(active, degraded, inactive)
}

func (s *WorkerSyncer) listWorkers(ctx context.Context) ([]workerRow, error) {
	const q = `
		SELECT id, base_url, IFNULL(api_key_encrypted, '')
		FROM workers
		WHERE status <> 'inactive'`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workers := make([]workerRow, 0)
	for rows.Next() {
		var w workerRow
		if err := rows.Scan(&w.ID, &w.BaseURL, &w.APIKey); err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

func (s *WorkerSyncer) discoverModels(ctx context.Context, baseURL string, apiKey string) ([]string, int, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	start := time.Now()
	resp, err := s.httpClient.Do(req)
	latencyMS := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, latencyMS, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, latencyMS, fmt.Errorf("status %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, latencyMS, err
	}

	models := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 {
		return nil, latencyMS, fmt.Errorf("no models discovered")
	}

	return models, latencyMS, nil
}

func (s *WorkerSyncer) markWorkerDegraded(ctx context.Context, workerID int64) {
	const q = `
		UPDATE workers
		SET status = 'degraded',
		    last_health_at = UTC_TIMESTAMP(),
		    updated_at = UTC_TIMESTAMP()
		WHERE id = ?`
	_, _ = s.db.ExecContext(ctx, q, workerID)
}

func (s *WorkerSyncer) markWorkerActive(ctx context.Context, workerID int64, latencyMS int) {
	const q = `
		UPDATE workers
		SET status = 'active',
		    last_health_at = UTC_TIMESTAMP(),
		    last_latency_ms = ?,
		    updated_at = UTC_TIMESTAMP()
		WHERE id = ?`
	_, _ = s.db.ExecContext(ctx, q, latencyMS, workerID)
}

func (s *WorkerSyncer) replaceWorkerModels(ctx context.Context, workerID int64, models []string) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM worker_models WHERE worker_id = ?`, workerID); err != nil {
		return
	}

	for _, model := range models {
		if _, err := tx.ExecContext(ctx, `INSERT INTO worker_models (worker_id, model_name, max_context_tokens) VALUES (?, ?, NULL)`, workerID, model); err != nil {
			return
		}
	}

	_ = tx.Commit()
}

func (s *WorkerSyncer) workerStatusCounts(ctx context.Context) (int, int, int, error) {
	const q = `
		SELECT
			SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) AS active,
			SUM(CASE WHEN status = 'degraded' THEN 1 ELSE 0 END) AS degraded,
			SUM(CASE WHEN status = 'inactive' THEN 1 ELSE 0 END) AS inactive
		FROM workers`

	var active, degraded, inactive sql.NullInt64
	if err := s.db.QueryRowContext(ctx, q).Scan(&active, &degraded, &inactive); err != nil {
		return 0, 0, 0, err
	}

	return int(active.Int64), int(degraded.Int64), int(inactive.Int64), nil
}
