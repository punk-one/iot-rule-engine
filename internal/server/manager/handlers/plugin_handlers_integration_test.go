package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestManagerScansConnectorAndPilotMigrationPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildPilotPluginsDir(t)
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/plugins?descriptors=true", nil)
	ctx.Request.URL.RawQuery = "descriptors=true"

	manager.ListPluginsHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Plugins []sdk.PluginInfo `json:"plugins"`
		Count   int              `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal plugins response failed: %v", err)
	}

	gotTypes := make(map[string]sdk.PluginType, len(resp.Plugins))
	for _, pluginInfo := range resp.Plugins {
		gotTypes[pluginInfo.ID] = pluginInfo.Type
	}

	expected := map[string]sdk.PluginType{
		"connect-s7":                  sdk.PluginTypeConnector,
		"connect-timescaledb":         sdk.PluginTypeConnector,
		"source-s7":                   sdk.PluginTypeSource,
		"sink-s7":                     sdk.PluginTypeSink,
		"sink-timeseries-timescaledb": sdk.PluginTypeSink,
	}

	if resp.Count != len(expected) {
		t.Fatalf("expected %d plugins, got %d (%+v)", len(expected), resp.Count, gotTypes)
	}

	for id, pluginType := range expected {
		if gotTypes[id] != pluginType {
			t.Fatalf("expected plugin %s type %s, got %s", id, pluginType, gotTypes[id])
		}
	}
}

func TestEnablePluginHandlerReloadsConnectorAndPilotPluginsFromDisk(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildPilotPluginsDir(t)
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	testCases := []struct {
		pluginID string
		wantType string
	}{
		{pluginID: "connect-s7", wantType: string(sdk.PluginTypeConnector)},
		{pluginID: "source-s7", wantType: string(sdk.PluginTypeSource)},
		{pluginID: "sink-timeseries-timescaledb", wantType: string(sdk.PluginTypeSink)},
	}

	for _, tc := range testCases {
		t.Run(tc.pluginID, func(t *testing.T) {
			if err := manager.pluginLoader.UnloadPlugin(tc.pluginID); err != nil {
				t.Fatalf("UnloadPlugin(%s) failed: %v", tc.pluginID, err)
			}

			if _, err := manager.pluginLoader.GetPluginClient(tc.pluginID); err == nil {
				t.Fatalf("expected %s to be unloaded", tc.pluginID)
			}

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/plugins/"+tc.pluginID+"/enable", nil)
			ctx.Params = gin.Params{{Key: "name", Value: tc.pluginID}}

			manager.EnablePluginHandler(ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
			}

			var meta PluginMeta
			if err := json.Unmarshal(recorder.Body.Bytes(), &meta); err != nil {
				t.Fatalf("unmarshal enable response failed: %v", err)
			}
			if meta.ID != tc.pluginID {
				t.Fatalf("expected plugin id %s, got %s", tc.pluginID, meta.ID)
			}
			if meta.Type != tc.wantType {
				t.Fatalf("expected plugin type %s, got %s", tc.wantType, meta.Type)
			}
			if !meta.Enabled {
				t.Fatalf("expected plugin %s to be enabled", tc.pluginID)
			}

			if _, err := manager.pluginLoader.GetPluginClient(tc.pluginID); err != nil {
				t.Fatalf("expected %s to be reloaded: %v", tc.pluginID, err)
			}
		})
	}
}

func TestListPluginsDescriptorIncludesPostgresSourceAndSinkBindings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildLocalPluginsDir(t, map[string]string{
		"connectors/connect_postgres": "connect-postgres-latest.so",
		"sources/source_postgres":     "source-relational-postgres-latest.so",
		"sinks/sink_postgres":         "sink-relational-postgres-latest.so",
	})
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/plugins?descriptors=true", nil)
	ctx.Request.URL.RawQuery = "descriptors=true"

	manager.ListPluginsHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Plugins []PluginDescriptorView `json:"plugins"`
		Count   int                    `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal plugins response failed: %v", err)
	}

	byID := make(map[string]PluginDescriptorView, len(resp.Plugins))
	for _, pluginInfo := range resp.Plugins {
		byID[pluginInfo.ID] = pluginInfo
	}

	connector, ok := byID["connect-postgres"]
	if !ok {
		t.Fatalf("expected connect-postgres in plugin list, got %+v", byID)
	}
	if connector.Plane != "connector" {
		t.Fatalf("expected connector plane, got %+v", connector)
	}
	if connector.ConnectorDescriptor == nil || connector.ConnectorDescriptor.Family != "postgres" {
		t.Fatalf("unexpected connector descriptor: %+v", connector.ConnectorDescriptor)
	}

	source, ok := byID["source-relational-postgres"]
	if !ok {
		t.Fatalf("expected source-relational-postgres in plugin list, got %+v", byID)
	}
	if source.Plane != "rule" {
		t.Fatalf("expected source plane rule, got %+v", source)
	}
	if source.ConnectorBinding == nil {
		t.Fatalf("expected source connector binding, got %+v", source)
	}
	if len(source.ConnectorBinding.AcceptedFamilies) != 1 || source.ConnectorBinding.AcceptedFamilies[0] != "postgres" {
		t.Fatalf("unexpected source accepted families: %+v", source.ConnectorBinding)
	}
	if len(source.ConnectorBinding.RequiredCapabilities) != 1 || source.ConnectorBinding.RequiredCapabilities[0] != "read" {
		t.Fatalf("unexpected source required capabilities: %+v", source.ConnectorBinding)
	}

	sink, ok := byID["sink-relational-postgres"]
	if !ok {
		t.Fatalf("expected sink-relational-postgres in plugin list, got %+v", byID)
	}
	if sink.Plane != "rule" {
		t.Fatalf("expected sink plane rule, got %+v", sink)
	}
	if sink.ConnectorBinding == nil {
		t.Fatalf("expected sink connector binding, got %+v", sink)
	}
	if len(sink.ConnectorBinding.AcceptedFamilies) != 1 || sink.ConnectorBinding.AcceptedFamilies[0] != "postgres" {
		t.Fatalf("unexpected sink accepted families: %+v", sink.ConnectorBinding)
	}
	if len(sink.ConnectorBinding.RequiredCapabilities) != 1 || sink.ConnectorBinding.RequiredCapabilities[0] != "write" {
		t.Fatalf("unexpected sink required capabilities: %+v", sink.ConnectorBinding)
	}
}

