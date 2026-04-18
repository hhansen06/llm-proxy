package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type AdminClaims struct {
	Subject string
	Scopes  []string
}

type adminClaimsKey struct{}
		"errors"
		"fmt"

		"github.com/coreos/go-oidc/v3/oidc"

		"llm-proxy/backend/internal/config"
func AdminOIDCRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
		Roles   []string
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "missing admin bearer token"})
			return
		}

	type AdminOIDC struct {
		verifier       *oidc.IDTokenVerifier
		audience       string
		requiredScopes map[string]struct{}
		requiredRoles  map[string]struct{}
		clientID       string
	}

	func NewAdminOIDC(cfg config.Config) (*AdminOIDC, error) {
		if cfg.OIDCIssuerURL == "" {
			return nil, errors.New("missing OIDC_ISSUER_URL")
		}
		if cfg.OIDCClientID == "" {
			return nil, errors.New("missing OIDC_CLIENT_ID")
		}

		provider, err := oidc.NewProvider(context.Background(), cfg.OIDCIssuerURL)
		if err != nil {
			return nil, fmt.Errorf("init oidc provider: %w", err)
		}

		verifyCfg := &oidc.Config{ClientID: cfg.OIDCClientID}
		if cfg.OIDCAudience != "" {
			verifyCfg = &oidc.Config{SkipClientIDCheck: true}
		}

		m := &AdminOIDC{
			verifier:       provider.Verifier(verifyCfg),
			audience:       cfg.OIDCAudience,
			requiredScopes: toSet(cfg.OIDCAdminScopes),
			requiredRoles:  toSet(cfg.OIDCAdminRoles),
			clientID:       cfg.OIDCClientID,
		}

		return m, nil
	}

	func (m *AdminOIDC) Require(next http.Handler) http.Handler {
		if token == "" {
			token, err := parseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, err.Error())

		// Placeholder: OIDC validation (issuer, audience, signature, exp, roles/scopes)
		claims := AdminClaims{Subject: "placeholder-admin", Scopes: []string{"admin"}}
			idToken, err := m.verifier.Verify(r.Context(), token)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "oidc token verification failed")

func AdminClaimsFromContext(ctx context.Context) (AdminClaims, bool) {
	v := ctx.Value(adminClaimsKey{})
			var raw map[string]any
			if err := idToken.Claims(&raw); err != nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid oidc token claims")
				return
			}

			if m.audience != "" && !audienceContains(raw["aud"], m.audience) {
				writeJSONError(w, http.StatusForbidden, "token audience not allowed")
				return
			}

			scopes := extractScopes(raw["scope"])
			roles := extractRoles(raw, m.clientID)
			if !m.authorized(scopes, roles) {
				writeJSONError(w, http.StatusForbidden, "insufficient admin permissions")
				return
			}

			subject, _ := raw["sub"].(string)
			claims := AdminClaims{Subject: subject, Scopes: scopes, Roles: roles}
}

func (m *AdminOIDC) authorized(scopes []string, roles []string) bool {
	if len(m.requiredScopes) == 0 && len(m.requiredRoles) == 0 {
		return true
	}

	for _, scope := range scopes {
		if _, ok := m.requiredScopes[scope]; ok {
			return true
		}
	}

	for _, role := range roles {
		if _, ok := m.requiredRoles[role]; ok {
			return true
		}
	}

	return false
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}

func parseBearerToken(auth string) (string, error) {
	if auth == "" {
		return "", errors.New("missing bearer token")
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid bearer token")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("invalid bearer token")
	}
	return token, nil
}

func audienceContains(raw any, required string) bool {
	switch v := raw.(type) {
	case string:
		return v == required
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == required {
				return true
			}
		}
	}
	return false
}

func extractScopes(scopeClaim any) []string {
	scopeStr, ok := scopeClaim.(string)
	if !ok || scopeStr == "" {
		return nil
	}
	parts := strings.Fields(scopeStr)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func extractRoles(raw map[string]any, clientID string) []string {
	roles := map[string]struct{}{}

	if realmRaw, ok := raw["realm_access"].(map[string]any); ok {
		if rr, ok := realmRaw["roles"].([]any); ok {
			for _, role := range rr {
				if roleStr, ok := role.(string); ok && roleStr != "" {
					roles[roleStr] = struct{}{}
				}
			}
		}
	}

	if resourceRaw, ok := raw["resource_access"].(map[string]any); ok {
		if clientRaw, ok := resourceRaw[clientID].(map[string]any); ok {
			if rr, ok := clientRaw["roles"].([]any); ok {
				for _, role := range rr {
					if roleStr, ok := role.(string); ok && roleStr != "" {
						roles[roleStr] = struct{}{}
					}
				}
			}
		}
	}

	out := make([]string, 0, len(roles))
	for role := range roles {
		out = append(out, role)
	}
	return out
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}
