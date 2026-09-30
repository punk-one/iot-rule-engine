package connector

import (
	"context"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type BoundClient struct {
	bus         *ResourceBus
	token       string
	resourceID  string
	priority    string
	usageWeight int
}

type boundStreamConsumer struct {
	bus             *ResourceBus
	token           string
	streamID        string
	maxDeliverBatch int
	buffer          []sdk.StreamMessage
}

func NewBoundClient(bus *ResourceBus, token, resourceID string) sdk.ConnectorClient {
	return NewBoundClientWithDefaults(bus, token, resourceID, "", 0)
}

func NewBoundClientWithDefaults(bus *ResourceBus, token, resourceID, priority string, usageWeight int) sdk.ConnectorClient {
	if bus == nil || token == "" || resourceID == "" {
		return noopClient{}
	}
	return &BoundClient{
		bus:         bus,
		token:       token,
		resourceID:  resourceID,
		priority:    priority,
		usageWeight: usageWeight,
	}
}

func (c *BoundClient) Read(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	c.applyDefaults(&req)
	return c.bus.Read(context.Background(), c.token, req)
}

func (c *BoundClient) Write(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	c.applyDefaults(&req)
	return c.bus.Write(context.Background(), c.token, req)
}

func (c *BoundClient) BatchRead(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	c.applyDefaults(&req)
	req.Kind = sdk.ConnectorRequestBatchRead
	return c.bus.Execute(context.Background(), c.token, req)
}

func (c *BoundClient) BatchWrite(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	c.applyDefaults(&req)
	req.Kind = sdk.ConnectorRequestBatchWrite
	return c.bus.Execute(context.Background(), c.token, req)
}

func (c *BoundClient) OpenStream(req sdk.StreamOpenRequest) (sdk.StreamConsumer, error) {
	if c == nil || c.bus == nil {
		return noopStreamConsumer{}, nil
	}
	if req.ResourceRef == "" {
		req.ResourceRef = c.resourceID
	}
	if req.Priority == "" && c.priority != "" {
		req.Priority = c.priority
	}
	if req.UsageWeight <= 0 && c.usageWeight > 0 {
		req.UsageWeight = c.usageWeight
	}
	resp, err := c.bus.OpenStream(context.Background(), c.token, req)
	if err != nil {
		return nil, err
	}
	return &boundStreamConsumer{
		bus:             c.bus,
		token:           c.token,
		streamID:        resp.StreamID,
		maxDeliverBatch: max(1, req.MaxDeliverBatch),
	}, nil
}

func (c *BoundClient) CurrentStatus(resourceRef string) (sdk.ResourceStatusEvent, bool) {
	if resourceRef != "" {
		return c.bus.CurrentStatus(resourceRef)
	}
	return c.bus.CurrentStatus(c.resourceID)
}

func (c *BoundClient) applyDefaults(req *sdk.ConnectorRequest) {
	if req == nil {
		return
	}
	if req.Priority == "" && c.priority != "" {
		req.Priority = c.priority
	}
	if req.UsageWeight <= 0 && c.usageWeight > 0 {
		req.UsageWeight = c.usageWeight
	}
}

type noopClient struct{}
type noopStreamConsumer struct{}

func (noopClient) Read(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopClient) Write(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopClient) BatchRead(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopClient) BatchWrite(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (noopClient) OpenStream(req sdk.StreamOpenRequest) (sdk.StreamConsumer, error) {
	return noopStreamConsumer{}, nil
}

func (noopClient) CurrentStatus(resourceRef string) (sdk.ResourceStatusEvent, bool) {
	return sdk.ResourceStatusEvent{}, false
}

func (c *boundStreamConsumer) ID() string {
	if c == nil {
		return ""
	}
	return c.streamID
}

func (c *boundStreamConsumer) Recv(timeout time.Duration) (sdk.StreamMessage, bool, error) {
	if c == nil || c.bus == nil {
		return sdk.StreamMessage{}, false, nil
	}
	if len(c.buffer) > 0 {
		message := c.buffer[0]
		c.buffer = c.buffer[1:]
		return message, true, nil
	}
	batchSize := c.maxDeliverBatch
	if batchSize <= 0 {
		batchSize = 1
	}
	resp, err := c.bus.ReceiveStream(context.Background(), c.token, sdk.StreamReceiveRequest{
		StreamID:      c.streamID,
		MaxMessages:   batchSize,
		WaitTimeoutMS: int(timeout / time.Millisecond),
	})
	if err != nil {
		return sdk.StreamMessage{}, false, err
	}
	if len(resp.Messages) == 0 {
		return sdk.StreamMessage{}, false, nil
	}
	if len(resp.Messages) > 1 {
		c.buffer = append(c.buffer[:0], resp.Messages[1:]...)
	}
	return resp.Messages[0], true, nil
}

func (c *boundStreamConsumer) Ack(deliveryID string, credits int) error {
	return c.bus.AckStream(context.Background(), c.token, sdk.StreamAckRequest{
		StreamID:   c.streamID,
		DeliveryID: deliveryID,
		Credits:    credits,
	})
}

func (c *boundStreamConsumer) Nack(deliveryID string, requeue bool, reason string, credits int) error {
	return c.bus.NackStream(context.Background(), c.token, sdk.StreamNackRequest{
		StreamID:   c.streamID,
		DeliveryID: deliveryID,
		Reason:     reason,
		Requeue:    requeue,
		Credits:    credits,
	})
}

func (c *boundStreamConsumer) GrantCredits(credits int) error {
	return c.bus.GrantCredits(context.Background(), c.token, sdk.StreamGrantCreditsRequest{
		StreamID: c.streamID,
		Credits:  credits,
	})
}

func (c *boundStreamConsumer) Close() error {
	return c.bus.CloseStream(context.Background(), c.token, sdk.StreamCloseRequest{
		StreamID: c.streamID,
		Reason:   "consumer_closed",
	})
}

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
