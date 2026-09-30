package dispatcher

import (
	"testing"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type stubPluginClient struct{}

func (stubPluginClient) Info() sdk.PluginInfo      { return sdk.PluginInfo{Type: sdk.PluginTypeSource} }
func (stubPluginClient) Start() error              { return nil }
func (stubPluginClient) Stop() error               { return nil }
func (stubPluginClient) SendEvent(sdk.Event) error { return nil }
func (stubPluginClient) SendEvents([]sdk.Event) error {
	return nil
}

func TestDispatcherStartupAuthorizationLifecycle(t *testing.T) {
	d := NewDispatcher("rule-test")

	if d.IsPluginRegistered("rule-test", "node-test") {
		t.Fatal("expected node to be unauthorized before startup authorization")
	}

	d.AuthorizePluginStartupRPC("rule-test", "node-test")
	if !d.IsPluginRegistered("rule-test", "node-test") {
		t.Fatal("expected startup-authorized node to pass registration check")
	}

	d.RegisterRule("rule-test", nil)
	d.RegisterPluginToRule("rule-test", "node-test", stubPluginClient{})
	if !d.IsPluginRegistered("rule-test", "node-test") {
		t.Fatal("expected registered node to remain authorized")
	}

	d.UnregisterRule("rule-test")
	if d.IsPluginRegistered("rule-test", "node-test") {
		t.Fatal("expected authorization to be removed after rule unregister")
	}
}
