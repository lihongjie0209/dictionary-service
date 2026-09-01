package httptransport

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lihongjie0209/dictionary-service/internal/auth"
	"github.com/lihongjie0209/dictionary-service/internal/config"
	platformauthz "github.com/lihongjie0209/microservice-platform-go/authz"
	"github.com/lihongjie0209/microservice-platform-go/principal"
)

type authorizationStub struct{ err error }

func (a authorizationStub) Authorize(context.Context, principal.Principal, platformauthz.Requirement) error {
	return a.err
}

func TestDictionaryHTTPRequirementCoversEveryBusinessRoute(t *testing.T) {
	t.Parallel()
	routes := []string{
		"/api/v1/dictionaries/create", "/api/v1/dictionaries/update", "/api/v1/dictionaries/get", "/api/v1/dictionaries/list",
		"/api/v1/dictionaries/items/upsert", "/api/v1/dictionaries/items/list", "/api/v1/dictionaries/items/delete",
		"/api/v1/dictionaries/publish", "/api/v1/dictionaries/query", "/api/v1/dictionaries/tree", "/api/v1/dictionaries/resolve",
		"/api/v1/dictionaries/providers/list",
	}
	for _, route := range routes {
		if requirement, ok := dictionaryHTTPRequirement(route); !ok || requirement.Resource == "" || requirement.Action == "" || (requirement.Scope != platformauthz.ScopePrincipal && requirement.Scope != platformauthz.ScopePlatform) {
			t.Fatalf("route %q requirement = %+v, %v", route, requirement, ok)
		}
	}
	if requirement, ok := dictionaryHTTPRequirement("/api/v1/dictionaries/providers/list"); !ok || requirement.Scope != platformauthz.ScopePlatform {
		t.Fatalf("provider list requirement = %+v, %v", requirement, ok)
	}
	for _, route := range []string{"/api/v1/dictionaries/providers/register", "/api/v1/dictionaries/providers/heartbeat", "/api/v1/dictionaries/providers/unregister"} {
		if _, ok := dictionaryHTTPRequirement(route); ok {
			t.Fatalf("PSK provider lifecycle route %q must not require a user RBAC decision", route)
		}
	}
	if _, ok := dictionaryHTTPRequirement("/api/v1/version"); ok {
		t.Fatal("version endpoint must not require a domain permission")
	}
}

func TestAuthorizationFailsClosedAndClassifiesOutage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "denied", err: platformauthz.ErrDenied, status: http.StatusForbidden},
		{name: "unavailable", err: platformauthz.ErrDecisionUnavailable, status: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RequestID(), func(c *gin.Context) {
				identity := principal.Principal{ID: "user-1", Type: principal.TypeUser, TenantID: "tenant-1", MembershipID: "membership-1"}
				c.Request = c.Request.WithContext(principal.WithContext(c.Request.Context(), identity))
				c.Next()
			}, Authorization(true, authorizationStub{err: test.err}, slog.New(slog.NewTextHandler(io.Discard, nil))))
			router.POST("/api/v1/dictionaries/list", func(c *gin.Context) { OK(c, nil) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/dictionaries/list", nil))
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
		})
	}
}

func TestRequestID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.POST("/test", func(c *gin.Context) { OK(c, nil) })
	request := httptest.NewRequest(http.MethodPost, "/test", nil)
	request.Header.Set("X-Request-ID", "client-request-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("X-Request-ID"); got != "client-request-1" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	var response Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.RequestID != "client-request-1" {
		t.Fatalf("request_id = %q", response.RequestID)
	}
}

func TestAuthentication_PSKPrecedesSkipAndJWT(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	const key = "01234567890123456789012345678901"
	service := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: key, TTL: time.Hour}, Auth: config.Auth{ClientID: "client", ClientSecret: "secret"}})
	for _, test := range []struct {
		name   string
		header string
		status int
	}{
		{name: "valid PSK", header: "PSK " + key, status: http.StatusOK},
		{name: "PSK route does not become public", status: http.StatusUnauthorized},
		{name: "bearer cannot access PSK route", header: "Bearer invalid", status: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			router := gin.New()
			router.Use(RequestID(), Authentication(service, slog.New(slog.NewTextHandler(io.Discard, nil)), config.Auth{
				SkipHTTPPaths: []string{"/api/v1/external/*"},
				PSK:           config.PSK{Enabled: true, Key: key, HTTPPaths: []string{"/api/v1/external/*"}},
			}))
			router.POST("/api/v1/external/callback", func(c *gin.Context) {
				value, ok := principal.FromContext(c.Request.Context())
				if test.status == http.StatusOK && (!ok || value.ID != "psk" || value.Type != principal.TypeSystem) {
					c.AbortWithStatus(http.StatusInternalServerError)
					return
				}
				OK(c, nil)
			})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/external/callback", nil)
			request.Header.Set("Authorization", test.header)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
		})
	}
}

func TestAuthentication_ProviderLifecycleCannotFallBackToJWT(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	const key = "01234567890123456789012345678901"
	service := auth.New(config.Config{JWT: config.JWT{Issuer: "test", Secret: key, TTL: time.Hour}, Auth: config.Auth{ClientID: "client", ClientSecret: "secret"}})
	token, err := service.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(Authentication(service, slog.New(slog.NewTextHandler(io.Discard, nil)), config.Auth{}))
	router.POST("/api/v1/dictionaries/providers/register", func(c *gin.Context) { OK(c, nil) })
	request := httptest.NewRequest(http.MethodPost, "/api/v1/dictionaries/providers/register", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestRequireJSON(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), RequireJSON())
	router.POST("/test", func(c *gin.Context) { OK(c, nil) })
	request := httptest.NewRequest(http.MethodPost, "/test", io.NopCloser(&oneByteReader{}))
	request.ContentLength = 1
	request.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestTimeoutPropagatesCancellation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := gin.New()
	router.Use(RequestID(), Timeout(time.Millisecond, logger))
	router.POST("/test", func(c *gin.Context) { <-c.Request.Context().Done() })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/test", nil))
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
	}
}

type oneByteReader struct{}

func (*oneByteReader) Read(buffer []byte) (int, error) { buffer[0] = 'x'; return 1, io.EOF }
