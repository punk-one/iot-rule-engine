package plugin

import (
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type pluginStartRPCStub struct {
	replyError string
	lastArgs   *sdk.StartArgs
}

func (s *pluginStartRPCStub) Start(args *sdk.StartArgs, reply *sdk.StartReply) error {
	if args != nil {
		copied := *args
		s.lastArgs = &copied
	}
	reply.Error = s.replyError
	return nil
}

type runtimeContextStub struct {
	ruleID string
	nodeID string
	events <-chan sdk.ResourceStatusEvent
}

func (c runtimeContextStub) RuleID() string             { return c.ruleID }
func (c runtimeContextStub) NodeID() string             { return c.nodeID }
func (c runtimeContextStub) Logger() sdk.Logger         { return &sdk.DefaultLogger{} }
func (c runtimeContextStub) Emitter() sdk.Emitter       { return noopEmitter{} }
func (c runtimeContextStub) Metrics() sdk.Metrics       { return noopMetrics{} }
func (c runtimeContextStub) Health() sdk.HealthReporter { return noopHealthReporter{} }
func (c runtimeContextStub) Connector() sdk.ConnectorClient {
	return noopConnectorClient{}
}
func (c runtimeContextStub) ResourceEvents() <-chan sdk.ResourceStatusEvent { return c.events }

type noopEmitter struct{}

func (noopEmitter) Publish(sdk.Event) error         { return nil }
func (noopEmitter) Ack(string) error                { return nil }
func (noopEmitter) PublishAck(sdk.AckMessage) error { return nil }
func (noopEmitter) EmitTo(string, sdk.Event) error  { return nil }
func (noopEmitter) EmitBatch([]sdk.Event) error     { return nil }

type noopMetrics struct{}

func (noopMetrics) IncCounter(string, map[string]string)       {}
func (noopMetrics) Observe(string, float64, map[string]string) {}

type noopHealthReporter struct{}

func (noopHealthReporter) Report(sdk.HealthReport) error                      { return nil }
func (noopHealthReporter) Healthy(string, map[string]string) error            { return nil }
func (noopHealthReporter) Degraded(string, string, map[string]string) error   { return nil }
func (noopHealthReporter) Recovering(string, string, map[string]string) error { return nil }
func (noopHealthReporter) Unhealthy(string, string, map[string]string) error  { return nil }
func (noopHealthReporter) Heartbeat(string, map[string]string) error          { return nil }

type noopConnectorClient struct{}

func (noopConnectorClient) Read(sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopConnectorClient) Write(sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopConnectorClient) BatchRead(sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopConnectorClient) BatchWrite(sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopConnectorClient) OpenStream(sdk.StreamOpenRequest) (sdk.StreamConsumer, error) {
	return noopStreamConsumer{}, nil
}

func (noopConnectorClient) CurrentStatus(string) (sdk.ResourceStatusEvent, bool) {
	return sdk.ResourceStatusEvent{}, false
}

type noopStreamConsumer struct{}

func (noopStreamConsumer) ID() string { return "" }

func (noopStreamConsumer) Recv(timeout time.Duration) (sdk.StreamMessage, bool, error) {
	return sdk.StreamMessage{}, false, nil
}

func (noopStreamConsumer) Ack(deliveryID string, credits int) error { return nil }

func (noopStreamConsumer) Nack(deliveryID string, requeue bool, reason string, credits int) error {
	return nil
}

func (noopStreamConsumer) GrantCredits(credits int) error { return nil }

func (noopStreamConsumer) Close() error { return nil }

func TestPluginWrapperStartDecodesStructuredSDKErrors(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	stub := &pluginStartRPCStub{
		replyError: `PUNK_PLUGIN_ERROR:{"kind":"fatal","message":"boom"}`,
	}

	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Plugin", stub); err != nil {
		t.Fatalf("register rpc server: %v", err)
	}
	go rpcServer.ServeConn(serverConn)

	rpcClient := rpc.NewClient(clientConn)
	defer rpcClient.Close()

	wrapper := &pluginWrapper{
		rpcClient: &sdk.PluginRPCClient{Client: rpcClient},
		info:      sdk.PluginInfo{ID: "test-plugin"},
	}

	err := wrapper.Start(runtimeContextStub{
		ruleID: "rule-1",
		nodeID: "node-1",
	})
	if err == nil {
		t.Fatal("expected start to fail")
	}
	if err.Error() != "boom" {
		t.Fatalf("expected decoded error message, got %q", err.Error())
	}
	if !sdk.IsFatal(err) {
		t.Fatalf("expected fatal sdk error, got %T: %v", err, err)
	}
	if stub.lastArgs == nil {
		t.Fatal("expected start args to be forwarded")
	}
	if stub.lastArgs.RuleID != "rule-1" || stub.lastArgs.NodeID != "node-1" {
		t.Fatalf("unexpected start args: %+v", stub.lastArgs)
	}
	if stub.lastArgs.Health.HeartbeatInterval <= 0 || stub.lastArgs.Health.MaxSilencePeriod <= 0 || stub.lastArgs.Health.QueueCapacity <= 0 {
		t.Fatalf("expected default health options, got %+v", stub.lastArgs.Health)
	}
	if stub.lastArgs.ResourceEventsEnabled {
		t.Fatalf("expected resource events to be disabled for nil channel, got %+v", stub.lastArgs)
	}
}

