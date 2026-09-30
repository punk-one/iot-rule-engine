package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	connectorruntime "github.com/punk-one/iot-rule-engine/internal/core/connector"
	coreplugin "github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

type recordingConnectorProvider struct {
	mu            sync.Mutex
	readCount     int
	writeRequests []sdk.ConnectorRequest
}

func (p *recordingConnectorProvider) CreateResource(ctx context.Context, resource sdk.ConnectorResource) (string, error) {
	return "provider-" + resource.ID, nil
}

func (p *recordingConnectorProvider) DestroyResource(ctx context.Context, providerHandle string) error {
	return nil
}

func (p *recordingConnectorProvider) Execute(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch req.Kind {
	case sdk.ConnectorRequestBatchRead:
		p.readCount++
		payload, _ := json.Marshal(map[string]any{
			"values": map[string]any{
				"running": true,
			},
		})
		return sdk.ConnectorResponse{Payload: payload}, nil
	case sdk.ConnectorRequestBatchWrite:
		p.writeRequests = append(p.writeRequests, req)
		payload, _ := json.Marshal(map[string]any{
			"written": len(p.writeRequests),
		})
		return sdk.ConnectorResponse{Payload: payload}, nil
	default:
		payload, _ := json.Marshal(map[string]any{
			"kind": string(req.Kind),
		})
		return sdk.ConnectorResponse{Payload: payload}, nil
	}
}

func (p *recordingConnectorProvider) OpenStream(ctx context.Context, providerHandle string, req sdk.StreamOpenRequest) (sdk.StreamOpenResponse, error) {
	return sdk.StreamOpenResponse{StreamID: "stream-1", DeliverySemantics: sdk.StreamDeliveryAtLeastOnce, InitialCredits: req.InitialCredits}, nil
}

func (p *recordingConnectorProvider) ReceiveStream(ctx context.Context, providerHandle string, req sdk.StreamReceiveRequest) (sdk.StreamReceiveResponse, error) {
	return sdk.StreamReceiveResponse{}, nil
}

func (p *recordingConnectorProvider) AckStream(ctx context.Context, providerHandle string, req sdk.StreamAckRequest) error {
	return nil
}

func (p *recordingConnectorProvider) NackStream(ctx context.Context, providerHandle string, req sdk.StreamNackRequest) error {
	return nil
}

func (p *recordingConnectorProvider) GrantCredits(ctx context.Context, providerHandle string, req sdk.StreamGrantCreditsRequest) error {
	return nil
}

func (p *recordingConnectorProvider) CloseStream(ctx context.Context, providerHandle string, req sdk.StreamCloseRequest) error {
	return nil
}

func (p *recordingConnectorProvider) Probe(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ResourceStatusEvent, error) {
	return sdk.ResourceStatusEvent{
		ResourceID:         "dummy-resource",
		ProviderPluginID:   "connect-dummy",
		Status:             sdk.ResourceStatusHealthy,
		ReasonCode:         "READY",
		Message:            "healthy",
		ObservedAtUnixNano: time.Now().UnixNano(),
	}, nil
}

func (p *recordingConnectorProvider) snapshot() (int, []sdk.ConnectorRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	requests := make([]sdk.ConnectorRequest, len(p.writeRequests))
	copy(requests, p.writeRequests)
	return p.readCount, requests
}

