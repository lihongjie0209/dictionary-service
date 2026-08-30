package dictionary

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrNotFound     = errors.New("dictionary resource not found")
	ErrStaleVersion = errors.New("stale dictionary resource version")
)

type Repository interface {
	CreateDictionary(context.Context, sqlx.ExtContext, Dictionary) error
	UpdateDictionary(context.Context, sqlx.ExtContext, Dictionary, int64) error
	GetDictionary(context.Context, string, string) (Dictionary, error)
	GetDictionaryByID(context.Context, string) (Dictionary, error)
	ListDictionaries(context.Context, string, string, string, int, int) ([]Dictionary, int64, error)
	GetDraftItem(context.Context, string) (Item, error)
	UpsertDraftItem(context.Context, sqlx.ExtContext, Item, int64) error
	DeleteDraftItem(context.Context, sqlx.ExtContext, string, int64, time.Time, string) error
	ListDraftItems(context.Context, string) ([]Item, error)
	CreateRelease(context.Context, sqlx.ExtContext, Release, []Item) error
	SetPublishedVersion(context.Context, sqlx.ExtContext, string, int64, int64, time.Time, string) error
	ListPublishedItems(context.Context, string, int64) ([]Item, error)
	QueryPublishedItems(context.Context, string, int64, Search) ([]Item, int64, error)
	GetProviderByService(context.Context, string) (Provider, error)
	GetProvider(context.Context, string) (Provider, error)
	CreateProvider(context.Context, sqlx.ExtContext, Provider) error
	UpdateProvider(context.Context, sqlx.ExtContext, Provider, int64) error
	RenewProvider(context.Context, sqlx.ExtContext, string, string, time.Time, time.Time, string) error
	UnregisterProvider(context.Context, sqlx.ExtContext, string, string, time.Time, string) error
	ListProviders(context.Context, string, int, int) ([]Provider, int64, error)
	BindDynamicDictionary(context.Context, sqlx.ExtContext, string, string, time.Time, string) error
	AddOutbox(context.Context, sqlx.ExtContext, OutboxEvent) error
}

const providerColumns = `id,service_name,target,status,capabilities_json,cache_ttl_seconds,timeout_milliseconds,lease_token_hash,lease_expires_at,version,created_at,updated_at,created_by,updated_by`

