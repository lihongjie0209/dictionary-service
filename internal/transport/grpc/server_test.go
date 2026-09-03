package grpctransport

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lihongjie0209/dictionary-service/internal/auth"
	"github.com/lihongjie0209/dictionary-service/internal/config"
	"github.com/lihongjie0209/dictionary-service/internal/requestid"
	platformauthz "github.com/lihongjie0209/microservice-platform-go/authz"
	"github.com/lihongjie0209/microservice-platform-go/principal"
	dictionaryv1 "github.com/lihongjie0209/platform-protos/gen/go/platform/dictionary/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestDictionaryGRPCRequirementCoversEveryProtectedBusinessMethod(t *testing.T) {
	t.Parallel()
	resolve := dictionaryGRPCRequirement(true)
	methods := []string{
		dictionaryv1.DictionaryService_CreateDictionary_FullMethodName, dictionaryv1.DictionaryService_UpdateDictionary_FullMethodName,
		dictionaryv1.DictionaryService_GetDictionary_FullMethodName, dictionaryv1.DictionaryService_ListDictionaries_FullMethodName,
		dictionaryv1.DictionaryService_UpsertItems_FullMethodName, dictionaryv1.DictionaryService_GetItem_FullMethodName, dictionaryv1.DictionaryService_DeleteItem_FullMethodName,
		dictionaryv1.DictionaryService_PublishDictionary_FullMethodName, dictionaryv1.DictionaryService_Query_FullMethodName,
		dictionaryv1.DictionaryService_Tree_FullMethodName, dictionaryv1.DictionaryService_ResolveCodes_FullMethodName,
		dictionaryv1.DictionaryService_ListProviders_FullMethodName,
	}
	for _, method := range methods {
		if requirement, ok := resolve(method); !ok || requirement.Resource == "" || requirement.Action == "" || (requirement.Scope != platformauthz.ScopePrincipal && requirement.Scope != platformauthz.ScopePlatform) {
			t.Fatalf("method %q requirement = %+v, %v", method, requirement, ok)
		}
	}
	if requirement, ok := resolve(dictionaryv1.DictionaryService_ListProviders_FullMethodName); !ok || requirement.Scope != platformauthz.ScopePlatform {
		t.Fatalf("provider list requirement = %+v, %v", requirement, ok)
	}
	for _, method := range []string{
		dictionaryv1.DictionaryService_RegisterProvider_FullMethodName,
		dictionaryv1.DictionaryService_HeartbeatProvider_FullMethodName,
		dictionaryv1.DictionaryService_UnregisterProvider_FullMethodName,
	} {
		if _, ok := resolve(method); ok {
			t.Fatalf("PSK provider lifecycle method %q must not require a tenant-member decision", method)
		}
	}
	if _, ok := dictionaryGRPCRequirement(false)(methods[0]); ok {
		t.Fatal("disabled authorization must not call the decision service")
	}
}

func TestRequestIDAndAuthenticationThroughGRPC(t *testing.T) {
	t.Parallel()
	authService := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: "01234567890123456789012345678901", TTL: time.Hour}, Auth: config.Auth{ClientID: "client", ClientSecret: "secret"}})
	token, err := authService.Issue("client")
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(requestIDInterceptor, authInterceptor(authService, config.Auth{})))
	grpc_health_v1.RegisterHealthServer(server, testHealthServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	ctx := metadata.AppendToOutgoingContext(requestid.WithContext(t.Context(), "grpc-test-1"), "authorization", "Bearer "+token, "x-request-id", "grpc-test-1")
	var header metadata.MD
	response, err := grpc_health_v1.NewHealthClient(connection).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.Header(&header))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health status = %s", response.GetStatus())
	}
	if got := header.Get("x-request-id"); len(got) != 1 || got[0] != "grpc-test-1" {
		t.Fatalf("x-request-id = %v", got)
	}
}

type testHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

func (testHealthServer) Check(context.Context, *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func TestAuthenticateGRPC_PSKWildcard(t *testing.T) {
	t.Parallel()
	const key = "01234567890123456789012345678901"
	authService := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: key, TTL: time.Hour}})
	cfg := config.Auth{
		SkipGRPCMethods: []string{"/hello.v1.UserService/*"},
		PSK:             config.PSK{Enabled: true, Key: key, GRPCMethods: []string{"/hello.v1.UserService/*"}},
	}
	for _, test := range []struct {
		name   string
		header string
		code   codes.Code
	}{
		{name: "valid", header: "PSK " + key, code: codes.OK},
		{name: "PSK precedes skip", code: codes.Unauthenticated},
		{name: "bearer rejected", header: "Bearer " + key, code: codes.Unauthenticated},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", test.header))
			authenticated, err := authenticateGRPC(ctx, "/hello.v1.UserService/GetUser", authService, cfg)
			if got := status.Code(err); got != test.code {
				t.Fatalf("status code = %s, want %s", got, test.code)
			}
			if test.code == codes.OK {
				value, ok := principal.FromContext(authenticated)
				if !ok || value.ID != "psk" || value.Type != principal.TypeSystem {
					t.Fatalf("principal = %#v, %v", value, ok)
				}
			}
		})
	}
}

func TestAuthenticateGRPC_ProviderLifecycleCannotFallBackToJWT(t *testing.T) {
	t.Parallel()
	const key = "01234567890123456789012345678901"
	service := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: key, TTL: time.Hour}, Auth: config.Auth{ClientID: "client", ClientSecret: "secret"}})
	token, err := service.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+token))
	_, err = authenticateGRPC(ctx, dictionaryv1.DictionaryService_RegisterProvider_FullMethodName, service, config.Auth{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("status code = %s, want %s", status.Code(err), codes.Unauthenticated)
	}
}

func TestAuthenticateGRPC_JWTInjectsPrincipal(t *testing.T) {
	t.Parallel()
	const key = "01234567890123456789012345678901"
	service := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: key, TTL: time.Hour}, Auth: config.Auth{ClientID: "client", ClientSecret: "secret"}})
	token, err := service.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+token))
	ctx, err = authenticateGRPC(ctx, "/hello.v1.UserService/GetUser", service, config.Auth{})
	if err != nil {
		t.Fatal(err)
	}
	value, ok := principal.FromContext(ctx)
	if !ok || value.ID != "user-1" || value.Type != principal.TypeServiceAccount {
		t.Fatalf("principal = %#v, %v", value, ok)
	}
}
