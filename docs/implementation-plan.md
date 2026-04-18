# Implementierungsplan

## Phase 1 - Foundations

- Go Service Scaffold
- Config-Management
- MariaDB Verbindung + Migrationsframework
- Basis-Observability (structured logs, metrics endpoint)

## Phase 2 - Data Model

- Tenants
- Workers + discovered models
- API tokens + debug mode + quota policy
- Request logs + usage rollups

## Phase 3 - Admin Security

- OIDC Validierung fuer Admin-Endpunkte
- Rollen/Scopes (`admin`, `ops`, `viewer`)
- Audit-Logs fuer Admin-Aktionen

## Phase 4 - Worker Discovery

- Worker registrieren/deaktivieren
- Model discovery (`/v1/models` am Worker)
- Periodische Refresh-Jobs + Healthchecks

## Phase 5 - OpenAI Compatibility

- `/v1/models` (aggregiert)
- `/v1/chat/completions` inkl. SSE Streaming
- `/v1/embeddings`
- `/v1/completions` und `/v1/responses` soweit praktisch kompatibel

## Phase 6 - Routing + Retry

- Scorebasierte Worker-Auswahl
- Transparentes Retry bei Upstream-Fehlern
- Outlier-Detection / Circuit Breaker

## Phase 7 - Governance

- Token-basierte Auth fuer Client-Endpunkte
- Quotas und Rate-Limits (tenant/token/model)
- Debug-Logging Toggle je Token

## Phase 8 - Admin UI

- Worker-Verwaltung
- Token-Management (create/revoke/debug)
- Live-Metriken und Nutzungsansicht

## Phase 9 - Hardening + Release

- E2E Tests (OpenAI SDK kompatibel)
- Lasttests
- Security review
- Deploy Blueprint fuer HA-Betrieb
