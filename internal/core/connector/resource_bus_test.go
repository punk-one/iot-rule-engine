package connector

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type dummyProvider struct {
	mu             sync.Mutex
	cond           *sync.Cond
	pluginID       string
	instanceID     string
	createCalls    int
	destroyCalls   int
	executeCalls   int
	probeCalls     int
	inflight       int
	maxObserved    int
	handles        map[string]string
	policies       map[string]sdk.ProviderPolicy
	started        chan struct{}
	release        chan struct{}
	order          []string
	probeEvent     sdk.ResourceStatusEvent
	executeErr     error
	streamOpenReq  sdk.StreamOpenRequest
	streamRecvReq  sdk.StreamReceiveRequest
	streamAckReq   sdk.StreamAckRequest
	streamNackReq  sdk.StreamNackRequest
	streamGrantReq sdk.StreamGrantCreditsRequest
	streamCloseReq sdk.StreamCloseRequest
}

func (p *dummyProvider) CreateResource(ctx context.Context, resource sdk.ConnectorResource) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cond == nil {
		p.cond = sync.NewCond(&p.mu)
	}
	if p.handles == nil {
		p.handles = make(map[string]string)
	}
	if p.policies == nil {
		p.policies = make(map[string]sdk.ProviderPolicy)
	}
	p.createCalls++
	handle := "handle-" + resource.ID
	p.handles[handle] = resource.ID
	p.policies[resource.ID] = resource.ProviderPolicy
	return handle, nil
}

func (p *dummyProvider) DestroyResource(ctx context.Context, providerHandle string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.destroyCalls++
	if resourceID, ok := p.handles[providerHandle]; ok {
		delete(p.handles, providerHandle)
		delete(p.policies, resourceID)
	}
	if p.cond != nil {
		p.cond.Broadcast()
	}
	return nil
}

func (p *dummyProvider) Execute(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	p.mu.Lock()
	p.executeCalls++
	executeCalls := p.executeCalls
	started := p.started
	release := p.release
	p.order = append(p.order, req.Target)
	p.mu.Unlock()

	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil && executeCalls == 1 {
		<-release
	}

	payload, _ := json.Marshal(map[string]string{
		"handle": providerHandle,
		"kind":   string(req.Kind),
	})
	return sdk.ConnectorResponse{Payload: payload}, p.executeErr
}

func (p *dummyProvider) Probe(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ResourceStatusEvent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probeCalls++
	if p.probeEvent.Status == "" {
		return sdk.ResourceStatusEvent{
			ResourceID:         providerHandle,
			ProviderPluginID:   "connect-dummy",
			Status:             sdk.ResourceStatusHealthy,
			ReasonCode:         "READY",
			Message:            "healthy",
			ObservedAtUnixNano: time.Now().UnixNano(),
		}, nil
	}
	event := p.probeEvent
	if event.ResourceID == "" {
		event.ResourceID = providerHandle
	}
	if event.ProviderPluginID == "" {
		event.ProviderPluginID = "connect-dummy"
	}
	if event.ObservedAtUnixNano == 0 {
		event.ObservedAtUnixNano = time.Now().UnixNano()
	}
	return event, nil
}

func (p *dummyProvider) OpenStream(ctx context.Context, providerHandle string, req sdk.StreamOpenRequest) (sdk.StreamOpenResponse, error) {
	p.mu.Lock()
	p.streamOpenReq = req
	p.mu.Unlock()
	return sdk.StreamOpenResponse{
		StreamID:          "stream-1",
		DeliverySemantics: sdk.StreamDeliveryAtLeastOnce,
		InitialCredits:    req.InitialCredits,
	}, nil
}

func (p *dummyProvider) ReceiveStream(ctx context.Context, providerHandle string, req sdk.StreamReceiveRequest) (sdk.StreamReceiveResponse, error) {
	p.mu.Lock()
	p.streamRecvReq = req
	p.mu.Unlock()
	return sdk.StreamReceiveResponse{
		Messages: []sdk.StreamMessage{{
			StreamID:            req.StreamID,
			DeliveryID:          "delivery-1",
			Sequence:            1,
			RawPayload:          []byte(`{"value":42}`),
			PublishedAtUnixNano: time.Now().UnixNano(),
		}},
	}, nil
}

func (p *dummyProvider) AckStream(ctx context.Context, providerHandle string, req sdk.StreamAckRequest) error {
	p.mu.Lock()
	p.streamAckReq = req
	p.mu.Unlock()
	return nil
}