func buildPilotPluginsDir(t *testing.T) string {
	t.Helper()

	pluginsDir := t.TempDir()
	builds := []struct {
		subdir      string
		binaryName  string
		packagePath string
	}{
		{
			subdir:      "connectors",
			binaryName:  "connect-s7-latest.so",
			packagePath: "../../../../../iot-rule-plugins/connectors/connect_s7",
		},
		{
			subdir:      "connectors",
			binaryName:  "connect-timescaledb-latest.so",
			packagePath: "../../../../../iot-rule-plugins/connectors/connect_timescaledb",
		},
		{
			subdir:      "sources",
			binaryName:  "source-s7-latest.so",
			packagePath: "../../../../../iot-rule-plugins/sources/source_s7",
		},
		{
			subdir:      "sinks",
			binaryName:  "sink-s7-latest.so",
			packagePath: "../../../../../iot-rule-plugins/sinks/sink_s7",
		},
		{
			subdir:      "sinks",
			binaryName:  "sink-timeseries-timescaledb-latest.so",
			packagePath: "../../../../../iot-rule-plugins/sinks/timescaledb_sink",
		},
	}

	for _, build := range builds {
		if _, err := os.Stat(build.packagePath); err != nil {
			if os.IsNotExist(err) {
				t.Skipf("optional plugin source is not available: %s", build.packagePath)
			}
			t.Fatalf("stat plugin source %s failed: %v", build.packagePath, err)
		}

		targetDir := filepath.Join(pluginsDir, build.subdir)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", targetDir, err)
		}

		binaryPath := filepath.Join(targetDir, build.binaryName)
		buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
		buildCmd.Dir = build.packagePath
		if output, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("build plugin %s failed: %v\n%s", build.packagePath, err, output)
		}
	}

	return pluginsDir
}
