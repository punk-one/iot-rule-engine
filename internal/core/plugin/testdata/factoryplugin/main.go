package main

import sdk "github.com/punk-one/iot-rule-plugin-sdk"

type fixturePlugin struct {
	sdk.BasePlugin
}

func (*fixturePlugin) Info() sdk.PluginInfo {
	return sdk.PluginInfo{
		ID:      "factory-process-fixture",
		Name:    "Factory Process Fixture",
		Version: "1.0.0",
		Type:    sdk.PluginTypeProcessor,
	}
}

func main() {
	sdk.ServeFactory(func() sdk.Plugin { return &fixturePlugin{} }, sdk.ServeOptions{})
}
