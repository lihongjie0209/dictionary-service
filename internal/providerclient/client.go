package providerclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/lihongjie0209/dictionary-service/internal/config"
	"github.com/lihongjie0209/dictionary-service/internal/dictionary"
	"github.com/lihongjie0209/dictionary-service/internal/grpcclient"
	"github.com/lihongjie0209/dictionary-service/internal/observability"
	commonv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/common/v1"
	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type connection struct {
	key  string
	conn *grpc.ClientConn
}

type Client struct {
	config  config.ProviderClient
	redis   *redis.Client
	metrics *observability.Metrics
	mu      sync.Mutex
	clients map[string]connection
	dial    func(grpcclient.Config) (*grpc.ClientConn, error)
}

func New(lc fx.Lifecycle, cfg config.Config, redisClient *redis.Client, metrics *observability.Metrics) *Client {
	client := &Client{config: cfg.ProviderClient, redis: redisClient, metrics: metrics, clients: map[string]connection{}, dial: grpcclient.Dial}
	lc.Append(fx.StopHook(func() error { return client.Close() }))
	return client
}

func (c *Client) ValidateTarget(target string) error {
	if !c.config.Enabled {
		return errors.New("provider client is disabled")
	}
	host, err := targetHost(target)
	if err != nil {
		return err
	}
	for _, suffix := range c.config.AllowedDNSSuffixes {
		if suffix != "" && strings.HasSuffix(host, strings.ToLower(suffix)) {
			return nil
		}
	}
	return fmt.Errorf("provider host %q is outside allowed DNS suffixes", host)
}

func (c *Client) Query(ctx context.Context, provider dictionary.Provider, tenantID, code string, search dictionary.Search) (dictionary.ProviderPage, error) {
	request := &dictionaryv1.DictionaryProviderServiceQueryRequest{
		Query: &dictionaryv1.QueryRequest{
			TenantId:       tenantID,
			DictionaryCode: code,
			Search: &dictionaryv1.SearchSpec{
				Keyword:    search.Keyword,
				Filters:    search.Filters,
				Sort:       search.Sort,
				Descending: search.Descending,
				Page: &commonv1.PageRequest{
					Page:     uint32(search.Page),
					PageSize: uint32(search.PageSize),
				},
				Cursor: search.Cursor,
				Limit:  uint32(search.Limit),
			},
		},
	}
	response := &dictionaryv1.DictionaryProviderServiceQueryResponse{}
	if err := c.call(ctx, provider, "query", request, response, func(client dictionaryv1.DictionaryProviderServiceClient, callCtx context.Context) (proto.Message, error) {
		return client.Query(callCtx, request)
	}); err != nil {
		return dictionary.ProviderPage{}, err
	}
	result := response.GetResult()
	page := result.GetResult().GetPage()
	items := make([]dictionary.Item, 0, len(result.GetItems()))
	for _, item := range result.GetItems() {
		items = append(items, dictionary.FromProtoItem(item))
	}
	return dictionary.ProviderPage{Items: items, Total: int64(page.GetTotal()), Page: int(page.GetPage()), PageSize: int(page.GetPageSize()), NextCursor: result.GetResult().GetNextCursor(), HasMore: result.GetResult().GetHasMore()}, nil
}

func (c *Client) Tree(ctx context.Context, provider dictionary.Provider, tenantID, code, mode, parentID, keyword string, maxDepth, maxNodes int, filters map[string]string) ([]dictionary.TreeNode, bool, error) {
	request := &dictionaryv1.DictionaryProviderServiceTreeRequest{Query: &dictionaryv1.TreeRequest{TenantId: tenantID, DictionaryCode: code, Mode: protoTreeMode(mode), ParentId: parentID, Keyword: keyword, MaxDepth: uint32(maxDepth), MaxNodes: uint32(maxNodes), Filters: filters}}
	response := &dictionaryv1.DictionaryProviderServiceTreeResponse{}
	if err := c.call(ctx, provider, "tree", request, response, func(client dictionaryv1.DictionaryProviderServiceClient, callCtx context.Context) (proto.Message, error) {
		return client.Tree(callCtx, request)
	}); err != nil {
		return nil, false, err
	}
	return fromProtoTree(response.GetResult().GetRoots()), response.GetResult().GetTruncated(), nil
}

func (c *Client) ResolveCodes(ctx context.Context, provider dictionary.Provider, tenantID, code string, codes []string) (map[string]dictionary.Item, error) {
	request := &dictionaryv1.DictionaryProviderServiceResolveCodesRequest{Query: &dictionaryv1.ResolveCodesRequest{TenantId: tenantID, DictionaryCode: code, Codes: codes}}
	response := &dictionaryv1.DictionaryProviderServiceResolveCodesResponse{}
	if err := c.call(ctx, provider, "resolve", request, response, func(client dictionaryv1.DictionaryProviderServiceClient, callCtx context.Context) (proto.Message, error) {
		return client.ResolveCodes(callCtx, request)
	}); err != nil {
		return nil, err
	}
	result := map[string]dictionary.Item{}
	for _, value := range response.GetResult().GetValues() {
		if value.GetFound() {
			result[value.GetCode()] = dictionary.FromProtoItem(value.GetItem())
		}
	}
	return result, nil
}

