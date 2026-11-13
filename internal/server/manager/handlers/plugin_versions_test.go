package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryArtifactCannotOverwriteAnotherPluginFilename(t *testing.T) {
	m := &Manager{}
	if _, status, err := m.commitRegistryArtifact("unused", "other-plugin-1.0.0.so", &sdk.PluginInfo{ID: "source-test", Version: "1.0.0"}); err == nil || status != 422 {
		t.Fatalf("cross-plugin filename accepted: %d %v", status, err)
	}
}

func TestVerifiedCacheRejectsReplacementAndTampering(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{pluginsDir: filepath.Join(dir, "plugins")}
	source, bundle := filepath.Join(dir, "source"), filepath.Join(dir, "bundle")
	raw := []byte("verified artifact")
	sum := sha256.Sum256(raw)
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	item := registryCatalogItem{Plugin: registryArtifact{Plugin: "source-test", Version: "v1.0.0", SHA256: hex.EncodeToString(sum[:])}}
	if err := m.cacheVerifiedRegistryArtifact(item, source, bundle); err != nil {
		t.Fatal(err)
	}
	changed := item
	changed.Plugin.SHA256 = "different"
	if err := m.cacheVerifiedRegistryArtifact(changed, source, bundle); err == nil {
		t.Fatal("cached immutable version overwritten")
	}
	path := filepath.Join(m.pluginsDir, ".verified", "source-test", "v1.0.0", "plugin")
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.cacheVerifiedRegistryArtifact(item, source, bundle); err == nil {
		t.Fatal("cache tampering accepted")
	}
}
