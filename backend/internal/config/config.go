package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv string

	HTTPAddr string

	DBHost         string
	DBPort         int
	DBName         string
	DBUser         string
	DBPassword     string
	DBMaxOpenConns int
	DBMaxIdleConns int

	RedisAddr string

	WorkerSyncIntervalSec int
	WorkerProbeTimeoutSec int

	OIDCIssuerURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCAudience     string
	OIDCAdminScopes  []string
	OIDCAdminRoles   []string
}

func Load() Config {
	return Config{
		AppEnv: getenv("APP_ENV", "development"),
		HTTPAddr: getenv("HTTP_ADDR", ":8080"),
		DBHost: getenv("DB_HOST", "127.0.0.1"),
		DBPort: mustAtoi(getenv("DB_PORT", "3306")),
		DBName: getenv("DB_NAME", "llm_proxy"),
		DBUser: getenv("DB_USER", "llm_proxy"),
		DBPassword: getenv("DB_PASSWORD", "llm_proxy"),
		DBMaxOpenConns: mustAtoi(getenv("DB_MAX_OPEN_CONNS", "25")),
		DBMaxIdleConns: mustAtoi(getenv("DB_MAX_IDLE_CONNS", "25")),
		RedisAddr: getenv("REDIS_ADDR", "127.0.0.1:6379"),
		WorkerSyncIntervalSec: mustAtoi(getenv("WORKER_SYNC_INTERVAL_SEC", "30")),
		WorkerProbeTimeoutSec: mustAtoi(getenv("WORKER_PROBE_TIMEOUT_SEC", "10")),
		OIDCIssuerURL: getenv("OIDC_ISSUER_URL", ""),
		OIDCClientID: getenv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret: getenv("OIDC_CLIENT_SECRET", ""),
		OIDCAudience: getenv("OIDC_AUDIENCE", ""),
		OIDCAdminScopes: parseCSV(getenv("OIDC_ADMIN_SCOPES", "admin")),
		OIDCAdminRoles: parseCSV(getenv("OIDC_ADMIN_ROLES", "admin")),
	}
}

func (c Config) MySQLDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4,utf8&loc=UTC",
		c.DBUser,
		c.DBPassword,
		c.DBHost,
		c.DBPort,
		c.DBName,
	)
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func mustAtoi(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("invalid integer %q: %v", v, err))
	}
	return n
}

func parseCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
