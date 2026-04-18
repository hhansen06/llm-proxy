package middleware

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"encoding/hex"
	"net/http"
)

type ClientTokenIdentity struct {
	TokenID      int64
	TenantID     int64
	DebugEnabled bool
	QuotaRPM     *int64
	QuotaTPD     *int64
}

type clientIdentityKey struct{}

func ClientBearerRequired(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := parseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				writeClientJSONError(w, http.StatusUnauthorized, err.Error())
				return
			}

			hash := sha256.Sum256([]byte(token))
			hashHex := hex.EncodeToString(hash[:])

			const q = `
				SELECT id, tenant_id, debug_enabled, is_revoked, quota_requests_per_min, quota_tokens_per_day
				FROM api_tokens
				WHERE token_hash = ?
				LIMIT 1`

			var (
				tokenID      int64
				tenantID     int64
				debugEnabled bool
				isRevoked    bool
				quotaRPM     sql.NullInt64
				quotaTPD     sql.NullInt64
			)

			err = db.QueryRowContext(r.Context(), q, hashHex).Scan(&tokenID, &tenantID, &debugEnabled, &isRevoked, &quotaRPM, &quotaTPD)
			if err == sql.ErrNoRows {
				writeClientJSONError(w, http.StatusUnauthorized, "invalid bearer token")
				return
			}
			if err != nil {
				writeClientJSONError(w, http.StatusInternalServerError, "token validation failed")
				return
			}

			if isRevoked {
				writeClientJSONError(w, http.StatusUnauthorized, "token revoked")
				return
			}

			if quotaRPM.Valid {
				const qRPM = `
					SELECT COUNT(1)
					FROM request_logs
					WHERE token_id = ? AND created_at >= (UTC_TIMESTAMP() - INTERVAL 1 MINUTE)`
				var usedRPM int64
				if err := db.QueryRowContext(r.Context(), qRPM, tokenID).Scan(&usedRPM); err != nil {
					writeClientJSONError(w, http.StatusInternalServerError, "quota check failed")
					return
				}
				if usedRPM >= quotaRPM.Int64 {
					writeClientJSONError(w, http.StatusTooManyRequests, "request quota exceeded")
					return
				}
			}

			if quotaTPD.Valid {
				const qTPD = `
					SELECT COALESCE(SUM(total_tokens), 0)
					FROM request_logs
					WHERE token_id = ? AND created_at >= UTC_DATE()`
				var usedTPD int64
				if err := db.QueryRowContext(r.Context(), qTPD, tokenID).Scan(&usedTPD); err != nil {
					writeClientJSONError(w, http.StatusInternalServerError, "quota check failed")
					return
				}
				if usedTPD >= quotaTPD.Int64 {
					writeClientJSONError(w, http.StatusTooManyRequests, "daily token quota exceeded")
					return
				}
			}

			identity := ClientTokenIdentity{TokenID: tokenID, TenantID: tenantID, DebugEnabled: debugEnabled}
			if quotaRPM.Valid {
				v := quotaRPM.Int64
				identity.QuotaRPM = &v
			}
			if quotaTPD.Valid {
				v := quotaTPD.Int64
				identity.QuotaTPD = &v
			}
			ctx := context.WithValue(r.Context(), clientIdentityKey{}, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClientIdentityFromContext(ctx context.Context) (ClientTokenIdentity, bool) {
	v := ctx.Value(clientIdentityKey{})
	identity, ok := v.(ClientTokenIdentity)
	return identity, ok
}

func writeClientJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
