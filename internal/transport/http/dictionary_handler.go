package httptransport

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lihongjie0209/dictionary-service/internal/apperror"
	"github.com/lihongjie0209/dictionary-service/internal/dictionary"
)

type DictionaryView struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenant_id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Kind             string          `json:"kind"`
	Status           string          `json:"status"`
	ProviderID       string          `json:"provider_id"`
	MetadataJSON     json.RawMessage `json:"metadata_json" swaggertype:"object"`
	PublishedVersion int64           `json:"published_version"`
	Version          int64           `json:"version"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	CreatedBy        string          `json:"created_by"`
	UpdatedBy        string          `json:"updated_by"`
}
type ItemView struct {
	ID             string          `json:"id"`
	DictionaryCode string          `json:"dictionary_code"`
	Code           string          `json:"code"`
	Name           string          `json:"name"`
	ParentID       string          `json:"parent_id"`
	ParentCode     string          `json:"parent_code"`
	Status         string          `json:"status"`
	MetadataJSON   json.RawMessage `json:"metadata_json" swaggertype:"object"`
	Leaf           bool            `json:"leaf"`
	Disabled       bool            `json:"disabled"`
	SortOrder      int32           `json:"sort_order"`
	Version        int64           `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	CreatedBy      string          `json:"created_by"`
	UpdatedBy      string          `json:"updated_by"`
}
type CreateDictionaryRequest struct {
	TenantID     string          `json:"tenant_id"`
	Code         string          `json:"code" binding:"required"`
	Name         string          `json:"name" binding:"required"`
	Description  string          `json:"description"`
	MetadataJSON json.RawMessage `json:"metadata_json" swaggertype:"object"`
}
type UpdateDictionaryRequest struct {
	ID           string          `json:"id" binding:"required"`
	Name         string          `json:"name" binding:"required"`
	Description  string          `json:"description"`
	Status       string          `json:"status" binding:"required"`
	MetadataJSON json.RawMessage `json:"metadata_json" swaggertype:"object"`
	Version      int64           `json:"version" binding:"required"`
}
type GetDictionaryRequest struct {
	TenantID string `json:"tenant_id"`
	Code     string `json:"code" binding:"required"`
}
type ListDictionariesRequest struct {
	TenantID string `json:"tenant_id"`
	Status   string `json:"status"`
	Keyword  string `json:"keyword"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
type UpsertItemsRequest struct {
	DictionaryID string     `json:"dictionary_id" binding:"required"`
	Items        []ItemView `json:"items" binding:"required"`
}
type ListDraftItemsRequest struct {
	DictionaryID string `json:"dictionary_id" binding:"required"`
}
type DeleteItemRequest struct {
	ID      string `json:"id" binding:"required"`
	Version int64  `json:"version" binding:"required"`
}
type PublishDictionaryRequest struct {
	DictionaryID      string `json:"dictionary_id" binding:"required"`
	DictionaryVersion int64  `json:"dictionary_version" binding:"required"`
	Comment           string `json:"comment"`
}
type QueryDictionaryRequest struct {
	TenantID       string            `json:"tenant_id"`
	DictionaryCode string            `json:"dictionary_code" binding:"required"`
	Keyword        string            `json:"keyword"`
	Sort           string            `json:"sort"`
	Filters        map[string]string `json:"filters"`
	Descending     bool              `json:"descending"`
	Page           int               `json:"page"`
	PageSize       int               `json:"page_size"`
	Cursor         string            `json:"cursor"`
	Limit          int               `json:"limit"`
}
type TreeDictionaryRequest struct {
	TenantID       string `json:"tenant_id"`
	DictionaryCode string `json:"dictionary_code" binding:"required"`
	Mode           string `json:"mode" binding:"required"`
	ParentID       string `json:"parent_id"`
	Keyword        string `json:"keyword"`
	MaxDepth       int    `json:"max_depth"`
	MaxNodes       int    `json:"max_nodes"`
}
type ResolveCodesRequest struct {
	TenantID       string   `json:"tenant_id"`
	DictionaryCode string   `json:"dictionary_code" binding:"required"`
	Codes          []string `json:"codes" binding:"required"`
}
type CapabilityRequest struct {
	DictionaryCode string   `json:"dictionary_code" binding:"required"`
	SupportsSearch bool     `json:"supports_search"`
	SupportsTree   bool     `json:"supports_tree"`
	SupportsCursor bool     `json:"supports_cursor"`
	FilterKeys     []string `json:"filter_keys"`
	SortKeys       []string `json:"sort_keys"`
	MaxPageSize    int32    `json:"max_page_size"`
	MaxTreeDepth   int32    `json:"max_tree_depth"`
	MaxTreeNodes   int32    `json:"max_tree_nodes"`
}
type RegisterProviderRequest struct {
	ServiceName         string              `json:"service_name" binding:"required"`
	Target              string              `json:"target" binding:"required"`
	Capabilities        []CapabilityRequest `json:"capabilities" binding:"required"`
	CacheTTLSeconds     int32               `json:"cache_ttl_seconds"`
	TimeoutMilliseconds int32               `json:"timeout_milliseconds"`
	LeaseSeconds        int32               `json:"lease_seconds"`
}
type ProviderLeaseRequest struct {
	ProviderID   string `json:"provider_id" binding:"required"`
	LeaseToken   string `json:"lease_token" binding:"required"`
	LeaseSeconds int32  `json:"lease_seconds"`
}
type ListProvidersRequest struct {
	Status   string `json:"status"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
type ProviderView struct {
	ID                  string          `json:"id"`
	ServiceName         string          `json:"service_name"`
	Target              string          `json:"target"`
	Status              string          `json:"status"`
	CapabilitiesJSON    json.RawMessage `json:"capabilities_json" swaggertype:"array,object"`
	CacheTTLSeconds     int32           `json:"cache_ttl_seconds"`
	TimeoutMilliseconds int32           `json:"timeout_milliseconds"`
	LeaseExpiresAt      time.Time       `json:"lease_expires_at"`
	Version             int64           `json:"version"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	CreatedBy           string          `json:"created_by"`
	UpdatedBy           string          `json:"updated_by"`
}
type DictionaryPageView struct {
	Items    []DictionaryView `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}
type ItemListView struct {
	Items []ItemView `json:"items"`
}
type PublishView struct {
	ReleaseVersion int64      `json:"release_version"`
	Items          []ItemView `json:"items"`
}
type QueryPageView struct {
	Items      []ItemView `json:"items"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	NextCursor string     `json:"next_cursor"`
	HasMore    bool       `json:"has_more"`
}
type TreeView struct {
	Item     ItemView   `json:"item"`
	Children []TreeView `json:"children"`
}
type TreeResultView struct {
	Roots     []TreeView `json:"roots"`
	Truncated bool       `json:"truncated"`
}
type ResolvedCodeView struct {
	Code  string   `json:"code"`
	Found bool     `json:"found"`
	Item  ItemView `json:"item"`
}
type ProviderRegistrationView struct {
	Provider   ProviderView `json:"provider"`
	LeaseToken string       `json:"lease_token"`
}
type ProviderLeaseView struct {
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}
type ProviderPageView struct {
	Items    []ProviderView `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// CreateDictionary godoc
// @Summary Create a static dictionary draft
// @Tags dictionaries
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body CreateDictionaryRequest true "Dictionary draft"
// @Success 200 {object} Response{body=DictionaryView}
// @Failure 400 {object} Response "Code 10001: invalid request"
// @Failure 409 {object} Response "Code 30009: duplicate dictionary"
// @Router /api/v1/dictionaries/create [post]
func (h *Handler) CreateDictionary(c *gin.Context) {
	var request CreateDictionaryRequest
	if !bind(c, h.logger, &request) {
		return
	}
	value, err := h.dictionaries.Create(c.Request.Context(), dictionary.DictionaryInput{TenantID: request.TenantID, Code: request.Code, Name: request.Name, Description: request.Description, MetadataJSON: string(rawJSON(request.MetadataJSON, false))})
	respond(c, h.logger, dictionaryView(value), err)
}

// UpdateDictionary godoc
// @Summary Update a dictionary using optimistic locking
// @Tags dictionaries
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body UpdateDictionaryRequest true "Dictionary and expected version"
// @Success 200 {object} Response{body=DictionaryView}
// @Failure 409 {object} Response "Code 30009: stale version"
// @Router /api/v1/dictionaries/update [post]
func (h *Handler) UpdateDictionary(c *gin.Context) {
	var request UpdateDictionaryRequest
	if !bind(c, h.logger, &request) {
		return
	}
	value, err := h.dictionaries.Update(c.Request.Context(), request.ID, dictionary.DictionaryInput{Name: request.Name, Description: request.Description, Status: request.Status, MetadataJSON: string(rawJSON(request.MetadataJSON, false))}, request.Version)
	respond(c, h.logger, dictionaryView(value), err)
}

// GetDictionary godoc
// @Summary Get a static or dynamic dictionary definition
// @Tags dictionaries
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body GetDictionaryRequest true "Tenant and dictionary code"
// @Success 200 {object} Response{body=DictionaryView}
// @Failure 404 {object} Response "Code 10004: dictionary not found"
// @Router /api/v1/dictionaries/get [post]
func (h *Handler) GetDictionary(c *gin.Context) {
	var request GetDictionaryRequest
	if !bind(c, h.logger, &request) {
		return
	}
	value, err := h.dictionaries.Get(c.Request.Context(), request.TenantID, request.Code)
	respond(c, h.logger, dictionaryView(value), err)
}

// ListDictionaries godoc
// @Summary List dictionary definitions
// @Tags dictionaries
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ListDictionariesRequest true "Search and pagination"
// @Success 200 {object} Response{body=DictionaryPageView}
// @Router /api/v1/dictionaries/list [post]
func (h *Handler) ListDictionaries(c *gin.Context) {
	var request ListDictionariesRequest
	if !bind(c, h.logger, &request) {
		return
	}
	value, err := h.dictionaries.List(c.Request.Context(), request.TenantID, request.Status, request.Keyword, request.Page, request.PageSize)
	items := make([]DictionaryView, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, dictionaryView(item))
	}
	respond(c, h.logger, gin.H{"items": items, "total": value.Total, "page": value.Page, "page_size": value.PageSize}, err)
}

// UpsertItems godoc
// @Summary Upsert dictionary draft items
// @Tags dictionary-items
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body UpsertItemsRequest true "Draft items and expected versions"
// @Success 200 {object} Response{body=ItemListView}
// @Failure 409 {object} Response "Code 30009: stale item version"
// @Router /api/v1/dictionaries/items/upsert [post]
func (h *Handler) UpsertItems(c *gin.Context) {
	var request UpsertItemsRequest
	if !bind(c, h.logger, &request) {
		return
	}
	items := make([]dictionary.Item, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, itemDomain(item))
	}
	values, err := h.dictionaries.UpsertItems(c.Request.Context(), request.DictionaryID, items)
	respond(c, h.logger, itemViews(values), err)
}