func (p *dummyProvider) NackStream(ctx context.Context, providerHandle string, req sdk.StreamNackRequest) error {
	p.mu.Lock()
	p.streamNackReq = req
	p.mu.Unlock()
	return nil
}

func (p *dummyProvider) GrantCredits(ctx context.Context, providerHandle string, req sdk.StreamGrantCreditsRequest) error {
	p.mu.Lock()
	p.streamGrantReq = req
	p.mu.Unlock()
	return nil
}

func (p *dummyProvider) CloseStream(ctx context.Context, providerHandle string, req sdk.StreamCloseRequest) error {
	p.mu.Lock()
	p.streamCloseReq = req
	p.mu.Unlock()
	return nil
}

func (p *dummyProvider) ProviderPluginID() string {
	if p.pluginID == "" {
		return "connect-dummy"
	}
	return p.pluginID
}

func (p *dummyProvider) ProviderInstanceID() string {
	return p.instanceID
}

func (p *dummyProvider) AcquireProviderSlot(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cond == nil {
		p.cond = sync.NewCond(&p.mu)
	}
	for {
		limit := p.maxInflightLimitLocked()
		if limit <= 0 || p.inflight < limit {
			p.inflight++
			if p.inflight > p.maxObserved {
				p.maxObserved = p.inflight
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		p.cond.Wait()
	}
}

func (p *dummyProvider) ReleaseProviderSlot() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inflight > 0 {
		p.inflight--
	}
	if p.cond != nil {
		p.cond.Broadcast()
	}
}

func (p *dummyProvider) maxInflightLimitLocked() int {
	limit := 0
	for _, policy := range p.policies {
		if policy.MaxInflightPerProvider <= 0 {
			continue
		}
		if limit == 0 || policy.MaxInflightPerProvider < limit {
			limit = policy.MaxInflightPerProvider
		}
	}
	return limit
}

func TestResourceBusWarmupAndSharedRuntime(t *testing.T) {
	bus := NewResourceBus(ResourceBusOptions{})
	provider := &dummyProvider{}
	bus.RegisterProvider("connect-dummy", provider)

	err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-1",
		PluginID: "connect-dummy",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("register resource: %v", err)
	}

	if err := bus.Warmup(context.Background(), "resource-1"); err != nil {
		t.Fatalf("warmup: %v", err)
	}

	tokenA, err := bus.Bind(context.Background(), "resource-1")
	if err != nil {
		t.Fatalf("bind tokenA: %v", err)
	}
	tokenB, err := bus.Bind(context.Background(), "resource-1")
	if err != nil {
		t.Fatalf("bind tokenB: %v", err)
	}

	if provider.createCalls != 1 {
		t.Fatalf("expected one provider create call, got %d", provider.createCalls)
	}

	if err := bus.Release(context.Background(), tokenA); err != nil {
		t.Fatalf("release tokenA: %v", err)
	}
	if provider.destroyCalls != 0 {
		t.Fatalf("expected no destroy while references remain, got %d", provider.destroyCalls)
	}

	if err := bus.Release(context.Background(), tokenB); err != nil {
		t.Fatalf("release tokenB: %v", err)
	}
	if provider.destroyCalls != 1 {
		t.Fatalf("expected destroy after last reference, got %d", provider.destroyCalls)
	}
}

func TestResourceBusMarksCongestedResponses(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	provider := &dummyProvider{
		started: started,
		release: release,
	}
	bus := NewResourceBus(ResourceBusOptions{CongestionThreshold: 1})
	bus.RegisterProvider("connect-dummy", provider)

	err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-2",
		PluginID: "connect-dummy",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.Bind(context.Background(), "resource-2")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() {
		_ = bus.Release(context.Background(), token)
	}()

	errCh := make(chan error, 1)
	go func() {
		_, runErr := bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "first"})
		errCh <- runErr
	}()
	<-started

	respCh := make(chan sdk.ConnectorResponse, 1)
	errSecondCh := make(chan error, 1)
	go func() {
		resp, runErr := bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "second"})
		respCh <- resp
		errSecondCh <- runErr
	}()
	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("first read: %v", err)
	}
	if err := <-errSecondCh; err != nil {
		t.Fatalf("second read: %v", err)
	}
	resp := <-respCh
	if !resp.Congested {
		t.Fatal("expected congested flag to be set on second response")
	}
	if resp.RetryAfterMS <= 0 {
		t.Fatalf("expected positive retry_after_ms, got %d", resp.RetryAfterMS)
	}

	status, ok := bus.CurrentStatus("resource-2")
	if !ok {
		t.Fatal("expected resource status")
	}
	if status.Status != sdk.ResourceStatusHealthy {
		t.Fatalf("expected resource status to recover to healthy, got %s", status.Status)
	}
}