func (r *SQLRepository) GetProviderByService(ctx context.Context, serviceName string) (Provider, error) {
	var value Provider
	err := r.db.GetContext(ctx, &value, r.db.Rebind(`SELECT `+providerColumns+` FROM dictionary_providers WHERE service_name=?`), serviceName)
	return value, notFound(err)
}
func (r *SQLRepository) GetProvider(ctx context.Context, id string) (Provider, error) {
	var value Provider
	err := r.db.GetContext(ctx, &value, r.db.Rebind(`SELECT `+providerColumns+` FROM dictionary_providers WHERE id=?`), id)
	return value, notFound(err)
}
func (r *SQLRepository) CreateProvider(ctx context.Context, e sqlx.ExtContext, value Provider) error {
	_, err := e.ExecContext(ctx, r.db.Rebind(`INSERT INTO dictionary_providers (`+providerColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), value.ID, value.ServiceName, value.Target, value.Status, value.CapabilitiesJSON, value.CacheTTLSeconds, value.TimeoutMilliseconds, value.LeaseTokenHash, value.LeaseExpiresAt, value.Version, value.CreatedAt, value.UpdatedAt, value.CreatedBy, value.UpdatedBy)
	return err
}
func (r *SQLRepository) UpdateProvider(ctx context.Context, e sqlx.ExtContext, value Provider, expected int64) error {
	result, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionary_providers SET target=?,status=?,capabilities_json=?,cache_ttl_seconds=?,timeout_milliseconds=?,lease_token_hash=?,lease_expires_at=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`), value.Target, value.Status, value.CapabilitiesJSON, value.CacheTTLSeconds, value.TimeoutMilliseconds, value.LeaseTokenHash, value.LeaseExpiresAt, value.UpdatedAt, value.UpdatedBy, value.ID, expected)
	return stale(result, err)
}
func (r *SQLRepository) RenewProvider(ctx context.Context, e sqlx.ExtContext, id, tokenHash string, expires, now time.Time, actor string) error {
	result, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionary_providers SET status='active',lease_expires_at=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND lease_token_hash=?`), expires, now, actor, id, tokenHash)
	return stale(result, err)
}
func (r *SQLRepository) UnregisterProvider(ctx context.Context, e sqlx.ExtContext, id, tokenHash string, now time.Time, actor string) error {
	result, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionary_providers SET status='inactive',version=version+1,updated_at=?,updated_by=? WHERE id=? AND lease_token_hash=?`), now, actor, id, tokenHash)
	return stale(result, err)
}
func (r *SQLRepository) ListProviders(ctx context.Context, status string, limit, offset int) ([]Provider, int64, error) {
	where := `1=1`
	args := []any{}
	if status != "" {
		where = `status=?`
		args = append(args, status)
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, r.db.Rebind(`SELECT COUNT(*) FROM dictionary_providers WHERE `+where), args...); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any{}, args...), limit, offset)
	items := []Provider{}
	err := r.db.SelectContext(ctx, &items, r.db.Rebind(`SELECT `+providerColumns+` FROM dictionary_providers WHERE `+where+` ORDER BY service_name,id LIMIT ? OFFSET ?`), queryArgs...)
	return items, total, err
}
func (r *SQLRepository) BindDynamicDictionary(ctx context.Context, e sqlx.ExtContext, id, providerID string, now time.Time, actor string) error {
	_, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionaries SET kind='dynamic',provider_id=?,status='active',version=version+1,updated_at=?,updated_by=? WHERE id=?`), providerID, now, actor, id)
	return err
}
func (r *SQLRepository) AddOutbox(ctx context.Context, e sqlx.ExtContext, value OutboxEvent) error {
	_, err := e.ExecContext(ctx, r.db.Rebind(`INSERT INTO dictionary_outbox_events (id,subject,envelope,attempts,available_at,published_at,last_error,version,created_at,updated_at,created_by,updated_by) VALUES (?,?,?,0,?,NULL,'',1,?,?,?,?)`), value.ID, value.Subject, value.Envelope, value.AvailableAt, value.CreatedAt, value.UpdatedAt, value.CreatedBy, value.UpdatedBy)
	return err
}

type SQLRepository struct{ db *sqlx.DB }

func NewRepository(db *sqlx.DB) Repository { return &SQLRepository{db: db} }

const dictionaryColumns = `id,tenant_id,code,name,description,kind,status,provider_id,metadata_json,published_version,version,created_at,updated_at,created_by,updated_by`
const itemColumns = `id,dictionary_id,code,name,parent_id,parent_code,leaf,sort_order,disabled,status,metadata_json,version,created_at,updated_at,created_by,updated_by`

func (r *SQLRepository) CreateDictionary(ctx context.Context, e sqlx.ExtContext, v Dictionary) error {
	_, err := e.ExecContext(ctx, r.db.Rebind(`INSERT INTO dictionaries (`+dictionaryColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), v.ID, v.TenantID, v.Code, v.Name, v.Description, v.Kind, v.Status, v.ProviderID, v.MetadataJSON, v.PublishedVersion, v.Version, v.CreatedAt, v.UpdatedAt, v.CreatedBy, v.UpdatedBy)
	return err
}