// ListDraftItems godoc
// @Summary List editable dictionary draft items
// @Tags dictionary-items
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ListDraftItemsRequest true "Dictionary ID"
// @Success 200 {object} Response{body=ItemListView}
// @Router /api/v1/dictionaries/items/list [post]
func (h *Handler) ListDraftItems(c *gin.Context) {
	var request ListDraftItemsRequest
	if !bind(c, h.logger, &request) {
		return
	}
	values, err := h.dictionaries.ListDraftItems(c.Request.Context(), request.DictionaryID)
	respond(c, h.logger, gin.H{"items": itemViews(values)}, err)
}

// DeleteItem godoc
// @Summary Delete a dictionary draft item using optimistic locking
// @Tags dictionary-items
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body DeleteItemRequest true "Item and expected version"
// @Success 200 {object} Response
// @Failure 409 {object} Response "Code 30009: stale item version"
// @Router /api/v1/dictionaries/items/delete [post]
func (h *Handler) DeleteItem(c *gin.Context) {
	var request DeleteItemRequest
	if !bind(c, h.logger, &request) {
		return
	}
	respond(c, h.logger, gin.H{}, h.dictionaries.DeleteItem(c.Request.Context(), request.ID, request.Version))
}

// PublishDictionary godoc
// @Summary Publish an immutable dictionary release
// @Tags dictionaries
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body PublishDictionaryRequest true "Dictionary expected version"
// @Success 200 {object} Response{body=PublishView}
// @Failure 409 {object} Response "Code 30009: stale version or concurrent publication"
// @Router /api/v1/dictionaries/publish [post]
func (h *Handler) PublishDictionary(c *gin.Context) {
	var request PublishDictionaryRequest
	if !bind(c, h.logger, &request) {
		return
	}
	release, items, err := h.dictionaries.Publish(c.Request.Context(), request.DictionaryID, request.DictionaryVersion, request.Comment)
	respond(c, h.logger, gin.H{"release_version": release.ReleaseVersion, "items": itemViews(items)}, err)
}

