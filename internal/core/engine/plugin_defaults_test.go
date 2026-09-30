package engine

import (
	"path/filepath"
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
)

func TestApplyPluginRuntimeDefaultsForBufferNode(t *testing.T) {
	exec := &Executor{
		pluginDefaults: PluginRuntimeDefaults{
			BufferDBPath: filepath.Join("data", "buffer.db"),
			NATS: bus.NATSOptions{
				URL:       "nats://127.0.0.1:4222",
				AuthMode:  "creds",
				CredsFile: "/etc/nats/user.creds",
			},
		},
	}

	rule := &schema.RuleFlow{
		ID: "rule-1",
		Nodes: []*schema.NodeConfig{
			{
				ID:     "buffer-1",
				Type:   "processor",
				Plugin: "buffer-node",
				Config: map[string]interface{}{},
			},
		},
	}

	instances := exec.buildPluginInstances(rule)
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}

	storageCfg, ok := instances[0].Config["storage"].(map[string]interface{})
	if !ok {
		t.Fatalf("storage config missing: %#v", instances[0].Config)
	}
	if storageCfg["db_path"] != filepath.Join("data", "buffer.db") {
		t.Fatalf("unexpected db path: %v", storageCfg["db_path"])
	}

	ackBusCfg, ok := instances[0].Config["ack_bus"].(map[string]interface{})
	if !ok {
		t.Fatalf("ack_bus config missing: %#v", instances[0].Config)
	}
	if ackBusCfg["url"] != "nats://127.0.0.1:4222" {
		t.Fatalf("unexpected ack bus url: %v", ackBusCfg["url"])
	}
	if ackBusCfg["auth_mode"] != "creds" {
		t.Fatalf("unexpected ack bus auth mode: %v", ackBusCfg["auth_mode"])
	}
	if ackBusCfg["creds_file"] != "/etc/nats/user.creds" {
		t.Fatalf("unexpected ack bus creds_file: %v", ackBusCfg["creds_file"])
	}
}

func TestApplyPluginRuntimeDefaultsKeepsExplicitConfig(t *testing.T) {
	exec := &Executor{
		pluginDefaults: PluginRuntimeDefaults{
			BufferDBPath: filepath.Join("data", "buffer.db"),
			NATS: bus.NATSOptions{
				URL: "nats://127.0.0.1:4222",
			},
		},
	}

	inst := BuildPluginInstances(&schema.RuleFlow{
		ID: "rule-1",
		Nodes: []*schema.NodeConfig{
			{
				ID:     "buffer-1",
				Type:   "processor",
				Plugin: "buffer-node",
				Config: map[string]interface{}{
					"storage": map[string]interface{}{
						"db_path": "/tmp/custom-buffer.db",
					},
					"ack_bus": map[string]interface{}{
						"url": "nats://custom:4222",
					},
				},
			},
		},
	})[0]

	exec.applyPluginRuntimeDefaults(inst)

	storageCfg := inst.Config["storage"].(map[string]interface{})
	if storageCfg["db_path"] != "/tmp/custom-buffer.db" {
		t.Fatalf("explicit db path should win, got %v", storageCfg["db_path"])
	}

	ackBusCfg := inst.Config["ack_bus"].(map[string]interface{})
	if ackBusCfg["url"] != "nats://custom:4222" {
		t.Fatalf("explicit ack bus url should win, got %v", ackBusCfg["url"])
	}
}
