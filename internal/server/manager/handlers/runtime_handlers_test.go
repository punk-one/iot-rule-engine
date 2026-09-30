package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
)

func TestGetNodeInstanceHandlerFindsCanvasNodeID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runtimeMgr := runtime.NewManager()
	instance, err := runtimeMgr.CreateRuleInstance("rule-1", "1")
	if err != nil {
		t.Fatalf("CreateRuleInstance failed: %v", err)
	}

	node := &runtime.NodeInstance{
		ID:       instance.ID + "-sink_mqtt",
		NodeID:   "sink_mqtt",
		PluginID: "sink-mqtt",
		Config: map[string]interface{}{
			"topic": "devices/telemetry",
			"retry": map[string]interface{}{"max": float64(3)},
		},
		Status: runtime.NodeStatusRunning,
	}
	if err := runtimeMgr.AddNodeInstance(instance.ID, node); err != nil {
		t.Fatalf("AddNodeInstance failed: %v", err)
	}

	handler := NewRuntimeHandlers(runtimeMgr, nil, "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/instances/"+instance.ID+"/nodes/sink_mqtt", nil)
	ctx.Request = req
	ctx.Params = gin.Params{
		{Key: "instance_id", Value: instance.ID},
		{Key: "node_id", Value: "sink_mqtt"},
	}

	handler.GetNodeInstanceHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var got runtime.NodeInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if got.NodeID != "sink_mqtt" {
		t.Fatalf("expected node_id sink_mqtt, got %s", got.NodeID)
	}
	if got.ID != node.ID {
		t.Fatalf("expected runtime node id %s, got %s", node.ID, got.ID)
	}
	if got.Config["topic"] != "devices/telemetry" {
		t.Fatalf("expected config topic devices/telemetry, got %#v", got.Config["topic"])
	}
}

func TestListRuleInstancesHandlerSortsByRuleName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	for _, rule := range []*schema.RuleFlow{
		{ID: "rule-b", Name: "Beta Rule", Version: 1},
		{ID: "rule-a", Name: "Alpha Rule", Version: 1},
	} {
		if err := db.SaveRule(rule); err != nil {
			t.Fatalf("SaveRule(%s) failed: %v", rule.ID, err)
		}
	}

	runtimeMgr := runtime.NewManager()
	for _, ruleID := range []string{"rule-b", "rule-a"} {
		instance, err := runtimeMgr.CreateRuleInstance(ruleID, "1")
		if err != nil {
			t.Fatalf("CreateRuleInstance(%s) failed: %v", ruleID, err)
		}
		instance.Status = runtime.RuleStatusRunning
	}

	handler := NewRuntimeHandlers(runtimeMgr, &Manager{db: db}, "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/instances", nil)

	handler.ListRuleInstancesHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Instances []struct {
			RuleID string `json:"rule_id"`
			Name   string `json:"name"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(resp.Instances))
	}
	if resp.Instances[0].Name != "Alpha Rule" || resp.Instances[1].Name != "Beta Rule" {
		t.Fatalf("expected instances sorted by name, got %+v", resp.Instances)
	}
}

func TestResolvePluginPathSupportsHyphenUnderscoreVariants(t *testing.T) {
	pluginsDir := t.TempDir()
	processorDir := filepath.Join(pluginsDir, "processors")
	if err := os.MkdirAll(processorDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	expectedPath := filepath.Join(processorDir, "buffer_node-latest.so")
	if err := os.WriteFile(expectedPath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	manager := &Manager{
		pluginLoader: plugin.NewLoader(pluginsDir),
		pluginsDir:   pluginsDir,
	}

	resolvedPath, err := manager.resolvePluginPath("buffer-node")
	if err != nil {
		t.Fatalf("resolvePluginPath failed: %v", err)
	}

	if resolvedPath != expectedPath {
		t.Fatalf("expected path %s, got %s", expectedPath, resolvedPath)
	}
}

func TestGetTraceEventsHandlerReturnsDebugFlagWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runtimeMgr := runtime.NewManager()
	instance, err := runtimeMgr.CreateRuleInstance("rule-1", "1")
	if err != nil {
		t.Fatalf("CreateRuleInstance failed: %v", err)
	}
	instance.DebugMode = false

	handler := NewRuntimeHandlers(runtimeMgr, nil, "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/instances/"+instance.ID+"/trace", nil)
	ctx.Request = req
	ctx.Params = gin.Params{
		{Key: "instance_id", Value: instance.ID},
	}

	handler.GetTraceEventsHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		DebugEnabled bool                 `json:"debug_enabled"`
		Events       []runtime.TraceEvent `json:"events"`
		Message      string               `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.DebugEnabled {
		t.Fatal("expected debug_enabled=false in trace response")
	}
	if len(resp.Events) != 0 {
		t.Fatalf("expected no trace events when debug mode is disabled, got %d", len(resp.Events))
	}
	if resp.Message == "" {
		t.Fatal("expected explanatory message when debug mode is disabled")
	}
}
