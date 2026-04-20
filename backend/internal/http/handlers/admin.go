package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	mysql "github.com/go-sql-driver/mysql"
)

func (h *Handlers) RegisterWorker(w http.ResponseWriter, r *http.Request) {
	type registerWorkerRequest struct {
		TenantID     int64  `json:"tenant_id"`
		Name         string `json:"name"`
		BaseURL      string `json:"base_url"`
		APIKey       string `json:"api_key"`
		CapacityHint int    `json:"capacity_hint"`
	}

	var req registerWorkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if req.Name == "" || req.BaseURL == "" {
		writeErr(w, http.StatusBadRequest, "name and base_url are required")
		return
	}
	if req.CapacityHint <= 0 {
		req.CapacityHint = 1
	}

	baseURL := strings.TrimRight(req.BaseURL, "/")
	models, err := discoverModels(r.Context(), baseURL, req.APIKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("model discovery failed: %v", err))
		return
	}

	const insertWorker = `
		INSERT INTO workers (tenant_id, name, base_url, api_key_encrypted, status, capacity_hint, last_health_at, last_latency_ms)
		VALUES (?, ?, ?, ?, 'active', ?, UTC_TIMESTAMP(), NULL)`

	res, err := h.db.ExecContext(r.Context(), insertWorker, nullableTenantID(req.TenantID), req.Name, baseURL, req.APIKey, req.CapacityHint)
	if err != nil {
		status, message := mapWorkerCreateDBError(err, req.TenantID)
		writeErr(w, status, message)
		return
	}

	workerID, err := res.LastInsertId()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to resolve worker id")
		return
	}

	const insertModel = `
		INSERT INTO worker_models (worker_id, model_name, max_context_tokens)
		VALUES (?, ?, NULL)
		ON DUPLICATE KEY UPDATE discovered_at = UTC_TIMESTAMP()`

	for _, model := range models {
		if _, err := h.db.ExecContext(r.Context(), insertModel, workerID, model); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to store discovered models")
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"worker_id": workerID,
		"models":    models,
	})
}

func (h *Handlers) ListWorkers(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT w.id, w.tenant_id, w.name, w.base_url, w.status, w.capacity_hint, w.last_health_at, w.last_latency_ms,
		       wm.model_name
		FROM workers w
		LEFT JOIN worker_models wm ON wm.worker_id = w.id
		ORDER BY w.id DESC, wm.model_name ASC`

	rows, err := h.db.QueryContext(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list workers")
		return
	}
	defer rows.Close()

	workersByID := map[int64]map[string]any{}
	order := make([]int64, 0)

	for rows.Next() {
		var (
			id           int64
			tenantID     sql.NullInt64
			name         string
			baseURL      string
			status       string
			capacityHint int
			lastHealthAt sql.NullTime
			lastLatency  sql.NullInt64
			modelName    sql.NullString
		)

		if err := rows.Scan(&id, &tenantID, &name, &baseURL, &status, &capacityHint, &lastHealthAt, &lastLatency, &modelName); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to parse workers")
			return
		}

		item, ok := workersByID[id]
		if !ok {
			item = map[string]any{
				"id":            id,
				"tenant_id":     nil,
				"name":          name,
				"base_url":      baseURL,
				"status":        status,
				"capacity_hint": capacityHint,
				"models":        []string{},
			}
			if tenantID.Valid {
				item["tenant_id"] = tenantID.Int64
			}
			if lastHealthAt.Valid {
				item["last_health_at"] = lastHealthAt.Time
			}
			if lastLatency.Valid {
				item["last_latency_ms"] = lastLatency.Int64
			}
			workersByID[id] = item
			order = append(order, id)
		}

		if modelName.Valid {
			models := item["models"].([]string)
			item["models"] = append(models, modelName.String)
		}
	}

	workers := make([]map[string]any, 0, len(order))
	for _, id := range order {
		workers = append(workers, workersByID[id])
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"workers": workers})
}

func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	type createTokenRequest struct {
		TenantID            int64  `json:"tenant_id"`
		Label               string `json:"label"`
		DebugEnabled        bool   `json:"debug_enabled"`
		QuotaRequestsPerMin *int64 `json:"quota_requests_per_min"`
		QuotaTokensPerDay   *int64 `json:"quota_tokens_per_day"`
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid json body"})
		return
	}

	if req.TenantID == 0 {
		req.TenantID = 1
	}
	if req.Label == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "label is required"})
		return
	}

	rawToken, err := generateToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "token generation failed"})
		return
	}

	hash := sha256.Sum256([]byte(rawToken))
	hashHex := hex.EncodeToString(hash[:])

	qRPM := nullableInt64(req.QuotaRequestsPerMin)
	qTPD := nullableInt64(req.QuotaTokensPerDay)

	const q = `
		INSERT INTO api_tokens (tenant_id, token_hash, label, debug_enabled, quota_requests_per_min, quota_tokens_per_day)
		VALUES (?, ?, ?, ?, ?, ?)`

	res, err := h.db.ExecContext(r.Context(), q, req.TenantID, hashHex, req.Label, req.DebugEnabled, qRPM, qTPD)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusBadRequest
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "token could not be created"})
		return
	}

	id, _ := res.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                     id,
		"tenant_id":              req.TenantID,
		"label":                  req.Label,
		"debug_enabled":          req.DebugEnabled,
		"quota_requests_per_min": req.QuotaRequestsPerMin,
		"quota_tokens_per_day":   req.QuotaTokensPerDay,
		"token":                  rawToken,
	})
}

func (h *Handlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT id, tenant_id, label, debug_enabled, is_revoked, quota_requests_per_min, quota_tokens_per_day, created_at
		FROM api_tokens
		ORDER BY id DESC
		LIMIT 200`

	rows, err := h.db.QueryContext(r.Context(), q)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to list tokens"})
		return
	}
	defer rows.Close()

	tokens := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id           int64
			tenantID     int64
			label        string
			debugEnabled bool
			isRevoked    bool
			qRPM         sql.NullInt64
			qTPD         sql.NullInt64
			createdAt    time.Time
		)

		if err := rows.Scan(&id, &tenantID, &label, &debugEnabled, &isRevoked, &qRPM, &qTPD, &createdAt); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to parse token rows"})
			return
		}

		item := map[string]any{
			"id":            id,
			"tenant_id":     tenantID,
			"label":         label,
			"debug_enabled": debugEnabled,
			"is_revoked":    isRevoked,
			"created_at":    createdAt,
		}
		if qRPM.Valid {
			item["quota_requests_per_min"] = qRPM.Int64
		}
		if qTPD.Valid {
			item["quota_tokens_per_day"] = qTPD.Int64
		}

		tokens = append(tokens, item)
	}

	if err := rows.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to iterate token rows"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"tokens": tokens})
}

