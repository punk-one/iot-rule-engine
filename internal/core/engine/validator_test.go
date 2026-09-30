package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestGetNodePluginTypeSupportsHyphenUnderscoreVariants(t *testing.T) {
	pluginsDir := t.TempDir()
	processorDir := filepath.Join(pluginsDir, "processors")
	if err := os.MkdirAll(processorDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	sourcePath := filepath.Join(processorDir, "buffer_node-latest.so")
	buildCmd := exec.Command("go", "build", "-o", sourcePath, "./testdata/buffer_node")
	buildCmd.Dir = "."
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build buffer-node fixture failed: %v\n%s", err, output)
	}

	loader := plugin.NewLoader(pluginsDir)
	nodeConfig := &schema.NodeConfig{ID: "buffer", Plugin: "buffer-node"}

	pluginType, err := getNodePluginType(nodeConfig, loader)
	if err != nil {
		t.Fatalf("getNodePluginType failed: %v", err)
	}
	if pluginType != sdk.PluginTypeProcessor {
		t.Fatalf("expected processor plugin type, got %s", pluginType)
	}
}
