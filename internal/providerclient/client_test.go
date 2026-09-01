package providerclient

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/lihongjie0209/dictionary-service/internal/config"
	"github.com/lihongjie0209/dictionary-service/internal/dictionary"
	"github.com/lihongjie0209/dictionary-service/internal/grpcclient"
	commonv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/common/v1"
	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestClient_QueryUsesProviderAndRedisCache(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	providerServer := &fakeProviderServer{}
	server := grpc.NewServer()
	dictionaryv1.RegisterDictionaryProviderServiceServer(server, providerServer)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	client := &Client{
		config:  config.ProviderClient{Enabled: true, AllowedDNSSuffixes: []string{"-service"}},
		redis:   redisClient,
		clients: map[string]connection{},
		dial: func(grpcclient.Config) (*grpc.ClientConn, error) {
			return grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
		},
	}
	t.Cleanup(func() { _ = client.Close() })
	provider := dictionary.Provider{ID: "provider-1", ServiceName: "tenant-service", Target: "tenant-service:9090", CacheTTLSeconds: 60, TimeoutMilliseconds: 1000}
	for attempt := 0; attempt < 2; attempt++ {
		page, err := client.Query(t.Context(), provider, "tenant-1", "application-1", "tenant.organization_units", dictionary.Search{Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Code != "engineering" {
			t.Fatalf("Query() = %+v", page)
		}
	}
	if calls := providerServer.calls.Load(); calls != 1 {
		t.Fatalf("provider Query calls = %d, want 1 due to cache", calls)
	}
	if applicationID, _ := providerServer.applicationID.Load().(string); applicationID != "application-1" {
		t.Fatalf("provider application_id = %q", applicationID)
	}
}

func TestClient_ValidateTargetRejectsSSRFAddresses(t *testing.T) {
	client := &Client{config: config.ProviderClient{Enabled: true, AllowedDNSSuffixes: []string{".svc.cluster.local"}}}
	for _, target := range []string{"127.0.0.1:9090", "localhost:9090", "metadata.google.internal:80", "missing-port"} {
		if err := client.ValidateTarget(target); err == nil {
			t.Fatalf("ValidateTarget(%q) error = nil", target)
		}
	}
	if err := client.ValidateTarget("tenant-service.platform.svc.cluster.local:9090"); err != nil {
		t.Fatalf("ValidateTarget(allowed) error = %v", err)
	}
}

type fakeProviderServer struct {
	dictionaryv1.UnimplementedDictionaryProviderServiceServer
	calls         atomic.Int32
	applicationID atomic.Value
}

func (s *fakeProviderServer) Query(_ context.Context, request *dictionaryv1.DictionaryProviderServiceQueryRequest) (*dictionaryv1.DictionaryProviderServiceQueryResponse, error) {
	s.calls.Add(1)
	s.applicationID.Store(request.GetQuery().GetApplicationId())
	return &dictionaryv1.DictionaryProviderServiceQueryResponse{Result: &dictionaryv1.QueryResponse{Items: []*dictionaryv1.DictionaryItem{{Id: "org-1", DictionaryCode: "tenant.organization_units", Code: "engineering", Name: "Engineering", Status: "active"}}, Result: &dictionaryv1.ResultPage{Page: &commonv1.PageResult{Total: 1, Page: 1, PageSize: 20}}}}, nil
}