func TestPilotSourcePluginStartsWithConnectorBinding(t *testing.T) {
	srv := initEmbeddedJetStream(t)
	defer shutdownEmbeddedJetStream(t, srv)

	pluginsDir := buildPilotEnginePlugins(t, []pluginBuildSpec{
		{
			subdir:      "sources",
			binaryName:  "source-s7-latest.so",
			packagePath: "../../../../iot-rule-plugins/sources/source_s7",
		},
	})

	loader := coreplugin.NewLoader(pluginsDir)
	if err := loader.ScanPlugins(); err != nil {
		t.Fatalf("ScanPlugins failed: %v", err)
	}
	defer loader.Close()

	dag, err := BuildDAGWithLoader(&schema.RuleFlow{
		ID:      "rule-pilot-source",
		Version: 1,
		Enabled: true,
		Nodes: []*schema.NodeConfig{
			{
				ID:     "source-node",
				Type:   "source",
				Plugin: "source-s7",
				Config: map[string]any{
					"connector_ref": "dummy-resource",
					"output": map[string]any{
						"device_code": "plc-01",
					},
					"points": []map[string]any{
						{
							"address":    "DB1.DBX0.0",
							"name":       "running",
							"value_type": "Bool",
						},
					},
					"polling": map[string]any{
						"batch_size":     1,
						"interval_ms":    100,
						"max_backoff_ms": 200,
					},
				},
			},
		},
	}, loader)
	if err != nil {
		t.Fatalf("BuildDAGWithLoader failed: %v", err)
	}

	provider := &recordingConnectorProvider{}
	resourceBus := connectorruntime.NewResourceBus(connectorruntime.ResourceBusOptions{})
	resourceBus.RegisterProvider("connect-dummy", provider)
	if err := resourceBus.RegisterResource(sdk.ConnectorResource{
		ID:       "dummy-resource",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("RegisterResource failed: %v", err)
	}

	executor := NewDAGNATSExecutor(dag, runtime.NewManager(), "inst-source")
	executor.SetResourceBus(resourceBus)
	if err := executor.Start(context.Background()); err != nil {
		t.Fatalf("executor.Start failed: %v", err)
	}
	defer executor.Stop()

	plugin := dag.Plugins["source-node"]
	ctx := executor.CreateRuntimeContext("source-node")
	if err := plugin.Start(ctx); err != nil {
		t.Fatalf("plugin.Start failed: %v", err)
	}
	defer plugin.Stop()

	if err := executor.RegisterPlugin("source-node", plugin); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	if _, ok := executor.resourceBindings["source-node"]; !ok {
		t.Fatal("expected source-node connector binding to be created")
	}
	if sub, ok := executor.resourceSubs["source-node"]; !ok || sub.events == nil {
		t.Fatal("expected source-node resource subscription to be created")
	}

	waitFor(t, 2*time.Second, func() bool {
		readCount, _ := provider.snapshot()
		return readCount > 0
	}, "source-s7 to perform at least one connector batch read")
}

func TestPilotSinkPluginForwardsPriorityAndUsageWeightToConnector(t *testing.T) {
	srv := initEmbeddedJetStream(t)
	defer shutdownEmbeddedJetStream(t, srv)

	pluginsDir := buildPilotEnginePlugins(t, []pluginBuildSpec{
		{
			subdir:      "sinks",
			binaryName:  "sink-s7-latest.so",
			packagePath: "../../../../iot-rule-plugins/sinks/sink_s7",
		},
	})

	loader := coreplugin.NewLoader(pluginsDir)
	if err := loader.ScanPlugins(); err != nil {
		t.Fatalf("ScanPlugins failed: %v", err)
	}
	defer loader.Close()

	dag, err := BuildDAGWithLoader(&schema.RuleFlow{
		ID:      "rule-pilot-sink",
		Version: 1,
		Enabled: true,
		Nodes: []*schema.NodeConfig{
			{
				ID:     "sink-node",
				Type:   "sink",
				Plugin: "sink-s7",
				Config: map[string]any{
					"connector_ref":    "dummy-resource",
					"priority":         "high",
					"usage_weight":     3,
					"max_usage_weight": 5,
					"input": map[string]any{
						"data_field": "data",
					},
					"buffer": map[string]any{
						"enabled":           true,
						"max_events":        10,
						"flush_interval_ms": 100,
					},
					"points": []map[string]any{
						{
							"address":    "DB1.DBX0.0",
							"name":       "running",
							"value_type": "Bool",
						},
					},
				},
			},
		},
	}, loader)
	if err != nil {
		t.Fatalf("BuildDAGWithLoader failed: %v", err)
	}

	provider := &recordingConnectorProvider{}
	resourceBus := connectorruntime.NewResourceBus(connectorruntime.ResourceBusOptions{})
	resourceBus.RegisterProvider("connect-dummy", provider)
	if err := resourceBus.RegisterResource(sdk.ConnectorResource{
		ID:       "dummy-resource",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("RegisterResource failed: %v", err)
	}

	executor := NewDAGNATSExecutor(dag, runtime.NewManager(), "inst-sink")
	executor.SetResourceBus(resourceBus)
	if err := executor.Start(context.Background()); err != nil {
		t.Fatalf("executor.Start failed: %v", err)
	}
	defer executor.Stop()

	plugin := dag.Plugins["sink-node"]
	ctx := executor.CreateRuntimeContext("sink-node")
	if err := plugin.Start(ctx); err != nil {
		t.Fatalf("plugin.Start failed: %v", err)
	}
	defer plugin.Stop()

	if err := executor.RegisterPlugin("sink-node", plugin); err != nil {
		t.Fatalf("RegisterPlugin failed: %v", err)
	}

	event := sdk.NewEvent(map[string]any{
		"data": map[string]any{
			"running": true,
		},
	}, nil)
	if err := plugin.OnEvent(event); err != nil {
		t.Fatalf("plugin.OnEvent failed: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		_, requests := provider.snapshot()
		return len(requests) > 0
	}, "sink-s7 to forward at least one connector batch write")

	_, requests := provider.snapshot()
	req := requests[len(requests)-1]
	if req.Kind != sdk.ConnectorRequestBatchWrite {
		t.Fatalf("expected batch write request, got %s", req.Kind)
	}
	if req.Priority != "high" {
		t.Fatalf("expected priority high, got %s", req.Priority)
	}
	if req.UsageWeight != 3 {
		t.Fatalf("expected usage weight 3, got %d", req.UsageWeight)
	}
	if req.Target != "points" {
		t.Fatalf("expected write target points, got %s", req.Target)
	}
}

type pluginBuildSpec struct {
	subdir      string
	binaryName  string
	packagePath string
}

func buildPilotEnginePlugins(t *testing.T, builds []pluginBuildSpec) string {
	t.Helper()

	pluginsDir := t.TempDir()
	for _, build := range builds {
		if _, err := os.Stat(build.packagePath); err != nil {
			if os.IsNotExist(err) {
				t.Skipf("optional plugin source is not available: %s", build.packagePath)
			}
			t.Fatalf("stat plugin source %s failed: %v", build.packagePath, err)
		}

		targetDir := filepath.Join(pluginsDir, build.subdir)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			t.Fatalf("mkdir %s failed: %v", targetDir, err)
		}

		binaryPath := filepath.Join(targetDir, build.binaryName)
		buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
		buildCmd.Dir = build.packagePath
		if output, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("build plugin %s failed: %v\n%s", build.packagePath, err, output)
		}
	}

	return pluginsDir
}

