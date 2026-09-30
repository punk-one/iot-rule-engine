package handlers

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestValidateRuleDeploymentReturnsStructuredErrors(t *testing.T) {
	pluginsDir := repoPluginsDir(t)
	manager := NewManager(nil, pluginsDir, "", nil)

	raw := `{
		"id": "acm006-s7-mqtt-debug-20260324143300",
		"nodes": [
			{
				"id": "src_s7",
				"type": "source",
				"name": "source-s7 acm006",
				"plugin": "source-s7",
				"config": {
					"connection": {
						"connect_type": "pg",
						"host": "192.0.2.87",
						"idle_timeout_sec": 30,
						"port": 10102,
						"rack": 0,
						"slot": 1,
						"timeout_sec": 5
					},
					"output": {
						"device_code": "acm006",
						"include_meta": true
					},
					"points": [
						{
							"address": "MD0",
							"name": "total_out",
							"value_type": "Int32"
						}
					],
					"polling": {
						"batch_size": 16,
						"interval_ms": 1000
					}
				},
				"wires": [
					"processor_1774415947674"
				]
			},
			{
				"id": "sink_mqtt",
				"type": "sink",
				"name": "sink-mqtt telemetry",
				"plugin": "sink-mqtt",
				"config": {
					"options": {
						"connect_timeout_sec": 10,
						"initial_retry_interval_ms": 1000,
						"max_reconnect_interval_sec": 60,
						"qos": 0,
						"retain": false,
						"write_timeout_sec": 10
					},
					"target": {
						"broker": "mqtt://198.51.100.10:1883",
						"client_id": "aa5777",
						"password": "test-only-password",
						"topic": "v1/acm/telemetry",
						"username": "lh"
					}
				},
				"wires": []
			},
			{
				"id": "processor_1774415947674",
				"type": "processor",
				"name": "processor_1774415947674",
				"plugin": "detect-change",
				"config": {
					"output": {
						"emit": "on_change"
					},
					"process": {
						"detect": {
							"direction": "any",
							"field": "payload.data.total_out",
							"type": "delta",
							"value": 0
						}
					},
					"state": {
						"evict": "fifo",
						"key_by": [
							"payload.device_code"
						],
						"max_keys": 2,
						"ttl": "1m"
					}
				},
				"wires": [
					"sink_mqtt"
				]
			}
		]
	}`

	var deployRule schema.RuleFlow
	if err := json.Unmarshal([]byte(raw), &deployRule); err != nil {
		t.Fatalf("failed to decode rule flow: %v", err)
	}

	errors := manager.validateRuleDeployment(&deployRule)
	if len(errors) != 1 {
		t.Fatalf("expected 1 validation error, got %d: %#v", len(errors), errors)
	}
	if errors[0].NodeID != "processor_1774415947674" {
		t.Fatalf("expected node_id processor_1774415947674, got %q", errors[0].NodeID)
	}
	if errors[0].Field != "process.detect.value" {
		t.Fatalf("expected field process.detect.value, got %q", errors[0].Field)
	}
	if !strings.Contains(errors[0].Message, "greater than 0") {
		t.Fatalf("expected message to mention greater than 0, got %q", errors[0].Message)
	}
}

func TestValidationErrorsFromRuntimeErrorParsesExecutorEnvelope(t *testing.T) {
	err := validationErrorsFromRuntimeError(
		assertErr(`executor sync failed with status 400: {"error":"failed to build DAG: failed to init plugin for node processor_1774415947674: process.detect.value must be greater than 0"}`),
		"EXECUTOR_SYNC_FAILED",
	)
	if len(err) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(err))
	}
	if err[0].NodeID != "processor_1774415947674" {
		t.Fatalf("expected node_id processor_1774415947674, got %q", err[0].NodeID)
	}
	if err[0].Field != "process.detect.value" {
		t.Fatalf("expected field process.detect.value, got %q", err[0].Field)
	}
}

func TestGatewayDeviceStatusRuleFixturePassesDeploymentValidation(t *testing.T) {
	pluginsDir := repoPluginsDir(t)
	manager := NewManager(nil, pluginsDir, "", nil)

	rulePath := handlerTestdataPath(t, "gateway-device-status-rule.json")
	raw, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read rule file: %v", err)
	}

	var deployRule schema.RuleFlow
	if err := json.Unmarshal(raw, &deployRule); err != nil {
		t.Fatalf("failed to decode rule flow: %v", err)
	}

	errors := manager.validateRuleDeployment(&deployRule)
	if len(errors) != 0 {
		t.Fatalf("expected no deployment validation errors, got %#v", errors)
	}
}

