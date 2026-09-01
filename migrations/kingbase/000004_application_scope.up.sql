ALTER TABLE dictionaries ADD COLUMN application_id TEXT NOT NULL DEFAULT '';
ALTER TABLE dictionaries ADD CONSTRAINT chk_dictionaries_scope CHECK (tenant_id <> '' OR application_id = '');
ALTER TABLE dictionaries DROP CONSTRAINT dictionaries_tenant_id_code_key;
ALTER TABLE dictionaries ADD CONSTRAINT dictionaries_scope_key UNIQUE (tenant_id, application_id, code);
DROP INDEX idx_dictionaries_list;
CREATE INDEX idx_dictionaries_list ON dictionaries (tenant_id, application_id, status, code);
