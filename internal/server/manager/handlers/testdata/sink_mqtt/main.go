package main

import sdk "github.com/punk-one/iot-rule-plugin-sdk"

type sinkMQTT struct {
	sdk.BasePlugin
}

func (p *sinkMQTT) Info() sdk.PluginInfo {
	return sdk.PluginInfo{
		ID:      "sink-mqtt",
		Name:    "MQTT Sink Test Fixture",
		Version: "0.0.1",
		Type:    sdk.PluginTypeSink,
	}
}

func main() {
	sdk.Serve(&sinkMQTT{}, sdk.ServeOptions{})
}