func TestResourceBusResolvesProviderLazily(t *testing.T) {
	bus := NewResourceBus(ResourceBusOptions{})
	provider := &dummyProvider{}
	resolveCalls := 0

	bus.SetProviderResolver(func(pluginID string) (Provider, error) {
		resolveCalls++
		if pluginID != "connect-dummy" {
			t.Fatalf("unexpected plugin id: %s", pluginID)
		}
		return provider, nil
	})

	err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-3",
		PluginID: "connect-dummy",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.Bind(context.Background(), "resource-3")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() {
		_ = bus.Release(context.Background(), token)
	}()

	if resolveCalls != 1 {
		t.Fatalf("expected resolver to be called once, got %d", resolveCalls)
	}
	if provider.createCalls != 1 {
		t.Fatalf("expected provider create to be called once, got %d", provider.createCalls)
	}
}

func TestResourceBusPrioritizesHighPriorityWrites(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	provider := &dummyProvider{
		started: started,
		release: release,
	}
	bus := NewResourceBus(ResourceBusOptions{MaxHighPriorityBurst: 2})
	bus.RegisterProvider("connect-dummy", provider)

	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-priority",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.Bind(context.Background(), "resource-priority")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	firstDone := make(chan struct{})
	go func() {
		_, _ = bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "read-1"})
		close(firstDone)
	}()
	<-started

	writeDone := make(chan struct{})
	readDone := make(chan struct{})
	go func() {
		_, _ = bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "read-2"})
		close(readDone)
	}()
	go func() {
		_, _ = bus.Write(context.Background(), token, sdk.ConnectorRequest{Target: "write-1"})
		close(writeDone)
	}()

	deadline := time.After(2 * time.Second)
	for {
		bus.mu.RLock()
		rt := bus.runtimes["resource-priority"]
		bus.mu.RUnlock()
		if rt != nil {
			rt.queueMu.Lock()
			pending := rt.pending
			rt.queueMu.Unlock()
			if pending >= 3 {
				break
			}
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for queued read/write operations")
		case <-time.After(10 * time.Millisecond):
		}
	}

	close(release)
	<-firstDone
	<-writeDone
	<-readDone

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.order) < 3 {
		t.Fatalf("unexpected execution order: %+v", provider.order)
	}
	if provider.order[1] != "write-1" {
		t.Fatalf("expected high-priority write second, got order %+v", provider.order)
	}
}

func TestResourceBusBroadcastsProbeEvents(t *testing.T) {
	provider := &dummyProvider{}
	bus := NewResourceBus(ResourceBusOptions{})
	bus.RegisterProvider("connect-dummy", provider)

	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-events",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.Bind(context.Background(), "resource-events")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	subID, ch, err := bus.Subscribe("resource-events")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer bus.Unsubscribe("resource-events", subID)

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected initial status event")
	}

	err = bus.PublishProbeStatus("resource-events", sdk.ResourceStatusEvent{
		Status:     sdk.ResourceStatusReadonly,
		ReasonCode: "READ_ONLY",
		Message:    "database is read-only",
	})
	if err != nil {
		t.Fatalf("publish probe status: %v", err)
	}

	select {
	case event := <-ch:
		if event.Status != sdk.ResourceStatusReadonly {
			t.Fatalf("expected readonly event, got %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("expected probe event")
	}
}

func TestResourceBusEnforcesBindingQuota(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	provider := &dummyProvider{
		started: started,
		release: release,
	}
	bus := NewResourceBus(ResourceBusOptions{})
	bus.RegisterProvider("connect-dummy", provider)

	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-quota",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.BindWithOptions(context.Background(), "resource-quota", BindingOptions{
		UsageWeight:    1,
		MaxUsageWeight: 1,
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	go func() {
		_, _ = bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "first"})
	}()
	<-started

	_, err = bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "second"})
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded error, got %v", err)
	}
	close(release)
}