func TestGatewayDeviceStatusRuleFixtureRoutesTimescaleWritesThroughTransformMap(t *testing.T) {
	rule := loadRuleFlowFromTestdata(t, "gateway-device-status-rule.json")

	nodes := make(map[string]*schema.NodeConfig, len(rule.Nodes))
	for _, node := range rule.Nodes {
		nodes[node.ID] = node
	}

	transformNode, ok := nodes["map_state_event_row"]
	if !ok {
		t.Fatal("expected map_state_event_row node in rule doc")
	}
	if transformNode.Plugin != "transform-map" {
		t.Fatalf("expected map_state_event_row plugin transform-map, got %q", transformNode.Plugin)
	}

	processNode, ok := nodes["processor_1776325821577"]
	if !ok {
		t.Fatal("expected process-status-event node in rule doc")
	}
	if !containsWire(processNode.Wires, "map_state_event_row") {
		t.Fatalf("expected process-status-event wires to include map_state_event_row, got %#v", processNode.Wires)
	}
	if containsWire(processNode.Wires, "sink_state_events_ts") {
		t.Fatalf("expected process-status-event to route timescaledb writes through transform-map, got %#v", processNode.Wires)
	}

	if !containsWire(transformNode.Wires, "sink_state_events_ts") {
		t.Fatalf("expected transform-map wires to include sink_state_events_ts, got %#v", transformNode.Wires)
	}

	processCfg, ok := transformNode.Config["process"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected transform-map process config, got %#v", transformNode.Config["process"])
	}
	templateCfg, ok := processCfg["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected transform-map template config, got %#v", processCfg["template"])
	}

	assertTemplateField(t, templateCfg, "device_code", "{{ .payload.device_code }}")
	assertTemplateField(t, templateCfg, "event_type", "status_event")
	assertTemplateField(t, templateCfg, "payload", "{{ json .payload.data }}")
	assertTemplateField(t, templateCfg, "source_system", "gateway")
	assertTemplateField(t, templateCfg, "transport_protocol", "mqtt")
	assertTemplateField(t, templateCfg, "time", "{{ rfc3339 .event.timestamp }}")
}

func loadRuleFlowFromTestdata(t *testing.T, filename string) *schema.RuleFlow {
	t.Helper()
	rulePath := handlerTestdataPath(t, filename)
	raw, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read rule file: %v", err)
	}

	var deployRule schema.RuleFlow
	if err := json.Unmarshal(raw, &deployRule); err != nil {
		t.Fatalf("failed to decode rule flow: %v", err)
	}
	return &deployRule
}

func containsWire(wires []string, want string) bool {
	for _, wire := range wires {
		if wire == want {
			return true
		}
	}
	return false
}

func assertTemplateField(t *testing.T, template map[string]interface{}, key string, want interface{}) {
	t.Helper()
	got, ok := template[key]
	if !ok {
		t.Fatalf("expected transform-map template to include %q, got %#v", key, template)
	}
	if got != want {
		t.Fatalf("expected transform-map template[%q] = %#v, got %#v", key, want, got)
	}
}

func repoPluginsDir(t *testing.T) string {
	t.Helper()
	pluginsDir := buildLocalPluginsDir(t, map[string]string{
		"processors/detect_change":        "detect-change-latest.so",
		"processors/process_status_event": "process-status-event-latest.so",
		"processors/transform_map":        "transform-map-latest.so",
		"sinks/sink_redis":                "sink-redis-latest.so",
		"sinks/timescaledb_sink":          "sink-timeseries-timescaledb-latest.so",
		"sources/source_mqtt":             "source-mqtt-latest.so",
	})

	sourcesDir := filepath.Join(pluginsDir, "sources")
	if err := os.MkdirAll(sourcesDir, 0o755); err != nil {
		t.Fatalf("create source fixture directory: %v", err)
	}
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(sourcesDir, "source-s7-latest.so"), "./testdata/source_s7")
	buildCmd.Dir = "."
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build source-s7 fixture: %v\n%s", err, output)
	}

	sinksDir := filepath.Join(pluginsDir, "sinks")
	if err := os.MkdirAll(sinksDir, 0o755); err != nil {
		t.Fatalf("create sink fixture directory: %v", err)
	}
	buildCmd = exec.Command("go", "build", "-o", filepath.Join(sinksDir, "sink-mqtt-latest.so"), "./testdata/sink_mqtt")
	buildCmd.Dir = "."
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build sink-mqtt fixture: %v\n%s", err, output)
	}

	return pluginsDir
}

func handlerTestdataPath(t *testing.T, filename string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	return filepath.Join(wd, "testdata", filename)
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
