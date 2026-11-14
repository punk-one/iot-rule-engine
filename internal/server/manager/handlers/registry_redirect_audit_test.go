package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistryRedirectRejectsSchemeDowngrade(t *testing.T) {
	check := (&Manager{}).registryHTTPClient().CheckRedirect
	from := httptest.NewRequest("GET", "https://registry.example:443/catalog", nil)
	downgrade := httptest.NewRequest("GET", "http://registry.example:443/catalog", nil)
	if err := check(downgrade, []*http.Request{from}); err == nil {
		t.Fatal("HTTPS to HTTP redirect was accepted")
	}
	sameOrigin := httptest.NewRequest("GET", "https://registry.example:443/next", nil)
	if err := check(sameOrigin, []*http.Request{from}); err != nil {
		t.Fatalf("same-origin redirect rejected: %v", err)
	}
}
