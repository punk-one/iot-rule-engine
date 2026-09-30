package instance

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type ContextFactory func(ruleID, nodeID string) (sdk.RuntimeContext, error)

type GoPluginRunner struct {
	loader     *plugin.Loader
	ctxFactory ContextFactory
}

func NewGoPluginRunner(loader *plugin.Loader, ctxFactory ContextFactory) *GoPluginRunner {
	return &GoPluginRunner{loader: loader, ctxFactory: ctxFactory}
}

func (r *GoPluginRunner) Start(inst *PluginInstance) error {
	path := inst.Plugin
	if _, err := os.Stat(path); os.IsNotExist(err) {
		resolvedPath, err := r.loader.ResolvePluginPath(inst.Plugin)
		if err != nil {
			return fmt.Errorf("failed to resolve plugin path for %s: %w", inst.Plugin, err)
		}
		path = resolvedPath
	}

	client, err := r.loader.AcquirePluginInstance(path)
	if err != nil {
		return fmt.Errorf("failed to acquire plugin %s (path: %s): %w", inst.Plugin, path, err)
	}
	inst.Internal = client
	release := func() {
		_ = r.loader.ReleasePluginInstance(client)
		inst.Internal = nil
	}

	if r.ctxFactory == nil {
		release()
		return fmt.Errorf("ContextFactory not configured")
	}
	ctx, err := r.ctxFactory(inst.RuleID, inst.NodeID)
	if err != nil {
		release()
		return fmt.Errorf("failed to create runtime context: %w", err)
	}

	_ = ctx.Health().Report(sdk.HealthReport{
		Status:  sdk.HealthInitializing,
		Code:    "SYS_INITIALIZING",
		Message: "plugin initializing",
		Details: map[string]string{"plugin": inst.Plugin},
	})

	configBytes, err := json.Marshal(inst.Config)
	if err != nil {
		release()
		_ = ctx.Health().Unhealthy("SYS_INIT_FAILED", err.Error(), nil)
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	pluginConfig := sdk.PluginConfig{
		Raw:    json.RawMessage(configBytes),
		RuleID: inst.RuleID,
		NodeID: inst.NodeID,
		Health: sdk.DefaultHealthOptions(),
	}

	if err := client.RPCClient.Init(pluginConfig); err != nil {
		release()
		_ = ctx.Health().Unhealthy("SYS_INIT_FAILED", err.Error(), nil)
		return fmt.Errorf("failed to init plugin: %w", err)
	}

	if err := client.RPCClient.Start(ctx); err != nil {
		release()
		_ = ctx.Health().Unhealthy("SYS_START_FAILED", err.Error(), nil)
		return fmt.Errorf("failed to start plugin: %w", err)
	}

	return nil
}

func (r *GoPluginRunner) Stop(inst *PluginInstance) error {
	client, ok := inst.Internal.(*plugin.PluginClient)
	if !ok || client == nil {
		return nil
	}
	inst.Internal = nil
	return r.loader.ReleasePluginInstance(client)
}
