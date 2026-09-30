package handlers

import (
	"bytes"
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
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestConnectorResourceCRUDHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildConnectorPluginsDir(t)
	manager := NewManager(db, pluginsDir, "", nil)

	resource := sdk.ConnectorResource{
		ID:            "conn-plc-01",
		Name:          "PLC 01",
		PluginID:      "connect-dummy",
		SessionPolicy: "shared_duplex",
		Enabled:       true,
	}

	body, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("marshal resource failed: %v", err)
	}

	createRecorder := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRecorder)
	createCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/connectors", bytes.NewReader(body))
	createCtx.Request.Header.Set("Content-Type", "application/json")
	manager.CreateConnectorResourceHandler(createCtx)

	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", createRecorder.Code, createRecorder.Body.String())
	}

	getRecorder := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRecorder)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/"+resource.ID, nil)
	getCtx.Params = gin.Params{{Key: "id", Value: resource.ID}}
	manager.GetConnectorResourceHandler(getCtx)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	resource.Name = "PLC 01 Updated"
	resource.Enabled = false
	updateBody, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("marshal update resource failed: %v", err)
	}

	updateRecorder := httptest.NewRecorder()
	updateCtx, _ := gin.CreateTestContext(updateRecorder)
	updateCtx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/connectors/"+resource.ID, bytes.NewReader(updateBody))
	updateCtx.Request.Header.Set("Content-Type", "application/json")
	updateCtx.Params = gin.Params{{Key: "id", Value: resource.ID}}
	manager.UpdateConnectorResourceHandler(updateCtx)

	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}

	listRecorder := httptest.NewRecorder()
	listCtx, _ := gin.CreateTestContext(listRecorder)
	listCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/connectors", nil)
	manager.ListConnectorResourcesHandler(listCtx)

	if listRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	var listResp struct {
		Connectors []ConnectorResourceView `json:"connectors"`
		Count      int                     `json:"count"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response failed: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Connectors) != 1 {
		t.Fatalf("expected 1 connector, got %+v", listResp)
	}
	if listResp.Connectors[0].Name != "PLC 01 Updated" {
		t.Fatalf("expected updated connector name, got %+v", listResp.Connectors[0])
	}

	deleteRecorder := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRecorder)
	deleteCtx.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/connectors/"+resource.ID, nil)
	deleteCtx.Params = gin.Params{{Key: "id", Value: resource.ID}}
	manager.DeleteConnectorResourceHandler(deleteCtx)

	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
}

func TestValidateNodeConfigFailsForMissingConnectorResource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildConnectorPluginsDir(t)
	manager := NewManager(db, pluginsDir, "", nil)

	err = manager.validateNodeConfig(&schema.NodeConfig{
		ID:     "src-1",
		Type:   "source",
		Plugin: "connect-dummy",
		Config: map[string]interface{}{
			"connector_ref": "missing-resource",
		},
	})
	if err == nil {
		t.Fatal("expected missing connector_ref to fail validation")
	}
	if got := err.Error(); got != "connector_ref missing-resource not found" {
		t.Fatalf("unexpected validation error: %s", got)
	}
}

func TestListCompatibleConnectorsFiltersByBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildPilotPluginsDir(t)
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	s7Connection, _ := json.Marshal(map[string]interface{}{
		"host": "192.0.2.87",
		"port": 10102,
		"rack": 0,
		"slot": 1,
	})
	tsConnection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/edgex_db?sslmode=disable",
	})

	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "s7-acm006",
		Name:       "ACM 006 PLC",
		PluginID:   "connect-s7",
		Connection: s7Connection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save s7 connector failed: %v", err)
	}
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "ts-main",
		Name:       "Timescale Main",
		PluginID:   "connect-timescaledb",
		Connection: tsConnection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save timescaledb connector failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/plugins/source-s7/compatible-connectors", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "source-s7"}}

	manager.ListCompatibleConnectorsHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Connectors []ConnectorResourceView `json:"connectors"`
		Count      int                     `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal compatible connectors response failed: %v", err)
	}
	if resp.Count != 1 || len(resp.Connectors) != 1 {
		t.Fatalf("expected exactly one compatible connector, got %+v", resp)
	}
	if resp.Connectors[0].ID != "s7-acm006" {
		t.Fatalf("expected s7 connector, got %+v", resp.Connectors[0])
	}
	if resp.Connectors[0].ConnectionSummary == "" {
		t.Fatalf("expected connection summary to be populated")
	}
}

