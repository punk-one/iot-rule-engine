package main

import sdk "github.com/punk-one/iot-rule-plugin-sdk"

type bufferNode struct {
	sdk.BasePlugin
}

func (p *bufferNode) Info() sdk.PluginInfo {
	return sdk.PluginInfo{
		ID:      "buffer-node",
		Name:    "Buffer Node Test Fixture",
		Version: "0.0.1",
		Type:    sdk.PluginTypeProcessor,
	}
}

func main() {
	sdk.Serve(&bufferNode{}, sdk.ServeOptions{})
}
