package engine

import (
	"context"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

// pluginContext implements sdk.RuntimeContext for the current engine.
type pluginContext struct {
	ruleID     string
	nodeID     string
	ctx        context.Context
	dispatcher *dispatcher.Dispatcher

	savePluginRPCCB func(client *sdk.PluginRPCClient)
	engineRPCServer interface{}
	onEmitCallback  func(eventID string)

	state          sdk.StateManager
	logger         sdk.Logger
	metrics        sdk.Metrics
	health         sdk.HealthReporter
	connector      sdk.ConnectorClient
	resourceEvents <-chan sdk.ResourceStatusEvent
}

func NewPluginContext(
	ctx context.Context,
	ruleID, nodeID string,
	disp *dispatcher.Dispatcher,
	state sdk.StateManager,
	connectorClient sdk.ConnectorClient,
	resourceEvents <-chan sdk.ResourceStatusEvent,
	saveCB func(*sdk.PluginRPCClient),
	engineRPCServer interface{},
	health sdk.HealthReporter,
	onEmitCB func(eventID string),
) sdk.RuntimeContext {
	if health == nil {
		health = nopNodeHealthReporter{}
	}
	return &pluginContext{
		ruleID:          ruleID,
		nodeID:          nodeID,
		ctx:             ctx,
		dispatcher:      disp,
		state:           state,
		savePluginRPCCB: saveCB,
		engineRPCServer: engineRPCServer,
		onEmitCallback:  onEmitCB,
		logger:          &sdk.DefaultLogger{},
		metrics:         newDispatcherMetrics(disp, ruleID, nodeID),
		health:          health,
		connector:       connectorClient,
		resourceEvents:  resourceEvents,
	}
}

type dispatcherMetrics struct {
	disp   *dispatcher.Dispatcher
	ruleID string
	nodeID string
}

func newDispatcherMetrics(disp *dispatcher.Dispatcher, ruleID, nodeID string) sdk.Metrics {
	return &dispatcherMetrics{disp: disp, ruleID: ruleID, nodeID: nodeID}
}

func (m *dispatcherMetrics) IncCounter(name string, labels map[string]string) {
	if m.disp != nil {
		m.disp.OnPluginMetric(m.ruleID, m.nodeID, name, 1, labels)
	}
}

func (m *dispatcherMetrics) Observe(name string, value float64, labels map[string]string) {
	if m.disp != nil {
		m.disp.OnPluginMetric(m.ruleID, m.nodeID, name, value, labels)
	}
}

func (c *pluginContext) GetEngineRPCServer() interface{} {
	return c.engineRPCServer
}

func (c *pluginContext) SavePluginRPCClient(client *sdk.PluginRPCClient) {
	if c.savePluginRPCCB != nil {
		c.savePluginRPCCB(client)
	}
}

func (c *pluginContext) SaveEngineRPC(engineRPC interface{}) {
	client, ok := engineRPC.(*sdk.PluginRPCClient)
	if !ok {
		return
	}
	c.SavePluginRPCClient(client)
}

func (c *pluginContext) RuleID() string             { return c.ruleID }
func (c *pluginContext) NodeID() string             { return c.nodeID }
func (c *pluginContext) Logger() sdk.Logger         { return c.logger }
func (c *pluginContext) Metrics() sdk.Metrics       { return c.metrics }
func (c *pluginContext) State() sdk.StateManager    { return c.state }
func (c *pluginContext) Health() sdk.HealthReporter { return c.health }
func (c *pluginContext) Connector() sdk.ConnectorClient {
	if c.connector == nil {
		c.connector = noopConnectorClient{}
	}
	return c.connector
}
func (c *pluginContext) ResourceEvents() <-chan sdk.ResourceStatusEvent { return c.resourceEvents }
func (c *pluginContext) Emitter() sdk.Emitter                           { return c }

func (c *pluginContext) Publish(event sdk.Event) error {
	owned := event.Clone()
	return c.dispatcher.OnPluginEmit(c.ruleID, c.nodeID, &owned)
}

func (c *pluginContext) EmitTo(label string, event sdk.Event) error {
	owned := event.Clone()
	if owned.Metadata == nil {
		owned.Metadata = make(map[string]string)
	}
	owned.Metadata["route_label"] = label
	return c.dispatcher.OnPluginEmit(c.ruleID, c.nodeID, &owned)
}

func (c *pluginContext) Ack(eventID string) error {
	if c.dispatcher != nil {
		c.dispatcher.OnPluginAck(c.ruleID, c.nodeID, eventID)
	}
	return nil
}

func (c *pluginContext) PublishAck(ack sdk.AckMessage) error {
	if ack.RuleID == "" {
		ack.RuleID = c.ruleID
	}
	if ack.FromNodeID == "" {
		ack.FromNodeID = c.nodeID
	}
	if ack.AckedAt.IsZero() {
		ack.AckedAt = time.Now()
	}
	if err := bus.PublishRuleAck(ack.RuleID, ack); err != nil {
		return err
	}
	if c.dispatcher != nil {
		c.dispatcher.OnPluginAck(c.ruleID, c.nodeID, ack.RecordID)
	}
	return nil
}

func (c *pluginContext) EmitBatch(events []sdk.Event) error {
	owned := make([]sdk.Event, len(events))
	for i := range events {
		owned[i] = events[i].Clone()
	}
	return c.dispatcher.OnPluginEmitBatch(c.ruleID, c.nodeID, owned)
}

type noopConnectorClient struct{}

func (noopConnectorClient) Read(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, sdk.ErrNotImplemented
}

func (noopConnectorClient) Write(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, sdk.ErrNotImplemented
}

func (noopConnectorClient) BatchRead(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, sdk.ErrNotImplemented
}

func (noopConnectorClient) BatchWrite(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, sdk.ErrNotImplemented
}

func (noopConnectorClient) OpenStream(req sdk.StreamOpenRequest) (sdk.StreamConsumer, error) {
	return nil, sdk.ErrNotImplemented
}

func (noopConnectorClient) CurrentStatus(resourceRef string) (sdk.ResourceStatusEvent, bool) {
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
