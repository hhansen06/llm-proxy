# llm-proxy

OpenAI-kompatibler API-Proxy fuer mehrere vLLM-Instanzen mit zentraler Auth, Multi-Tenant-Betrieb, Lastverteilung und Nutzungsmetriken.

## Kernfeatures (MVP)

- OpenAI-kompatible Endpunkte (`/v1/models`, `/v1/chat/completions`, spaeter auch `/v1/embeddings`)
- Registrierung mehrerer vLLM-Worker per Admin-API
- Automatische Modell-Erkundung je Worker
- Modell-Aggregation und Request-Routing zu passenden Workern
- Health-/Latenz-/Kapazitaetsbasiertes Load-Balancing mit transparentem Retry
- OIDC fuer Admin-Authentifizierung
- Bearer Tokens fuer Clientzugriff, inkl. Quotas und Rate-Limits
- Request-Metering und Audit-Logging (default: nur Metadaten)
- Debug-Modus pro Token fuer erweitertes Logging
- Echtzeit-Metriken fuer Dashboard/Prometheus
- Admin-UI zur Verwaltung von Workern, Tokens und Nutzung

## Aktuell implementierte API-Endpunkte

- Admin (OIDC-geschuetzt):
   - `POST /admin/workers`
   - `GET /admin/workers`
   - `POST /admin/workers/{id}/deactivate`
   - `POST /admin/workers/{id}/refresh`
   - `POST /admin/tokens`
   - `GET /admin/tokens`
   - `POST /admin/tokens/{id}/revoke`
   - `POST /admin/tokens/{id}/debug`
   - `GET /admin/usage/metrics`
   - `GET /admin/requests`
- Monitoring (intern):
   - `GET /metrics`
- Client (Bearer-Token):
   - `GET /v1/models`
   - `POST /v1/chat/completions`
   - `POST /v1/embeddings`
   - `POST /v1/completions`
   - `POST /v1/responses`

## Verhalten

- Worker-Registrierung triggert Model-Discovery gegen den Worker-Endpunkt `GET /v1/models`.
- Ein Hintergrund-Sync prueft periodisch alle nicht-inaktiven Worker und aktualisiert Status, Latenz und Modellliste.
- Requests werden pro Modell auf passende Worker geroutet.
- Mehrere Worker pro Modell werden nach Health/Latenz/Kapazitaet gescored.
- Upstream-Fehler (`5xx` oder Transportfehler) werden transparent auf den naechsten Kandidaten retried.
- Stream-Requests (`stream=true`) werden als echtes passthrough (SSE) mit kontinuierlichem Flush durchgereicht.
- Pro Request wird ein Eintrag in `request_logs` geschrieben.
- Token-Quotas werden in der Auth-Middleware geprueft:
   - Requests pro Minute
   - Gesamt-Token pro Tag
- Debug-Modus pro Token schreibt zusaetzlichen Payload-Log in `debug_payload`.
- Prometheus-Metriken stehen unter `/metrics` bereit (HTTP, Proxy, Upstream, Worker-Sync).

## Kurze API-Beispiele

### Admin JWT von Keycloak holen (curl)

Beispiel-Variablen:

```bash
KEYCLOAK_URL="https://keycloak.example.internal"
REALM="llm-proxy"
CLIENT_ID="llm-proxy-admin"
CLIENT_SECRET="change-me"
PROXY_URL="http://localhost:8080"
```

Variante 1: Client-Credentials-Flow (Service Account)

```bash
ADMIN_JWT=$(curl -sS -X POST \
   "$KEYCLOAK_URL/realms/$REALM/protocol/openid-connect/token" \
   -H "Content-Type: application/x-www-form-urlencoded" \
   --data-urlencode "grant_type=client_credentials" \
   --data-urlencode "client_id=$CLIENT_ID" \
   --data-urlencode "client_secret=$CLIENT_SECRET" \
   | jq -r '.access_token')

echo "$ADMIN_JWT" | cut -c1-40
```

Variante 2: Passwort-Flow (nur wenn in Keycloak explizit erlaubt)

