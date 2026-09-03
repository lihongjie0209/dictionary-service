package dictionary

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lihongjie0209/dictionary-service/internal/apperror"
	"github.com/lihongjie0209/microservice-platform-go/appaccess"
	"github.com/lihongjie0209/microservice-platform-go/principal"
	commonv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/common/v1"
	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"google.golang.org/protobuf/proto"
)

type draftItemsRepository struct {
	Repository
	dictionary Dictionary
	items      []Item
	pageCall   *draftPageCall
}

type draftPageCall struct {
	dictionaryID string
	keyword      string
	limit        int
	offset       int
}

type scopeRepository struct {
	Repository
	values map[string]Dictionary
	calls  []string
}

func (r *scopeRepository) GetDictionary(_ context.Context, tenantID, applicationID, code string) (Dictionary, error) {
	key := tenantID + "/" + applicationID + "/" + code
	r.calls = append(r.calls, key)
	value, ok := r.values[key]
	if !ok {
		return Dictionary{}, ErrNotFound
	}
	return value, nil
}

type applicationVerifier struct{ err error }

func (v applicationVerifier) Verify(context.Context, string, string) error { return v.err }

type outboxRepository struct {
	Repository
	event OutboxEvent
}

func (r *outboxRepository) AddOutbox(_ context.Context, _ sqlx.ExtContext, event OutboxEvent) error {
	r.event = event
	return nil
}

func (r draftItemsRepository) GetDictionaryByID(context.Context, string) (Dictionary, error) {
	return r.dictionary, nil
}

func (r draftItemsRepository) ListDraftItems(context.Context, string) ([]Item, error) {
	return r.items, nil
}

func (r draftItemsRepository) ListDraftItemsPage(_ context.Context, dictionaryID, keyword string, limit, offset int) ([]Item, int64, error) {
	if r.pageCall != nil {
		r.pageCall.dictionaryID = dictionaryID
		r.pageCall.keyword = keyword
		r.pageCall.limit = limit
		r.pageCall.offset = offset
	}
	return r.items, int64(len(r.items)), nil
}

