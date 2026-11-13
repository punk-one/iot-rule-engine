package main

import sdk "github.com/punk-one/iot-rule-plugin-sdk"

var fixtureVersion = "1.0.0"

type fixturePlugin struct {
	sdk.BasePlugin
}

func (*fixturePlugin) Info() sdk.PluginInfo {
	return sdk.PluginInfo{
		ID:      "factory-process-fixture",
		Name:    "Factory Process Fixture",
		Version: fixtureVersion,
		Type:    sdk.PluginTypeProcessor,
	}
}

func main() {
	sdk.ServeFactory(func() sdk.Plugin { return &fixturePlugin{} }, sdk.ServeOptions{})
}