func (r *SQLRepository) UpdateDictionary(ctx context.Context, e sqlx.ExtContext, v Dictionary, expected int64) error {
	result, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionaries SET name=?,description=?,status=?,metadata_json=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`), v.Name, v.Description, v.Status, v.MetadataJSON, v.UpdatedAt, v.UpdatedBy, v.ID, expected)
	return stale(result, err)
}

func (r *SQLRepository) GetDictionary(ctx context.Context, tenantID, code string) (Dictionary, error) {
	var value Dictionary
	err := r.db.GetContext(ctx, &value, r.db.Rebind(`SELECT `+dictionaryColumns+` FROM dictionaries WHERE tenant_id=? AND code=?`), tenantID, code)
	return value, notFound(err)
}

func (r *SQLRepository) GetDictionaryByID(ctx context.Context, id string) (Dictionary, error) {
	var value Dictionary
	err := r.db.GetContext(ctx, &value, r.db.Rebind(`SELECT `+dictionaryColumns+` FROM dictionaries WHERE id=?`), id)
	return value, notFound(err)
}

func (r *SQLRepository) ListDictionaries(ctx context.Context, tenantID, status, keyword string, limit, offset int) ([]Dictionary, int64, error) {
	where := `tenant_id=?`
	args := []any{tenantID}
	if status != "" {
		where += ` AND status=?`
		args = append(args, status)
	}
	if keyword != "" {
		where += ` AND (LOWER(code) LIKE ? OR LOWER(name) LIKE ?)`
		like := `%` + strings.ToLower(keyword) + `%`
		args = append(args, like, like)
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, r.db.Rebind(`SELECT COUNT(*) FROM dictionaries WHERE `+where), args...); err != nil {
		return nil, 0, fmt.Errorf("count dictionaries: %w", err)
	}
	queryArgs := append(append([]any{}, args...), limit, offset)
	items := []Dictionary{}
	if err := r.db.SelectContext(ctx, &items, r.db.Rebind(`SELECT `+dictionaryColumns+` FROM dictionaries WHERE `+where+` ORDER BY code,id LIMIT ? OFFSET ?`), queryArgs...); err != nil {
		return nil, 0, fmt.Errorf("list dictionaries: %w", err)
	}
	return items, total, nil
}

func (r *SQLRepository) GetDraftItem(ctx context.Context, id string) (Item, error) {
	var value Item
	err := r.db.GetContext(ctx, &value, r.db.Rebind(`SELECT `+itemColumns+` FROM dictionary_item_drafts WHERE id=? AND status<>'deleted'`), id)
	return value, notFound(err)
}

func (r *SQLRepository) UpsertDraftItem(ctx context.Context, e sqlx.ExtContext, v Item, expected int64) error {
	if expected == 0 {
		_, err := e.ExecContext(ctx, r.db.Rebind(`INSERT INTO dictionary_item_drafts (`+itemColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), v.ID, v.DictionaryID, v.Code, v.Name, v.ParentID, v.ParentCode, v.Leaf, v.SortOrder, v.Disabled, v.Status, v.MetadataJSON, v.Version, v.CreatedAt, v.UpdatedAt, v.CreatedBy, v.UpdatedBy)
		return err
	}
	result, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionary_item_drafts SET code=?,name=?,parent_id=?,parent_code=?,leaf=?,sort_order=?,disabled=?,status=?,metadata_json=?,version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status<>'deleted'`), v.Code, v.Name, v.ParentID, v.ParentCode, v.Leaf, v.SortOrder, v.Disabled, v.Status, v.MetadataJSON, v.UpdatedAt, v.UpdatedBy, v.ID, expected)
	return stale(result, err)
}