func TestValidateDictionary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   DictionaryInput
		wantErr bool
	}{
		{name: "valid", input: DictionaryInput{Code: "order.status", Name: "Order status", MetadataJSON: `{}`}},
		{name: "invalid code", input: DictionaryInput{Code: "Order Status", Name: "Order status"}, wantErr: true},
		{name: "missing name", input: DictionaryInput{Code: "order.status"}, wantErr: true},
		{name: "invalid metadata", input: DictionaryInput{Code: "order.status", Name: "Order status", MetadataJSON: `{`}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateDictionary(test.input, true)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateDictionary() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestListDraftItemsReturnsEditableStaticItems(t *testing.T) {
	t.Parallel()
	repository := draftItemsRepository{
		dictionary: Dictionary{ID: "dictionary-1", Kind: KindStatic},
		items:      []Item{{ID: "item-1", DictionaryID: "dictionary-1", Code: "active"}},
	}
	service := NewService(repository, nil, nil, nil)
	items, err := service.ListDraftItems(systemContext(t), " dictionary-1 ")
	if err != nil || len(items) != 1 || items[0].ID != "item-1" {
		t.Fatalf("ListDraftItems() = (%+v, %v)", items, err)
	}
}

func TestListDraftItemsPageUsesBoundedDefaults(t *testing.T) {
	t.Parallel()
	call := &draftPageCall{}
	repository := draftItemsRepository{
		dictionary: Dictionary{ID: "dictionary-1", Kind: KindStatic},
		items:      []Item{{ID: "item-1", DictionaryID: "dictionary-1", Code: "active"}},
		pageCall:   call,
	}
	service := NewService(repository, nil, nil, nil)
	page, err := service.ListDraftItemsPage(systemContext(t), " dictionary-1 ", " active ", 0, 0)
	if err != nil || len(page.Items) != 1 || page.Total != 1 || page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("ListDraftItemsPage() = (%+v, %v)", page, err)
	}
	if call.dictionaryID != "dictionary-1" || call.keyword != "active" || call.limit != 20 || call.offset != 0 {
		t.Fatalf("repository page call = %+v", call)
	}
}

func TestListDraftItemsRejectsDynamicDictionary(t *testing.T) {
	t.Parallel()
	service := NewService(draftItemsRepository{dictionary: Dictionary{ID: "dictionary-1", Kind: KindDynamic}}, nil, nil, nil)
	if _, err := service.ListDraftItems(systemContext(t), "dictionary-1"); err == nil {
		t.Fatal("ListDraftItems() accepted a dynamic dictionary")
	}
}

func TestAuthorizeScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		identity      principal.Principal
		tenantID      string
		applicationID string
		allowGlobal   bool
		wantCode      int
	}{
		{name: "matching tenant", identity: principal.Principal{ID: "user-1", Type: principal.TypeUser, TenantID: "tenant-1"}, tenantID: "tenant-1"},
		{name: "matching application", identity: principal.Principal{ID: "user-1", Type: principal.TypeUser, TenantID: "tenant-1"}, tenantID: "tenant-1", applicationID: "application-1"},
		{name: "different tenant", identity: principal.Principal{ID: "user-1", Type: principal.TypeUser, TenantID: "tenant-1"}, tenantID: "tenant-2", wantCode: apperror.CodeForbidden},
		{name: "global dictionary", identity: principal.Principal{ID: "user-1", Type: principal.TypeUser, TenantID: "tenant-1"}, allowGlobal: true},
		{name: "global mutation denied", identity: principal.Principal{ID: "user-1", Type: principal.TypeUser, TenantID: "tenant-1"}, wantCode: apperror.CodeForbidden},
		{name: "service account", identity: principal.Principal{ID: "service-1", Type: principal.TypeServiceAccount}, tenantID: "tenant-2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := principal.WithContext(t.Context(), test.identity)
			service := NewService(nil, nil, nil, nil)
			if got := applicationErrorCode(service.authorizeScope(ctx, test.tenantID, test.applicationID, test.allowGlobal)); got != test.wantCode {
				t.Fatalf("authorizeScope() code = %d, want %d", got, test.wantCode)
			}
		})
	}
}

func TestGetResolvesApplicationThenTenantThenPlatform(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		values map[string]Dictionary
		wantID string
		calls  int
	}{
		{name: "application override", values: map[string]Dictionary{"tenant-1/application-1/order.status": {ID: "application"}}, wantID: "application", calls: 1},
		{name: "tenant fallback", values: map[string]Dictionary{"tenant-1//order.status": {ID: "tenant"}}, wantID: "tenant", calls: 2},
		{name: "platform fallback", values: map[string]Dictionary{"//order.status": {ID: "platform"}}, wantID: "platform", calls: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repository := &scopeRepository{values: test.values}
			service := NewService(repository, nil, nil, nil)
			value, err := service.Get(systemContext(t), "tenant-1", "application-1", "order.status")
			if err != nil || value.ID != test.wantID || len(repository.calls) != test.calls {
				t.Fatalf("Get() = (%+v, %v), calls=%v", value, err, repository.calls)
			}
		})
	}
}

func TestAuthorizeScopeRejectsMissingApplicationGrant(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil, nil)
	service.applications = applicationVerifier{err: appaccess.ErrNotGranted}
	ctx := principal.WithContext(t.Context(), principal.Principal{ID: "user-1", Type: principal.TypeUser, TenantID: "tenant-1"})
	if code := applicationErrorCode(service.authorizeScope(ctx, "tenant-1", "application-1", true)); code != apperror.CodeForbidden {
		t.Fatalf("authorizeScope() code = %d", code)
	}
}

