package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"net/http/httptest"
)

func TestVerifyTrustedArtifact(t *testing.T) {
	dir := t.TempDir()
	artifactName := "processor-1.0.0.so"
	artifactPath := filepath.Join(dir, artifactName)
	content := []byte("trusted plugin fixture")
	if err := os.WriteFile(artifactPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	manifestPath := filepath.Join(dir, "release-manifest.json")
	manifest := fmt.Sprintf(`{"schema_version":1,"plugins":[{"id":"processor","version":"1.0.0","type":"processor","sdk":"github.com/punk-one/iot-rule-plugin-sdk@v1.0.0","artifact":"processors/%s","sha256":"%s"}]}`,
		artifactName, hex.EncodeToString(digest[:]))
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	manager := &Manager{installPolicy: PluginInstallPolicy{Enabled: true, TrustedManifestPath: manifestPath}}
	record, err := manager.verifyTrustedArtifact(artifactPath, artifactName)
	if err != nil {
		t.Fatalf("verify trusted artifact: %v", err)
	}
	if record.ID != "processor" || record.Version != "1.0.0" {
		t.Fatalf("unexpected record: %+v", record)
	}

	if err := os.WriteFile(artifactPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.verifyTrustedArtifact(artifactPath, artifactName); err == nil {
		t.Fatal("expected tampered artifact to be rejected")
	}
}

func TestRuntimePluginInstallIsDisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/plugins/upload", nil)
	manager := &Manager{}
	if manager.preparePluginUpload(ctx) {
		t.Fatal("runtime plugin installation unexpectedly enabled")
	}
	if recorder.Code != 403 {
		t.Fatalf("status=%d, want 403", recorder.Code)
	}
}

func TestSafeUploadNameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../plugin.so", "dir/plugin.so", "plugin.txt", ""} {
		if _, err := safeUploadName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
	if got, err := safeUploadName("plugin-1.0.0.so"); err != nil || got != "plugin-1.0.0.so" {
		t.Fatalf("expected safe name, got %q err=%v", got, err)
	}
}
