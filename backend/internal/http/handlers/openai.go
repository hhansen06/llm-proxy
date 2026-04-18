package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	authmw "llm-proxy/backend/internal/http/middleware"
)

func (h *Handlers) ListModels(w http.ResponseWriter, r *http.Request) {
	identity, ok := authmw.ClientIdentityFromContext(r.Context())
	if !ok {
		h.metrics.IncProxyRequest("/v1/models", false, "unauthorized")
		writeErr(w, http.StatusUnauthorized, "missing token identity")
		return
	}

	const q = `
		SELECT DISTINCT wm.model_name
		FROM worker_models wm
		JOIN workers w ON w.id = wm.worker_id
		WHERE w.tenant_id = ? AND w.status IN ('active','degraded')
		ORDER BY wm.model_name ASC`

	rows, err := h.db.QueryContext(r.Context(), q, identity.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list models")
		return
	}
	defer rows.Close()

	data := make([]map[string]any, 0)
	for rows.Next() {
		var modelName string
		if err := rows.Scan(&modelName); err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to parse models")
			return
		}
		data = append(data, map[string]any{
			"id":      modelName,
			"object":  "model",
			"created": 0,
			"owned_by": "llm-proxy",
		})
	}

	resp := map[string]any{
		"object": "list",
		"data":   data,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	h.proxyToWorker(w, r, "/v1/chat/completions")
}

func (h *Handlers) Embeddings(w http.ResponseWriter, r *http.Request) {
	h.proxyToWorker(w, r, "/v1/embeddings")
}

func (h *Handlers) Completions(w http.ResponseWriter, r *http.Request) {
	h.proxyToWorker(w, r, "/v1/completions")
}

func (h *Handlers) Responses(w http.ResponseWriter, r *http.Request) {
	h.proxyToWorker(w, r, "/v1/responses")
}

type workerCandidate struct {
	ID          int64
	BaseURL     string
	APIKey      string
	Status      string
	Capacity    int
	LatencyMS   int64
	HasLatency  bool
	Score       float64
}

