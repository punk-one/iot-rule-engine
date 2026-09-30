package connector

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	coreplugin "github.com/punk-one/iot-rule-engine/internal/core/plugin"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestHostResolvesExtractedConnectorPlugins(t *testing.T) {
	tmpDir := t.TempDir()
	connectorsDir := filepath.Join(tmpDir, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatalf("mkdir connectors dir: %v", err)
	}

	buildConnector(t, filepath.Join(connectorsDir, "connect-timescaledb-latest.so"), "../../../../iot-rule-plugins/connectors/connect_timescaledb")
	buildConnector(t, filepath.Join(connectorsDir, "connect-s7-latest.so"), "../../../../iot-rule-plugins/connectors/connect_s7")

	loader := coreplugin.NewLoader(tmpDir)
	host := NewHost(loader)
	defer host.Close()

	bus := NewResourceBus(ResourceBusOptions{})
	bus.SetProviderResolver(host.Resolve)

	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "tsdb-resource",
		PluginID: "connect-timescaledb",
		Connection: mustJSON(t, map[string]any{
			"dsn": "host=127.0.0.1 user=test dbname=test",
		}),
		Enabled: true,
	}); err != nil {
		t.Fatalf("register tsdb resource: %v", err)
	}
	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:            "s7-resource",
		PluginID:      "connect-s7",
		SessionPolicy: "split_rw",
		Connection: mustJSON(t, map[string]any{
			"host": "192.0.2.10",
			"port": 102,
		}),
		Enabled: true,
	}); err != nil {
		t.Fatalf("register s7 resource: %v", err)
	}

	tsdbToken, err := bus.Bind(context.Background(), "tsdb-resource")
	if err != nil {
		t.Fatalf("bind tsdb resource: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), tsdbToken) }()

	s7Token, err := bus.Bind(context.Background(), "s7-resource")
	if err != nil {
		t.Fatalf("bind s7 resource: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), s7Token) }()

	tsdbResp, err := bus.Execute(context.Background(), tsdbToken, sdk.ConnectorRequest{
		Kind:   sdk.ConnectorRequestMetadata,
		Target: "info",
	})
	if err != nil {
		t.Fatalf("execute tsdb metadata info: %v", err)
	}
	var tsdbInfo map[string]any
	if err := json.Unmarshal(tsdbResp.Payload, &tsdbInfo); err != nil {
		t.Fatalf("unmarshal tsdb payload: %v", err)
	}
	if tsdbInfo["plugin_id"] != "connect-timescaledb" {
		t.Fatalf("unexpected tsdb metadata: %+v", tsdbInfo)
	}

	s7Resp, err := bus.Execute(context.Background(), s7Token, sdk.ConnectorRequest{
		Kind:   sdk.ConnectorRequestMetadata,
		Target: "parse_addresses",
		Payload: mustJSON(t, map[string]any{
			"points": []map[string]any{
				{"name": "running", "address": "DB1.DBX0.0", "value_type": "Bool"},
			},
		}),
	})
	if err != nil {
		t.Fatalf("execute s7 parse_addresses: %v", err)
	}
	var parsed struct {
		Points []map[string]any `json:"points"`
	}
	if err := json.Unmarshal(s7Resp.Payload, &parsed); err != nil {
		t.Fatalf("unmarshal s7 payload: %v", err)
	}
	if len(parsed.Points) != 1 || parsed.Points[0]["db_number"] != float64(1) {
		t.Fatalf("unexpected s7 metadata payload: %+v", parsed.Points)
	}
}

func buildConnector(t *testing.T, binaryPath, packagePath string) {
	t.Helper()
	if _, err := os.Stat(packagePath); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("optional connector source is not available: %s", packagePath)
		}
		t.Fatalf("stat connector source %s failed: %v", packagePath, err)
	}
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = packagePath
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build connector %s: %v\n%s", packagePath, err, output)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}