// QueryDictionary godoc
// @Summary Query a static release or registered dynamic dictionary
// @Tags dictionary-query
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body QueryDictionaryRequest true "Allow-listed search, filter, sort and pagination"
// @Success 200 {object} Response{body=QueryPageView}
// @Failure 503 {object} Response "Code 50003: provider unavailable"
// @Router /api/v1/dictionaries/query [post]
func (h *Handler) QueryDictionary(c *gin.Context) {
	var request QueryDictionaryRequest
	if !bind(c, h.logger, &request) {
		return
	}
	value, err := h.dictionaries.Query(c.Request.Context(), request.TenantID, request.DictionaryCode, dictionary.Search{Keyword: request.Keyword, Filters: request.Filters, Sort: request.Sort, Descending: request.Descending, Page: request.Page, PageSize: request.PageSize, Cursor: request.Cursor, Limit: request.Limit})
	respond(c, h.logger, gin.H{"items": itemViews(value.Items), "total": value.Total, "page": value.Page, "page_size": value.PageSize, "next_cursor": value.NextCursor, "has_more": value.HasMore}, err)
}

// TreeDictionary godoc
// @Summary Read a bounded dictionary tree
// @Tags dictionary-query
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body TreeDictionaryRequest true "Tree mode and bounds"
// @Success 200 {object} Response{body=TreeResultView}
// @Router /api/v1/dictionaries/tree [post]
func (h *Handler) TreeDictionary(c *gin.Context) {
	var request TreeDictionaryRequest
	if !bind(c, h.logger, &request) {
		return
	}
	value, truncated, err := h.dictionaries.Tree(c.Request.Context(), request.TenantID, request.DictionaryCode, request.Mode, request.ParentID, request.Keyword, request.MaxDepth, request.MaxNodes)
	respond(c, h.logger, gin.H{"roots": treeViews(value), "truncated": truncated}, err)
}

