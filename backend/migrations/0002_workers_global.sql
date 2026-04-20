-- Allow global workers by making tenant_id optional (NULL means global).
ALTER TABLE workers
  MODIFY COLUMN tenant_id BIGINT UNSIGNED NULL;