func TestResourceBusRunsHealthProbeLoop(t *testing.T) {
	provider := &dummyProvider{
		probeEvent: sdk.ResourceStatusEvent{
			Status:     sdk.ResourceStatusDegraded,
			ReasonCode: "LATENCY_HIGH",
			Message:    "probe latency too high",
		},
	}
	bus := NewResourceBus(ResourceBusOptions{})
	bus.RegisterProvider("connect-dummy", provider)

	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:           "resource-health",
		PluginID:     "connect-dummy",
		Enabled:      true,
		HealthPolicy: []byte(`{"enabled":true,"interval_ms":20,"target":"health"}`),
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.Bind(context.Background(), "resource-health")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-time.After(20 * time.Millisecond):
			provider.mu.Lock()
			probeCalls := provider.probeCalls
			provider.mu.Unlock()
			status, ok := bus.CurrentStatus("resource-health")
			if ok && status.Status == sdk.ResourceStatusDegraded && probeCalls > 0 {
				return
			}
		case <-deadline:
			t.Fatal("expected degraded health event from probe loop")
		}
	}
}

func TestDemandManagedResourceUsesOperationsInsteadOfProbeLoopForHealth(t *testing.T) {
	provider := &dummyProvider{}
	bus := NewResourceBus(ResourceBusOptions{})
	bus.RegisterProvider("connect-dummy", provider)
	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID: "resource-on-demand", PluginID: "connect-dummy", Enabled: true,
		SessionPolicy: sdk.ConnectorSessionPolicyScheduledReuse,
		HealthPolicy:  []byte(`{"enabled":true,"interval_ms":20,"target":"health"}`),
	}); err != nil {
		t.Fatal(err)
	}
	token, err := bus.Bind(context.Background(), "resource-on-demand")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	time.Sleep(80 * time.Millisecond)
	provider.mu.Lock()
	probeCalls := provider.probeCalls
	provider.mu.Unlock()
	if probeCalls != 0 {
		t.Fatalf("on-demand resource probe calls=%d, want 0", probeCalls)
	}

	if _, err := bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "points"}); err != nil {
		t.Fatal(err)
	}
	status, ok := bus.CurrentStatus("resource-on-demand")
	if !ok || status.Status != sdk.ResourceStatusHealthy || status.ReasonCode != "OPERATION_OK" {
		t.Fatalf("unexpected passive health status: %+v", status)
	}
}

func TestOnDemandResourceBecomesDisconnectedAfterRepeatedOperationFailures(t *testing.T) {
	provider := &dummyProvider{executeErr: errors.New("device unavailable")}
	bus := NewResourceBus(ResourceBusOptions{})
	bus.RegisterProvider("connect-s7", provider)
	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID: "s7-legacy", PluginID: "connect-s7", Enabled: true, SessionPolicy: "split_rw",
	}); err != nil {
		t.Fatal(err)
	}
	token, err := bus.Bind(context.Background(), "s7-legacy")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	for attempt := 1; attempt <= 3; attempt++ {
		_, _ = bus.Read(context.Background(), token, sdk.ConnectorRequest{Target: "points"})
		status, _ := bus.CurrentStatus("s7-legacy")
		want := sdk.ResourceStatusDegraded
		if attempt == 3 {
			want = sdk.ResourceStatusDisconnected
		}
		if status.Status != want || status.ReasonCode != "OPERATION_FAILED" {
			t.Fatalf("attempt %d status=%+v, want %s", attempt, status, want)
		}
	}
}

func TestResourceBusResolvesProviderPerResourceAndPublishesInstanceID(t *testing.T) {
	bus := NewResourceBus(ResourceBusOptions{})
	resolveCalls := 0

	bus.SetResourceProviderResolver(func(resource sdk.ConnectorResource) (Provider, error) {
		resolveCalls++
		return &dummyProvider{
			pluginID:   resource.PluginID,
			instanceID: "connect-dummy#1",
		}, nil
	})

	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-resolver",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.Bind(context.Background(), "resource-resolver")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	if resolveCalls != 1 {
		t.Fatalf("expected resource resolver to be called once, got %d", resolveCalls)
	}

	status, ok := bus.CurrentStatus("resource-resolver")
	if !ok {
		t.Fatal("expected resource status")
	}
	if status.ProviderPluginID != "connect-dummy" {
		t.Fatalf("unexpected provider plugin id: %+v", status)
	}
	if status.ProviderInstanceID != "connect-dummy#1" {
		t.Fatalf("unexpected provider instance id: %+v", status)
	}
}

