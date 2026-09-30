package plugin

import (
	goplugin "github.com/hashicorp/go-plugin"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func handshakeConfig() goplugin.HandshakeConfig {
	return sdk.DefaultHandshakeConfig()
}
