package rpc

import (
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type streamConsumerStub struct {
	id       string
	ackReq   struct {
		deliveryID string
		credits    int
	}
	nackReq struct {
		deliveryID string
		requeue    bool
		reason     string
		credits    int
	}
	grantCredits int
	closed       bool
}

func (s *streamConsumerStub) ID() string { return s.id }

func (s *streamConsumerStub) Recv(timeout time.Duration) (sdk.StreamMessage, bool, error) {
	return sdk.StreamMessage{
		StreamID:   s.id,
		DeliveryID: "delivery-1",
		Sequence:   1,
		RawPayload: []byte(`{"value":42}`),
	}, true, nil
}

func (s *streamConsumerStub) Ack(deliveryID string, credits int) error {
	s.ackReq.deliveryID = deliveryID
	s.ackReq.credits = credits
	return nil
}

func (s *streamConsumerStub) Nack(deliveryID string, requeue bool, reason string, credits int) error {
	s.nackReq.deliveryID = deliveryID
	s.nackReq.requeue = requeue
	s.nackReq.reason = reason
	s.nackReq.credits = credits
	return nil
}

func (s *streamConsumerStub) GrantCredits(credits int) error {
	s.grantCredits = credits
	return nil
}

func (s *streamConsumerStub) Close() error {
	s.closed = true
	return nil
}

type streamConnectorStub struct {
	openReq sdk.StreamOpenRequest
	stream  *streamConsumerStub
}

func (s *streamConnectorStub) Read(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (s *streamConnectorStub) Write(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (s *streamConnectorStub) BatchRead(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (s *streamConnectorStub) BatchWrite(req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return sdk.ConnectorResponse{}, nil
}

func (s *streamConnectorStub) OpenStream(req sdk.StreamOpenRequest) (sdk.StreamConsumer, error) {
	s.openReq = req
	if s.stream == nil {
		s.stream = &streamConsumerStub{id: "stream-1"}
	}
	return s.stream, nil
}

func (s *streamConnectorStub) CurrentStatus(resourceRef string) (sdk.ResourceStatusEvent, bool) {
	return sdk.ResourceStatusEvent{}, false
}

func TestEngineRPCServerConnectorStreamLifecycle(t *testing.T) {
	connector := &streamConnectorStub{}
	server := NewEngineRPCServerWithRegistry(
		nil,
		&sdk.DefaultLogger{},
		"rule-test",
		"node-test",
		nil,
		nil,
		nil,
		allowRegistry{},
		nil,
		connector,
		nil,
	)

	var openReply ConnectorOpenStreamRPCReply
	if err := server.ConnectorOpenStreamRPC(&sdk.StreamOpenRequest{
		ResourceRef:    "mqtt-1",
		Target:         "messages",
		InitialCredits: 4,
	}, &openReply); err != nil {
		t.Fatalf("open rpc failed: %v", err)
	}
	if connector.openReq.ResourceRef != "mqtt-1" || connector.openReq.InitialCredits != 4 {
		t.Fatalf("unexpected open request: %#v", connector.openReq)
	}

	var recvReply ConnectorReceiveStreamRPCReply
	if err := server.ConnectorReceiveStreamRPC(&sdk.StreamReceiveRequest{
		StreamID:      "stream-1",
		WaitTimeoutMS: 100,
	}, &recvReply); err != nil {
		t.Fatalf("receive rpc failed: %v", err)
	}
	if len(recvReply.Response.Messages) != 1 || recvReply.Response.Messages[0].DeliveryID != "delivery-1" {
		t.Fatalf("unexpected receive response: %#v", recvReply)
	}

	if err := server.ConnectorAckStreamRPC(&sdk.StreamAckRequest{
		StreamID:   "stream-1",
		DeliveryID: "delivery-1",
		Credits:    2,
	}, &ConnectorAckStreamRPCReply{}); err != nil {
		t.Fatalf("ack rpc failed: %v", err)
	}
	if connector.stream.ackReq.deliveryID != "delivery-1" || connector.stream.ackReq.credits != 2 {
		t.Fatalf("unexpected ack request: %#v", connector.stream.ackReq)
	}

	if err := server.ConnectorNackStreamRPC(&sdk.StreamNackRequest{
		StreamID:   "stream-1",
		DeliveryID: "delivery-2",
		Requeue:    true,
		Reason:     "retry",
		Credits:    1,
	}, &ConnectorNackStreamRPCReply{}); err != nil {
		t.Fatalf("nack rpc failed: %v", err)
	}
	if connector.stream.nackReq.deliveryID != "delivery-2" || !connector.stream.nackReq.requeue {
		t.Fatalf("unexpected nack request: %#v", connector.stream.nackReq)
	}

	if err := server.ConnectorGrantCreditsRPC(&sdk.StreamGrantCreditsRequest{
		StreamID: "stream-1",
		Credits:  3,
	}, &ConnectorGrantCreditsRPCReply{}); err != nil {
		t.Fatalf("grant rpc failed: %v", err)
	}
	if connector.stream.grantCredits != 3 {
		t.Fatalf("unexpected grant credits: %d", connector.stream.grantCredits)
	}

	if err := server.ConnectorCloseStreamRPC(&sdk.StreamCloseRequest{
		StreamID: "stream-1",
		Reason:   "stop",
	}, &ConnectorCloseStreamRPCReply{}); err != nil {
		t.Fatalf("close rpc failed: %v", err)
	}
	if !connector.stream.closed {
		t.Fatal("expected stream to close")
	}
}

func TestEngineRPCServerNextResourceEventRPCWaitsForTimeoutWhenEventsUnavailable(t *testing.T) {
	server := NewEngineRPCServerWithRegistry(
		nil,
		&sdk.DefaultLogger{},
		"rule-test",
		"node-test",
		nil,
		nil,
		nil,
		allowRegistry{},
		nil,
		nil,
		nil,
	)

	start := time.Now()
	var reply NextResourceEventRPCReply
	if err := server.NextResourceEventRPC(&NextResourceEventRPCArgs{TimeoutMS: 50}, &reply); err != nil {
		t.Fatalf("NextResourceEventRPC failed: %v", err)
	}
	elapsed := time.Since(start)

	if reply.OK {
		t.Fatal("expected no resource event when channel is unavailable")
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("expected rpc to wait close to timeout, got %s", elapsed)
	}
}

func TestEngineRPCServerNextResourceEventRPCReturnsEventWhenAvailable(t *testing.T) {
	events := make(chan sdk.ResourceStatusEvent, 1)
	expected := sdk.ResourceStatusEvent{
		ResourceID: "mqtt-1",
		Status:     sdk.ResourceStatusHealthy,
		Message:    "resource is healthy",
	}
	events <- expected

	server := NewEngineRPCServerWithRegistry(
		nil,
		&sdk.DefaultLogger{},
		"rule-test",
		"node-test",
		nil,
		nil,
		nil,
		allowRegistry{},
		nil,
		nil,
		events,
	)

	start := time.Now()
	var reply NextResourceEventRPCReply
	if err := server.NextResourceEventRPC(&NextResourceEventRPCArgs{TimeoutMS: 200}, &reply); err != nil {
		t.Fatalf("NextResourceEventRPC failed: %v", err)
	}
	elapsed := time.Since(start)

	if !reply.OK {
		t.Fatal("expected resource event to be returned")
	}
	if reply.Event.ResourceID != expected.ResourceID {
		t.Fatalf("unexpected resource id: got %q want %q", reply.Event.ResourceID, expected.ResourceID)
	}
	if reply.Event.Status != expected.Status {
		t.Fatalf("unexpected status: got %q want %q", reply.Event.Status, expected.Status)
	}
	if reply.Event.Message != expected.Message {
		t.Fatalf("unexpected message: got %q want %q", reply.Event.Message, expected.Message)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected rpc to return promptly when event is available, got %s", elapsed)
	}
}
