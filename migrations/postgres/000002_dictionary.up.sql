CREATE TABLE dictionaries (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL DEFAULT '',
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    provider_id TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    published_version BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    UNIQUE (tenant_id, code)
);
CREATE INDEX idx_dictionaries_list ON dictionaries (tenant_id, status, code);

CREATE TABLE dictionary_item_drafts (
    id TEXT PRIMARY KEY,
    dictionary_id TEXT NOT NULL REFERENCES dictionaries(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    parent_id TEXT NOT NULL DEFAULT '',
    parent_code TEXT NOT NULL DEFAULT '',
    leaf BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    UNIQUE (dictionary_id, code)
);
CREATE INDEX idx_dictionary_item_draft_tree ON dictionary_item_drafts (dictionary_id, parent_id, sort_order, id);

CREATE TABLE dictionary_releases (
    id TEXT PRIMARY KEY,
    dictionary_id TEXT NOT NULL REFERENCES dictionaries(id),
    release_version BIGINT NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    UNIQUE (dictionary_id, release_version)
);

CREATE TABLE dictionary_release_items (
    id TEXT NOT NULL,
    release_id TEXT NOT NULL REFERENCES dictionary_releases(id),
    dictionary_id TEXT NOT NULL,
    release_version BIGINT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    parent_id TEXT NOT NULL DEFAULT '',
    parent_code TEXT NOT NULL DEFAULT '',
    leaf BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    PRIMARY KEY (release_id, id),
    UNIQUE (dictionary_id, release_version, code)
);
CREATE INDEX idx_dictionary_release_tree ON dictionary_release_items (dictionary_id, release_version, parent_id, sort_order, id);

CREATE TABLE dictionary_providers (
    id TEXT PRIMARY KEY,
    service_name TEXT NOT NULL UNIQUE,
    target TEXT NOT NULL,
    status TEXT NOT NULL,
    capabilities_json TEXT NOT NULL,
    cache_ttl_seconds INTEGER NOT NULL,
    timeout_milliseconds INTEGER NOT NULL,
    lease_token_hash TEXT NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL
);
CREATE INDEX idx_dictionary_provider_lease ON dictionary_providers (status, lease_expires_at);

CREATE TABLE dictionary_outbox_events (
    id TEXT PRIMARY KEY,
    subject TEXT NOT NULL,
    envelope BYTEA NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL
);
CREATE INDEX idx_dictionary_outbox_pending ON dictionary_outbox_events (published_at, available_at);
