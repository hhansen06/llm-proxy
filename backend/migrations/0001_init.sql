CREATE TABLE IF NOT EXISTS tenants (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_tenants_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS workers (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  tenant_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(128) NOT NULL,
  base_url VARCHAR(512) NOT NULL,
  api_key_encrypted TEXT NULL,
  status ENUM('active','inactive','degraded') NOT NULL DEFAULT 'active',
  capacity_hint INT NOT NULL DEFAULT 1,
  last_health_at TIMESTAMP NULL,
  last_latency_ms INT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_workers_tenant (tenant_id),
  CONSTRAINT fk_workers_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS worker_models (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  worker_id BIGINT UNSIGNED NOT NULL,
  model_name VARCHAR(255) NOT NULL,
  max_context_tokens INT NULL,
  discovered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_worker_model (worker_id, model_name),
  KEY idx_model_name (model_name),
  CONSTRAINT fk_worker_models_worker FOREIGN KEY (worker_id) REFERENCES workers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS api_tokens (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  tenant_id BIGINT UNSIGNED NOT NULL,
  token_hash CHAR(64) NOT NULL,
  label VARCHAR(128) NOT NULL,
  debug_enabled TINYINT(1) NOT NULL DEFAULT 0,
  is_revoked TINYINT(1) NOT NULL DEFAULT 0,
  quota_requests_per_min INT NULL,
  quota_tokens_per_day BIGINT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uq_api_tokens_hash (token_hash),
  KEY idx_api_tokens_tenant (tenant_id),
  CONSTRAINT fk_api_tokens_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS request_logs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  request_id VARCHAR(64) NOT NULL,
  tenant_id BIGINT UNSIGNED NOT NULL,
  token_id BIGINT UNSIGNED NOT NULL,
  worker_id BIGINT UNSIGNED NULL,
  model_name VARCHAR(255) NOT NULL,
  prompt_tokens INT NOT NULL DEFAULT 0,
  completion_tokens INT NOT NULL DEFAULT 0,
  total_tokens INT NOT NULL DEFAULT 0,
  duration_ms INT NOT NULL,
  http_status INT NOT NULL,
  debug_payload LONGTEXT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_request_logs_created_at (created_at),
  KEY idx_request_logs_tenant (tenant_id),
  KEY idx_request_logs_token (token_id),
  KEY idx_request_logs_model (model_name),
  UNIQUE KEY uq_request_id (request_id),
  CONSTRAINT fk_request_logs_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
  CONSTRAINT fk_request_logs_token FOREIGN KEY (token_id) REFERENCES api_tokens(id),
  CONSTRAINT fk_request_logs_worker FOREIGN KEY (worker_id) REFERENCES workers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO tenants (id, name) VALUES (1, 'default');
