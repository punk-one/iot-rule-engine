package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	connectorruntime "github.com/punk-one/iot-rule-engine/internal/core/connector"
)

type testConnectorProvider struct{}

func (testConnectorProvider) CreateResource(ctx context.Context, resource sdk.ConnectorResource) (string, error) {
	return "provider-" + resource.ID, nil
}

func (testConnectorProvider) DestroyResource(ctx context.Context, providerHandle string) error {
	return nil
}

func (testConnectorProvider) Execute(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	payload, _ := json.Marshal(map[string]string{
		"provider_handle": providerHandle,
		"kind":            string(req.Kind),
		"target":          req.Target,
	})
	return sdk.ConnectorResponse{Payload: payload}, nil
}

func (testConnectorProvider) OpenStream(ctx context.Context, providerHandle string, req sdk.StreamOpenRequest) (sdk.StreamOpenResponse, error) {
	return sdk.StreamOpenResponse{StreamID: "stream-1", DeliverySemantics: sdk.StreamDeliveryAtLeastOnce, InitialCredits: req.InitialCredits}, nil
}

func (testConnectorProvider) ReceiveStream(ctx context.Context, providerHandle string, req sdk.StreamReceiveRequest) (sdk.StreamReceiveResponse, error) {
	return sdk.StreamReceiveResponse{}, nil
}

func (testConnectorProvider) AckStream(ctx context.Context, providerHandle string, req sdk.StreamAckRequest) error {
	return nil
}

func (testConnectorProvider) NackStream(ctx context.Context, providerHandle string, req sdk.StreamNackRequest) error {
	return nil
}

func (testConnectorProvider) GrantCredits(ctx context.Context, providerHandle string, req sdk.StreamGrantCreditsRequest) error {
	return nil
}

func (testConnectorProvider) CloseStream(ctx context.Context, providerHandle string, req sdk.StreamCloseRequest) error {
	return nil
}

func (testConnectorProvider) Probe(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ResourceStatusEvent, error) {
	return sdk.ResourceStatusEvent{
		ResourceID:         "dummy-resource",
		ProviderPluginID:   "connect-dummy",
		ProviderInstanceID: "connect-dummy#1",
		Status:             sdk.ResourceStatusHealthy,
		ReasonCode:         "READY",
		Message:            "healthy",
		ObservedAtUnixNano: time.Now().UnixNano(),
	}, nil
}

func (testConnectorProvider) ProviderPluginID() string   { return "connect-dummy" }
func (testConnectorProvider) ProviderInstanceID() string { return "connect-dummy#1" }

func TestCreateRuntimeContextProvidesBoundConnectorClient(t *testing.T) {
	resourceBus := connectorruntime.NewResourceBus(connectorruntime.ResourceBusOptions{})
	resourceBus.RegisterProvider("connect-dummy", testConnectorProvider{})
	if err := resourceBus.RegisterResource(sdk.ConnectorResource{
		ID:       "dummy-resource",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	executor := &DAGNATSExecutor{
		dag: &DAG{
			RuleID: "rule-test",
			RuleFlow: &schema.RuleFlow{
				ID: "rule-test",
				Nodes: []*schema.NodeConfig{
					{
						ID: "source-node",
						Config: map[string]interface{}{
							"connector_ref": "dummy-resource",
						},
					},
				},
			},
		},
		resourceBus:      resourceBus,
		resourceBindings: make(map[string]string),
		pluginRPCs:       make(map[string]*sdk.PluginRPCClient),
	}

	ctx := executor.CreateRuntimeContext("source-node")
	resp, err := ctx.Connector().Read(sdk.ConnectorRequest{Target: "DB1.DBX0.0"})
	if err != nil {
		t.Fatalf("connector read: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["provider_handle"] != "provider-dummy-resource" {
		t.Fatalf("unexpected provider_handle: %v", payload)
	}
	if payload["kind"] != string(sdk.ConnectorRequestRead) {
		t.Fatalf("unexpected kind: %v", payload)
	}

	status, ok := ctx.Connector().CurrentStatus("")
	if !ok {
		t.Fatal("expected current resource status")
	}
	if status.ResourceID != "dummy-resource" {
		t.Fatalf("unexpected resource status: %+v", status)
	}
	if status.ProviderInstanceID != "connect-dummy#1" {
		t.Fatalf("unexpected provider instance id: %+v", status)
	}
	if ctx.ResourceEvents() == nil {
		t.Fatal("expected resource events channel to be wired")
	}
}