func (h *Handlers) RevokeToken(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "id")
	if tokenID == "" {
		writeErr(w, http.StatusBadRequest, "missing token id")
		return
	}

	const q = `UPDATE api_tokens SET is_revoked = 1, updated_at = UTC_TIMESTAMP() WHERE id = ?`
	res, err := h.db.ExecContext(r.Context(), q, tokenID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeErr(w, http.StatusNotFound, "token not found")
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"status": "revoked"})
}

func (h *Handlers) SetTokenDebug(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Enabled bool `json:"enabled"`
	}

	tokenID := chi.URLParam(r, "id")
	if tokenID == "" {
		writeErr(w, http.StatusBadRequest, "missing token id")
		return
	}

	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}

	const q = `UPDATE api_tokens SET debug_enabled = ?, updated_at = UTC_TIMESTAMP() WHERE id = ?`
	res, err := h.db.ExecContext(r.Context(), q, req.Enabled, tokenID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update token debug mode")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeErr(w, http.StatusNotFound, "token not found")
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"debug_enabled": req.Enabled})
}

func (h *Handlers) DeactivateWorker(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")
	if workerID == "" {
		writeErr(w, http.StatusBadRequest, "missing worker id")
		return
	}

	const q = `UPDATE workers SET status = 'inactive', updated_at = UTC_TIMESTAMP() WHERE id = ?`
	res, err := h.db.ExecContext(r.Context(), q, workerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to deactivate worker")
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeErr(w, http.StatusNotFound, "worker not found")
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"status": "inactive"})
}

func (h *Handlers) RefreshWorkerModels(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")
	if workerID == "" {
		writeErr(w, http.StatusBadRequest, "missing worker id")
		return
	}

	const q = `SELECT base_url, IFNULL(api_key_encrypted, '') FROM workers WHERE id = ? LIMIT 1`
	var (
		baseURL string
		apiKey  string
	)
	if err := h.db.QueryRowContext(r.Context(), q, workerID).Scan(&baseURL, &apiKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "worker not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "failed to resolve worker")
		return
	}

	models, err := discoverModels(r.Context(), strings.TrimRight(baseURL, "/"), apiKey)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("model refresh failed: %v", err))
		return
	}

	const clearQ = `DELETE FROM worker_models WHERE worker_id = ?`
	if _, err := h.db.ExecContext(r.Context(), clearQ, workerID); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to clear worker models")
		return
	}

	const insertModel = `
		INSERT INTO worker_models (worker_id, model_name, max_context_tokens)
		VALUES (?, ?, NULL)`
	for _, model := range models {
		if _, err := h.db.ExecContext(r.Context(), insertModel, workerID, model); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to persist refreshed models")
			return
		}
	}

	const updateQ = `UPDATE workers SET status = 'active', last_health_at = UTC_TIMESTAMP(), updated_at = UTC_TIMESTAMP() WHERE id = ?`
	_, _ = h.db.ExecContext(r.Context(), updateQ, workerID)

	_ = json.NewEncoder(w).Encode(map[string]any{"worker_id": workerID, "models": models})
}