func (h *Handlers) proxyToWorker(w http.ResponseWriter, r *http.Request, endpoint string) {
	identity, ok := authmw.ClientIdentityFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "missing token identity")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.metrics.IncProxyRequest(endpoint, false, "bad_request")
		writeErr(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	modelName, stream, err := parseModelAndStream(body)
	if err != nil {
		h.metrics.IncProxyRequest(endpoint, false, "bad_request")
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	workers, err := h.selectWorkers(r.Context(), identity.TenantID, modelName)
	if err != nil {
		h.metrics.IncProxyRequest(endpoint, stream, "resolve_workers_error")
		writeErr(w, http.StatusInternalServerError, "failed to resolve workers")
		return
	}
	if len(workers) == 0 {
		h.metrics.IncProxyRequest(endpoint, stream, "no_worker")
		writeErr(w, http.StatusBadGateway, "no healthy worker found for model")
		return
	}

	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	var lastErr error
	for idx, worker := range workers {
		attemptRequestID := fmt.Sprintf("%s-a%d", requestID, idx+1)
		start := time.Now()

		if stream {
			status, respBody, streamErr := h.forwardStream(r.Context(), w, endpoint, r.Header, body, worker)
			duration := int(time.Since(start).Milliseconds())
			h.updateWorkerHealth(r.Context(), worker.ID, duration)

			if streamErr != nil {
				h.metrics.IncUpstreamAttempt(endpoint, "transport_error")
				lastErr = streamErr
				h.logRequest(r.Context(), attemptRequestID, identity, modelName, worker.ID, duration, 502, 0, 0, 0, body, nil)
				continue
			}

			if status >= 500 && idx < len(workers)-1 {
				h.metrics.IncUpstreamAttempt(endpoint, "upstream_5xx_retry")
				h.logRequest(r.Context(), attemptRequestID, identity, modelName, worker.ID, duration, status, 0, 0, 0, body, respBody)
				continue
			}

			if status >= 500 {
				h.metrics.IncUpstreamAttempt(endpoint, "upstream_5xx")
			} else {
				h.metrics.IncUpstreamAttempt(endpoint, "success")
			}

			h.logRequest(r.Context(), attemptRequestID, identity, modelName, worker.ID, duration, status, 0, 0, 0, body, respBody)
			h.metrics.IncProxyRequest(endpoint, true, outcomeFromStatus(status))
			return
		}

		status, respBody, promptTokens, completionTokens, totalTokens, proxyErr := h.forwardNonStream(r.Context(), endpoint, r.Header, body, worker)
		duration := int(time.Since(start).Milliseconds())
		h.updateWorkerHealth(r.Context(), worker.ID, duration)

		if proxyErr != nil {
			h.metrics.IncUpstreamAttempt(endpoint, "transport_error")
			lastErr = proxyErr
			h.logRequest(r.Context(), attemptRequestID, identity, modelName, worker.ID, duration, 502, 0, 0, 0, body, nil)
			continue
		}

		if status >= 500 && idx < len(workers)-1 {
			h.metrics.IncUpstreamAttempt(endpoint, "upstream_5xx_retry")
			h.logRequest(r.Context(), attemptRequestID, identity, modelName, worker.ID, duration, status, promptTokens, completionTokens, totalTokens, body, respBody)
			continue
		}

		if status >= 500 {
			h.metrics.IncUpstreamAttempt(endpoint, "upstream_5xx")
		} else {
			h.metrics.IncUpstreamAttempt(endpoint, "success")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(respBody)

		h.logRequest(r.Context(), attemptRequestID, identity, modelName, worker.ID, duration, status, promptTokens, completionTokens, totalTokens, body, respBody)
		h.metrics.IncProxyRequest(endpoint, false, outcomeFromStatus(status))
		return
	}

	msg := "all workers failed"
	if lastErr != nil {
		msg = msg + ": " + lastErr.Error()
	}
	h.metrics.IncProxyRequest(endpoint, stream, "all_workers_failed")
	writeErr(w, http.StatusBadGateway, msg)
}

func (h *Handlers) selectWorkers(ctx context.Context, tenantID int64, modelName string) ([]workerCandidate, error) {
	const q = `
		SELECT w.id, w.base_url, IFNULL(w.api_key_encrypted, ''), w.status, w.capacity_hint, w.last_latency_ms
		FROM workers w
		JOIN worker_models wm ON wm.worker_id = w.id
		WHERE w.tenant_id = ?
		  AND wm.model_name = ?
		  AND w.status IN ('active','degraded')`

	rows, err := h.db.QueryContext(ctx, q, tenantID, modelName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workers := make([]workerCandidate, 0)
	for rows.Next() {
		var (
			item       workerCandidate
			latencyRaw sql.NullInt64
		)
		if err := rows.Scan(&item.ID, &item.BaseURL, &item.APIKey, &item.Status, &item.Capacity, &latencyRaw); err != nil {
			return nil, err
		}
		if item.Capacity <= 0 {
			item.Capacity = 1
		}
		if latencyRaw.Valid {
			item.LatencyMS = latencyRaw.Int64
			item.HasLatency = true
		}
		item.Score = workerScore(item)
		workers = append(workers, item)
	}

	sort.SliceStable(workers, func(i, j int) bool {
		return workers[i].Score > workers[j].Score
	})

	return workers, nil
}

func workerScore(w workerCandidate) float64 {
	health := 0.0
	switch w.Status {
	case "active":
		health = 1.0
	case "degraded":
		health = 0.6
	}

	latency := 0.5
	if w.HasLatency {
		latency = 1.0 / (1.0 + float64(maxInt64(1, w.LatencyMS))/1000.0)
	}

	capN := math.Min(1.0, float64(w.Capacity)/16.0)

	return 0.5*health + 0.3*latency + 0.2*capN
}

func (h *Handlers) forwardNonStream(ctx context.Context, endpoint string, incomingHeaders http.Header, body []byte, worker workerCandidate) (int, []byte, int, int, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(worker.BaseURL, "/")+endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, 0, 0, 0, err
	}
	copyHeaders(req.Header, incomingHeaders)
	req.Header.Set("Content-Type", "application/json")
	if worker.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+worker.APIKey)
	} else {
		req.Header.Del("Authorization")
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return 0, nil, 0, 0, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, 0, 0, 0, err
	}

	promptTokens, completionTokens, totalTokens := parseUsage(respBody)

	return resp.StatusCode, respBody, promptTokens, completionTokens, totalTokens, nil
}

