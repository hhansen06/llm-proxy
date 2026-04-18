package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"llm-proxy/backend/internal/config"
	"llm-proxy/backend/internal/observability"
)

type Handlers struct {
	cfg        config.Config
	db         *sql.DB
	httpClient *http.Client
	metrics    *observability.Metrics
}

func New(cfg config.Config, db *sql.DB, metrics *observability.Metrics) *Handlers {
	return &Handlers{
		cfg: cfg,
		db:  db,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		metrics: metrics,
	}
}