// ResolveCodes godoc
// @Summary Resolve dictionary codes in one request
// @Tags dictionary-query
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ResolveCodesRequest true "Codes to resolve"
// @Success 200 {object} Response{body=[]ResolvedCodeView}
// @Router /api/v1/dictionaries/resolve [post]
func (h *Handler) ResolveCodes(c *gin.Context) {
	var request ResolveCodesRequest
	if !bind(c, h.logger, &request) {
		return
	}
	values, err := h.dictionaries.ResolveCodes(c.Request.Context(), request.TenantID, request.DictionaryCode, request.Codes)
	result := make([]gin.H, 0, len(request.Codes))
	for _, code := range request.Codes {
		item, found := values[code]
		result = append(result, gin.H{"code": code, "found": found, "item": itemView(item)})
	}
	respond(c, h.logger, result, err)
}

// RegisterProvider godoc
// @Summary Register or replace a dynamic dictionary provider lease
// @Tags dictionary-providers
// @Accept json
// @Produce json
// @Security PSK
// @Param request body RegisterProviderRequest true "Provider target and capabilities"
// @Success 200 {object} Response{body=ProviderRegistrationView}
// @Router /api/v1/dictionaries/providers/register [post]
func (h *Handler) RegisterProvider(c *gin.Context) {
	var request RegisterProviderRequest
	if !bind(c, h.logger, &request) {
		return
	}
	capabilities := make([]dictionary.Capability, 0, len(request.Capabilities))
	for _, value := range request.Capabilities {
		capabilities = append(capabilities, dictionary.Capability{DictionaryCode: value.DictionaryCode, SupportsSearch: value.SupportsSearch, SupportsTree: value.SupportsTree, SupportsCursor: value.SupportsCursor, FilterKeys: value.FilterKeys, SortKeys: value.SortKeys, MaxPageSize: value.MaxPageSize, MaxTreeDepth: value.MaxTreeDepth, MaxTreeNodes: value.MaxTreeNodes})
	}
	provider, token, err := h.dictionaries.RegisterProvider(c.Request.Context(), dictionary.ProviderInput{ServiceName: request.ServiceName, Target: request.Target, Capabilities: capabilities, CacheTTLSeconds: request.CacheTTLSeconds, TimeoutMilliseconds: request.TimeoutMilliseconds, LeaseSeconds: request.LeaseSeconds})
	respond(c, h.logger, gin.H{"provider": providerView(provider), "lease_token": token}, err)
}

// HeartbeatProvider godoc
// @Summary Renew a dynamic provider lease
// @Tags dictionary-providers
// @Accept json
// @Produce json
// @Security PSK
// @Param request body ProviderLeaseRequest true "Provider lease token"
// @Success 200 {object} Response{body=ProviderLeaseView}
// @Router /api/v1/dictionaries/providers/heartbeat [post]
func (h *Handler) HeartbeatProvider(c *gin.Context) {
	var request ProviderLeaseRequest
	if !bind(c, h.logger, &request) {
		return
	}
	expires, err := h.dictionaries.HeartbeatProvider(c.Request.Context(), request.ProviderID, request.LeaseToken, request.LeaseSeconds)
	respond(c, h.logger, gin.H{"lease_expires_at": expires}, err)
}

