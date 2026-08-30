//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/lihongjie0209/dictionary-service/internal/app"
	"github.com/lihongjie0209/dictionary-service/internal/auth"
	"github.com/lihongjie0209/dictionary-service/internal/config"
	commonv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/common/v1"
	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"github.com/nats-io/nats.go"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func TestHTTPAndGRPCEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	postgresContainer, err := postgres.Run(ctx, "postgres:17-alpine", postgres.WithDatabase("app"), postgres.WithUsername("app"), postgres.WithPassword("app"), postgres.BasicWaitStrategies(), postgres.WithSQLDriver("pgx"))
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, postgresContainer)
	dsn, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	migrationPath, _ := filepath.Abs(filepath.Join("..", "migrations", "postgres"))

	redisContainer, err := rediscontainer.Run(ctx, "redis:7.4-alpine")
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, redisContainer)
	redisURL, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	redisOptions, err := goredis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	natsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "nats:2.14.6-alpine", ExposedPorts: []string{"4222/tcp"}, Cmd: []string{"--jetstream", "--store_dir=/data"}, WaitingFor: wait.ForLog("Server is ready")}, Started: true})
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, natsContainer)
	natsHost, err := natsContainer.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	natsPort, err := natsContainer.MappedPort(ctx, "4222/tcp")
	if err != nil {
		t.Fatal(err)
	}
	natsURL := "nats://" + net.JoinHostPort(natsHost, natsPort.Port())

	httpAddress := freeAddress(t)
	grpcAddress := freeAddress(t)
	const secret = "01234567890123456789012345678901"
	cfg := config.Config{
		Runtime:        config.Runtime{ActiveProfile: "integration"},
		App:            config.App{Name: "integration", Env: "integration", ShutdownTimeout: 10 * time.Second},
		HTTP:           config.HTTP{Address: httpAddress, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, RequestTimeout: 5 * time.Second, MaxBodyBytes: 1 << 20},
		GRPC:           config.GRPC{Enabled: true, Address: grpcAddress, MaxReceiveBytes: 4 << 20},
		Log:            config.Log{Level: "error", Format: "json", File: filepath.Join(t.TempDir(), "app.log"), MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1},
		Database:       config.Database{Enabled: true, Type: "postgres", DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, ConnMaxIdleTime: time.Minute, PingTimeout: 10 * time.Second},
		Migration:      config.Migration{AutoUp: true, Path: migrationPath, DatabaseURL: dsn, Table: "integration_e2e_schema_migrations"},
		Redis:          config.Redis{Enabled: true, Address: redisOptions.Addr, DB: redisOptions.DB, DialTimeout: 5 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second},
		Health:         config.Health{DatabaseTimeout: 2 * time.Second, RedisTimeout: 2 * time.Second},
		Observability:  config.Observability{MetricsEnabled: true},
		JWT:            config.JWT{Issuer: "integration", Secret: secret, TTL: time.Hour},
		Auth:           config.Auth{ClientID: "client", ClientSecret: "secret", SkipHTTPPaths: []string{"/api/v1/version"}, SkipGRPCMethods: []string{"/grpc.health.v1.Health/*"}, PSK: config.PSK{Enabled: true, Key: secret, GRPCMethods: []string{"/platform.dictionary.v1.DictionaryService/RegisterProvider"}}},
		ProviderClient: config.ProviderClient{Enabled: true, AllowedDNSSuffixes: []string{"-service"}, PSK: secret, AllowInsecure: true, Retry: config.Retry{MaxAttempts: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}},
		Cron:           config.Cron{Enabled: false, Timezone: "UTC"},
		User:           config.User{CacheTTL: time.Minute, LockTTL: 10 * time.Second, LockRetryDelay: 20 * time.Millisecond},
		Idempotency:    config.Idempotency{Enabled: true, ProcessingTTL: 30 * time.Second, ResultTTL: time.Hour, FailureTTL: time.Minute},
		EventBus:       config.EventBus{Enabled: true, URLs: []string{natsURL}, StreamName: "PLATFORM_EVENTS", Subjects: []string{"platform.>"}, Storage: "memory", MaxAge: time.Hour, DuplicateWindow: time.Minute, ConnectTimeout: 5 * time.Second, ReconnectWait: time.Second, PublishTimeout: 5 * time.Second, ConsumerAckWait: 30 * time.Second, ConsumerMaxDeliver: 3, DispatchInterval: 20 * time.Millisecond, DispatchBatchSize: 20, DispatchLease: 30 * time.Second, DispatchRetryDelay: 100 * time.Millisecond},
	}
	application := app.New(cfg)
	if err := application.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = application.Stop(stopCtx)
	})
	token, err := auth.New(cfg).Issue("client")
	if err != nil {
		t.Fatal(err)
	}
	natsConnection, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(natsConnection.Close)
	publishedEvents, err := natsConnection.SubscribeSync("platform.dictionary.dictionary.published.v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := natsConnection.Flush(); err != nil {
		t.Fatal(err)
	}

	baseURL := "http://" + httpAddress
	if status := postJSON(t, baseURL+"/api/v1/version", "", "", `{}`); status != http.StatusOK {
		t.Fatalf("public version status = %d", status)
	}
	if status := postJSON(t, baseURL+"/api/v1/me", "Bearer "+token, "", `{}`); status != http.StatusOK {
		t.Fatalf("JWT status = %d", status)
	}
	created := postSuccess(t, baseURL+"/api/v1/dictionaries/create", "Bearer "+token, `{"code":"order.status","name":"Order status","metadata_json":"{}"}`)
	var dictionaryValue struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	decodeBody(t, created, &dictionaryValue)
	postSuccess(t, baseURL+"/api/v1/dictionaries/items/upsert", "Bearer "+token, fmt.Sprintf(`{"dictionary_id":%q,"items":[{"code":"pending","name":"Pending","leaf":true,"status":"active","metadata_json":"{}"},{"code":"paid","name":"Paid","leaf":true,"status":"active","metadata_json":"{}"}]}`, dictionaryValue.ID))
	postSuccess(t, baseURL+"/api/v1/dictionaries/publish", "Bearer "+token, fmt.Sprintf(`{"dictionary_id":%q,"dictionary_version":%d,"comment":"integration"}`, dictionaryValue.ID, dictionaryValue.Version))
	publishedMessage, err := publishedEvents.NextMsg(10 * time.Second)
	if err != nil {
		t.Fatalf("dictionary published event: %v", err)
	}
	envelope := &commonv1.EventEnvelope{}
	if err := proto.Unmarshal(publishedMessage.Data, envelope); err != nil || envelope.GetEventType() != "platform.dictionary.v1.DictionaryPublished" || envelope.GetAggregateId() != dictionaryValue.ID {
		t.Fatalf("published envelope = %+v, error=%v", envelope, err)
	}
	queried := postSuccess(t, baseURL+"/api/v1/dictionaries/query", "Bearer "+token, `{"dictionary_code":"order.status","keyword":"paid","page":1,"page_size":20}`)
	var queryResult struct {
		Items []struct {
			Code string `json:"code"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	decodeBody(t, queried, &queryResult)
	if queryResult.Total != 1 || len(queryResult.Items) != 1 || queryResult.Items[0].Code != "paid" {
		t.Fatalf("unexpected query result: %+v", queryResult)
	}

	connection, err := grpc.NewClient(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	healthResponse, err := grpc_health_v1.NewHealthClient(connection).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil || healthResponse.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health = %v, %v", healthResponse, err)
	}
	pskCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "PSK "+secret)
	if _, err := dictionaryv1.NewDictionaryServiceClient(connection).RegisterProvider(pskCtx, &dictionaryv1.RegisterProviderRequest{ServiceName: "integration-provider", Target: "integration-service:9090", Capabilities: []*dictionaryv1.ProviderCapability{{DictionaryCode: "integration.values"}}, LeaseSeconds: 60}); err != nil {
		t.Fatalf("PSK RegisterProvider: %v", err)
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func postJSON(t *testing.T, target, authorization, key, body string) int {
	t.Helper()
	_, status := postJSONBody(t, target, authorization, key, body)
	return status
}
func postJSONBody(t *testing.T, target, authorization, key, body string) ([]byte, int) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var validJSON any
	if err := json.Unmarshal(data, &validJSON); err != nil {
		t.Fatalf("invalid JSON response: %v (%s)", err, data)
	}
	return data, response.StatusCode
}

type apiResponse struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Body      json.RawMessage `json:"body"`
	RequestID string          `json:"request_id"`
}

func postSuccess(t *testing.T, target, authorization, body string) apiResponse {
	t.Helper()
	data, status := postJSONBody(t, target, authorization, "", body)
	var response apiResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || response.Code != 0 || response.RequestID == "" {
		t.Fatalf("POST %s status=%d response=%s", target, status, data)
	}
	return response
}
func decodeBody(t *testing.T, response apiResponse, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body, target); err != nil {
		t.Fatal(err)
	}
}

var _ = fmt.Sprintf
