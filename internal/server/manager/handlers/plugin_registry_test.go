package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryURLRequiresHTTPSAndSameOrigin(t *testing.T) {
	m := &Manager{installPolicy: PluginInstallPolicy{RegistryURL: "https://rule.punkiot.com"}}
	if _, err := m.registryURL("/api/v1/plugins"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.registryURL("https://evil.example/plugin.so"); err == nil {
		t.Fatal("expected cross-origin URL rejection")
	}
	m.installPolicy.RegistryURL = "http://rule.punkiot.com"
	if _, err := m.registryURL("/api/v1/plugins"); err == nil {
		t.Fatal("expected insecure registry URL rejection")
	}
	m.installPolicy.RegistryURL = "http://127.0.0.1:8080"
	if _, err := m.registryURL("/api/v1/plugins"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugin.so")
	content := []byte("synthetic plugin")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	if err := verifyFileSHA256(path, hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	if err := verifyFileSHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected digest mismatch")
	}
}
