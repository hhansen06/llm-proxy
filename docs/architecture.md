# Architektur - LLM Proxy

## Ziele

- Zentraler OpenAI-kompatibler Zugang fuer mehrere vLLM-Worker
- Multi-Tenant-faehiger Betrieb mit tokenbasierter Client-Auth
- Sichere Admin-Steuerung ueber OIDC
- Lastverteilung anhand Health, Latenz und Kapazitaet
- Revisionssicheres Request-Metering und selektives Debug-Logging

## Komponenten

1. API Gateway Layer (OpenAI-kompatible Endpunkte)
2. Admin API (Worker, Tokens, Quotas, Debug-Modus)
3. Worker Registry und Model Discovery
4. Scheduler/Router (Health+Latency+Capacity)
5. AuthN/AuthZ Layer
6. Accounting und Logging Pipeline
7. Admin-UI

## Routing-Strategie (geplant)

Score pro Worker fuer ein Modell:

`score = health_weight * health + latency_weight * normalized_latency + capacity_weight * normalized_capacity`

- `health`: 0 oder 1 (degraded als Zwischenwert moeglich)
- `normalized_latency`: invers normiert, niedriger ist besser
- `normalized_capacity`: aus laufender Auslastung oder konfigurierter Parallelitaet

Auswahl:

1. Nur gesunde Worker mit passendem Modell
2. Kandidat mit bestem Score
3. Bei Fehler: transparenter Retry auf naechstbesten Kandidaten

## Logging-Modi

- Standard: Metadaten (tenant/token-id, model, token counts, worker, duration, status)
- Debug pro Token: erweitert um Request/Response-Payloads (redaction optional)

## HA-Setup

- Mehrere Proxy-Instanzen hinter internem L4/L7 Load Balancer
- Gemeinsame MariaDB und Redis
- Healthchecks und readiness probes fuer Rolling Updates