func waitFor(t *testing.T, timeout time.Duration, predicate func() bool, description string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func initEmbeddedJetStream(t *testing.T) *natsserver.Server {
	t.Helper()

	port := freePort(t)
	storeDir := filepath.Join(t.TempDir(), "nats-store")
	srv := runEmbeddedJetStreamServer(t, port, storeDir)

	if existing := bus.GetJetStream(); existing != nil && existing.GetConn() != nil {
		existing.GetConn().Close()
	}

	if err := bus.InitJetStream(bus.NATSOptions{
		URL: fmt.Sprintf("nats://127.0.0.1:%d", port),
	}); err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("InitJetStream failed: %v", err)
	}

	return srv
}

func shutdownEmbeddedJetStream(t *testing.T, srv *natsserver.Server) {
	t.Helper()

	if existing := bus.GetJetStream(); existing != nil && existing.GetConn() != nil {
		existing.GetConn().Close()
	}
	if srv != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
	}
}

func runEmbeddedJetStreamServer(t *testing.T, port int, storeDir string) *natsserver.Server {
	t.Helper()

	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      port,
		JetStream: true,
		StoreDir:  storeDir,
		NoLog:     true,
		NoSigs:    true,
	}

	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create nats-server: %v", err)
	}

	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		srv.Shutdown()
		t.Fatal("embedded nats-server was not ready in time")
	}

	return srv
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type: %T", listener.Addr())
	}
	return addr.Port
}
