package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestQueryTokenIsRejected(t *testing.T) {
	server := &Server{config: ServerConfig{AuthEnabled: true, AuthToken: "secret"}}
	router := server.SetupRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/plugins?token=secret", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("query token status=%d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestEnabledAuthWithoutProviderFailsClosed(t *testing.T) {
	server := &Server{config: ServerConfig{AuthEnabled: true}}
	router := server.SetupRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth provider status=%d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestExplicitInsecureModeAllowsAdminOperations(t *testing.T) {
	server := &Server{config: ServerConfig{AuthEnabled: false, PProfEnabled: true}}
	router := server.SetupRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("explicit insecure mode status=%d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestPProfRequiresAuthenticatedAdmin(t *testing.T) {
	server := &Server{config: ServerConfig{AuthEnabled: true, AuthToken: "secret", PProfEnabled: true}}
	router := server.SetupRouter()

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized pprof status=%d", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	request.Header.Set("Authorization", "Bearer secret")
	router.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized pprof status=%d", authorized.Code)
	}
}

func TestCORSUsesExplicitAllowList(t *testing.T) {
	server := &Server{config: ServerConfig{AllowedOrigins: []string{"https://console.example"}}}
	router := server.SetupRouter()

	allowed := httptest.NewRecorder()
	allowedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	allowedRequest.Header.Set("Origin", "https://console.example")
	router.ServeHTTP(allowed, allowedRequest)
	if allowed.Code != http.StatusOK || allowed.Header().Get("Access-Control-Allow-Origin") != "https://console.example" {
		t.Fatalf("allowed origin result: status=%d origin=%q", allowed.Code, allowed.Header().Get("Access-Control-Allow-Origin"))
	}

	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	deniedRequest.Header.Set("Origin", "https://evil.example")
	router.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied origin status=%d", denied.Code)
	}
}

func TestCORSAllowsSameOriginWithoutAllowList(t *testing.T) {
	server := &Server{config: ServerConfig{}}
	router := server.SetupRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://console.example/api/v1/health", nil)
	request.Header.Set("Origin", "http://console.example")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("same-origin status=%d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://console.example" {
		t.Fatalf("same-origin response header=%q", got)
	}
}

func TestCORSRejectsDifferentSchemeWithoutAllowList(t *testing.T) {
	server := &Server{config: ServerConfig{}}
	router := server.SetupRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://console.example/api/v1/health", nil)
	request.Header.Set("Origin", "http://console.example")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-scheme status=%d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestCORSAllowsConfiguredDevelopmentPreflight(t *testing.T) {
	server := &Server{config: ServerConfig{AllowedOrigins: []string{"http://localhost:3000"}}}
	router := server.SetupRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "http://localhost:11011/api/v1/auth/login", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("development preflight status=%d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("development preflight response header=%q", got)
	}
}

func TestRequireAdminRejectsOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_username", "operator")
		c.Next()
	})
	router.GET("/admin", requireAdmin(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("operator admin route status=%d", recorder.Code)
	}
}
