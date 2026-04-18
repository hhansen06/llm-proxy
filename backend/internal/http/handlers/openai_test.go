package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/backend/internal/observability"
)

func TestParseModelAndStream(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"test-model","stream":true}`)
	model, stream, err := parseModelAndStream(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "test-model" {
		t.Fatalf("model mismatch: got %q", model)
	}
	if !stream {
		t.Fatalf("expected stream=true")
	}
}

func TestParseModelAndStreamMissingModel(t *testing.T) {
	t.Parallel()

	_, _, err := parseModelAndStream([]byte(`{"stream":false}`))
	if err == nil {
		t.Fatalf("expected error for missing model")
	}
}

func TestParseUsage(t *testing.T) {
	t.Parallel()

	prompt, completion, total := parseUsage([]byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	if prompt != 11 || completion != 7 || total != 18 {
		t.Fatalf("unexpected usage values: %d %d %d", prompt, completion, total)
	}
}

func TestWorkerScorePrefersActiveOverDegraded(t *testing.T) {
	t.Parallel()

	active := workerScore(workerCandidate{Status: "active", Capacity: 4, HasLatency: true, LatencyMS: 200})
	degraded := workerScore(workerCandidate{Status: "degraded", Capacity: 4, HasLatency: true, LatencyMS: 200})
	if active <= degraded {
		t.Fatalf("expected active score > degraded score: %f <= %f", active, degraded)
	}
}

func TestForwardNonStream(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"usage": map[string]any{
				"prompt_tokens":     3,
				"completion_tokens": 5,
				"total_tokens":      8,
			},
		})
	}))
	defer upstream.Close()

	h := &Handlers{
		httpClient: upstream.Client(),
		metrics:    observability.NewMetrics(),
	}

	status, respBody, prompt, completion, total, err := h.forwardNonStream(
		context.Background(),
		"/v1/chat/completions",
		http.Header{"Content-Type": []string{"application/json"}},
		[]byte(`{"model":"x","messages":[]}`),
		workerCandidate{BaseURL: upstream.URL},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if len(respBody) == 0 {
		t.Fatalf("expected response body")
	}
	if prompt != 3 || completion != 5 || total != 8 {
		t.Fatalf("unexpected usage values: %d %d %d", prompt, completion, total)
	}
}

func TestLimitedBuffer(t *testing.T) {
	t.Parallel()

	b := &limitedBuffer{max: 5}
	_, _ = b.Write([]byte("hello"))
	_, _ = b.Write([]byte("world"))
	if !bytes.Equal(b.Bytes(), []byte("hello")) {
		t.Fatalf("unexpected limited buffer contents: %q", string(b.Bytes()))
	}
}
