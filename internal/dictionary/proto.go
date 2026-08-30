package dictionary

import (
	"encoding/json"

	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ToProtoDictionary(value Dictionary) *dictionaryv1.Dictionary {
	return &dictionaryv1.Dictionary{
		Id:               value.ID,
		TenantId:         value.TenantID,
		Code:             value.Code,
		Name:             value.Name,
		Description:      value.Description,
		Kind:             protoKind(value.Kind),
		Status:           value.Status,
		ProviderId:       value.ProviderID,
		PublishedVersion: value.PublishedVersion,
		Metadata:         toStruct(value.MetadataJSON),
		Audit:            toProtoAudit(value.AuditFields),
	}
}

func ToProtoItem(value Item) *dictionaryv1.DictionaryItem {
	return &dictionaryv1.DictionaryItem{
		Id:             value.ID,
		DictionaryCode: value.DictionaryCode,
		Code:           value.Code,
		Name:           value.Name,
		ParentId:       value.ParentID,
		ParentCode:     value.ParentCode,
		Leaf:           value.Leaf,
		SortOrder:      value.SortOrder,
		Disabled:       value.Disabled,
		Status:         value.Status,
		Metadata:       toStruct(value.MetadataJSON),
		Audit:          toProtoAudit(value.AuditFields),
	}
}

func FromProtoItem(value *dictionaryv1.DictionaryItem) Item {
	if value == nil {
		return Item{}
	}
	item := Item{ID: value.GetId(), DictionaryCode: value.GetDictionaryCode(), Code: value.GetCode(), Name: value.GetName(), ParentID: value.GetParentId(), ParentCode: value.GetParentCode(), Leaf: value.GetLeaf(), SortOrder: value.GetSortOrder(), Disabled: value.GetDisabled(), Status: value.GetStatus(), MetadataJSON: fromStruct(value.GetMetadata())}
	if value.GetAudit() != nil {
		item.Version = value.GetAudit().GetVersion()
	}
	return item
}

func ToProtoTree(nodes []TreeNode) []*dictionaryv1.TreeNode {
	result := make([]*dictionaryv1.TreeNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, &dictionaryv1.TreeNode{Item: ToProtoItem(node.Item), Children: ToProtoTree(node.Children)})
	}
	return result
}

func ToProtoProvider(value Provider) *dictionaryv1.Provider {
	capabilities := []Capability{}
	_ = json.Unmarshal([]byte(value.CapabilitiesJSON), &capabilities)
	protoCapabilities := make([]*dictionaryv1.ProviderCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		protoCapabilities = append(protoCapabilities, ToProtoCapability(capability))
	}
	return &dictionaryv1.Provider{Id: value.ID, ServiceName: value.ServiceName, Target: value.Target, Status: value.Status, Capabilities: protoCapabilities, CacheTtlSeconds: uint32(value.CacheTTLSeconds), TimeoutMilliseconds: uint32(value.TimeoutMilliseconds), LeaseExpiresAt: timestamppb.New(value.LeaseExpiresAt), Audit: toProtoAudit(value.AuditFields)}
}

func ToProtoCapability(value Capability) *dictionaryv1.ProviderCapability {
	return &dictionaryv1.ProviderCapability{DictionaryCode: value.DictionaryCode, SupportsSearch: value.SupportsSearch, SupportsTree: value.SupportsTree, SupportsCursor: value.SupportsCursor, FilterKeys: value.FilterKeys, SortKeys: value.SortKeys, MaxPageSize: uint32(value.MaxPageSize), MaxTreeDepth: uint32(value.MaxTreeDepth), MaxTreeNodes: uint32(value.MaxTreeNodes)}
}

func FromProtoCapability(value *dictionaryv1.ProviderCapability) Capability {
	if value == nil {
		return Capability{}
	}
	return Capability{DictionaryCode: value.GetDictionaryCode(), SupportsSearch: value.GetSupportsSearch(), SupportsTree: value.GetSupportsTree(), SupportsCursor: value.GetSupportsCursor(), FilterKeys: value.GetFilterKeys(), SortKeys: value.GetSortKeys(), MaxPageSize: int32(value.GetMaxPageSize()), MaxTreeDepth: int32(value.GetMaxTreeDepth()), MaxTreeNodes: int32(value.GetMaxTreeNodes())}
}

func toProtoAudit(value AuditFields) *dictionaryv1.AuditFields {
	return &dictionaryv1.AuditFields{Version: value.Version, CreatedAt: timestamppb.New(value.CreatedAt), UpdatedAt: timestamppb.New(value.UpdatedAt), CreatedBy: value.CreatedBy, UpdatedBy: value.UpdatedBy}
}

func protoKind(value string) dictionaryv1.DictionaryKind {
	if value == KindDynamic {
		return dictionaryv1.DictionaryKind_DICTIONARY_KIND_DYNAMIC
	}
	return dictionaryv1.DictionaryKind_DICTIONARY_KIND_STATIC
}

func toStruct(value string) *structpb.Struct {
	result := map[string]any{}
	if json.Unmarshal([]byte(defaultJSON(value)), &result) != nil {
		return &structpb.Struct{Fields: map[string]*structpb.Value{}}
	}
	converted, _ := structpb.NewStruct(result)
	return converted
}

func fromStruct(value *structpb.Struct) string {
	if value == nil {
		return "{}"
	}
	encoded, err := json.Marshal(value.AsMap())
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
