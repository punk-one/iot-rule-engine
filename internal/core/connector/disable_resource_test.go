package connector

import (
	"context"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"testing"
)

func TestDisablingUnboundPreloadedResourceReleasesProviderAndKeepsConfig(t *testing.T) {
	bus := NewResourceBus(ResourceBusOptions{})
	provider := &dummyProvider{}
	bus.RegisterProvider("connect-dummy", provider)
	resource := sdk.ConnectorResource{ID: "kept-config", PluginID: "connect-dummy", Enabled: true, WarmupPolicy: sdk.WarmupPolicyPreload}
	if err := bus.RegisterResource(resource); err != nil {
		t.Fatal(err)
	}
	if err := bus.Warmup(context.Background(), resource.ID); err != nil {
		t.Fatal(err)
	}
	resource.Enabled = false
	if err := bus.RegisterResource(resource); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	destroyed := provider.destroyCalls
	provider.mu.Unlock()
	if destroyed != 1 {
		t.Fatalf("idle provider resource still retained: %d", destroyed)
	}
	bus.mu.RLock()
	saved, exists := bus.resources[resource.ID]
	bus.mu.RUnlock()
	if !exists || saved.Enabled {
		t.Fatal("configuration was lost or reenabled")
	}
	resource.Enabled = true
	if err := bus.RegisterResource(resource); err != nil {
		t.Fatal(err)
	}
	if err := bus.Warmup(context.Background(), resource.ID); err != nil {
		t.Fatal(err)
	}
	if err := bus.UnregisterResource(context.Background(), resource.ID); err != nil {
		t.Fatal(err)
	}
}