func (h *Handlers) forwardStream(ctx context.Context, w http.ResponseWriter, endpoint string, incomingHeaders http.Header, body []byte, worker workerCandidate) (int, []byte, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return 0, nil, fmt.Errorf("streaming not supported by response writer")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(worker.BaseURL, "/")+endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	copyHeaders(req.Header, incomingHeaders)
	req.Header.Set("Content-Type", "application/json")
	if worker.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+worker.APIKey)
	} else {
		req.Header.Del("Authorization")
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return resp.StatusCode, nil, readErr
		}
		return resp.StatusCode, respBody, nil
	}

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	buffer := &limitedBuffer{max: 16 * 1024}
	_, err = io.Copy(flushWriter{ResponseWriter: w, flusher: flusher}, io.TeeReader(resp.Body, buffer))
	if err != nil {
		return resp.StatusCode, buffer.Bytes(), err
	}

	return resp.StatusCode, buffer.Bytes(), nil
}

func parseModelAndStream(body []byte) (string, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false, fmt.Errorf("invalid json body")
	}

	modelVal, ok := payload["model"].(string)
	if !ok || strings.TrimSpace(modelVal) == "" {
		return "", false, fmt.Errorf("model is required")
	}

	stream := false
	if streamVal, ok := payload["stream"].(bool); ok {
		stream = streamVal
	}

	return modelVal, stream, nil
}

func parseUsage(body []byte) (int, int, int) {
	var payload struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0, 0
	}
	return payload.Usage.PromptTokens, payload.Usage.CompletionTokens, payload.Usage.TotalTokens
}

func (h *Handlers) updateWorkerHealth(ctx context.Context, workerID int64, latencyMS int) {
	const q = `
		UPDATE workers
		SET last_health_at = UTC_TIMESTAMP(),
		    last_latency_ms = ?
		WHERE id = ?`
	_, _ = h.db.ExecContext(ctx, q, latencyMS, workerID)
}

func (h *Handlers) logRequest(
	ctx context.Context,
	requestID string,
	identity authmw.ClientTokenIdentity,
	modelName string,
	workerID int64,
	durationMS int,
	status int,
	promptTokens int,
	completionTokens int,
	totalTokens int,
	requestBody []byte,
	responseBody []byte,
) {
	debugPayload := any(nil)
	if identity.DebugEnabled {
		debugPayload = string(buildDebugPayload(requestBody, responseBody))
	}

	const q = `
		INSERT INTO request_logs (
			request_id, tenant_id, token_id, worker_id, model_name,
			prompt_tokens, completion_tokens, total_tokens, duration_ms, http_status, debug_payload
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, _ = h.db.ExecContext(
		ctx,
		q,
		requestID,
		identity.TenantID,
		identity.TokenID,
		workerID,
		modelName,
		promptTokens,
		completionTokens,
		totalTokens,
		durationMS,
		status,
		debugPayload,
	)
}

func buildDebugPayload(reqBody []byte, respBody []byte) []byte {
	const maxLen = 16 * 1024
	out := append([]byte("request="), reqBody...)
	out = append(out, []byte("\nresponse=")...)
	out = append(out, respBody...)
	if len(out) > maxLen {
		return out[:maxLen]
	}
	return out
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func copyHeaders(dst http.Header, src http.Header) {
	for k, vals := range src {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Authorization") {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

type flushWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.ResponseWriter.Write(p)
	fw.flusher.Flush()
	return n, err
}

type limitedBuffer struct {
	buf []byte
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 || len(b.buf) >= b.max {
		return len(p), nil
	}
	left := b.max - len(b.buf)
	if len(p) > left {
		b.buf = append(b.buf, p[:left]...)
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf
}

func copyResponseHeaders(dst http.Header, src http.Header) {
	for k, vals := range src {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
		}
		dst.Del(k)
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func outcomeFromStatus(status int) string {
	if status >= 200 && status < 300 {
		return "success"
	}
	if status >= 400 && status < 500 {
		return "client_error"
	}
	if status >= 500 {
		return "server_error"
	}
	return "other"
}
