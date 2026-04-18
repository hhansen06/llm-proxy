# Kubernetes Deployment (HA)

Dieses Verzeichnis enthaelt ein HA-Startsetup fuer den LLM Proxy.

## Enthalten

- Deployment mit 3 Replikas
- Service fuer internen Zugriff
- PDB fuer kontrollierte Evictions
- HPA fuer autoscaling
- NetworkPolicy fuer internen Zugriff
- ConfigMap und Secret-Template

## Anwenden

1. Secret anpassen:

```bash
cp deploy/k8s/secret.example.yaml deploy/k8s/secret.yaml
# Werte in deploy/k8s/secret.yaml setzen
```

2. Kustomization auf secret.yaml umstellen (secret.example entfernen).

3. Ausrollen:

```bash
kubectl apply -k deploy/k8s
```

## Wichtige Hinweise

- `DB_HOST` und `REDIS_ADDR` in configmap auf eure Services anpassen.
- `OIDC_ISSUER_URL` intern erreichbar halten.
- Image `ghcr.io/example/llm-proxy:latest` auf euer Registry-Image setzen.
