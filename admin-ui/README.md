# Admin UI

Statische SPA fuer die wichtigsten Admin-Aktionen gegen die Proxy-API.

## Funktionen

- Worker registrieren
- Worker refresh/deactivate
- Token erstellen
- Token debug togglen und revoke
- Live-Uebersicht fuer Worker, Modelle und Tokens
- Live Usage Kennzahlen (1m/24h)
- Request-Logs mit Filter (token_id, model, limit)

## Lokaler Start

Beispiel mit Python static server:

```bash
cd admin-ui
python3 -m http.server 5174
```

Dann im Browser:

- http://localhost:5174

Die UI erwartet:

- Backend URL (z. B. http://localhost:8080)
- Gueltiges Admin JWT im OIDC-Format