func (c *Client) call(ctx context.Context, provider dictionary.Provider, operation string, request, response proto.Message, invoke func(dictionaryv1.DictionaryProviderServiceClient, context.Context) (proto.Message, error)) error {
	cacheKey, err := c.cacheKey(provider, operation, request)
	if err != nil {
		return err
	}
	if c.redis != nil && provider.CacheTTLSeconds > 0 {
		if encoded, getErr := c.redis.Get(ctx, cacheKey).Bytes(); getErr == nil {
			return protojson.Unmarshal(encoded, response)
		} else if !errors.Is(getErr, redis.Nil) {
			return fmt.Errorf("read provider cache: %w", getErr)
		}
	}
	client, err := c.client(provider)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(provider.TimeoutMilliseconds)*time.Millisecond)
	defer cancel()
	value, err := invoke(client, callCtx)
	if err != nil {
		return err
	}
	if value == nil {
		return errors.New("dynamic dictionary provider returned an empty response")
	}
	proto.Reset(response)
	proto.Merge(response, value)
	if c.redis != nil && provider.CacheTTLSeconds > 0 {
		encoded, marshalErr := protojson.Marshal(response)
		if marshalErr != nil {
			return fmt.Errorf("marshal provider cache response: %w", marshalErr)
		}
		if setErr := c.redis.Set(ctx, cacheKey, encoded, time.Duration(provider.CacheTTLSeconds)*time.Second).Err(); setErr != nil {
			return fmt.Errorf("write provider cache: %w", setErr)
		}
	}
	return nil
}

func (c *Client) client(provider dictionary.Provider) (dictionaryv1.DictionaryProviderServiceClient, error) {
	if err := c.ValidateTarget(provider.Target); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s|%d", provider.Target, provider.TimeoutMilliseconds)
	c.mu.Lock()
	defer c.mu.Unlock()
	if current, ok := c.clients[provider.ID]; ok && current.key == key {
		return dictionaryv1.NewDictionaryProviderServiceClient(current.conn), nil
	}
	if current, ok := c.clients[provider.ID]; ok {
		_ = current.conn.Close()
		delete(c.clients, provider.ID)
	}
	conn, err := c.dial(grpcclient.Config{Name: provider.ServiceName, Target: provider.Target, Timeout: time.Duration(provider.TimeoutMilliseconds) * time.Millisecond, PSK: c.config.PSK, Retry: c.config.Retry, Breaker: c.config.Breaker, Metrics: c.metrics, TLS: grpcclient.TLSConfig{Enabled: c.config.TLS.Enabled, ServerName: c.config.TLS.ServerName, CAFile: c.config.TLS.CAFile, CertFile: c.config.TLS.CertFile, KeyFile: c.config.TLS.KeyFile, AllowInsecureToken: c.config.AllowInsecure}})
	if err != nil {
		return nil, err
	}
	c.clients[provider.ID] = connection{key: key, conn: conn}
	return dictionaryv1.NewDictionaryProviderServiceClient(conn), nil
}

func (c *Client) cacheKey(provider dictionary.Provider, operation string, request proto.Message) (string, error) {
	encoded, err := protojson.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal provider cache key: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "dictionary:provider:" + provider.ID + ":" + operation + ":" + hex.EncodeToString(sum[:]), nil
}
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var closeErr error
	for id, value := range c.clients {
		if err := value.conn.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("close provider %s: %w", id, err)
		}
	}
	c.clients = map[string]connection{}
	return closeErr
}
func targetHost(target string) (string, error) {
	value := strings.TrimPrefix(target, "dns:///")
	host, port, err := net.SplitHostPort(value)
	if err != nil || port == "" || host == "" || net.ParseIP(host) != nil || strings.EqualFold(host, "localhost") {
		return "", errors.New("provider target must be an allowed DNS name with port")
	}
	return strings.ToLower(strings.TrimSuffix(host, ".")), nil
}
func protoTreeMode(value string) dictionaryv1.TreeMode {
	switch value {
	case "children":
		return dictionaryv1.TreeMode_TREE_MODE_CHILDREN
	case "search_with_ancestors":
		return dictionaryv1.TreeMode_TREE_MODE_SEARCH_WITH_ANCESTORS
	default:
		return dictionaryv1.TreeMode_TREE_MODE_FULL
	}
}
func fromProtoTree(values []*dictionaryv1.TreeNode) []dictionary.TreeNode {
	result := make([]dictionary.TreeNode, 0, len(values))
	for _, value := range values {
		result = append(result, dictionary.TreeNode{Item: dictionary.FromProtoItem(value.GetItem()), Children: fromProtoTree(value.GetChildren())})
	}
	return result
}

var _ dictionary.ProviderGateway = (*Client)(nil)
