CREATE INDEX dictionary_outbox_retention_idx ON dictionary_outbox_events (published_at, id) WHERE published_at IS NOT NULL;