func TestAddEventPreservesApplicationScope(t *testing.T) {
	t.Parallel()

	repository := &outboxRepository{}
	service := NewService(repository, nil, nil, nil)
	at := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	err := service.addEvent(
		t.Context(), nil,
		"platform.dictionary.dictionary.published.v1",
		"platform.dictionary.v1.DictionaryPublished",
		"dictionary-1", "dictionary", "tenant-1", "application-1", "actor-1", at,
		&dictionaryv1.DictionaryPublishedEvent{},
	)
	if err != nil {
		t.Fatal(err)
	}
	var envelope commonv1.EventEnvelope
	if err := proto.Unmarshal(repository.event.Envelope, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.GetTenantId() != "tenant-1" || envelope.GetApplicationId() != "application-1" {
		t.Fatalf("event scope = %q/%q", envelope.GetTenantId(), envelope.GetApplicationId())
	}
}

func systemContext(t *testing.T) context.Context {
	t.Helper()
	return principal.SystemContext(t.Context(), "test-system")
}

func applicationErrorCode(err error) int {
	if err == nil {
		return 0
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return -1
}

func TestBuildTree(t *testing.T) {
	t.Parallel()
	items := []Item{
		{ID: "root", Code: "root", Name: "Root", SortOrder: 1},
		{ID: "child-b", ParentID: "root", Code: "child.b", Name: "Beta", SortOrder: 2},
		{ID: "child-a", ParentID: "root", Code: "child.a", Name: "Alpha", SortOrder: 1},
	}
	roots, truncated, err := buildTree(items, "full", "", "", 8, 100)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(roots) != 1 || len(roots[0].Children) != 2 || roots[0].Children[0].Item.ID != "child-a" {
		t.Fatalf("unexpected tree: roots=%+v truncated=%v", roots, truncated)
	}
}

func TestBuildTreeSearchIncludesAncestors(t *testing.T) {
	t.Parallel()
	items := []Item{{ID: "root", Code: "region", Name: "Region"}, {ID: "city", ParentID: "root", Code: "shanghai", Name: "Shanghai"}}
	roots, _, err := buildTree(items, "search_with_ancestors", "", "shang", 8, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || len(roots[0].Children) != 1 || roots[0].Children[0].Item.ID != "city" {
		t.Fatalf("search tree did not preserve ancestors: %+v", roots)
	}
}

func TestValidateTreeRejectsCycleAndMissingParent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		items []Item
	}{
		{name: "cycle", items: []Item{{ID: "a", ParentID: "b"}, {ID: "b", ParentID: "a"}}},
		{name: "missing parent", items: []Item{{ID: "a", ParentID: "missing"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validateTree(test.items); err == nil {
				t.Fatal("validateTree() error = nil")
			}
		})
	}
}

func TestTranslatePreservesApplicationError(t *testing.T) {
	t.Parallel()
	original := paginationError()
	if got := translate(original); !errors.Is(got, original) {
		t.Fatalf("translate() = %v, want original %v", got, original)
	}
}

func TestValidateProviderAppliesLimits(t *testing.T) {
	t.Parallel()
	value, err := validateProvider(ProviderInput{ServiceName: "order-service", Target: "order-service:9090", Capabilities: []Capability{{DictionaryCode: "order.status", SupportsSearch: true}}, LeaseSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if value.TimeoutMilliseconds != 3000 || value.Capabilities[0].MaxPageSize != 100 {
		t.Fatalf("defaults not applied: %+v", value)
	}
}

func TestValidateProviderRejectsDuplicateDictionary(t *testing.T) {
	t.Parallel()
	_, err := validateProvider(ProviderInput{ServiceName: "order-service", Target: "order-service:9090", Capabilities: []Capability{{DictionaryCode: "order.status"}, {DictionaryCode: "order.status"}}, LeaseSeconds: 60})
	if err == nil {
		t.Fatal("validateProvider() error = nil")
	}
}

func TestLeaseTokenHash(t *testing.T) {
	t.Parallel()
	token, hashed, err := newLeaseToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || hashed == token || hashToken(token) != hashed {
		t.Fatal("lease token was empty, stored in plaintext, or hashed inconsistently")
	}
}

func paginationError() error {
	_, _, err := pagination(1, 101)
	return err
}
