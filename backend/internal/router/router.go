package router

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"llm-proxy/backend/internal/config"
	"llm-proxy/backend/internal/http/handlers"
	authmw "llm-proxy/backend/internal/http/middleware"
	"llm-proxy/backend/internal/observability"
)

func New(cfg config.Config, db *sql.DB, metrics *observability.Metrics) (http.Handler, error) {
	adminOIDC, err := authmw.NewAdminOIDC(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize admin oidc middleware: %w", err)
	}

	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(120 * time.Second))
	r.Use(observeHTTP(metrics))

	h := handlers.New(cfg, db, metrics)

	r.Get("/healthz", h.Healthz)
	r.Handle("/metrics", metrics.Handler())

	r.Route("/v1", func(r chi.Router) {
		r.Use(authmw.ClientBearerRequired(db))
		r.Get("/models", h.ListModels)
		r.Post("/chat/completions", h.ChatCompletions)
		r.Post("/embeddings", h.Embeddings)
		r.Post("/completions", h.Completions)
		r.Post("/responses", h.Responses)
	})

	r.Route("/admin", func(r chi.Router) {
		r.Use(adminOIDC.Require)
		r.Post("/workers", h.RegisterWorker)
		r.Get("/workers", h.ListWorkers)
		r.Post("/workers/{id}/deactivate", h.DeactivateWorker)
		r.Post("/workers/{id}/refresh", h.RefreshWorkerModels)
		r.Post("/tokens", h.CreateToken)
		r.Get("/tokens", h.ListTokens)
		r.Post("/tokens/{id}/revoke", h.RevokeToken)
		r.Post("/tokens/{id}/debug", h.SetTokenDebug)
		r.Get("/usage/metrics", h.UsageMetrics)
		r.Get("/requests", h.ListRequestLogs)
	})

	return r, nil
}

func observeHTTP(metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			path := r.URL.Path
			if rc := chi.RouteContext(r.Context()); rc != nil {
				if p := rc.RoutePattern(); p != "" {
					path = p
				}
			}

			metrics.ObserveHTTP(r.Method, path, ww.Status(), time.Since(start))
		})
	}
}