func TestValidateNodeConfigRejectsIncompatibleConnectorFamily(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildPilotPluginsDir(t)
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	connection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/edgex_db?sslmode=disable",
	})
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "ts-main",
		Name:       "Timescale Main",
		PluginID:   "connect-timescaledb",
		Connection: connection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save timescaledb connector failed: %v", err)
	}

	err = manager.validateNodeConfig(&schema.NodeConfig{
		ID:     "src-1",
		Type:   "source",
		Plugin: "source-s7",
		Config: map[string]interface{}{
			"connector_ref": "ts-main",
			"output": map[string]interface{}{
				"device_code": "acm006",
			},
			"points": []interface{}{
				map[string]interface{}{
					"address":    "MD0",
					"name":       "total_out",
					"value_type": "Int32",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected incompatible connector_ref to fail validation")
	}
	if got := err.Error(); got == "" || !bytes.Contains([]byte(got), []byte("incompatible")) {
		t.Fatalf("expected incompatible validation error, got %v", err)
	}
}

func TestListCompatibleConnectorsFiltersPostgresFamily(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildLocalPluginsDir(t, map[string]string{
		"connectors/connect_postgres":    "connect-postgres-latest.so",
		"connectors/connect_timescaledb": "connect-timescaledb-latest.so",
		"sources/source_postgres":        "source-relational-postgres-latest.so",
		"sources/source_timescaledb":     "source-timeseries-timescaledb-latest.so",
		"sinks/sink_postgres":            "sink-relational-postgres-latest.so",
		"sinks/timescaledb_sink":         "sink-timeseries-timescaledb-latest.so",
	})
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	postgresConnection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/appdb?sslmode=disable",
	})
	timescaleConnection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/tsdb?sslmode=disable",
	})

	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "pg-main",
		Name:       "Postgres Main",
		PluginID:   "connect-postgres",
		Connection: postgresConnection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save postgres connector failed: %v", err)
	}
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "ts-main",
		Name:       "Timescale Main",
		PluginID:   "connect-timescaledb",
		Connection: timescaleConnection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save timescaledb connector failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/plugins/source-relational-postgres/compatible-connectors", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "source-relational-postgres"}}

	manager.ListCompatibleConnectorsHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Connectors []ConnectorResourceView `json:"connectors"`
		Count      int                     `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal compatible connectors response failed: %v", err)
	}
	if resp.Count != 1 || len(resp.Connectors) != 1 {
		t.Fatalf("expected exactly one compatible connector, got %+v", resp)
	}
	if resp.Connectors[0].ID != "pg-main" {
		t.Fatalf("expected postgres connector, got %+v", resp.Connectors[0])
	}
	if resp.Connectors[0].ConnectionSummary != "127.0.0.1:5432/appdb" {
		t.Fatalf("unexpected connection summary: %+v", resp.Connectors[0].ConnectionSummary)
	}
}

func TestValidateNodeConfigAcceptsCompatiblePostgresConnector(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildLocalPluginsDir(t, map[string]string{
		"connectors/connect_postgres": "connect-postgres-latest.so",
		"sources/source_postgres":     "source-relational-postgres-latest.so",
	})
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	connection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/appdb?sslmode=disable",
	})
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "pg-main",
		Name:       "Postgres Main",
		PluginID:   "connect-postgres",
		Connection: connection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save postgres connector failed: %v", err)
	}

	err = manager.validateNodeConfig(&schema.NodeConfig{
		ID:     "src-pg-1",
		Type:   "source",
		Plugin: "source-relational-postgres",
		Config: map[string]interface{}{
			"connector_ref": "pg-main",
			"output": map[string]interface{}{
				"device_code": "dev-1",
			},
			"query": map[string]interface{}{
				"sql": "select 1",
			},
		},
	})
	if err != nil {
		t.Fatalf("expected postgres connector to be accepted, got %v", err)
	}
}

func TestValidateNodeConfigRejectsTimescaleConnectorForPostgresFamily(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildLocalPluginsDir(t, map[string]string{
		"connectors/connect_timescaledb": "connect-timescaledb-latest.so",
		"sources/source_postgres":        "source-relational-postgres-latest.so",
	})
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	connection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/tsdb?sslmode=disable",
	})
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "ts-main",
		Name:       "Timescale Main",
		PluginID:   "connect-timescaledb",
		Connection: connection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save timescaledb connector failed: %v", err)
	}

	err = manager.validateNodeConfig(&schema.NodeConfig{
		ID:     "src-pg-1",
		Type:   "source",
		Plugin: "source-relational-postgres",
		Config: map[string]interface{}{
			"connector_ref": "ts-main",
			"output": map[string]interface{}{
				"device_code": "dev-1",
			},
			"query": map[string]interface{}{
				"sql": "select 1",
			},
		},
	})
	if err == nil {
		t.Fatal("expected timescaledb connector to be rejected by postgres source")
	}
	if got := err.Error(); got == "" || !bytes.Contains([]byte(got), []byte("incompatible")) {
		t.Fatalf("expected incompatible validation error, got %v", err)
	}
}

func TestListConnectorResourcesIncludesPostgresDisplayFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildLocalPluginsDir(t, map[string]string{
		"connectors/connect_postgres": "connect-postgres-latest.so",
	})
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	connection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/appdb?sslmode=disable",
	})
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "pg-main",
		Name:       "Postgres Main",
		PluginID:   "connect-postgres",
		Connection: connection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save postgres connector failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/connectors", nil)

	manager.ListConnectorResourcesHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Connectors []ConnectorResourceView `json:"connectors"`
		Count      int                     `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal connector list response failed: %v", err)
	}
	if resp.Count != 1 || len(resp.Connectors) != 1 {
		t.Fatalf("expected one connector, got %+v", resp)
	}

	view := resp.Connectors[0]
	if view.ID != "pg-main" {
		t.Fatalf("expected pg-main connector, got %+v", view)
	}
	if view.PluginInfo == nil {
		t.Fatalf("expected plugin info, got %+v", view)
	}
	if view.PluginInfo.ID != "connect-postgres" || view.PluginInfo.Plane != "connector" {
		t.Fatalf("unexpected plugin info: %+v", view.PluginInfo)
	}
	if view.PluginInfo.ConnectorDescriptor == nil || view.PluginInfo.ConnectorDescriptor.Family != "postgres" {
		t.Fatalf("unexpected connector descriptor: %+v", view.PluginInfo.ConnectorDescriptor)
	}
	if view.ConnectionPreview["host"] != "127.0.0.1" || view.ConnectionPreview["port"] != "5432" || view.ConnectionPreview["database"] != "appdb" {
		t.Fatalf("unexpected connection preview: %+v", view.ConnectionPreview)
	}
	if view.ConnectionSummary != "127.0.0.1:5432/appdb" {
		t.Fatalf("unexpected connection summary: %+v", view.ConnectionSummary)
	}
	if view.Compatibility != nil {
		t.Fatalf("expected compatibility to be nil for connector list view, got %+v", view.Compatibility)
	}
	if view.ConnectorStatus == nil || view.ConnectorStatus.Status != sdk.ResourceStatusUnknown {
		t.Fatalf("unexpected connector status: %+v", view.ConnectorStatus)
	}
}