```bash
KC_USER="admin-user"
KC_PASS="admin-pass"

ADMIN_JWT=$(curl -sS -X POST \
   "$KEYCLOAK_URL/realms/$REALM/protocol/openid-connect/token" \
   -H "Content-Type: application/x-www-form-urlencoded" \
   --data-urlencode "grant_type=password" \
   --data-urlencode "client_id=$CLIENT_ID" \
   --data-urlencode "client_secret=$CLIENT_SECRET" \
   --data-urlencode "username=$KC_USER" \
   --data-urlencode "password=$KC_PASS" \
   | jq -r '.access_token')
```

Mit dem JWT gegen die Admin-API:

```bash
curl -sS -X GET "$PROXY_URL/admin/workers" \
   -H "Authorization: Bearer $ADMIN_JWT"
```

Token fuer Clients erzeugen:

```bash
curl -sS -X POST "$PROXY_URL/admin/tokens" \
   -H "Authorization: Bearer $ADMIN_JWT" \
   -H "Content-Type: application/json" \
   -d '{
      "tenant_id": 1,
      "label": "team-a",
      "debug_enabled": false,
      "quota_requests_per_min": 60,
      "quota_tokens_per_day": 1000000
   }'
```

Worker registrieren:

```bash
curl -X POST http://localhost:8080/admin/workers \
   -H "Authorization: Bearer <admin-oidc-jwt>" \
   -H "Content-Type: application/json" \
   -d '{
      "tenant_id": 1,
      "name": "vllm-a",
      "base_url": "http://10.0.0.10:8000",
      "api_key": "",
      "capacity_hint": 4
   }'
```

Token erzeugen:

```bash
curl -X POST http://localhost:8080/admin/tokens \
   -H "Authorization: Bearer <admin-oidc-jwt>" \
   -H "Content-Type: application/json" \
   -d '{
      "tenant_id": 1,
      "label": "team-a",
      "debug_enabled": false,
      "quota_requests_per_min": 60,
      "quota_tokens_per_day": 1000000
   }'
```

Chat Completion ueber Proxy:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
   -H "Authorization: Bearer <client-token>" \
   -H "Content-Type: application/json" \
   -d '{
      "model": "Qwen/Qwen2.5-7B-Instruct",
      "messages": [{"role":"user","content":"Hallo"}],
      "stream": false
   }'
```

## Geplante Projektstruktur

- `backend/`: Go API-Server
- `admin-ui/`: Web UI fuer Admin-Aufgaben
- `docs/`: Architektur, API, Betriebsdokumentation

## Lokaler Start (fruehes Scaffold)

1. Docker Compose starten:
   - `docker compose up -d --build`
2. Backend Health pruefen:
   - `curl http://localhost:8080/healthz`

### MITM/Private CA Build-Umgebung

Der Backend-Build uebernimmt CA-Zertifikate vom Host-Pfad `/usr/local/share/ca-certificates` in den Build-Container und fuehrt `update-ca-certificates` aus. In `docker-compose.yml` ist dafuer ein zusaetzlicher Build-Context (`host_certs`) hinterlegt.

## Naechste Implementierungsschritte

1. MariaDB-Schema und Migrationen finalisieren
2. OIDC-Integration (Admin API)
3. Worker Discovery gegen vLLM-Endpunkte
4. OpenAI-kompatibles Forwarding inkl. Streaming
5. Quota/Rate-Limit und Request-Logging
6. Admin-UI anbinden

## Tests

- Unit-Test-Grundgeruest ist vorhanden in:
   - `backend/internal/http/handlers/openai_test.go`

Lokal ausfuehren (wenn Go installiert ist):

```bash
cd backend
go test ./...
```

## HA Deployment (Kubernetes)

Ein erstes HA-Setup liegt unter:

- `deploy/k8s`

Enthaelt Deployment (3 Replikas), Service, PDB, HPA, NetworkPolicy sowie ConfigMap/Secret-Template.

Kurzanleitung:

```bash
kubectl apply -k deploy/k8s
```

Vorher Secret und Infrastruktur-Hosts in den Manifests anpassen.