func (h *Handlers) UsageMetrics(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT
			token_id,
			COUNT(*) AS requests_24h,
			COALESCE(SUM(total_tokens), 0) AS tokens_24h,
			SUM(CASE WHEN created_at >= (UTC_TIMESTAMP() - INTERVAL 1 MINUTE) THEN 1 ELSE 0 END) AS requests_1m,
			SUM(CASE WHEN created_at >= (UTC_TIMESTAMP() - INTERVAL 1 MINUTE) THEN total_tokens ELSE 0 END) AS tokens_1m
		FROM request_logs
		WHERE created_at >= (UTC_TIMESTAMP() - INTERVAL 24 HOUR)
		GROUP BY token_id
		ORDER BY requests_24h DESC
		LIMIT 500`

	rows, err := h.db.QueryContext(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to query usage metrics")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var (
			tokenID     int64
			requests24h int64
			tokens24h   int64
			requests1m  int64
			tokens1m    int64
		)
		if err := rows.Scan(&tokenID, &requests24h, &tokens24h, &requests1m, &tokens1m); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to parse usage metrics")
			return
		}
		items = append(items, map[string]any{
			"token_id":     tokenID,
			"requests_24h": requests24h,
			"tokens_24h":   tokens24h,
			"requests_1m":  requests1m,
			"tokens_1m":    tokens1m,
		})
	}

	total := map[string]any{
		"requests_24h": int64(0),
		"tokens_24h":   int64(0),
		"requests_1m":  int64(0),
		"tokens_1m":    int64(0),
	}
	for _, item := range items {
		total["requests_24h"] = total["requests_24h"].(int64) + item["requests_24h"].(int64)
		total["tokens_24h"] = total["tokens_24h"].(int64) + item["tokens_24h"].(int64)
		total["requests_1m"] = total["requests_1m"].(int64) + item["requests_1m"].(int64)
		total["tokens_1m"] = total["tokens_1m"].(int64) + item["tokens_1m"].(int64)
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"generated_at": time.Now().UTC(),
		"total":        total,
		"by_token":     items,
	})
}

func (h *Handlers) ListRequestLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := strings.TrimSpace(r.URL.Query().Get("limit")); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	tokenFilter := strings.TrimSpace(r.URL.Query().Get("token_id"))
	modelFilter := strings.TrimSpace(r.URL.Query().Get("model"))

	q := `
		SELECT id, request_id, tenant_id, token_id, worker_id, model_name,
		       prompt_tokens, completion_tokens, total_tokens, duration_ms, http_status, created_at
		FROM request_logs
		WHERE 1=1`
	args := make([]any, 0, 3)

	if tokenFilter != "" {
		q += ` AND token_id = ?`
		args = append(args, tokenFilter)
	}
	if modelFilter != "" {
		q += ` AND model_name = ?`
		args = append(args, modelFilter)
	}

	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := h.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to query request logs")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id               int64
			requestID        string
			tenantID         int64
			tokenID          int64
			workerID         sql.NullInt64
			modelName        string
			promptTokens     int
			completionTokens int
			totalTokens      int
			durationMS       int
			httpStatus       int
			createdAt        time.Time
		)
		if err := rows.Scan(&id, &requestID, &tenantID, &tokenID, &workerID, &modelName, &promptTokens, &completionTokens, &totalTokens, &durationMS, &httpStatus, &createdAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to parse request logs")
			return
		}

		item := map[string]any{
			"id":                id,
			"request_id":        requestID,
			"tenant_id":         tenantID,
			"token_id":          tokenID,
			"model_name":        modelName,
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      totalTokens,
			"duration_ms":       durationMS,
			"http_status":       httpStatus,
			"created_at":        createdAt,
		}
		if workerID.Valid {
			item["worker_id"] = workerID.Int64
		}

		items = append(items, item)
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"logs": items})
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "lp_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTenantID(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func discoverModels(ctx context.Context, baseURL string, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("worker reports no models")
	}

	return models, nil
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func mapWorkerCreateDBError(err error, tenantID int64) (int, string) {
	var myErr *mysql.MySQLError
	if !errors.As(err, &myErr) {
		return http.StatusInternalServerError, "failed to create worker"
	}

	switch myErr.Number {
	case 1452:
		return http.StatusBadRequest, fmt.Sprintf("tenant_id %d does not exist", tenantID)
	case 1406:
		return http.StatusBadRequest, "worker payload too long for database column"
	case 1366:
		return http.StatusBadRequest, "invalid value type for worker payload"
	default:
		return http.StatusInternalServerError, "failed to create worker"
	}
}
