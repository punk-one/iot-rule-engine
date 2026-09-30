package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestEnsurePluginNotReferencedRejectsEveryStoredRuleState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	manager := &Manager{db: db}
	for _, rule := range []*schema.RuleFlow{
		{
			ID: "active-rule", Name: "Active Rule", Enabled: true, LifecycleStatus: "active",
			Nodes: []*schema.NodeConfig{{ID: "sink", Plugin: "sink-mqtt"}},
		},
		{
			ID: "draft-rule", Name: "Draft Rule", Enabled: false, LifecycleStatus: "draft",
			Nodes: []*schema.NodeConfig{{ID: "source", Plugin: "sink-mqtt"}},
		},
		{
			ID: "connector-rule", Name: "Connector Rule", Enabled: false, LifecycleStatus: "draft",
			Nodes: []*schema.NodeConfig{{ID: "source", Plugin: "source-s7", Config: map[string]interface{}{"connector_ref": "cnc101"}}},
		},
	} {
		if err := db.SaveRule(rule); err != nil {
			t.Fatalf("SaveRule(%s) failed: %v", rule.ID, err)
		}
	}
	if err := db.SaveConnectorResource(&sdk.ConnectorResource{
		ID: "cnc101", Name: "CNC 101", PluginID: "sink-mqtt", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveConnectorResource failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if manager.ensurePluginNotReferenced(ctx, "sink-mqtt") {
		t.Fatal("expected referenced plugin to be rejected")
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Code      string                             `json:"code"`
		PluginID  string                             `json:"plugin_id"`
		Rules     []pluginRuleReference              `json:"rules"`
		Resources []pluginConnectorResourceReference `json:"connector_resources"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if response.Code != "PLUGIN_IN_USE" || response.PluginID != "sink-mqtt" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if len(response.Rules) != 3 {
		t.Fatalf("expected direct and connector references, got %+v", response.Rules)
	}
	if len(response.Resources) != 1 || response.Resources[0].ID != "cnc101" {
		t.Fatalf("expected connector resource reference, got %+v", response.Resources)
	}
}

func TestEnsurePluginNotReferencedAllowsUnusedPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := rulesdb.Open(rulesdb.Config{DataDir: t.TempDir(), StorageType: "sqlite"})
	if err != nil {
		t.Fatalf("Open rules db failed: %v", err)
	}
	defer db.Close()

	manager := &Manager{db: db}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if !manager.ensurePluginNotReferenced(ctx, "unused-plugin") {
		t.Fatalf("expected unused plugin to be allowed, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
