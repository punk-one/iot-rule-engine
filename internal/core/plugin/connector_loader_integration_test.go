package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestLoaderSupportsConnectorPlugins(t *testing.T) {
	tmpDir := t.TempDir()
	connectorsDir := filepath.Join(tmpDir, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatalf("mkdir connectors dir: %v", err)
	}

	binaryPath := filepath.Join(connectorsDir, "connect-dummy-latest.so")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../connector/testdata/dummy_connector")
	buildCmd.Dir = "."
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build dummy connector: %v\n%s", err, output)
	}

	loader := NewLoader(tmpDir)

	info, err := loader.VerifyPlugin(binaryPath)
	if err != nil {
		t.Fatalf("verify connector plugin: %v", err)
	}
	if info.Type != sdk.PluginTypeConnector {
		t.Fatalf("expected connector plugin type, got %s", info.Type)
	}

	client, err := loader.LoadConnectorPlugin(binaryPath)
	if err != nil {
		t.Fatalf("load connector plugin: %v", err)
	}
	if client.Info.Type != sdk.PluginTypeConnector {
		t.Fatalf("expected loaded connector plugin type, got %s", client.Info.Type)
	}
	if client.ConnectorRPCClient == nil {
		t.Fatal("expected connector rpc client to be set")
	}

	resolved, err := loader.GetConnectorPluginClient(info.ID)
	if err != nil {
		t.Fatalf("get connector plugin client: %v", err)
	}
	if resolved.Path != binaryPath {
		t.Fatalf("unexpected connector path: %s", resolved.Path)
	}

	if err := loader.UnloadPlugin(info.ID); err != nil {
		t.Fatalf("unload connector plugin: %v", err)
	}

	if err := loader.ScanPlugins(); err != nil {
		t.Fatalf("scan plugins: %v", err)
	}

	scanned, err := loader.GetConnectorPluginClient(info.ID)
	if err != nil {
		t.Fatalf("get scanned connector plugin client: %v", err)
	}
	if scanned.Info.Type != sdk.PluginTypeConnector {
		t.Fatalf("expected scanned connector plugin type, got %s", scanned.Info.Type)
	}
}
