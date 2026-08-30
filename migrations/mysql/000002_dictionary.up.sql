CREATE TABLE dictionaries (
    id VARCHAR(36) PRIMARY KEY, tenant_id VARCHAR(36) NOT NULL DEFAULT '', code VARCHAR(128) NOT NULL,
    name VARCHAR(255) NOT NULL, description TEXT NOT NULL, kind VARCHAR(32) NOT NULL, status VARCHAR(32) NOT NULL,
    provider_id VARCHAR(36) NOT NULL DEFAULT '', metadata_json TEXT NOT NULL, published_version BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
    created_by VARCHAR(255) NOT NULL, updated_by VARCHAR(255) NOT NULL,
    UNIQUE KEY uq_dictionary_tenant_code (tenant_id, code), INDEX idx_dictionaries_list (tenant_id, status, code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE dictionary_item_drafts (
    id VARCHAR(36) PRIMARY KEY, dictionary_id VARCHAR(36) NOT NULL, code VARCHAR(128) NOT NULL,
    name VARCHAR(255) NOT NULL, parent_id VARCHAR(36) NOT NULL DEFAULT '', parent_code VARCHAR(128) NOT NULL DEFAULT '',
    leaf BOOLEAN NOT NULL DEFAULT TRUE, sort_order INTEGER NOT NULL DEFAULT 0, disabled BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(32) NOT NULL, metadata_json TEXT NOT NULL, version BIGINT NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL, created_by VARCHAR(255) NOT NULL, updated_by VARCHAR(255) NOT NULL,
    UNIQUE KEY uq_dictionary_item_code (dictionary_id, code), INDEX idx_dictionary_item_draft_tree (dictionary_id, parent_id, sort_order, id),
    CONSTRAINT fk_dictionary_item_dictionary FOREIGN KEY (dictionary_id) REFERENCES dictionaries(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE dictionary_releases (
    id VARCHAR(36) PRIMARY KEY, dictionary_id VARCHAR(36) NOT NULL, release_version BIGINT NOT NULL,
    comment TEXT NOT NULL, status VARCHAR(32) NOT NULL, version BIGINT NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL, created_by VARCHAR(255) NOT NULL, updated_by VARCHAR(255) NOT NULL,
    UNIQUE KEY uq_dictionary_release (dictionary_id, release_version),
    CONSTRAINT fk_dictionary_release_dictionary FOREIGN KEY (dictionary_id) REFERENCES dictionaries(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE dictionary_release_items (
    id VARCHAR(36) NOT NULL, release_id VARCHAR(36) NOT NULL, dictionary_id VARCHAR(36) NOT NULL,
    release_version BIGINT NOT NULL, code VARCHAR(128) NOT NULL, name VARCHAR(255) NOT NULL,
    parent_id VARCHAR(36) NOT NULL DEFAULT '', parent_code VARCHAR(128) NOT NULL DEFAULT '', leaf BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0, disabled BOOLEAN NOT NULL DEFAULT FALSE, status VARCHAR(32) NOT NULL,
    metadata_json TEXT NOT NULL, version BIGINT NOT NULL DEFAULT 1, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
    created_by VARCHAR(255) NOT NULL, updated_by VARCHAR(255) NOT NULL, PRIMARY KEY (release_id, id),
    UNIQUE KEY uq_dictionary_release_item (dictionary_id, release_version, code),
    INDEX idx_dictionary_release_tree (dictionary_id, release_version, parent_id, sort_order, id),
    CONSTRAINT fk_dictionary_release_item_release FOREIGN KEY (release_id) REFERENCES dictionary_releases(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE dictionary_providers (
    id VARCHAR(36) PRIMARY KEY, service_name VARCHAR(128) NOT NULL UNIQUE, target VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL, capabilities_json TEXT NOT NULL, cache_ttl_seconds INTEGER NOT NULL,
    timeout_milliseconds INTEGER NOT NULL, lease_token_hash VARCHAR(64) NOT NULL, lease_expires_at DATETIME(6) NOT NULL,
    version BIGINT NOT NULL DEFAULT 1, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
    created_by VARCHAR(255) NOT NULL, updated_by VARCHAR(255) NOT NULL, INDEX idx_dictionary_provider_lease (status, lease_expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE dictionary_outbox_events (
    id VARCHAR(36) PRIMARY KEY, subject VARCHAR(255) NOT NULL, envelope LONGBLOB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0, available_at DATETIME(6) NOT NULL, published_at DATETIME(6) NULL,
    last_error TEXT NOT NULL, version BIGINT NOT NULL DEFAULT 1, created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
    created_by VARCHAR(255) NOT NULL, updated_by VARCHAR(255) NOT NULL, INDEX idx_dictionary_outbox_pending (published_at, available_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
