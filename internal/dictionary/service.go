package dictionary

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lihongjie0209/dictionary-service/internal/apperror"
	"github.com/lihongjie0209/dictionary-service/internal/cache"
	"github.com/lihongjie0209/dictionary-service/internal/database"
	"github.com/lihongjie0209/microservice-platform-go/appaccess"
	platformevents "github.com/lihongjie0209/microservice-platform-go/eventbus"
	"github.com/lihongjie0209/microservice-platform-go/principal"
	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"google.golang.org/protobuf/proto"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,127}$`)

type Service struct {
	repository   Repository
	transactor   *database.Transactor
	locker       *cache.Locker
	gateway      ProviderGateway
	applications appaccess.Verifier
	now          func() time.Time
}

type allowAllApplications struct{}

func (allowAllApplications) Verify(context.Context, string, string) error { return nil }

func NewService(repository Repository, transactor *database.Transactor, locker *cache.Locker, gateway ProviderGateway) *Service {
	return &Service{repository: repository, transactor: transactor, locker: locker, gateway: gateway, applications: allowAllApplications{}, now: time.Now}
}

func NewRuntimeService(repository Repository, transactor *database.Transactor, locker *cache.Locker, gateway ProviderGateway, applications appaccess.Verifier) (*Service, error) {
	if applications == nil {
		return nil, errors.New("application verifier is required")
	}
	service := NewService(repository, transactor, locker, gateway)
	service.applications = applications
	return service, nil
}

type DictionaryInput struct {
	TenantID, ApplicationID, Code, Name, Description, Status, MetadataJSON string
}

type ProviderInput struct {
	ServiceName         string
	Target              string
	Capabilities        []Capability
	CacheTTLSeconds     int32
	TimeoutMilliseconds int32
	LeaseSeconds        int32
}

func (s *Service) Create(ctx context.Context, input DictionaryInput) (Dictionary, error) {
	actor, err := actor(ctx)
	if err != nil {
		return Dictionary{}, err
	}
	if err := s.authorizeScope(ctx, input.TenantID, input.ApplicationID, false); err != nil {
		return Dictionary{}, err
	}
	input, err = validateDictionary(input, true)
	if err != nil {
		return Dictionary{}, err
	}
	now := s.now()
	value := Dictionary{ID: uuid.NewString(), TenantID: input.TenantID, ApplicationID: input.ApplicationID, Code: input.Code, Name: input.Name, Description: input.Description, Kind: KindStatic, Status: StatusDraft, MetadataJSON: defaultJSON(input.MetadataJSON), AuditFields: AuditFields{Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: actor, UpdatedBy: actor}}
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error { return s.repository.CreateDictionary(ctx, tx, value) })
	return value, translate(err)
}

func (s *Service) Update(ctx context.Context, id string, input DictionaryInput, expected int64) (Dictionary, error) {
	actor, err := actor(ctx)
	if err != nil {
		return Dictionary{}, err
	}
	if expected < 1 {
		return Dictionary{}, apperror.Invalid("version must be positive", nil)
	}
	current, err := s.repository.GetDictionaryByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return Dictionary{}, translate(err)
	}
	if err := s.authorizeScope(ctx, current.TenantID, current.ApplicationID, false); err != nil {
		return Dictionary{}, err
	}
	input.TenantID, input.ApplicationID, input.Code = current.TenantID, current.ApplicationID, current.Code
	input, err = validateDictionary(input, false)
	if err != nil {
		return Dictionary{}, err
	}
	current.Name, current.Description, current.Status, current.MetadataJSON = input.Name, input.Description, input.Status, defaultJSON(input.MetadataJSON)
	current.UpdatedAt, current.UpdatedBy = s.now(), actor
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error { return s.repository.UpdateDictionary(ctx, tx, current, expected) })
	current.Version = expected + 1
	return current, translate(err)
}

func (s *Service) Get(ctx context.Context, tenantID, applicationID, code string) (Dictionary, error) {
	tenantID, applicationID, code = strings.TrimSpace(tenantID), strings.TrimSpace(applicationID), strings.TrimSpace(code)
	if err := s.authorizeScope(ctx, tenantID, applicationID, true); err != nil {
		return Dictionary{}, err
	}
	value, err := s.repository.GetDictionary(ctx, tenantID, applicationID, code)
	if errors.Is(err, ErrNotFound) && applicationID != "" {
		value, err = s.repository.GetDictionary(ctx, tenantID, "", code)
	}
	if errors.Is(err, ErrNotFound) && tenantID != "" {
		value, err = s.repository.GetDictionary(ctx, "", "", code)
	}
	if errors.Is(err, ErrNotFound) {
		if resolver, ok := s.gateway.(ProviderResolver); ok {
			provider, _, resolveErr := resolver.ResolveProvider(ctx, code)
			if resolveErr == nil {
				return Dictionary{Code: code, Name: code, Kind: KindDynamic, Status: StatusActive, ProviderID: provider.ID, MetadataJSON: "{}"}, nil
			}
		}
	}
	return value, translate(err)
}

func (s *Service) List(ctx context.Context, tenantID, applicationID, status, keyword string, page, pageSize int) (Page[Dictionary], error) {
	if err := s.authorizeScope(ctx, tenantID, applicationID, true); err != nil {
		return Page[Dictionary]{}, err
	}
	page, pageSize, err := pagination(page, pageSize)
	if err != nil {
		return Page[Dictionary]{}, err
	}
	items, total, err := s.repository.ListDictionaries(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(applicationID), strings.TrimSpace(status), strings.TrimSpace(keyword), pageSize, (page-1)*pageSize)
	return Page[Dictionary]{Items: items, Total: total, Page: page, PageSize: pageSize}, translate(err)
}

func (s *Service) UpsertItems(ctx context.Context, dictionaryID string, items []Item) ([]Item, error) {
	actor, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	dictionary, err := s.repository.GetDictionaryByID(ctx, strings.TrimSpace(dictionaryID))
	if err != nil {
		return nil, translate(err)
	}
	if err := s.authorizeScope(ctx, dictionary.TenantID, dictionary.ApplicationID, false); err != nil {
		return nil, err
	}
	if dictionary.Kind != KindStatic {
		return nil, apperror.Conflict("dynamic dictionaries cannot store static items", nil)
	}
	if len(items) == 0 || len(items) > 500 {
		return nil, apperror.Invalid("items must contain between 1 and 500 entries", nil)
	}
	now := s.now()
	seen := make(map[string]struct{}, len(items))
	expectedVersions := make([]int64, len(items))
	for index := range items {
		item := &items[index]
		expectedVersions[index] = item.Version
		item.DictionaryID = dictionary.ID
		item.Code, item.Name, item.ParentID, item.ParentCode = strings.TrimSpace(item.Code), strings.TrimSpace(item.Name), strings.TrimSpace(item.ParentID), strings.TrimSpace(item.ParentCode)
		if !codePattern.MatchString(item.Code) || item.Name == "" || !json.Valid([]byte(defaultJSON(item.MetadataJSON))) {
			return nil, apperror.Invalid("item code, name, or metadata is invalid", nil)
		}
		if _, duplicate := seen[item.Code]; duplicate {
			return nil, apperror.Conflict("duplicate item code in request", nil)
		}
		seen[item.Code] = struct{}{}
		if item.Status == "" {
			item.Status = StatusActive
		}
		item.MetadataJSON = defaultJSON(item.MetadataJSON)
		item.UpdatedAt, item.UpdatedBy = now, actor
		if item.ID == "" {
			item.ID = uuid.NewString()
			item.Version, item.CreatedAt, item.CreatedBy = 1, now, actor
		}
	}
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error {
		for index, item := range items {
			if err := s.repository.UpsertDraftItem(ctx, tx, item, expectedVersions[index]); err != nil {
				return err
			}
		}
		return nil
	})
	return items, translate(err)
}

func (s *Service) ListDraftItems(ctx context.Context, dictionaryID string) ([]Item, error) {
	dictionary, err := s.repository.GetDictionaryByID(ctx, strings.TrimSpace(dictionaryID))
	if err != nil {
		return nil, translate(err)
	}
	if err := s.authorizeScope(ctx, dictionary.TenantID, dictionary.ApplicationID, false); err != nil {
		return nil, err
	}
	if dictionary.Kind != KindStatic {
		return nil, apperror.Conflict("dynamic dictionaries do not have draft items", nil)
	}
	items, err := s.repository.ListDraftItems(ctx, dictionary.ID)
	return items, translate(err)
}

func (s *Service) ListDraftItemsPage(ctx context.Context, dictionaryID, keyword string, page, pageSize int) (Page[Item], error) {
	dictionary, err := s.authorizedStaticDictionary(ctx, dictionaryID)
	if err != nil {
		return Page[Item]{}, err
	}
	page, pageSize, err = pagination(page, pageSize)
	if err != nil {
		return Page[Item]{}, err
	}
	items, total, err := s.repository.ListDraftItemsPage(ctx, dictionary.ID, strings.TrimSpace(keyword), pageSize, (page-1)*pageSize)
	return Page[Item]{Items: items, Total: total, Page: page, PageSize: pageSize}, translate(err)
}

func (s *Service) GetItem(ctx context.Context, id string) (Item, error) {
	item, err := s.repository.GetDraftItem(ctx, strings.TrimSpace(id))
	if err != nil {
		return Item{}, translate(err)
	}
	if _, err := s.authorizedStaticDictionary(ctx, item.DictionaryID); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Service) authorizedStaticDictionary(ctx context.Context, dictionaryID string) (Dictionary, error) {
	dictionary, err := s.repository.GetDictionaryByID(ctx, strings.TrimSpace(dictionaryID))
	if err != nil {
		return Dictionary{}, translate(err)
	}
	if err := s.authorizeScope(ctx, dictionary.TenantID, dictionary.ApplicationID, false); err != nil {
		return Dictionary{}, err
	}
	if dictionary.Kind != KindStatic {
		return Dictionary{}, apperror.Conflict("dynamic dictionaries do not have draft items", nil)
	}
	return dictionary, nil
}

func (s *Service) DeleteItem(ctx context.Context, id string, expected int64) error {
	actor, err := actor(ctx)
	if err != nil {
		return err
	}
	item, err := s.repository.GetDraftItem(ctx, id)
	if err != nil {
		return translate(err)
	}
	dictionary, err := s.repository.GetDictionaryByID(ctx, item.DictionaryID)
	if err != nil {
		return translate(err)
	}
	if err := s.authorizeScope(ctx, dictionary.TenantID, dictionary.ApplicationID, false); err != nil {
		return err
	}
	items, err := s.repository.ListDraftItems(ctx, item.DictionaryID)
	if err != nil {
		return translate(err)
	}
	for _, candidate := range items {
		if candidate.ParentID == id {
			return apperror.Conflict("dictionary item with children cannot be deleted", nil)
		}
	}
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error { return s.repository.DeleteDraftItem(ctx, tx, id, expected, s.now(), actor) })
	return translate(err)
}

func (s *Service) Publish(ctx context.Context, dictionaryID string, expected int64, comment string) (Release, []Item, error) {
	actor, err := actor(ctx)
	if err != nil {
		return Release{}, nil, err
	}
	dictionary, err := s.repository.GetDictionaryByID(ctx, strings.TrimSpace(dictionaryID))
	if err != nil {
		return Release{}, nil, translate(err)
	}
	if err := s.authorizeScope(ctx, dictionary.TenantID, dictionary.ApplicationID, false); err != nil {
		return Release{}, nil, err
	}
	if s.locker == nil {
		return Release{}, nil, apperror.Unavailable("dictionary publish lock unavailable", nil)
	}
	mutex, acquired, err := s.locker.TryLock(ctx, "dictionary:publish:"+dictionaryID, 30*time.Second)
	if err != nil {
		return Release{}, nil, apperror.Unavailable("acquire dictionary publish lock", err)
	}
	if !acquired {
		return Release{}, nil, apperror.Conflict("dictionary publication is already running", nil)
	}
	defer func() { _ = mutex.Unlock(context.WithoutCancel(ctx)) }()
	items, err := s.repository.ListDraftItems(ctx, dictionaryID)
	if err != nil {
		return Release{}, nil, translate(err)
	}
	if len(items) == 0 {
		return Release{}, nil, apperror.Conflict("at least one dictionary item is required", nil)
	}
	if err := validateTree(items); err != nil {
		return Release{}, nil, err
	}
	now := s.now()
	release := Release{ID: uuid.NewString(), DictionaryID: dictionaryID, ReleaseVersion: dictionary.PublishedVersion + 1, Comment: strings.TrimSpace(comment), Status: StatusActive, AuditFields: AuditFields{Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: actor, UpdatedBy: actor}}
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error {
		if err := s.repository.CreateRelease(ctx, tx, release, items); err != nil {
			return err
		}
		if err := s.repository.SetPublishedVersion(ctx, tx, dictionaryID, release.ReleaseVersion, expected, now, actor); err != nil {
			return err
		}
		dictionary.PublishedVersion, dictionary.Status, dictionary.Version = release.ReleaseVersion, StatusActive, expected+1
		dictionary.UpdatedAt, dictionary.UpdatedBy = now, actor
		return s.addEvent(ctx, tx, "platform.dictionary.dictionary.published.v1", "platform.dictionary.v1.DictionaryPublished", dictionary.ID, "dictionary", dictionary.TenantID, dictionary.ApplicationID, actor, now, &dictionaryv1.DictionaryPublishedEvent{Dictionary: ToProtoDictionary(dictionary), PublishedVersion: release.ReleaseVersion})
	})
	return release, items, translate(err)
}

func (s *Service) Query(ctx context.Context, tenantID, applicationID, code string, search Search) (Page[Item], error) {
	dictionary, err := s.Get(ctx, tenantID, applicationID, code)
	if err != nil {
		return Page[Item]{}, err
	}
	if dictionary.Kind != KindStatic {
		if search.Page <= 0 {
			search.Page = 1
		}
		provider, capability, err := s.dynamicProvider(ctx, dictionary, search)
		if err != nil {
			return Page[Item]{}, err
		}
		if search.PageSize <= 0 {
			search.PageSize = 20
		}
		if search.PageSize > int(capability.MaxPageSize) {
			return Page[Item]{}, apperror.Invalid("page_size exceeds provider capability", nil)
		}
		if search.Limit <= 0 {
			search.Limit = search.PageSize
		}
		if search.Limit > int(capability.MaxPageSize) {
			return Page[Item]{}, apperror.Invalid("limit exceeds provider capability", nil)
		}
		value, err := s.gateway.Query(ctx, provider, tenantID, applicationID, code, search)
		return Page[Item](value), providerError(err)
	}
	if dictionary.PublishedVersion == 0 {
		return Page[Item]{}, apperror.NotFound("published dictionary not found")
	}
	search.Page, search.PageSize, err = pagination(search.Page, search.PageSize)
	if err != nil {
		return Page[Item]{}, err
	}
	if !allowedSort(search.Sort) || !allowedFilters(search.Filters) {
		return Page[Item]{}, apperror.Invalid("unsupported dictionary filter or sort", nil)
	}
	items, total, err := s.repository.QueryPublishedItems(ctx, dictionary.ID, dictionary.PublishedVersion, search)
	for index := range items {
		items[index].DictionaryCode = dictionary.Code
	}
	return Page[Item]{Items: items, Total: total, Page: search.Page, PageSize: search.PageSize}, translate(err)
}

func (s *Service) Tree(ctx context.Context, tenantID, applicationID, code, mode, parentID, keyword string, maxDepth, maxNodes int) ([]TreeNode, bool, error) {
	dictionary, err := s.Get(ctx, tenantID, applicationID, code)
	if err != nil {
		return nil, false, err
	}
	if dictionary.Kind == KindDynamic {
		provider, capability, err := s.dynamicProvider(ctx, dictionary, Search{})
		if err != nil {
			return nil, false, err
		}
		if !capability.SupportsTree {
			return nil, false, apperror.Invalid("dynamic dictionary does not support tree queries", nil)
		}
		if maxDepth <= 0 {
			maxDepth = int(capability.MaxTreeDepth)
		}
		if maxNodes <= 0 {
			maxNodes = int(capability.MaxTreeNodes)
		}
		if maxDepth > int(capability.MaxTreeDepth) || maxNodes > int(capability.MaxTreeNodes) {
			return nil, false, apperror.Invalid("tree limits exceed provider capability", nil)
		}
		value, truncated, err := s.gateway.Tree(ctx, provider, tenantID, applicationID, code, mode, parentID, keyword, maxDepth, maxNodes, nil)
		return value, truncated, providerError(err)
	}
	if dictionary.PublishedVersion == 0 {
		return nil, false, apperror.NotFound("published static dictionary not found")
	}
	if maxDepth <= 0 {
		maxDepth = 8
	}
	if maxNodes <= 0 {
		maxNodes = 1000
	}
	if maxDepth > 32 || maxNodes > 5000 {
		return nil, false, apperror.Invalid("tree limits exceed platform maximum", nil)
	}
	items, err := s.repository.ListPublishedItems(ctx, dictionary.ID, dictionary.PublishedVersion)
	if err != nil {
		return nil, false, translate(err)
	}
	return buildTree(items, mode, parentID, keyword, maxDepth, maxNodes)
}

func (s *Service) ResolveCodes(ctx context.Context, tenantID, applicationID, code string, codes []string) (map[string]Item, error) {
	if len(codes) > 500 {
		return nil, apperror.Invalid("at most 500 codes can be resolved", nil)
	}
	dictionary, err := s.Get(ctx, tenantID, applicationID, code)
	if err != nil {
		return nil, err
	}
	if dictionary.Kind == KindDynamic {
		provider, _, providerErr := s.dynamicProvider(ctx, dictionary, Search{})
		if providerErr != nil {
			return nil, providerErr
		}
		value, providerErr := s.gateway.ResolveCodes(ctx, provider, tenantID, applicationID, code, codes)
		return value, providerError(providerErr)
	}
	items, err := s.repository.ListPublishedItems(ctx, dictionary.ID, dictionary.PublishedVersion)
	if err != nil {
		return nil, translate(err)
	}
	wanted := make(map[string]struct{}, len(codes))
	for _, value := range codes {
		wanted[value] = struct{}{}
	}
	result := make(map[string]Item, len(codes))
	for _, item := range items {
		if _, ok := wanted[item.Code]; ok {
			item.DictionaryCode = dictionary.Code
			result[item.Code] = item
		}
	}
	return result, nil
}

func (s *Service) RegisterProvider(ctx context.Context, input ProviderInput) (Provider, string, error) {
	actor, err := actor(ctx)
	if err != nil {
		return Provider{}, "", err
	}
	input, err = validateProvider(input)
	if err != nil {
		return Provider{}, "", err
	}
	if s.gateway == nil {
		return Provider{}, "", apperror.Unavailable("dynamic dictionary gateway is disabled", nil)
	}
	if err := s.gateway.ValidateTarget(input.Target); err != nil {
		return Provider{}, "", apperror.Invalid("provider target is not allowed", err)
	}
	token, tokenHash, err := newLeaseToken()
	if err != nil {
		return Provider{}, "", apperror.Internal(fmt.Errorf("generate provider lease token: %w", err))
	}
	capabilitiesJSON, err := json.Marshal(input.Capabilities)
	if err != nil {
		return Provider{}, "", apperror.Invalid("provider capabilities are invalid", err)
	}
	now := s.now()
	current, getErr := s.repository.GetProviderByService(ctx, input.ServiceName)
	create := errors.Is(getErr, ErrNotFound)
	if getErr != nil && !create {
		return Provider{}, "", translate(getErr)
	}
	if create {
		current = Provider{ID: uuid.NewString(), ServiceName: input.ServiceName, AuditFields: AuditFields{Version: 1, CreatedAt: now, CreatedBy: actor}}
	}
	current.Target, current.Status, current.CapabilitiesJSON = input.Target, StatusActive, string(capabilitiesJSON)
	current.CacheTTLSeconds, current.TimeoutMilliseconds = input.CacheTTLSeconds, input.TimeoutMilliseconds
	current.LeaseTokenHash, current.LeaseExpiresAt, current.UpdatedAt, current.UpdatedBy = tokenHash, now.Add(time.Duration(input.LeaseSeconds)*time.Second), now, actor
	expectedVersion := current.Version
	dictionaryValues := make(map[string]Dictionary, len(input.Capabilities))
	for _, capability := range input.Capabilities {
		dictionaryValue, lookupErr := s.repository.GetDictionary(ctx, "", "", capability.DictionaryCode)
		if lookupErr != nil && !errors.Is(lookupErr, ErrNotFound) {
			return Provider{}, "", translate(lookupErr)
		}
		if lookupErr == nil && dictionaryValue.Kind == KindStatic && dictionaryValue.ProviderID == "" {
			return Provider{}, "", apperror.Conflict("dynamic provider dictionary code conflicts with a static dictionary", nil)
		}
		dictionaryValues[capability.DictionaryCode] = dictionaryValue
	}
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error {
		if create {
			if err := s.repository.CreateProvider(ctx, tx, current); err != nil {
				return err
			}
		} else if err := s.repository.UpdateProvider(ctx, tx, current, expectedVersion); err != nil {
			return err
		}
		for _, capability := range input.Capabilities {
			dictionaryValue := dictionaryValues[capability.DictionaryCode]
			if dictionaryValue.ID == "" {
				dictionaryValue = Dictionary{ID: uuid.NewString(), Code: capability.DictionaryCode, Name: capability.DictionaryCode, Kind: KindDynamic, Status: StatusActive, ProviderID: current.ID, MetadataJSON: "{}", AuditFields: AuditFields{Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: actor, UpdatedBy: actor}}
				if err := s.repository.CreateDictionary(ctx, tx, dictionaryValue); err != nil {
					return err
				}
				continue
			}
			if err := s.repository.BindDynamicDictionary(ctx, tx, dictionaryValue.ID, current.ID, now, actor); err != nil {
				return err
			}
		}
		changeType := "updated"
		if create {
			changeType = "registered"
		}
		eventProvider := current
		if !create {
			eventProvider.Version = expectedVersion + 1
		}
		return s.addEvent(ctx, tx, "platform.dictionary.provider.changed.v1", "platform.dictionary.v1.ProviderChanged", current.ID, "dictionary_provider", "", "", actor, now, &dictionaryv1.ProviderChangedEvent{Provider: ToProtoProvider(eventProvider), ChangeType: changeType})
	})
	if err != nil {
		return Provider{}, "", translate(err)
	}
	if !create {
		current.Version++
	}
	return current, token, nil
}

func (s *Service) HeartbeatProvider(ctx context.Context, id, token string, leaseSeconds int32) (time.Time, error) {
	actor, err := actor(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if leaseSeconds < 15 || leaseSeconds > 300 || token == "" {
		return time.Time{}, apperror.Invalid("provider lease must be between 15 and 300 seconds", nil)
	}
	now := s.now()
	expires := now.Add(time.Duration(leaseSeconds) * time.Second)
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error {
		return s.repository.RenewProvider(ctx, tx, id, hashToken(token), expires, now, actor)
	})
	if errors.Is(err, ErrStaleVersion) {
		return time.Time{}, apperror.Unauthorized("invalid provider lease token")
	}
	return expires, translate(err)
}

func (s *Service) UnregisterProvider(ctx context.Context, id, token string) error {
	actor, err := actor(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		return apperror.Unauthorized("invalid provider lease token")
	}
	provider, err := s.repository.GetProvider(ctx, id)
	if err != nil {
		return translate(err)
	}
	now := s.now()
	err = s.transactor.Within(ctx, nil, func(tx *sqlx.Tx) error {
		if err := s.repository.UnregisterProvider(ctx, tx, id, hashToken(token), now, actor); err != nil {
			return err
		}
		provider.Status = StatusInactive
		provider.Version++
		provider.UpdatedAt, provider.UpdatedBy = now, actor
		return s.addEvent(ctx, tx, "platform.dictionary.provider.changed.v1", "platform.dictionary.v1.ProviderChanged", provider.ID, "dictionary_provider", "", "", actor, now, &dictionaryv1.ProviderChangedEvent{Provider: ToProtoProvider(provider), ChangeType: "unregistered"})
	})
	if errors.Is(err, ErrStaleVersion) {
		return apperror.Unauthorized("invalid provider lease token")
	}
	return translate(err)
}

func (s *Service) ListProviders(ctx context.Context, status string, page, pageSize int) (Page[Provider], error) {
	page, pageSize, err := pagination(page, pageSize)
	if err != nil {
		return Page[Provider]{}, err
	}
	items, total, err := s.repository.ListProviders(ctx, strings.TrimSpace(status), pageSize, (page-1)*pageSize)
	return Page[Provider]{Items: items, Total: total, Page: page, PageSize: pageSize}, translate(err)
}

func validateProvider(input ProviderInput) (ProviderInput, error) {
	input.ServiceName, input.Target = strings.TrimSpace(input.ServiceName), strings.TrimSpace(input.Target)
	if !codePattern.MatchString(input.ServiceName) || input.Target == "" || strings.ContainsAny(input.Target, "@?#") || len(input.Capabilities) == 0 || len(input.Capabilities) > 100 {
		return input, apperror.Invalid("provider service, target, or capabilities are invalid", nil)
	}
	if input.CacheTTLSeconds < 0 || input.CacheTTLSeconds > 3600 {
		return input, apperror.Invalid("provider cache TTL is invalid", nil)
	}
	if input.TimeoutMilliseconds == 0 {
		input.TimeoutMilliseconds = 3000
	}
	if input.TimeoutMilliseconds < 100 || input.TimeoutMilliseconds > 30000 {
		return input, apperror.Invalid("provider timeout is invalid", nil)
	}
	if input.LeaseSeconds == 0 {
		input.LeaseSeconds = 60
	}
	if input.LeaseSeconds < 15 || input.LeaseSeconds > 300 {
		return input, apperror.Invalid("provider lease is invalid", nil)
	}
	seen := map[string]bool{}
	for index := range input.Capabilities {
		capability := &input.Capabilities[index]
		capability.DictionaryCode = strings.TrimSpace(capability.DictionaryCode)
		if !codePattern.MatchString(capability.DictionaryCode) || seen[capability.DictionaryCode] {
			return input, apperror.Invalid("provider dictionary code is invalid or duplicated", nil)
		}
		seen[capability.DictionaryCode] = true
		if capability.MaxPageSize == 0 {
			capability.MaxPageSize = 100
		}
		if capability.SupportsTree && capability.MaxTreeDepth == 0 {
			capability.MaxTreeDepth = 8
		}
		if capability.SupportsTree && capability.MaxTreeNodes == 0 {
			capability.MaxTreeNodes = 1000
		}
		if capability.MaxPageSize < 1 || capability.MaxPageSize > 500 || capability.MaxTreeDepth > 32 || capability.MaxTreeNodes > 5000 {
			return input, apperror.Invalid("provider capability limits are invalid", nil)
		}
		for _, key := range append(append([]string{}, capability.FilterKeys...), capability.SortKeys...) {
			if !codePattern.MatchString("x." + key) {
				return input, apperror.Invalid("provider filter or sort key is invalid", nil)
			}
		}
	}
	return input, nil
}

func newLeaseToken() (string, string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(buffer)
	return token, hashToken(token), nil
}
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) dynamicProvider(ctx context.Context, dictionary Dictionary, search Search) (Provider, Capability, error) {
	if s.gateway == nil {
		return Provider{}, Capability{}, apperror.Unavailable("dynamic dictionary gateway is disabled", nil)
	}
	if resolver, ok := s.gateway.(ProviderResolver); ok {
		provider, capability, err := resolver.ResolveProvider(ctx, dictionary.Code)
		if err != nil {
			return Provider{}, Capability{}, providerError(err)
		}
		if err := validateSearchCapability(search, capability); err != nil {
			return Provider{}, Capability{}, err
		}
		return provider, capability, nil
	}
	provider, err := s.repository.GetProvider(ctx, dictionary.ProviderID)
	if err != nil {
		return Provider{}, Capability{}, translate(err)
	}
	if provider.Status != StatusActive || !provider.LeaseExpiresAt.After(s.now()) {
		return Provider{}, Capability{}, apperror.Unavailable("dynamic dictionary provider lease is inactive", nil)
	}
	capabilities := []Capability{}
	if err := json.Unmarshal([]byte(provider.CapabilitiesJSON), &capabilities); err != nil {
		return Provider{}, Capability{}, apperror.Internal(fmt.Errorf("decode provider capabilities: %w", err))
	}
	for _, capability := range capabilities {
		if capability.DictionaryCode != dictionary.Code {
			continue
		}
		if err := validateSearchCapability(search, capability); err != nil {
			return Provider{}, Capability{}, err
		}
		return provider, capability, nil
	}
	return Provider{}, Capability{}, apperror.Unavailable("dynamic dictionary capability is not registered", nil)
}
func validateSearchCapability(search Search, capability Capability) error {
	if search.Keyword != "" && !capability.SupportsSearch {
		return apperror.Invalid("dynamic dictionary does not support search", nil)
	}
	if search.Cursor != "" && !capability.SupportsCursor {
		return apperror.Invalid("dynamic dictionary does not support cursor pagination", nil)
	}
	if !keysAllowed(search.Filters, capability.FilterKeys) || (search.Sort != "" && !contains(capability.SortKeys, search.Sort)) {
		return apperror.Invalid("unsupported provider filter or sort", nil)
	}
	return nil
}
func keysAllowed(values map[string]string, allowed []string) bool {
	for key := range values {
		if !contains(allowed, key) {
			return false
		}
	}
	return true
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func providerError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return err
	}
	return apperror.Unavailable("dynamic dictionary provider call failed", err)
}

func (s *Service) addEvent(ctx context.Context, tx *sqlx.Tx, subject, eventType, aggregateID, aggregateType, tenantID, applicationID, actor string, at time.Time, payload proto.Message) error {
	envelope, err := platformevents.NewEnvelope(platformevents.Metadata{EventID: uuid.NewString(), EventType: eventType, AggregateID: aggregateID, AggregateType: aggregateType, TenantID: tenantID, ApplicationID: applicationID, SchemaVersion: 1, ActorID: actor, OccurredAt: at}, payload)
	if err != nil {
		return err
	}
	encoded, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}
	return s.repository.AddOutbox(ctx, tx, OutboxEvent{ID: envelope.GetEventId(), Subject: subject, Envelope: encoded, AvailableAt: at, CreatedAt: at, UpdatedAt: at, CreatedBy: actor, UpdatedBy: actor})
}

func validateDictionary(input DictionaryInput, creating bool) (DictionaryInput, error) {
	input.TenantID, input.ApplicationID = strings.TrimSpace(input.TenantID), strings.TrimSpace(input.ApplicationID)
	input.Code, input.Name = strings.TrimSpace(input.Code), strings.TrimSpace(input.Name)
	input.Description, input.Status, input.MetadataJSON = strings.TrimSpace(input.Description), strings.TrimSpace(input.Status), defaultJSON(input.MetadataJSON)
	if (input.TenantID == "" && input.ApplicationID != "") || !codePattern.MatchString(input.Code) || input.Name == "" || !json.Valid([]byte(input.MetadataJSON)) {
		return input, apperror.Invalid("dictionary code, name, or metadata is invalid", nil)
	}
	if !creating && input.Status != StatusDraft && input.Status != StatusActive && input.Status != "disabled" {
		return input, apperror.Invalid("dictionary status is invalid", nil)
	}
	return input, nil
}

func validateTree(items []Item) error {
	byID := make(map[string]Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, item := range items {
		seen := map[string]bool{item.ID: true}
		parent := item.ParentID
		for parent != "" {
			if seen[parent] {
				return apperror.Conflict("dictionary tree contains a cycle", nil)
			}
			seen[parent] = true
			value, ok := byID[parent]
			if !ok {
				return apperror.Invalid("dictionary item parent does not exist", nil)
			}
			parent = value.ParentID
		}
	}
	return nil
}

func buildTree(items []Item, mode, parentID, keyword string, maxDepth, maxNodes int) ([]TreeNode, bool, error) {
	if err := validateTree(items); err != nil {
		return nil, false, err
	}
	if mode == "children" {
		filtered := items[:0]
		for _, item := range items {
			if item.ParentID == parentID {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	keep := map[string]bool{}
	if mode == "search_with_ancestors" && keyword != "" {
		byID := map[string]Item{}
		for _, item := range items {
			byID[item.ID] = item
		}
		needle := strings.ToLower(keyword)
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Code), needle) || strings.Contains(strings.ToLower(item.Name), needle) {
				for current := item.ID; current != ""; current = byID[current].ParentID {
					keep[current] = true
				}
			}
		}
	}
	children := map[string][]Item{}
	for _, item := range items {
		if len(keep) == 0 || keep[item.ID] {
			children[item.ParentID] = append(children[item.ParentID], item)
		}
	}
	for key := range children {
		sort.SliceStable(children[key], func(i, j int) bool {
			if children[key][i].SortOrder == children[key][j].SortOrder {
				return children[key][i].ID < children[key][j].ID
			}
			return children[key][i].SortOrder < children[key][j].SortOrder
		})
	}
	count, truncated := 0, false
	var assemble func(string, int) []TreeNode
	assemble = func(parent string, depth int) []TreeNode {
		if depth > maxDepth {
			truncated = true
			return nil
		}
		result := []TreeNode{}
		for _, item := range children[parent] {
			if count >= maxNodes {
				truncated = true
				break
			}
			count++
			result = append(result, TreeNode{Item: item, Children: assemble(item.ID, depth+1)})
		}
		return result
	}
	root := ""
	if mode == "children" {
		root = parentID
	}
	return assemble(root, 1), truncated, nil
}

func allowedSort(value string) bool {
	return value == "" || value == "sort_order" || value == "code" || value == "name"
}
func allowedFilters(filters map[string]string) bool {
	for key := range filters {
		if key != "parent_id" && key != "disabled" {
			return false
		}
	}
	return true
}
func pagination(page, size int) (int, int, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		return 0, 0, apperror.Invalid("page_size must not exceed 100", nil)
	}
	return page, size, nil
}
func defaultJSON(value string) string {
	if strings.TrimSpace(value) == "" {
		return "{}"
	}
	return value
}
func actor(ctx context.Context) (string, error) {
	value, ok := principal.FromContext(ctx)
	if !ok || strings.TrimSpace(value.ID) == "" {
		return "", apperror.Unauthorized("authenticated actor is required")
	}
	return value.ID, nil
}

func (s *Service) authorizeScope(ctx context.Context, tenantID, applicationID string, allowGlobal bool) error {
	identity, ok := principal.FromContext(ctx)
	if !ok {
		return apperror.Unauthorized("authenticated actor is required")
	}
	tenantID, applicationID = strings.TrimSpace(tenantID), strings.TrimSpace(applicationID)
	if tenantID == "" && applicationID != "" {
		return apperror.Invalid("application_id requires tenant_id", nil)
	}
	switch identity.Type {
	case principal.TypeServiceAccount, principal.TypeSystem:
		// Machine callers still need a valid application grant for application-owned dictionaries.
	case principal.TypeUser:
		if allowGlobal && tenantID == "" {
			return nil
		}
		if identity.TenantID == "" || identity.TenantID != tenantID {
			return apperror.Forbidden("tenant access denied")
		}
	default:
		return apperror.Forbidden("tenant access denied")
	}
	if applicationID == "" {
		return nil
	}
	err := s.applications.Verify(ctx, tenantID, applicationID)
	if errors.Is(err, appaccess.ErrNotGranted) {
		return apperror.Forbidden("tenant application access denied")
	}
	if err != nil {
		return apperror.Unavailable("verify tenant application access", err)
	}
	return nil
}
func translate(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return err
	}
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("dictionary resource not found")
	}
	if errors.Is(err, ErrStaleVersion) {
		return apperror.Conflict("resource version is stale", err)
	}
	return apperror.Internal(fmt.Errorf("dictionary persistence: %w", err))
}
