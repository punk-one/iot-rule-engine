package connector

import (
	"context"
	"encoding/json"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"testing"
)

func TestConnectionUpdateRetiresIdleProviderAndRejectsBoundChanges(t *testing.T) {
	b := NewResourceBus(ResourceBusOptions{})
	p := &dummyProvider{}
	b.RegisterProvider("dummy", p)
	r := sdk.ConnectorResource{ID: "update", PluginID: "dummy", Enabled: true, Connection: json.RawMessage(`{"host":"old.invalid"}`)}
	if err := b.RegisterResource(r); err != nil {
		t.Fatal(err)
	}
	if err := b.Warmup(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	r.Connection = json.RawMessage(`{"host":"new.invalid"}`)
	if err := b.RegisterResource(r); err != nil {
		t.Fatal(err)
	}
	if p.destroyCalls != 1 {
		t.Fatal("old provider was not retired")
	}
	token, err := b.Bind(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release(context.Background(), token)
	if p.createCalls != 2 {
		t.Fatal("new provider was not created")
	}
	r.Connection = json.RawMessage(`{"host":"rejected.invalid"}`)
	if err = b.RegisterResource(r); err == nil {
		t.Fatal("bound connection update accepted")
	}
	if string(b.resources[r.ID].Connection) != `{"host":"new.invalid"}` {
		t.Fatal("failed update changed stored configuration")
	}
}
