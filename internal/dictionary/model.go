package dictionary

import (
	"context"
	"time"
)

const (
	KindStatic     = "static"
	KindDynamic    = "dynamic"
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusInactive = "inactive"
)

type AuditFields struct {
	Version   int64     `db:"version" json:"version"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	CreatedBy string    `db:"created_by" json:"created_by"`
	UpdatedBy string    `db:"updated_by" json:"updated_by"`
}

type Dictionary struct {
	ID               string `db:"id" json:"id"`
	TenantID         string `db:"tenant_id" json:"tenant_id"`
	Code             string `db:"code" json:"code"`
	Name             string `db:"name" json:"name"`
	Description      string `db:"description" json:"description"`
	Kind             string `db:"kind" json:"kind"`
	Status           string `db:"status" json:"status"`
	ProviderID       string `db:"provider_id" json:"provider_id"`
	MetadataJSON     string `db:"metadata_json" json:"metadata_json"`
	PublishedVersion int64  `db:"published_version" json:"published_version"`
	AuditFields
}

type Item struct {
	ID             string `db:"id" json:"id"`
	DictionaryID   string `db:"dictionary_id" json:"dictionary_id"`
	DictionaryCode string `db:"dictionary_code" json:"dictionary_code,omitempty"`
	Code           string `db:"code" json:"code"`
	Name           string `db:"name" json:"name"`
	ParentID       string `db:"parent_id" json:"parent_id"`
	ParentCode     string `db:"parent_code" json:"parent_code"`
	Leaf           bool   `db:"leaf" json:"leaf"`
	SortOrder      int32  `db:"sort_order" json:"sort_order"`
	Disabled       bool   `db:"disabled" json:"disabled"`
	Status         string `db:"status" json:"status"`
	MetadataJSON   string `db:"metadata_json" json:"metadata_json"`
	AuditFields
}

type Release struct {
	ID             string `db:"id" json:"id"`
	DictionaryID   string `db:"dictionary_id" json:"dictionary_id"`
	ReleaseVersion int64  `db:"release_version" json:"release_version"`
	Comment        string `db:"comment" json:"comment"`
	Status         string `db:"status" json:"status"`
	AuditFields
}

type Provider struct {
	ID                  string    `db:"id" json:"id"`
	ServiceName         string    `db:"service_name" json:"service_name"`
	Target              string    `db:"target" json:"target"`
	Status              string    `db:"status" json:"status"`
	CapabilitiesJSON    string    `db:"capabilities_json" json:"capabilities_json"`
	CacheTTLSeconds     int32     `db:"cache_ttl_seconds" json:"cache_ttl_seconds"`
	TimeoutMilliseconds int32     `db:"timeout_milliseconds" json:"timeout_milliseconds"`
	LeaseTokenHash      string    `db:"lease_token_hash" json:"-"`
	LeaseExpiresAt      time.Time `db:"lease_expires_at" json:"lease_expires_at"`
	AuditFields
}

type Capability struct {
	DictionaryCode string   `json:"dictionary_code"`
	SupportsSearch bool     `json:"supports_search"`
	SupportsTree   bool     `json:"supports_tree"`
	SupportsCursor bool     `json:"supports_cursor"`
	FilterKeys     []string `json:"filter_keys"`
	SortKeys       []string `json:"sort_keys"`
	MaxPageSize    int32    `json:"max_page_size"`
	MaxTreeDepth   int32    `json:"max_tree_depth"`
	MaxTreeNodes   int32    `json:"max_tree_nodes"`
}

type Search struct {
	Keyword    string
	Filters    map[string]string
	Sort       string
	Descending bool
	Page       int
	PageSize   int
	Cursor     string
	Limit      int
}

type ProviderPage struct {
	Items      []Item
	Total      int64
	Page       int
	PageSize   int
	NextCursor string
	HasMore    bool
}

type ProviderGateway interface {
	ValidateTarget(string) error
	Query(context.Context, Provider, string, string, Search) (ProviderPage, error)
	Tree(context.Context, Provider, string, string, string, string, string, int, int, map[string]string) ([]TreeNode, bool, error)
	ResolveCodes(context.Context, Provider, string, string, []string) (map[string]Item, error)
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	Total      int64  `json:"total"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more,omitempty"`
}

type TreeNode struct {
	Item     Item       `json:"item"`
	Children []TreeNode `json:"children"`
}

type OutboxEvent struct {
	ID, Subject                       string
	Envelope                          []byte
	AvailableAt, CreatedAt, UpdatedAt time.Time
	CreatedBy, UpdatedBy              string
}