// UnregisterProvider godoc
// @Summary Mark a dynamic provider inactive
// @Tags dictionary-providers
// @Accept json
// @Produce json
// @Security PSK
// @Param request body ProviderLeaseRequest true "Provider lease token"
// @Success 200 {object} Response
// @Router /api/v1/dictionaries/providers/unregister [post]
func (h *Handler) UnregisterProvider(c *gin.Context) {
	var request ProviderLeaseRequest
	if !bind(c, h.logger, &request) {
		return
	}
	respond(c, h.logger, gin.H{}, h.dictionaries.UnregisterProvider(c.Request.Context(), request.ProviderID, request.LeaseToken))
}

// ListProviders godoc
// @Summary List registered dynamic providers
// @Tags dictionary-providers
// @Accept json
// @Produce json
// @Security Bearer
// @Param request body ListProvidersRequest true "Provider status and pagination"
// @Success 200 {object} Response{body=ProviderPageView}
// @Router /api/v1/dictionaries/providers/list [post]
func (h *Handler) ListProviders(c *gin.Context) {
	var request ListProvidersRequest
	if !bind(c, h.logger, &request) {
		return
	}
	value, err := h.dictionaries.ListProviders(c.Request.Context(), request.Status, request.Page, request.PageSize)
	items := make([]ProviderView, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, providerView(item))
	}
	respond(c, h.logger, gin.H{"items": items, "total": value.Total, "page": value.Page, "page_size": value.PageSize}, err)
}

func dictionaryView(value dictionary.Dictionary) DictionaryView {
	return DictionaryView{ID: value.ID, TenantID: value.TenantID, Code: value.Code, Name: value.Name, Description: value.Description, Kind: value.Kind, Status: value.Status, ProviderID: value.ProviderID, MetadataJSON: rawJSON(json.RawMessage(value.MetadataJSON), false), PublishedVersion: value.PublishedVersion, Version: value.Version, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy}
}
func itemView(value dictionary.Item) ItemView {
	return ItemView{ID: value.ID, DictionaryCode: value.DictionaryCode, Code: value.Code, Name: value.Name, ParentID: value.ParentID, ParentCode: value.ParentCode, Leaf: value.Leaf, SortOrder: value.SortOrder, Disabled: value.Disabled, Status: value.Status, MetadataJSON: rawJSON(json.RawMessage(value.MetadataJSON), false), Version: value.Version, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy}
}
func itemViews(values []dictionary.Item) []ItemView {
	result := make([]ItemView, 0, len(values))
	for _, value := range values {
		result = append(result, itemView(value))
	}
	return result
}
func itemDomain(value ItemView) dictionary.Item {
	return dictionary.Item{ID: value.ID, Code: value.Code, Name: value.Name, ParentID: value.ParentID, ParentCode: value.ParentCode, Leaf: value.Leaf, SortOrder: value.SortOrder, Disabled: value.Disabled, Status: value.Status, MetadataJSON: string(rawJSON(value.MetadataJSON, false)), AuditFields: dictionary.AuditFields{Version: value.Version}}
}
func treeViews(values []dictionary.TreeNode) []gin.H {
	result := make([]gin.H, 0, len(values))
	for _, value := range values {
		result = append(result, gin.H{"item": itemView(value.Item), "children": treeViews(value.Children)})
	}
	return result
}
func providerView(value dictionary.Provider) ProviderView {
	return ProviderView{ID: value.ID, ServiceName: value.ServiceName, Target: value.Target, Status: value.Status, CapabilitiesJSON: rawJSON(json.RawMessage(value.CapabilitiesJSON), true), CacheTTLSeconds: value.CacheTTLSeconds, TimeoutMilliseconds: value.TimeoutMilliseconds, LeaseExpiresAt: value.LeaseExpiresAt, Version: value.Version, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy}
}

func rawJSON(value json.RawMessage, array bool) json.RawMessage {
	if len(value) > 0 && json.Valid(value) {
		return value
	}
	if array {
		return json.RawMessage(`[]`)
	}
	return json.RawMessage(`{}`)
}
func bind(c *gin.Context, logger *slog.Logger, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		Fail(c, logger, apperror.Invalid("invalid request", err))
		return false
	}
	return true
}
func respond(c *gin.Context, logger *slog.Logger, body any, err error) {
	if err != nil {
		Fail(c, logger, err)
		return
	}
	OK(c, body)
}