func TestListCompatibleConnectorsShowsPostgresSinkDisplayFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	pluginsDir := buildLocalPluginsDir(t, map[string]string{
		"connectors/connect_postgres":    "connect-postgres-latest.so",
		"connectors/connect_timescaledb": "connect-timescaledb-latest.so",
		"sinks/sink_postgres":            "sink-relational-postgres-latest.so",
	})
	manager := NewManager(db, pluginsDir, "", nil)
	defer manager.pluginLoader.Close()

	postgresConnection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/appdb?sslmode=disable",
	})
	timescaleConnection, _ := json.Marshal(map[string]interface{}{
		"dsn": "postgres://user:pass@127.0.0.1:5432/tsdb?sslmode=disable",
	})

	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "pg-main",
		Name:       "Postgres Main",
		PluginID:   "connect-postgres",
		Connection: postgresConnection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save postgres connector failed: %v", err)
	}
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID:         "ts-main",
		Name:       "Timescale Main",
		PluginID:   "connect-timescaledb",
		Connection: timescaleConnection,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("save timescaledb connector failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/plugins/sink-relational-postgres/compatible-connectors", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "sink-relational-postgres"}}

	manager.ListCompatibleConnectorsHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Plugin     PluginDescriptorView    `json:"plugin"`
		Connectors []ConnectorResourceView `json:"connectors"`
		Count      int                     `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal compatible connectors response failed: %v", err)
	}
	if resp.Plugin.ID != "sink-relational-postgres" || resp.Plugin.ConnectorBinding == nil {
		t.Fatalf("unexpected plugin descriptor: %+v", resp.Plugin)
	}
	if len(resp.Plugin.ConnectorBinding.AcceptedFamilies) != 1 || resp.Plugin.ConnectorBinding.AcceptedFamilies[0] != "postgres" {
		t.Fatalf("unexpected sink accepted families: %+v", resp.Plugin.ConnectorBinding)
	}
	if resp.Count != 1 || len(resp.Connectors) != 1 {
		t.Fatalf("expected exactly one compatible connector, got %+v", resp)
	}

	view := resp.Connectors[0]
	if view.ID != "pg-main" {
		t.Fatalf("expected postgres connector, got %+v", view)
	}
	if view.ConnectionSummary != "127.0.0.1:5432/appdb" {
		t.Fatalf("unexpected connection summary: %+v", view.ConnectionSummary)
	}
	if view.Compatibility == nil || !view.Compatibility.Compatible {
		t.Fatalf("expected compatible connector view, got %+v", view.Compatibility)
	}
	if view.PluginInfo == nil || view.PluginInfo.ConnectorDescriptor == nil || view.PluginInfo.ConnectorDescriptor.Family != "postgres" {
		t.Fatalf("unexpected connector plugin info: %+v", view.PluginInfo)
	}
}

func buildLocalPluginsDir(t *testing.T, buildMap map[string]string) string {
	t.Helper()

	pluginsDir := t.TempDir()
	repoRoot := filepath.Join("..", "..", "..", "..")
	pluginsRepo := filepath.Join(repoRoot, "..", "iot-rule-plugins")
	for packagePath, binaryName := range buildMap {
		category := filepath.Dir(packagePath)
		targetDir := filepath.Join(pluginsDir, filepath.Base(category))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", targetDir, err)
		}

		binaryPath := filepath.Join(targetDir, binaryName)
		buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
		buildCmd.Dir = filepath.Join(pluginsRepo, packagePath)
		if output, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("build plugin %s failed: %v\n%s", packagePath, err, output)
		}
	}

	return pluginsDir
}

func buildConnectorPluginsDir(t *testing.T) string {
	t.Helper()

	pluginsDir := t.TempDir()
	connectorsDir := filepath.Join(pluginsDir, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatalf("mkdir connectors dir: %v", err)
	}

	binaryPath := filepath.Join(connectorsDir, "connect-dummy-latest.so")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../../../core/connector/testdata/dummy_connector")
	buildCmd.Dir = "."
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy connector: %v\n%s", err, output)
	}

	return pluginsDir
}
