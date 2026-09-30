package connector

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	coreplugin "github.com/punk-one/iot-rule-engine/internal/core/plugin"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestHostResolvesRealConnectorPlugin(t *testing.T) {
	tmpDir := t.TempDir()
	connectorsDir := filepath.Join(tmpDir, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatalf("mkdir connectors dir: %v", err)
	}

	binaryPath := filepath.Join(connectorsDir, "connect-dummy-latest.so")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./testdata/dummy_connector")
	buildCmd.Dir = "."
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build dummy connector: %v\n%s", err, output)
	}

	loader := coreplugin.NewLoader(tmpDir)
	host := NewHost(loader)
	defer host.Close()

	resourceBus := NewResourceBus(ResourceBusOptions{})
	resourceBus.SetResourceProviderResolver(host.ResolveForResource)
	if err := resourceBus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-it",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := resourceBus.Bind(context.Background(), "resource-it")
	if err != nil {
		t.Fatalf("bind resource: %v", err)
	}
	defer func() {
		_ = resourceBus.Release(context.Background(), token)
	}()

	resp, err := resourceBus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "hello"})
	if err != nil {
		t.Fatalf("resource read: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	if payload["provider_handle"] != "provider-resource-it" {
		t.Fatalf("unexpected provider handle: %+v", payload)
	}
	if payload["kind"] != string(sdk.ConnectorRequestRead) {
		t.Fatalf("unexpected request kind: %+v", payload)
	}
	if payload["target"] != "hello" {
		t.Fatalf("unexpected request target: %+v", payload)
	}

	status, ok := resourceBus.CurrentStatus("resource-it")
	if !ok {
		t.Fatal("expected current status")
	}
	if status.ProviderInstanceID == "" {
		t.Fatalf("expected provider instance id in status, got %+v", status)
	}
}

func TestHostKeepsOneProviderProcessAcrossResources(t *testing.T) {
	tmpDir := t.TempDir()
	connectorsDir := filepath.Join(tmpDir, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatalf("mkdir connectors dir: %v", err)
	}

	binaryPath := filepath.Join(connectorsDir, "connect-dummy-latest.so")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./testdata/dummy_connector")
	buildCmd.Dir = "."
	output, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build dummy connector: %v\n%s", err, output)
	}

	loader := coreplugin.NewLoader(tmpDir)
	host := NewHost(loader)
	defer host.Close()

	resourceBus := NewResourceBus(ResourceBusOptions{})
	resourceBus.SetResourceProviderResolver(host.ResolveForResource)

	resources := []sdk.ConnectorResource{
		{
			ID:       "resource-a",
			PluginID: "connect-dummy",
			Enabled:  true,
			ProviderPolicy: sdk.ProviderPolicy{
				MaxResourcesPerProvider: 1,
			},
		},
		{
			ID:       "resource-b",
			PluginID: "connect-dummy",
			Enabled:  true,
			ProviderPolicy: sdk.ProviderPolicy{
				MaxResourcesPerProvider: 1,
			},
		},
	}
	for _, resource := range resources {
		if err := resourceBus.RegisterResource(resource); err != nil {
			t.Fatalf("register resource %s: %v", resource.ID, err)
		}
	}

	tokenA, err := resourceBus.Bind(context.Background(), "resource-a")
	if err != nil {
		t.Fatalf("bind resource-a: %v", err)
	}
	defer func() { _ = resourceBus.Release(context.Background(), tokenA) }()

	tokenB, err := resourceBus.Bind(context.Background(), "resource-b")
	if err != nil {
		t.Fatalf("bind resource-b: %v", err)
	}
	defer func() { _ = resourceBus.Release(context.Background(), tokenB) }()

	statusA, ok := resourceBus.CurrentStatus("resource-a")
	if !ok {
		t.Fatal("expected status for resource-a")
	}
	statusB, ok := resourceBus.CurrentStatus("resource-b")
	if !ok {
		t.Fatal("expected status for resource-b")
	}
	if statusA.ProviderInstanceID == "" || statusB.ProviderInstanceID == "" {
		t.Fatalf("expected provider instance ids, got A=%+v B=%+v", statusA, statusB)
	}
	if statusA.ProviderInstanceID != statusB.ProviderInstanceID {
		t.Fatalf("expected one provider instance, got A=%+v B=%+v", statusA, statusB)
	}
	if len(host.providers["connect-dummy"]) != 1 {
		t.Fatalf("provider processes=%d, want 1", len(host.providers["connect-dummy"]))
	}
}
