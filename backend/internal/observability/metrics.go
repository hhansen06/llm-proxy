package observability

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	httpRequestsTotal        *prometheus.CounterVec
	httpRequestDuration      *prometheus.HistogramVec
	upstreamAttemptsTotal    *prometheus.CounterVec
	proxyRequestsTotal       *prometheus.CounterVec
	workerStatusGauge        *prometheus.GaugeVec
	workerSyncRunsTotal      prometheus.Counter
	workerSyncDuration       prometheus.Histogram
	workerSyncErrorsTotal    prometheus.Counter
}

var (
	newMetricsOnce sync.Once
	globalMetrics  *Metrics
)

func NewMetrics() *Metrics {
	newMetricsOnce.Do(func() {
		globalMetrics = &Metrics{
			httpRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
				Name: "llm_proxy_http_requests_total",
				Help: "Total number of incoming HTTP requests.",
			}, []string{"method", "path", "status"}),
			httpRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
				Name:    "llm_proxy_http_request_duration_seconds",
				Help:    "Duration of incoming HTTP requests.",
				Buckets: prometheus.DefBuckets,
			}, []string{"method", "path"}),
			upstreamAttemptsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
				Name: "llm_proxy_upstream_attempts_total",
				Help: "Total upstream attempts grouped by endpoint and result.",
			}, []string{"endpoint", "result"}),
			proxyRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
				Name: "llm_proxy_proxy_requests_total",
				Help: "Total proxy requests grouped by endpoint, stream mode and outcome.",
			}, []string{"endpoint", "stream", "outcome"}),
			workerStatusGauge: promauto.NewGaugeVec(prometheus.GaugeOpts{
				Name: "llm_proxy_workers_status",
				Help: "Current number of workers by status.",
			}, []string{"status"}),
			workerSyncRunsTotal: promauto.NewCounter(prometheus.CounterOpts{
				Name: "llm_proxy_worker_sync_runs_total",
				Help: "Total number of worker sync runs.",
			}),
			workerSyncDuration: promauto.NewHistogram(prometheus.HistogramOpts{
				Name:    "llm_proxy_worker_sync_duration_seconds",
				Help:    "Duration of worker sync runs.",
				Buckets: prometheus.DefBuckets,
			}),
			workerSyncErrorsTotal: promauto.NewCounter(prometheus.CounterOpts{
				Name: "llm_proxy_worker_sync_errors_total",
				Help: "Total number of worker sync errors.",
			}),
		}
	})
	return globalMetrics
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

func (m *Metrics) ObserveHTTP(method string, path string, status int, duration time.Duration) {
	m.httpRequestsTotal.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	m.httpRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func (m *Metrics) IncUpstreamAttempt(endpoint string, result string) {
	m.upstreamAttemptsTotal.WithLabelValues(endpoint, result).Inc()
}

func (m *Metrics) IncProxyRequest(endpoint string, stream bool, outcome string) {
	streamLabel := "false"
	if stream {
		streamLabel = "true"
	}
	m.proxyRequestsTotal.WithLabelValues(endpoint, streamLabel, outcome).Inc()
}

func (m *Metrics) SetWorkerStatusCounts(active int, degraded int, inactive int) {
	m.workerStatusGauge.WithLabelValues("active").Set(float64(active))
	m.workerStatusGauge.WithLabelValues("degraded").Set(float64(degraded))
	m.workerStatusGauge.WithLabelValues("inactive").Set(float64(inactive))
}

func (m *Metrics) ObserveWorkerSyncRun(duration time.Duration) {
	m.workerSyncRunsTotal.Inc()
	m.workerSyncDuration.Observe(duration.Seconds())
}

func (m *Metrics) IncWorkerSyncError() {
	m.workerSyncErrorsTotal.Inc()
}
