package handlers

import (
	"testing"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestInferConnectorDescriptorForRedis(t *testing.T) {
	view := describePlugin(sdk.PluginInfo{
		ID:   "connect-redis",
		Type: sdk.PluginTypeConnector,
	})

	if view.ConnectorDescriptor == nil {
		t.Fatal("expected redis connector descriptor")
	}
	if view.ConnectorDescriptor.Family != "redis" {
		t.Fatalf("expected family redis, got %q", view.ConnectorDescriptor.Family)
	}
	if len(view.ConnectorDescriptor.DisplayFields) != 2 || view.ConnectorDescriptor.DisplayFields[0] != "addr" {
		t.Fatalf("unexpected display fields: %#v", view.ConnectorDescriptor.DisplayFields)
	}
}

func TestInferIndustrialConnectorSessionPolicies(t *testing.T) {
	for _, pluginID := range []string{"connect-s7", "connect-fanuc"} {
		view := describePlugin(sdk.PluginInfo{ID: pluginID, Type: sdk.PluginTypeConnector})
		if view.ConnectorDescriptor == nil || !containsString(view.ConnectorDescriptor.Capabilities, "session:per_operation") {
			t.Fatalf("%s must advertise per-operation sessions: %#v", pluginID, view.ConnectorDescriptor)
		}
	}
}

func TestInferConnectorBindingForRedisPlugins(t *testing.T) {
	sourceView := describePlugin(sdk.PluginInfo{
		ID:   "source-redis",
		Type: sdk.PluginTypeSource,
	})
	if sourceView.ConnectorBinding == nil {
		t.Fatal("expected redis source binding")
	}
	if len(sourceView.ConnectorBinding.AcceptedFamilies) != 1 || sourceView.ConnectorBinding.AcceptedFamilies[0] != "redis" {
		t.Fatalf("unexpected source binding: %#v", sourceView.ConnectorBinding)
	}
	if len(sourceView.ConnectorBinding.RequiredCapabilities) != 1 || sourceView.ConnectorBinding.RequiredCapabilities[0] != "read" {
		t.Fatalf("unexpected source capabilities: %#v", sourceView.ConnectorBinding.RequiredCapabilities)
	}

	sinkView := describePlugin(sdk.PluginInfo{
		ID:   "sink-redis",
		Type: sdk.PluginTypeSink,
	})
	if sinkView.ConnectorBinding == nil {
		t.Fatal("expected redis sink binding")
	}
	if len(sinkView.ConnectorBinding.AcceptedFamilies) != 1 || sinkView.ConnectorBinding.AcceptedFamilies[0] != "redis" {
		t.Fatalf("unexpected sink binding: %#v", sinkView.ConnectorBinding)
	}
	if len(sinkView.ConnectorBinding.RequiredCapabilities) != 1 || sinkView.ConnectorBinding.RequiredCapabilities[0] != "write" {
		t.Fatalf("unexpected sink capabilities: %#v", sinkView.ConnectorBinding.RequiredCapabilities)
	}
}

func TestInferConnectorDescriptorForPostgres(t *testing.T) {
	view := describePlugin(sdk.PluginInfo{
		ID:   "connect-postgres",
		Type: sdk.PluginTypeConnector,
	})

	if view.ConnectorDescriptor == nil {
		t.Fatal("expected postgres connector descriptor")
	}
	if view.ConnectorDescriptor.Family != "postgres" {
		t.Fatalf("expected family postgres, got %q", view.ConnectorDescriptor.Family)
	}
	if view.ConnectorDescriptor.Label != "PostgreSQL" {
		t.Fatalf("expected label PostgreSQL, got %q", view.ConnectorDescriptor.Label)
	}
}

func TestParseConnectionPreviewForPostgresDSN(t *testing.T) {
	descriptor := &ConnectorDescriptor{Family: "postgres"}
	resource := sdk.ConnectorResource{
		Connection: []byte(`{"dsn":"postgres://user:pass@127.0.0.1:5432/appdb?sslmode=disable"}`),
	}

	preview := parseConnectionPreview(resource, descriptor)
	if preview["host"] != "127.0.0.1" || preview["port"] != "5432" || preview["database"] != "appdb" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}
