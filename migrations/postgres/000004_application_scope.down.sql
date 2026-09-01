DROP INDEX idx_dictionaries_list;
CREATE INDEX idx_dictionaries_list ON dictionaries (tenant_id, status, code);
ALTER TABLE dictionaries DROP CONSTRAINT dictionaries_scope_key;
ALTER TABLE dictionaries ADD CONSTRAINT dictionaries_tenant_id_code_key UNIQUE (tenant_id, code);
ALTER TABLE dictionaries DROP CONSTRAINT chk_dictionaries_scope;
ALTER TABLE dictionaries DROP COLUMN application_id;