func TestPluginWrapperStartForwardsResourceEventsCapability(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	stub := &pluginStartRPCStub{}

	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Plugin", stub); err != nil {
		t.Fatalf("register rpc server: %v", err)
	}
	go rpcServer.ServeConn(serverConn)

	rpcClient := rpc.NewClient(clientConn)
	defer rpcClient.Close()

	wrapper := &pluginWrapper{
		rpcClient: &sdk.PluginRPCClient{Client: rpcClient},
		info:      sdk.PluginInfo{ID: "test-plugin"},
	}

	events := make(chan sdk.ResourceStatusEvent)
	defer close(events)

	if err := wrapper.Start(runtimeContextStub{
		ruleID: "rule-2",
		nodeID: "node-2",
		events: events,
	}); err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}
	if stub.lastArgs == nil {
		t.Fatal("expected start args to be forwarded")
	}
	if !stub.lastArgs.ResourceEventsEnabled {
		t.Fatalf("expected resource events capability to be forwarded, got %+v", stub.lastArgs)
	}
}

func TestShouldReplaceLoadedPluginPrefersLatestArtifact(t *testing.T) {
	existing := &PluginClient{
		Path: "/plugins/sources/source-s7-1.0.0.so",
		Info: sdk.PluginInfo{ID: "source-s7", Version: "1.0.0"},
	}
	candidate := &PluginClient{
		Path: "/plugins/sources/source-s7-latest.so",
		Info: sdk.PluginInfo{ID: "source-s7", Version: "2.0.0"},
	}

	replace, reason := shouldReplaceLoadedPlugin(existing, candidate)
	if !replace {
		t.Fatalf("expected candidate to replace existing, reason=%s", reason)
	}
}

func TestShouldReplaceLoadedPluginRejectsOlderVersion(t *testing.T) {
	existing := &PluginClient{
		Path: "/plugins/sources/source-s7-latest.so",
		Info: sdk.PluginInfo{ID: "source-s7", Version: "2.0.0"},
	}
	candidate := &PluginClient{
		Path: "/plugins/sources/source-s7-1.0.0.so",
		Info: sdk.PluginInfo{ID: "source-s7", Version: "1.0.0"},
	}

	replace, reason := shouldReplaceLoadedPlugin(existing, candidate)
	if replace {
		t.Fatalf("expected existing plugin to win, reason=%s", reason)
	}
}

func TestAcquirePluginInstanceSharesOneProcessAndIsolatesRPCObjects(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "factory-plugin")
	build := exec.Command("go", "build", "-o", binaryPath, "./testdata/factoryplugin")
	buildOutput, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build factory plugin: %v\n%s", err, buildOutput)
	}

	loader := NewLoader(t.TempDir())
	first, err := loader.AcquirePluginInstance(binaryPath)
	if err != nil {
		t.Fatalf("acquire first instance: %v", err)
	}
	aliasPath := binaryPath + "-latest.so"
	if err := os.Symlink(binaryPath, aliasPath); err != nil {
		_ = loader.ReleasePluginInstance(first)
		t.Fatalf("create plugin alias: %v", err)
	}
	second, err := loader.AcquirePluginInstance(aliasPath)
	if err != nil {
		_ = loader.ReleasePluginInstance(first)
		t.Fatalf("acquire second instance: %v", err)
	}

	if first.Client != second.Client {
		t.Fatal("instances were started in different operating-system processes")
	}
	if first.RPCClient == second.RPCClient {
		t.Fatal("instances unexpectedly share one rule-node RPC object")
	}
	owner := first.owner
	if owner == nil || second.owner != owner {
		t.Fatal("instances do not reference the same process owner")
	}
	owner.mutex.RLock()
	active := owner.ActiveInstances
	owner.mutex.RUnlock()
	if active != 2 {
		t.Fatalf("active instances=%d, want 2", active)
	}
	if err := loader.UnloadPlugin(owner.Info.ID); err == nil {
		t.Fatal("expected unload with active instances to fail")
	}
	if _, err := loader.ReloadPlugin(owner.Info.ID); err == nil {
		t.Fatal("expected reload with active instances to fail")
	}

	if err := loader.ReleasePluginInstance(first); err != nil {
		t.Fatalf("release first instance: %v", err)
	}
	if err := loader.ReleasePluginInstance(first); err != nil {
		t.Fatalf("release first instance twice: %v", err)
	}
	if err := loader.ReleasePluginInstance(second); err != nil {
		t.Fatalf("release second instance: %v", err)
	}
	owner.mutex.RLock()
	active = owner.ActiveInstances
	owner.mutex.RUnlock()
	if active != 0 {
		t.Fatalf("active instances after release=%d, want 0", active)
	}
	loader.SetRestartPolicy(RestartPolicy{
		BaseDelay:        time.Millisecond,
		MaxDelay:         time.Millisecond,
		MaxRestarts:      2,
		JitterRatio:      0.01,
		FailureThreshold: 2,
		CircuitCooldown:  time.Second,
	})
	reloaded, err := loader.ReloadPlugin(owner.Info.ID)
	if err != nil {
		t.Fatalf("reload idle process: %v", err)
	}
	if reloaded == owner || reloaded.Client == owner.Client {
		t.Fatal("reload retained the exited process")
	}
	if reloaded.Client.Exited() {
		t.Fatal("reloaded process is already exited")
	}
	if err := loader.UnloadPlugin(reloaded.Info.ID); err != nil {
		t.Fatalf("unload idle process: %v", err)
	}
}
