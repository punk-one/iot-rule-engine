package engine

import (
	"encoding/json"
	"path/filepath"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin/instance"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
)

type PluginRuntimeDefaults struct {
	BufferDBPath string
	NATS         bus.NATSOptions
}

func (e *Executor) SetPluginRuntimeDefaults(defaults PluginRuntimeDefaults) {
	e.pluginDefaults = defaults
}

func (e *Executor) buildPluginInstances(rule *schema.RuleFlow) []*instance.PluginInstance {
	instances := BuildPluginInstances(rule)
	for _, inst := range instances {
		e.applyPluginRuntimeDefaults(inst)
	}
	return instances
}

func (e *Executor) applyPluginRuntimeDefaults(inst *instance.PluginInstance) {
	if inst == nil {
		return
	}

	switch inst.Plugin {
	case "buffer-node":
		if inst.Config == nil {
			inst.Config = make(map[string]any)
		}

		storageCfg := ensureNestedMap(inst.Config, "storage")
		if storageCfg["db_path"] == nil || storageCfg["db_path"] == "" {
			dbPath := e.pluginDefaults.BufferDBPath
			if dbPath == "" {
				dbPath = filepath.Join("data", "buffer.db")
			}
			storageCfg["db_path"] = dbPath
		}

		ackBusCfg := ensureNestedMap(inst.Config, "ack_bus")
		if ackBusCfg["url"] == nil || ackBusCfg["url"] == "" {
			ackBusCfg["url"] = e.pluginDefaults.NATS.URL
			ackBusCfg["auth_mode"] = e.pluginDefaults.NATS.AuthMode
			ackBusCfg["max_reconnects"] = e.pluginDefaults.NATS.MaxReconnects
			ackBusCfg["reconnect_wait_ms"] = e.pluginDefaults.NATS.ReconnectWait.Milliseconds()
			ackBusCfg["username"] = e.pluginDefaults.NATS.Username
			ackBusCfg["password"] = e.pluginDefaults.NATS.Password
			ackBusCfg["token"] = e.pluginDefaults.NATS.Token
			ackBusCfg["cert_file"] = e.pluginDefaults.NATS.CertFile
			ackBusCfg["key_file"] = e.pluginDefaults.NATS.KeyFile
			ackBusCfg["ca_file"] = e.pluginDefaults.NATS.CAFile
			ackBusCfg["creds_file"] = e.pluginDefaults.NATS.CredsFile
		}
	}
}

func ensureNestedMap(root map[string]any, key string) map[string]any {
	if root == nil {
		root = make(map[string]any)
	}
	if existing, ok := root[key].(map[string]any); ok {
		return existing
	}

	nested := make(map[string]any)
	root[key] = nested
	return nested
}

func cloneConfigMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	data, err := json.Marshal(input)
	if err != nil {
		out := make(map[string]any, len(input))
		for k, v := range input {
			out[k] = v
		}
		return out
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		out = make(map[string]any, len(input))
		for k, v := range input {
			out[k] = v
		}
	}
	return out
}