func (r *SQLRepository) DeleteDraftItem(ctx context.Context, e sqlx.ExtContext, id string, expected int64, now time.Time, actor string) error {
	result, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionary_item_drafts SET status='deleted',version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=? AND status<>'deleted'`), now, actor, id, expected)
	return stale(result, err)
}

func (r *SQLRepository) ListDraftItems(ctx context.Context, dictionaryID string) ([]Item, error) {
	items := []Item{}
	err := r.db.SelectContext(ctx, &items, r.db.Rebind(`SELECT `+itemColumns+` FROM dictionary_item_drafts WHERE dictionary_id=? AND status<>'deleted' ORDER BY parent_id,sort_order,id`), dictionaryID)
	return items, err
}

func (r *SQLRepository) CreateRelease(ctx context.Context, e sqlx.ExtContext, release Release, items []Item) error {
	_, err := e.ExecContext(ctx, r.db.Rebind(`INSERT INTO dictionary_releases (id,dictionary_id,release_version,comment,status,version,created_at,updated_at,created_by,updated_by) VALUES (?,?,?,?,?,?,?,?,?,?)`), release.ID, release.DictionaryID, release.ReleaseVersion, release.Comment, release.Status, release.Version, release.CreatedAt, release.UpdatedAt, release.CreatedBy, release.UpdatedBy)
	if err != nil {
		return err
	}
	for _, item := range items {
		_, err = e.ExecContext(ctx, r.db.Rebind(`INSERT INTO dictionary_release_items (id,release_id,dictionary_id,release_version,code,name,parent_id,parent_code,leaf,sort_order,disabled,status,metadata_json,version,created_at,updated_at,created_by,updated_by) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), item.ID, release.ID, release.DictionaryID, release.ReleaseVersion, item.Code, item.Name, item.ParentID, item.ParentCode, item.Leaf, item.SortOrder, item.Disabled, item.Status, item.MetadataJSON, item.Version, release.CreatedAt, release.UpdatedAt, release.CreatedBy, release.UpdatedBy)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLRepository) SetPublishedVersion(ctx context.Context, e sqlx.ExtContext, id string, published, expected int64, now time.Time, actor string) error {
	result, err := e.ExecContext(ctx, r.db.Rebind(`UPDATE dictionaries SET published_version=?,status='active',version=version+1,updated_at=?,updated_by=? WHERE id=? AND version=?`), published, now, actor, id, expected)
	return stale(result, err)
}

func (r *SQLRepository) ListPublishedItems(ctx context.Context, dictionaryID string, release int64) ([]Item, error) {
	items := []Item{}
	err := r.db.SelectContext(ctx, &items, r.db.Rebind(`SELECT `+itemColumns+` FROM dictionary_release_items WHERE dictionary_id=? AND release_version=? AND status='active' ORDER BY parent_id,sort_order,id`), dictionaryID, release)
	return items, err
}

func (r *SQLRepository) QueryPublishedItems(ctx context.Context, dictionaryID string, release int64, search Search) ([]Item, int64, error) {
	where := `dictionary_id=? AND release_version=? AND status='active'`
	args := []any{dictionaryID, release}
	if search.Keyword != "" {
		where += ` AND (LOWER(code) LIKE ? OR LOWER(name) LIKE ?)`
		like := `%` + strings.ToLower(search.Keyword) + `%`
		args = append(args, like, like)
	}
	if parentID, ok := search.Filters["parent_id"]; ok {
		where += ` AND parent_id=?`
		args = append(args, parentID)
	}
	if disabled, ok := search.Filters["disabled"]; ok {
		where += ` AND disabled=?`
		args = append(args, disabled == "true")
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, r.db.Rebind(`SELECT COUNT(*) FROM dictionary_release_items WHERE `+where), args...); err != nil {
		return nil, 0, err
	}
	order := `sort_order ASC,id ASC`
	switch search.Sort {
	case "code":
		order = `code ASC,id ASC`
	case "name":
		order = `name ASC,id ASC`
	}
	if search.Descending {
		order = strings.ReplaceAll(order, "ASC", "DESC")
	}
	queryArgs := append(append([]any{}, args...), search.PageSize, (search.Page-1)*search.PageSize)
	items := []Item{}
	err := r.db.SelectContext(ctx, &items, r.db.Rebind(`SELECT `+itemColumns+` FROM dictionary_release_items WHERE `+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`), queryArgs...)
	return items, total, err
}

func stale(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return ErrStaleVersion
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
