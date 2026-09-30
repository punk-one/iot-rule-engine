package plugin

import (
	"testing"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestEngineHandshakeIgnoresLegacyMutableSDKVariable(t *testing.T) {
	original := sdk.HandshakeConfig
	t.Cleanup(func() { sdk.HandshakeConfig = original })
	sdk.HandshakeConfig.ProtocolVersion = 99
	sdk.HandshakeConfig.MagicCookieValue = "mutated"

	got := handshakeConfig()
	if got.ProtocolVersion != 1 || got.MagicCookieKey != "PUNK_PLUGIN_MAGIC" || got.MagicCookieValue != "punk-rule-engine" {
		t.Fatalf("engine did not use immutable v1 handshake: %#v", got)
	}
}