func TestResourceBusEnforcesProviderInflightLimitAcrossResources(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	provider := &dummyProvider{
		started: started,
		release: release,
	}
	bus := NewResourceBus(ResourceBusOptions{})
	bus.RegisterProvider("connect-dummy", provider)

	for _, resourceID := range []string{"resource-provider-a", "resource-provider-b"} {
		if err := bus.RegisterResource(sdk.ConnectorResource{
			ID:       resourceID,
			PluginID: "connect-dummy",
			Enabled:  true,
			ProviderPolicy: sdk.ProviderPolicy{
				MaxInflightPerProvider: 1,
			},
		}); err != nil {
			t.Fatalf("register resource %s: %v", resourceID, err)
		}
	}

	tokenA, err := bus.Bind(context.Background(), "resource-provider-a")
	if err != nil {
		t.Fatalf("bind resource-provider-a: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), tokenA) }()

	tokenB, err := bus.Bind(context.Background(), "resource-provider-b")
	if err != nil {
		t.Fatalf("bind resource-provider-b: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), tokenB) }()

	firstDone := make(chan struct{})
	go func() {
		_, _ = bus.Read(context.Background(), tokenA, sdk.ConnectorRequest{Target: "first"})
		close(firstDone)
	}()
	<-started

	secondDone := make(chan struct{})
	go func() {
		_, _ = bus.Read(context.Background(), tokenB, sdk.ConnectorRequest{Target: "second"})
		close(secondDone)
	}()

	select {
	case <-started:
		t.Fatal("expected second request to wait for provider slot")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected second request to start after provider slot release")
	}

	<-firstDone
	<-secondDone

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.maxObserved > 1 {
		t.Fatalf("expected provider max inflight to stay at 1, got %d", provider.maxObserved)
	}
}

func TestResourceBusForwardsStreamLifecycle(t *testing.T) {
	provider := &dummyProvider{}
	bus := NewResourceBus(ResourceBusOptions{})
	bus.RegisterProvider("connect-dummy", provider)

	if err := bus.RegisterResource(sdk.ConnectorResource{
		ID:       "resource-stream",
		PluginID: "connect-dummy",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	token, err := bus.Bind(context.Background(), "resource-stream")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() { _ = bus.Release(context.Background(), token) }()

	openResp, err := bus.OpenStream(context.Background(), token, sdk.StreamOpenRequest{
		ResourceRef:    "resource-stream",
		Target:         "messages",
		InitialCredits: 4,
	})
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if openResp.StreamID == "" {
		t.Fatalf("unexpected open response: %#v", openResp)
	}
	if provider.streamOpenReq.Target != "messages" || provider.streamOpenReq.InitialCredits != 4 {
		t.Fatalf("unexpected provider open request: %#v", provider.streamOpenReq)
	}

	recvResp, err := bus.ReceiveStream(context.Background(), token, sdk.StreamReceiveRequest{
		StreamID:      openResp.StreamID,
		MaxMessages:   1,
		WaitTimeoutMS: 100,
	})
	if err != nil {
		t.Fatalf("receive stream: %v", err)
	}
	if len(recvResp.Messages) != 1 || recvResp.Messages[0].DeliveryID != "delivery-1" {
		t.Fatalf("unexpected receive response: %#v", recvResp)
	}

	if err := bus.AckStream(context.Background(), token, sdk.StreamAckRequest{
		StreamID:   openResp.StreamID,
		DeliveryID: "delivery-1",
		Credits:    1,
	}); err != nil {
		t.Fatalf("ack stream: %v", err)
	}
	if provider.streamAckReq.DeliveryID != "delivery-1" {
		t.Fatalf("unexpected ack request: %#v", provider.streamAckReq)
	}

	if err := bus.NackStream(context.Background(), token, sdk.StreamNackRequest{
		StreamID:   openResp.StreamID,
		DeliveryID: "delivery-2",
		Reason:     "decode_failed",
	}); err != nil {
		t.Fatalf("nack stream: %v", err)
	}
	if provider.streamNackReq.DeliveryID != "delivery-2" {
		t.Fatalf("unexpected nack request: %#v", provider.streamNackReq)
	}

	if err := bus.GrantCredits(context.Background(), token, sdk.StreamGrantCreditsRequest{
		StreamID: openResp.StreamID,
		Credits:  3,
	}); err != nil {
		t.Fatalf("grant credits: %v", err)
	}
	if provider.streamGrantReq.Credits != 3 {
		t.Fatalf("unexpected grant request: %#v", provider.streamGrantReq)
	}

	if err := bus.CloseStream(context.Background(), token, sdk.StreamCloseRequest{
		StreamID: openResp.StreamID,
		Reason:   "stop",
	}); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if provider.streamCloseReq.StreamID != "stream-1" {
		t.Fatalf("unexpected close request: %#v", provider.streamCloseReq)
	}
}
