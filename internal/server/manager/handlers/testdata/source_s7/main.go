package main

import sdk "github.com/punk-one/iot-rule-plugin-sdk"

type sourceS7 struct {
	sdk.BasePlugin
}

func (p *sourceS7) Info() sdk.PluginInfo {
	return sdk.PluginInfo{
		ID:      "source-s7",
		Name:    "S7 Source Test Fixture",
		Version: "0.0.1",
		Type:    sdk.PluginTypeSource,
	}
}

func main() {
	sdk.Serve(&sourceS7{}, sdk.ServeOptions{})
}
