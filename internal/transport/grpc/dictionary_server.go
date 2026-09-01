package grpctransport

import (
	"context"
	"encoding/json"

	"github.com/lihongjie0209/dictionary-service/internal/dictionary"
	commonv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/common/v1"
	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type dictionaryServer struct {
	dictionaryv1.UnimplementedDictionaryServiceServer
	service *dictionary.Service
}

func (s *dictionaryServer) CreateDictionary(ctx context.Context, request *dictionaryv1.CreateDictionaryRequest) (*dictionaryv1.CreateDictionaryResponse, error) {
	value, err := s.service.Create(ctx, dictionary.DictionaryInput{TenantID: request.GetTenantId(), ApplicationID: request.GetApplicationId(), Code: request.GetCode(), Name: request.GetName(), Description: request.GetDescription(), MetadataJSON: structJSON(request.GetMetadata())})
	return &dictionaryv1.CreateDictionaryResponse{Dictionary: dictionary.ToProtoDictionary(value)}, err
}
func (s *dictionaryServer) UpdateDictionary(ctx context.Context, request *dictionaryv1.UpdateDictionaryRequest) (*dictionaryv1.UpdateDictionaryResponse, error) {
	value, err := s.service.Update(ctx, request.GetId(), dictionary.DictionaryInput{Name: request.GetName(), Description: request.GetDescription(), Status: request.GetStatus(), MetadataJSON: structJSON(request.GetMetadata())}, request.GetVersion())
	return &dictionaryv1.UpdateDictionaryResponse{Dictionary: dictionary.ToProtoDictionary(value)}, err
}
func (s *dictionaryServer) GetDictionary(ctx context.Context, request *dictionaryv1.GetDictionaryRequest) (*dictionaryv1.GetDictionaryResponse, error) {
	value, err := s.service.Get(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetCode())
	return &dictionaryv1.GetDictionaryResponse{Dictionary: dictionary.ToProtoDictionary(value)}, err
}
func (s *dictionaryServer) ListDictionaries(ctx context.Context, request *dictionaryv1.ListDictionariesRequest) (*dictionaryv1.ListDictionariesResponse, error) {
	page, size := pageValues(request.GetPage())
	value, err := s.service.List(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetStatus(), request.GetKeyword(), page, size)
	items := make([]*dictionaryv1.Dictionary, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, dictionary.ToProtoDictionary(item))
	}
	return &dictionaryv1.ListDictionariesResponse{Dictionaries: items, Page: &commonv1.PageResult{Page: uint32(value.Page), PageSize: uint32(value.PageSize), Total: uint64(value.Total)}}, err
}
func (s *dictionaryServer) UpsertItems(ctx context.Context, request *dictionaryv1.UpsertItemsRequest) (*dictionaryv1.UpsertItemsResponse, error) {
	items := make([]dictionary.Item, 0, len(request.GetItems()))
	for _, item := range request.GetItems() {
		items = append(items, dictionary.FromProtoItem(item))
	}
	values, err := s.service.UpsertItems(ctx, request.GetDictionaryId(), items)
	result := make([]*dictionaryv1.DictionaryItem, 0, len(values))
	for _, item := range values {
		result = append(result, dictionary.ToProtoItem(item))
	}
	return &dictionaryv1.UpsertItemsResponse{Items: result}, err
}
func (s *dictionaryServer) DeleteItem(ctx context.Context, request *dictionaryv1.DeleteItemRequest) (*dictionaryv1.DeleteItemResponse, error) {
	return &dictionaryv1.DeleteItemResponse{}, s.service.DeleteItem(ctx, request.GetId(), request.GetVersion())
}
func (s *dictionaryServer) PublishDictionary(ctx context.Context, request *dictionaryv1.PublishDictionaryRequest) (*dictionaryv1.PublishDictionaryResponse, error) {
	release, values, err := s.service.Publish(ctx, request.GetDictionaryId(), request.GetDictionaryVersion(), request.GetComment())
	items := make([]*dictionaryv1.DictionaryItem, 0, len(values))
	for _, item := range values {
		items = append(items, dictionary.ToProtoItem(item))
	}
	return &dictionaryv1.PublishDictionaryResponse{PublishedVersion: release.ReleaseVersion, Items: items}, err
}
func (s *dictionaryServer) Query(ctx context.Context, request *dictionaryv1.QueryRequest) (*dictionaryv1.QueryResponse, error) {
	search := request.GetSearch()
	page, size := pageValues(nil)
	if search != nil {
		page, size = pageValues(search.GetPage())
	}
	value, err := s.service.Query(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetDictionaryCode(), dictionary.Search{Keyword: search.GetKeyword(), Filters: search.GetFilters(), Sort: search.GetSort(), Descending: search.GetDescending(), Page: page, PageSize: size, Cursor: search.GetCursor(), Limit: int(search.GetLimit())})
	items := make([]*dictionaryv1.DictionaryItem, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, dictionary.ToProtoItem(item))
	}
	return &dictionaryv1.QueryResponse{Items: items, Result: &dictionaryv1.ResultPage{Page: &commonv1.PageResult{Page: uint32(value.Page), PageSize: uint32(value.PageSize), Total: uint64(value.Total)}, NextCursor: value.NextCursor, HasMore: value.HasMore}}, err
}
func (s *dictionaryServer) Tree(ctx context.Context, request *dictionaryv1.TreeRequest) (*dictionaryv1.TreeResponse, error) {
	value, truncated, err := s.service.Tree(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetDictionaryCode(), treeMode(request.GetMode()), request.GetParentId(), request.GetKeyword(), int(request.GetMaxDepth()), int(request.GetMaxNodes()))
	return &dictionaryv1.TreeResponse{Roots: dictionary.ToProtoTree(value), Truncated: truncated}, err
}
func (s *dictionaryServer) ResolveCodes(ctx context.Context, request *dictionaryv1.ResolveCodesRequest) (*dictionaryv1.ResolveCodesResponse, error) {
	values, err := s.service.ResolveCodes(ctx, request.GetTenantId(), request.GetApplicationId(), request.GetDictionaryCode(), request.GetCodes())
	result := make([]*dictionaryv1.ResolvedCode, 0, len(request.GetCodes()))
	for _, code := range request.GetCodes() {
		item, found := values[code]
		result = append(result, &dictionaryv1.ResolvedCode{Code: code, Found: found, Item: dictionary.ToProtoItem(item)})
	}
	return &dictionaryv1.ResolveCodesResponse{Values: result}, err
}
func (s *dictionaryServer) RegisterProvider(ctx context.Context, request *dictionaryv1.RegisterProviderRequest) (*dictionaryv1.RegisterProviderResponse, error) {
	capabilities := make([]dictionary.Capability, 0, len(request.GetCapabilities()))
	for _, capability := range request.GetCapabilities() {
		capabilities = append(capabilities, dictionary.FromProtoCapability(capability))
	}
	value, token, err := s.service.RegisterProvider(ctx, dictionary.ProviderInput{ServiceName: request.GetServiceName(), Target: request.GetTarget(), Capabilities: capabilities, CacheTTLSeconds: int32(request.GetCacheTtlSeconds()), TimeoutMilliseconds: int32(request.GetTimeoutMilliseconds()), LeaseSeconds: int32(request.GetLeaseSeconds())})
	return &dictionaryv1.RegisterProviderResponse{Provider: dictionary.ToProtoProvider(value), LeaseToken: token}, err
}
func (s *dictionaryServer) HeartbeatProvider(ctx context.Context, request *dictionaryv1.HeartbeatProviderRequest) (*dictionaryv1.HeartbeatProviderResponse, error) {
	expires, err := s.service.HeartbeatProvider(ctx, request.GetProviderId(), request.GetLeaseToken(), int32(request.GetLeaseSeconds()))
	return &dictionaryv1.HeartbeatProviderResponse{LeaseExpiresAt: timestamppb.New(expires)}, err
}
func (s *dictionaryServer) UnregisterProvider(ctx context.Context, request *dictionaryv1.UnregisterProviderRequest) (*dictionaryv1.UnregisterProviderResponse, error) {
	return &dictionaryv1.UnregisterProviderResponse{}, s.service.UnregisterProvider(ctx, request.GetProviderId(), request.GetLeaseToken())
}
func (s *dictionaryServer) ListProviders(ctx context.Context, request *dictionaryv1.ListProvidersRequest) (*dictionaryv1.ListProvidersResponse, error) {
	page, size := pageValues(request.GetPage())
	value, err := s.service.ListProviders(ctx, request.GetStatus(), page, size)
	items := make([]*dictionaryv1.Provider, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, dictionary.ToProtoProvider(item))
	}
	return &dictionaryv1.ListProvidersResponse{Providers: items, Page: &commonv1.PageResult{Page: uint32(value.Page), PageSize: uint32(value.PageSize), Total: uint64(value.Total)}}, err
}
func pageValues(value *commonv1.PageRequest) (int, int) {
	if value == nil {
		return 1, 20
	}
	return int(value.GetPage()), int(value.GetPageSize())
}
func treeMode(value dictionaryv1.TreeMode) string {
	switch value {
	case dictionaryv1.TreeMode_TREE_MODE_CHILDREN:
		return "children"
	case dictionaryv1.TreeMode_TREE_MODE_SEARCH_WITH_ANCESTORS:
		return "search_with_ancestors"
	default:
		return "full"
	}
}
func structJSON(value *structpb.Struct) string {
	if value == nil {
		return "{}"
	}
	encoded, err := json.Marshal(value.AsMap())
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
