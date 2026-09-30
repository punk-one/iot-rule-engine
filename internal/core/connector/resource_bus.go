package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

var (
	ErrProviderNotFound = errors.New("connector provider not found")
	ErrResourceNotFound = errors.New("connector resource not found")
	ErrBindingNotFound  = errors.New("connector binding not found")
	ErrQuotaExceeded    = errors.New("connector quota exceeded")
)

type Provider interface {
	CreateResource(ctx context.Context, resource sdk.ConnectorResource) (string, error)
	DestroyResource(ctx context.Context, providerHandle string) error
	Execute(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error)
	OpenStream(ctx context.Context, providerHandle string, req sdk.StreamOpenRequest) (sdk.StreamOpenResponse, error)
	ReceiveStream(ctx context.Context, providerHandle string, req sdk.StreamReceiveRequest) (sdk.StreamReceiveResponse, error)
	AckStream(ctx context.Context, providerHandle string, req sdk.StreamAckRequest) error
	NackStream(ctx context.Context, providerHandle string, req sdk.StreamNackRequest) error
	GrantCredits(ctx context.Context, providerHandle string, req sdk.StreamGrantCreditsRequest) error
	CloseStream(ctx context.Context, providerHandle string, req sdk.StreamCloseRequest) error
	Probe(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ResourceStatusEvent, error)
}

type providerDescriptor interface {
	ProviderPluginID() string
	ProviderInstanceID() string
}

type providerInflightLimiter interface {
	AcquireProviderSlot(ctx context.Context) error
	ReleaseProviderSlot()
}

type ResourceBusOptions struct {
	CongestionThreshold  int
	CongestedRetryAfter  time.Duration
	MaxHighPriorityBurst int
	SubscriptionBuffer   int
}

type BindingOptions struct {
	DefaultPriority string
	UsageWeight     int
	MaxUsageWeight  int
}

type ResourceBus struct {
	mu                   sync.RWMutex
	providers            map[string]Provider
	resources            map[string]sdk.ConnectorResource
	runtimes             map[string]*resourceRuntime
	bindings             map[string]*binding
	streams              map[string]*streamBinding
	subscribers          map[string]map[string]chan sdk.ResourceStatusEvent
	providerResolver     func(pluginID string) (Provider, error)
	resourceResolver     func(resource sdk.ConnectorResource) (Provider, error)
	congestionThreshold  int
	congestedRetryAfter  time.Duration
	maxHighPriorityBurst int
	subscriptionBuffer   int
}

type ResourceSnapshot struct {
	Resource       sdk.ConnectorResource
	Status         sdk.ResourceStatusEvent
	RefCount       int
	Pending        int
	InflightWeight int
}

type resourceRuntime struct {
	resource       sdk.ConnectorResource
	provider       Provider
	providerPlugin string
	providerID     string
	providerHandle string
	refCount       int

	queueMu        sync.Mutex
	cond           *sync.Cond
	highQueue      []*requestOp
	normalQueue    []*requestOp
	pending        int
	inflightWeight int
	highServed     int
	closed         bool

	status              sdk.ResourceStatusEvent
	probeStatus         sdk.ResourceStatusEvent
	consecutiveFailures int
	quota               quotaPolicy
	health              healthPolicy
	stopCh              chan struct{}
}

type binding struct {
	token           string
	resourceID      string
	defaultPriority string
	usageWeight     int
	maxUsageWeight  int
	pendingWeight   int
}

type streamBinding struct {
	id               string
	token            string
	resourceID       string
	provider         Provider
	providerHandle   string
	providerStreamID string
}

type requestOp struct {
	ctx        context.Context
	req        sdk.ConnectorRequest
	binding    *binding
	weight     int
	high       bool
	congested  bool
	enqueuedAt time.Time
	resultCh   chan requestResult
}

type requestResult struct {
	resp sdk.ConnectorResponse
	err  error
}

type quotaPolicy struct {
	MaxInflightRequests int `json:"max_inflight_requests,omitempty"`
	MaxInflightWeight   int `json:"max_inflight_weight,omitempty"`
	DefaultUsageWeight  int `json:"default_usage_weight,omitempty"`
}

type healthPolicy struct {
	Enabled    bool   `json:"enabled,omitempty"`
	IntervalMS int    `json:"interval_ms,omitempty"`
	TimeoutMS  int    `json:"timeout_ms,omitempty"`
	Target     string `json:"target,omitempty"`
	Priority   string `json:"priority,omitempty"`
}

func NewResourceBus(opts ResourceBusOptions) *ResourceBus {
	threshold := opts.CongestionThreshold
	if threshold <= 0 {
		threshold = 64
	}
	retryAfter := opts.CongestedRetryAfter
	if retryAfter <= 0 {
		retryAfter = 500 * time.Millisecond
	}
	maxBurst := opts.MaxHighPriorityBurst
	if maxBurst <= 0 {
		maxBurst = 3
	}
	subBuffer := opts.SubscriptionBuffer
	if subBuffer <= 0 {
		subBuffer = 8
	}
	return &ResourceBus{
		providers:            make(map[string]Provider),
		resources:            make(map[string]sdk.ConnectorResource),
		runtimes:             make(map[string]*resourceRuntime),
		bindings:             make(map[string]*binding),
		streams:              make(map[string]*streamBinding),
		subscribers:          make(map[string]map[string]chan sdk.ResourceStatusEvent),
		congestionThreshold:  threshold,
		congestedRetryAfter:  retryAfter,
		maxHighPriorityBurst: maxBurst,
		subscriptionBuffer:   subBuffer,
	}
}

func (b *ResourceBus) RegisterProvider(pluginID string, provider Provider) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.providers[pluginID] = provider
}

func (b *ResourceBus) SetProviderResolver(resolver func(pluginID string) (Provider, error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.providerResolver = resolver
}

func (b *ResourceBus) SetResourceProviderResolver(resolver func(resource sdk.ConnectorResource) (Provider, error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resourceResolver = resolver
}

func (b *ResourceBus) RegisterResource(resource sdk.ConnectorResource) error {
	if resource.ID == "" {
		return fmt.Errorf("connector resource id is required")
	}
	if resource.PluginID == "" {
		return fmt.Errorf("connector resource plugin_id is required")
	}

	b.mu.Lock()
	b.resources[resource.ID] = resource
	rt := b.runtimes[resource.ID]
	b.mu.Unlock()

	if rt == nil {
		return nil
	}

	rt.queueMu.Lock()
	rt.resource = resource
	rt.quota = parseQuotaPolicy(resource.QuotaPolicy)
	rt.health = parseHealthPolicy(resource.HealthPolicy)
	event := rt.recomputeStatusLocked(b.congestionThreshold)
	rt.queueMu.Unlock()

	b.publishStatus(event)
	return nil
}

func (b *ResourceBus) UnregisterResource(ctx context.Context, resourceID string) error {
	b.mu.Lock()
	rt := b.runtimes[resourceID]
	if rt != nil && rt.refCount > 0 {
		b.mu.Unlock()
		return fmt.Errorf("connector resource %s is still in use", resourceID)
	}
	delete(b.resources, resourceID)
	delete(b.runtimes, resourceID)
	delete(b.subscribers, resourceID)
	b.mu.Unlock()

	if rt != nil {
		rt.close()
		if rt.provider != nil && rt.providerHandle != "" {
			return rt.provider.DestroyResource(ctx, rt.providerHandle)
		}
	}
	return nil
}

func (b *ResourceBus) Warmup(ctx context.Context, resourceID string) error {
	_, err := b.ensureRuntime(ctx, resourceID)
	return err
}

func (b *ResourceBus) Bind(ctx context.Context, resourceID string) (string, error) {
	return b.BindWithOptions(ctx, resourceID, BindingOptions{})
}

func (b *ResourceBus) BindWithOptions(ctx context.Context, resourceID string, opts BindingOptions) (string, error) {
	rt, err := b.ensureRuntime(ctx, resourceID)
	if err != nil {
		return "", err
	}

	if opts.UsageWeight <= 0 {
		opts.UsageWeight = rt.quota.DefaultUsageWeight
	}
	if opts.UsageWeight <= 0 {
		opts.UsageWeight = 1
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	rt.refCount++
	token := uuid.NewString()
	b.bindings[token] = &binding{
		token:           token,
		resourceID:      resourceID,
		defaultPriority: opts.DefaultPriority,
		usageWeight:     opts.UsageWeight,
		maxUsageWeight:  opts.MaxUsageWeight,
	}
	return token, nil
}

func (b *ResourceBus) Release(ctx context.Context, token string) error {
	b.mu.Lock()
	binding, ok := b.bindings[token]
	if !ok {
		b.mu.Unlock()
		return ErrBindingNotFound
	}
	delete(b.bindings, token)
	streamsToClose := make([]*streamBinding, 0)
	for streamID, stream := range b.streams {
		if stream != nil && stream.token == token {
			streamsToClose = append(streamsToClose, stream)
			delete(b.streams, streamID)
		}
	}

	rt, ok := b.runtimes[binding.resourceID]
	if !ok {
		b.mu.Unlock()
		return nil
	}
	rt.refCount--
	if rt.refCount > 0 {
		b.mu.Unlock()
		return nil
	}

	delete(b.runtimes, binding.resourceID)
	provider := rt.provider
	handle := rt.providerHandle
	b.mu.Unlock()

	for _, stream := range streamsToClose {
		_ = stream.provider.CloseStream(ctx, stream.providerHandle, sdk.StreamCloseRequest{
			StreamID: stream.providerStreamID,
			Reason:   "binding_released",
		})
	}

	rt.close()
	if provider == nil || handle == "" {
		return nil
	}
	return provider.DestroyResource(ctx, handle)
}

func (b *ResourceBus) Subscribe(resourceID string) (string, <-chan sdk.ResourceStatusEvent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.resources[resourceID]; !ok {
		return "", nil, ErrResourceNotFound
	}

	subID := uuid.NewString()
	ch := make(chan sdk.ResourceStatusEvent, b.subscriptionBuffer)
	if _, ok := b.subscribers[resourceID]; !ok {
		b.subscribers[resourceID] = make(map[string]chan sdk.ResourceStatusEvent)
	}
	b.subscribers[resourceID][subID] = ch

	if rt, ok := b.runtimes[resourceID]; ok {
		rt.queueMu.Lock()
		current := rt.status
		rt.queueMu.Unlock()
		select {
		case ch <- current:
		default:
		}
	} else {
		select {
		case ch <- sdk.ResourceStatusEvent{
			ResourceID:       resourceID,
			ProviderPluginID: b.resources[resourceID].PluginID,
			Status:           sdk.ResourceStatusUnknown,
		}:
		default:
		}
	}

	return subID, ch, nil
}

func (b *ResourceBus) Unsubscribe(resourceID, subscriptionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	resourceSubs, ok := b.subscribers[resourceID]
	if !ok {
		return
	}
	ch, ok := resourceSubs[subscriptionID]
	if !ok {
		return
	}
	delete(resourceSubs, subscriptionID)
	if len(resourceSubs) == 0 {
		delete(b.subscribers, resourceID)
	}
	_ = ch
}

func (b *ResourceBus) Read(ctx context.Context, token string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	req.Kind = sdk.ConnectorRequestRead
	return b.execute(ctx, token, req)
}

func (b *ResourceBus) Write(ctx context.Context, token string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	req.Kind = sdk.ConnectorRequestWrite
	return b.execute(ctx, token, req)
}

func (b *ResourceBus) Execute(ctx context.Context, token string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return b.execute(ctx, token, req)
}

func (b *ResourceBus) OpenStream(ctx context.Context, token string, req sdk.StreamOpenRequest) (sdk.StreamOpenResponse, error) {
	b.mu.RLock()
	binding, ok := b.bindings[token]
	if !ok {
		b.mu.RUnlock()
		return sdk.StreamOpenResponse{}, ErrBindingNotFound
	}
	rt, ok := b.runtimes[binding.resourceID]
	b.mu.RUnlock()
	if !ok {
		return sdk.StreamOpenResponse{}, ErrResourceNotFound
	}

	resp, err := rt.provider.OpenStream(ctx, rt.providerHandle, req)
	if err != nil {
		return sdk.StreamOpenResponse{}, err
	}
	streamID := uuid.NewString()
	b.mu.Lock()
	b.streams[streamID] = &streamBinding{
		id:               streamID,
		token:            token,
		resourceID:       binding.resourceID,
		provider:         rt.provider,
		providerHandle:   rt.providerHandle,
		providerStreamID: resp.StreamID,
	}
	b.mu.Unlock()
	resp.StreamID = streamID
	return resp, nil
}

func (b *ResourceBus) ReceiveStream(ctx context.Context, token string, req sdk.StreamReceiveRequest) (sdk.StreamReceiveResponse, error) {
	stream, err := b.getStream(token, req.StreamID)
	if err != nil {
		return sdk.StreamReceiveResponse{}, err
	}
	req.StreamID = stream.providerStreamID
	return stream.provider.ReceiveStream(ctx, stream.providerHandle, req)
}

func (b *ResourceBus) AckStream(ctx context.Context, token string, req sdk.StreamAckRequest) error {
	stream, err := b.getStream(token, req.StreamID)
	if err != nil {
		return err
	}
	req.StreamID = stream.providerStreamID
	return stream.provider.AckStream(ctx, stream.providerHandle, req)
}

func (b *ResourceBus) NackStream(ctx context.Context, token string, req sdk.StreamNackRequest) error {
	stream, err := b.getStream(token, req.StreamID)
	if err != nil {
		return err
	}
	req.StreamID = stream.providerStreamID
	return stream.provider.NackStream(ctx, stream.providerHandle, req)
}

func (b *ResourceBus) GrantCredits(ctx context.Context, token string, req sdk.StreamGrantCreditsRequest) error {
	stream, err := b.getStream(token, req.StreamID)
	if err != nil {
		return err
	}
	req.StreamID = stream.providerStreamID
	return stream.provider.GrantCredits(ctx, stream.providerHandle, req)
}

func (b *ResourceBus) CloseStream(ctx context.Context, token string, req sdk.StreamCloseRequest) error {
	stream, err := b.getStream(token, req.StreamID)
	if err != nil {
		return err
	}
	req.StreamID = stream.providerStreamID
	if err := stream.provider.CloseStream(ctx, stream.providerHandle, req); err != nil {
		return err
	}
	b.mu.Lock()
	delete(b.streams, stream.id)
	b.mu.Unlock()
	return nil
}

func (b *ResourceBus) CurrentStatus(resourceID string) (sdk.ResourceStatusEvent, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if rt, ok := b.runtimes[resourceID]; ok {
		rt.queueMu.Lock()
		defer rt.queueMu.Unlock()
		return rt.status, true
	}
	resource, ok := b.resources[resourceID]
	if !ok {
		return sdk.ResourceStatusEvent{}, false
	}
	return sdk.ResourceStatusEvent{
		ResourceID:       resource.ID,
		ProviderPluginID: resource.PluginID,
		Status:           sdk.ResourceStatusUnknown,
	}, true
}

func (b *ResourceBus) getStream(token, streamID string) (*streamBinding, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	stream, ok := b.streams[streamID]
	if !ok {
		return nil, ErrBindingNotFound
	}
	if stream.token != token {
		return nil, ErrBindingNotFound
	}
	return stream, nil
}

func (b *ResourceBus) PublishProbeStatus(resourceID string, event sdk.ResourceStatusEvent) error {
	b.mu.RLock()
	rt := b.runtimes[resourceID]
	b.mu.RUnlock()
	if rt == nil {
		return ErrResourceNotFound
	}

	rt.queueMu.Lock()
	rt.probeStatus = normalizeProbeEvent(event, rt.resource)
	updated := rt.recomputeStatusLocked(b.congestionThreshold)
	rt.queueMu.Unlock()
	b.publishStatus(updated)
	return nil
}

func (b *ResourceBus) Snapshot(resourceID string) (ResourceSnapshot, bool) {
	b.mu.RLock()
	resource, resourceExists := b.resources[resourceID]
	rt := b.runtimes[resourceID]
	b.mu.RUnlock()

	if rt == nil {
		if !resourceExists {
			return ResourceSnapshot{}, false
		}
		return ResourceSnapshot{
			Resource: resource,
			Status: sdk.ResourceStatusEvent{
				ResourceID:       resource.ID,
				ProviderPluginID: resource.PluginID,
				Status:           sdk.ResourceStatusUnknown,
			},
		}, true
	}

	rt.queueMu.Lock()
	defer rt.queueMu.Unlock()
	return ResourceSnapshot{
		Resource:       rt.resource,
		Status:         rt.status,
		RefCount:       rt.refCount,
		Pending:        rt.pending,
		InflightWeight: rt.inflightWeight,
	}, true
}

func (b *ResourceBus) ListSnapshots() []ResourceSnapshot {
	b.mu.RLock()
	ids := make([]string, 0, len(b.resources))
	for resourceID := range b.resources {
		ids = append(ids, resourceID)
	}
	b.mu.RUnlock()

	snapshots := make([]ResourceSnapshot, 0, len(ids))
	for _, resourceID := range ids {
		if snapshot, ok := b.Snapshot(resourceID); ok {
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots
}

func (b *ResourceBus) execute(ctx context.Context, token string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	b.mu.RLock()
	binding, ok := b.bindings[token]
	if !ok {
		b.mu.RUnlock()
		return sdk.ConnectorResponse{}, ErrBindingNotFound
	}
	rt, ok := b.runtimes[binding.resourceID]
	b.mu.RUnlock()
	if !ok {
		return sdk.ConnectorResponse{}, ErrResourceNotFound
	}

	weight := req.UsageWeight
	if weight <= 0 {
		weight = binding.usageWeight
	}
	if weight <= 0 {
		weight = 1
	}
	req.ResourceToken = token
	req.UsageWeight = weight

	op := &requestOp{
		ctx:        ctx,
		req:        req,
		binding:    binding,
		weight:     weight,
		high:       isHighPriority(req),
		enqueuedAt: time.Now(),
		resultCh:   make(chan requestResult, 1),
	}

	rt.queueMu.Lock()
	if err := b.checkQuotaLocked(rt, binding, weight); err != nil {
		rt.queueMu.Unlock()
		return sdk.ConnectorResponse{
			Congested:    true,
			RetryAfterMS: int(b.congestedRetryAfter / time.Millisecond),
		}, err
	}
	rt.pending++
	rt.inflightWeight += weight
	binding.pendingWeight += weight
	if op.high {
		rt.highQueue = append(rt.highQueue, op)
	} else {
		rt.normalQueue = append(rt.normalQueue, op)
	}
	op.congested = b.congestionThreshold > 0 && rt.pending >= b.congestionThreshold
	event := rt.recomputeStatusLocked(b.congestionThreshold)
	rt.cond.Signal()
	rt.queueMu.Unlock()
	b.publishStatus(event)

	select {
	case result := <-op.resultCh:
		return result.resp, result.err
	case <-ctx.Done():
		return sdk.ConnectorResponse{}, ctx.Err()
	}
}

func (b *ResourceBus) checkQuotaLocked(rt *resourceRuntime, binding *binding, weight int) error {
	if binding.maxUsageWeight > 0 && binding.pendingWeight+weight > binding.maxUsageWeight {
		return fmt.Errorf("%w: binding weight limit exceeded", ErrQuotaExceeded)
	}
	if rt.quota.MaxInflightRequests > 0 && rt.pending+1 > rt.quota.MaxInflightRequests {
		return fmt.Errorf("%w: max inflight requests exceeded", ErrQuotaExceeded)
	}
	if rt.quota.MaxInflightWeight > 0 && rt.inflightWeight+weight > rt.quota.MaxInflightWeight {
		return fmt.Errorf("%w: max inflight weight exceeded", ErrQuotaExceeded)
	}
	return nil
}

func (b *ResourceBus) ensureRuntime(ctx context.Context, resourceID string) (*resourceRuntime, error) {
	b.mu.RLock()
	if rt, ok := b.runtimes[resourceID]; ok {
		b.mu.RUnlock()
		return rt, nil
	}
	resource, ok := b.resources[resourceID]
	provider := b.providers[resource.PluginID]
	resolver := b.providerResolver
	resourceResolver := b.resourceResolver
	b.mu.RUnlock()

	if !ok {
		return nil, ErrResourceNotFound
	}
	if provider == nil {
		switch {
		case resourceResolver != nil:
			resolvedProvider, err := resourceResolver(resource)
			if err != nil {
				return nil, err
			}
			provider = resolvedProvider
		case resolver != nil:
			resolvedProvider, err := resolver(resource.PluginID)
			if err != nil {
				return nil, err
			}
			provider = resolvedProvider
		default:
			return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, resource.PluginID)
		}
	}

	handle, err := provider.CreateResource(ctx, resource)
	if err != nil {
		return nil, err
	}

	rt := newResourceRuntime(resource, provider, handle)

	b.mu.Lock()
	if existing, ok := b.runtimes[resourceID]; ok {
		b.mu.Unlock()
		rt.close()
		if provider != nil && handle != "" {
			_ = provider.DestroyResource(ctx, handle)
		}
		return existing, nil
	}
	b.runtimes[resourceID] = rt
	b.mu.Unlock()

	// Publish the immutable initial state before any worker can update it. This
	// ordering also avoids a race between a fast health probe and initial status.
	b.publishStatus(rt.status)
	go b.runResource(rt)
	if rt.health.Enabled {
		go b.runProbeLoop(rt)
	}
	return rt, nil
}

func newResourceRuntime(resource sdk.ConnectorResource, provider Provider, handle string) *resourceRuntime {
	providerPluginID, providerInstanceID := providerIdentity(provider, resource)
	reasonCode := "READY"
	message := "resource ready"
	if isOnDemandResource(resource) {
		reasonCode = "DEMAND_MANAGED"
		message = "demand-managed resource ready"
	}
	rt := &resourceRuntime{
		resource:       resource,
		provider:       provider,
		providerPlugin: providerPluginID,
		providerID:     providerInstanceID,
		providerHandle: handle,
		stopCh:         make(chan struct{}),
		quota:          parseQuotaPolicy(resource.QuotaPolicy),
		health:         parseHealthPolicy(resource.HealthPolicy),
		probeStatus: sdk.ResourceStatusEvent{
			ResourceID:         resource.ID,
			ProviderPluginID:   providerPluginID,
			ProviderInstanceID: providerInstanceID,
			Status:             sdk.ResourceStatusHealthy,
			ReasonCode:         reasonCode,
			Message:            message,
			ObservedAtUnixNano: time.Now().UnixNano(),
		},
	}
	rt.cond = sync.NewCond(&rt.queueMu)
	rt.status = rt.probeStatus
	return rt
}

func (b *ResourceBus) runResource(rt *resourceRuntime) {
	for {
		rt.queueMu.Lock()
		for !rt.closed && len(rt.highQueue) == 0 && len(rt.normalQueue) == 0 {
			rt.cond.Wait()
		}
		if rt.closed {
			rt.queueMu.Unlock()
			return
		}
		op := b.nextOperationLocked(rt)
		rt.queueMu.Unlock()

		var (
			resp         sdk.ConnectorResponse
			err          error
			acquiredSlot bool
		)
		if limiter, ok := rt.provider.(providerInflightLimiter); ok {
			if err = limiter.AcquireProviderSlot(op.ctx); err != nil {
				resp = sdk.ConnectorResponse{}
			} else {
				acquiredSlot = true
			}
		}
		if err == nil {
			resp, err = rt.provider.Execute(op.ctx, rt.providerHandle, op.req)
		}
		if acquiredSlot {
			rt.provider.(providerInflightLimiter).ReleaseProviderSlot()
		}

		rt.queueMu.Lock()
		rt.pending--
		rt.inflightWeight -= op.weight
		op.binding.pendingWeight -= op.weight
		if rt.inflightWeight < 0 {
			rt.inflightWeight = 0
		}
		if op.binding.pendingWeight < 0 {
			op.binding.pendingWeight = 0
		}
		if op.congested {
			resp.Congested = true
			resp.RetryAfterMS = int(b.congestedRetryAfter / time.Millisecond)
		}
		if isOnDemandResource(rt.resource) && isConnectorDataOperation(op.req.Kind) {
			rt.recordOnDemandOperationLocked(op.req.Kind, err)
		}
		event := rt.recomputeStatusLocked(b.congestionThreshold)
		rt.queueMu.Unlock()

		b.publishStatus(event)
		op.resultCh <- requestResult{resp: resp, err: err}
	}
}

func (b *ResourceBus) nextOperationLocked(rt *resourceRuntime) *requestOp {
	if len(rt.highQueue) > 0 && (len(rt.normalQueue) == 0 || rt.highServed < b.maxHighPriorityBurst) {
		op := rt.highQueue[0]
		rt.highQueue = rt.highQueue[1:]
		rt.highServed++
		return op
	}
	if len(rt.normalQueue) > 0 {
		op := rt.normalQueue[0]
		rt.normalQueue = rt.normalQueue[1:]
		rt.highServed = 0
		return op
	}
	op := rt.highQueue[0]
	rt.highQueue = rt.highQueue[1:]
	rt.highServed = 1
	return op
}

func (b *ResourceBus) runProbeLoop(rt *resourceRuntime) {
	interval := time.Duration(rt.health.IntervalMS) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-rt.stopCh:
			return
		case <-ticker.C:
			rt.queueMu.Lock()
			health := rt.health
			resource := rt.resource
			rt.queueMu.Unlock()
			if !health.Enabled || isOnDemandResource(resource) {
				continue
			}
			req := sdk.ConnectorRequest{
				Kind:      sdk.ConnectorRequestMetadata,
				Target:    health.Target,
				Priority:  health.Priority,
				TimeoutMS: health.TimeoutMS,
			}
			ctx := context.Background()
			var cancel context.CancelFunc
			if req.TimeoutMS > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond)
			}
			event, err := rt.provider.Probe(ctx, rt.providerHandle, req)
			if cancel != nil {
				cancel()
			}
			if err != nil {
				event = sdk.ResourceStatusEvent{
					ResourceID:         rt.resource.ID,
					ProviderPluginID:   rt.providerPlugin,
					ProviderInstanceID: rt.providerID,
					Status:             sdk.ResourceStatusDisconnected,
					ReasonCode:         "PROBE_FAILED",
					Message:            err.Error(),
					ObservedAtUnixNano: time.Now().UnixNano(),
				}
			}
			rt.queueMu.Lock()
			rt.probeStatus = normalizeProbeEvent(event, rt.resource)
			updated := rt.recomputeStatusLocked(b.congestionThreshold)
			rt.queueMu.Unlock()
			b.publishStatus(updated)
		}
	}
}

func (rt *resourceRuntime) recordOnDemandOperationLocked(kind sdk.ConnectorRequestKind, operationErr error) {
	details := map[string]string{"request_kind": string(kind)}
	if operationErr == nil {
		rt.consecutiveFailures = 0
		rt.probeStatus = sdk.ResourceStatusEvent{
			ResourceID:         rt.resource.ID,
			ProviderPluginID:   rt.providerPlugin,
			ProviderInstanceID: rt.providerID,
			Status:             sdk.ResourceStatusHealthy,
			ReasonCode:         "OPERATION_OK",
			Message:            "last on-demand operation succeeded; transport idle",
			Details:            details,
			ObservedAtUnixNano: time.Now().UnixNano(),
		}
		return
	}

	rt.consecutiveFailures++
	details["consecutive_failures"] = fmt.Sprintf("%d", rt.consecutiveFailures)
	status := sdk.ResourceStatusDegraded
	if rt.consecutiveFailures >= 3 {
		status = sdk.ResourceStatusDisconnected
	}
	rt.probeStatus = sdk.ResourceStatusEvent{
		ResourceID:         rt.resource.ID,
		ProviderPluginID:   rt.providerPlugin,
		ProviderInstanceID: rt.providerID,
		Status:             status,
		ReasonCode:         "OPERATION_FAILED",
		Message:            operationErr.Error(),
		Details:            details,
		ObservedAtUnixNano: time.Now().UnixNano(),
	}
}

func isOnDemandResource(resource sdk.ConnectorResource) bool {
	policy := strings.TrimSpace(resource.SessionPolicy)
	if strings.EqualFold(policy, sdk.ConnectorSessionPolicyPerOperation) ||
		strings.EqualFold(policy, sdk.ConnectorSessionPolicyScheduledReuse) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(resource.PluginID)) {
	case "connect-s7", "connect-fanuc":
		// Preserve the new lifecycle for legacy resource rows whose session_policy
		// is still shared_duplex or split_rw. Persisted data is not rewritten here.
		return true
	default:
		return false
	}
}

func isConnectorDataOperation(kind sdk.ConnectorRequestKind) bool {
	switch kind {
	case sdk.ConnectorRequestRead, sdk.ConnectorRequestBatchRead,
		sdk.ConnectorRequestWrite, sdk.ConnectorRequestBatchWrite:
		return true
	default:
		return false
	}
}

func (b *ResourceBus) publishStatus(event sdk.ResourceStatusEvent) {
	if event.ResourceID == "" {
		return
	}

	b.mu.RLock()
	resourceSubs := b.subscribers[event.ResourceID]
	chs := make([]chan sdk.ResourceStatusEvent, 0, len(resourceSubs))
	for _, ch := range resourceSubs {
		chs = append(chs, ch)
	}
	b.mu.RUnlock()

	for _, ch := range chs {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}

func (rt *resourceRuntime) recomputeStatusLocked(congestionThreshold int) sdk.ResourceStatusEvent {
	next := rt.probeStatus
	if next.ResourceID == "" {
		next = sdk.ResourceStatusEvent{
			ResourceID:         rt.resource.ID,
			ProviderPluginID:   rt.providerPlugin,
			ProviderInstanceID: rt.providerID,
			Status:             sdk.ResourceStatusHealthy,
			ReasonCode:         "READY",
			Message:            "resource ready",
		}
	}
	if next.ProviderPluginID == "" {
		next.ProviderPluginID = rt.providerPlugin
	}
	if next.ProviderInstanceID == "" {
		next.ProviderInstanceID = rt.providerID
	}
	if congestionThreshold > 0 && rt.pending >= congestionThreshold {
		next = sdk.ResourceStatusEvent{
			ResourceID:         rt.resource.ID,
			ProviderPluginID:   rt.providerPlugin,
			ProviderInstanceID: rt.providerID,
			Status:             sdk.ResourceStatusCongested,
			ReasonCode:         "QUEUE_CONGESTED",
			Message:            "resource queue is congested",
			ObservedAtUnixNano: time.Now().UnixNano(),
		}
	}
	next.ObservedAtUnixNano = time.Now().UnixNano()
	rt.status = next
	return rt.status
}

func (rt *resourceRuntime) close() {
	rt.queueMu.Lock()
	defer rt.queueMu.Unlock()
	if rt.closed {
		return
	}
	rt.closed = true
	close(rt.stopCh)
	rt.cond.Broadcast()
}

func parseQuotaPolicy(raw json.RawMessage) quotaPolicy {
	policy := quotaPolicy{
		DefaultUsageWeight: 1,
	}
	if len(raw) == 0 {
		return policy
	}
	_ = json.Unmarshal(raw, &policy)
	if policy.DefaultUsageWeight <= 0 {
		policy.DefaultUsageWeight = 1
	}
	return policy
}

func parseHealthPolicy(raw json.RawMessage) healthPolicy {
	policy := healthPolicy{
		Target:   "health",
		Priority: "high",
	}
	if len(raw) == 0 {
		return policy
	}
	_ = json.Unmarshal(raw, &policy)
	if policy.Target == "" {
		policy.Target = "health"
	}
	if policy.Priority == "" {
		policy.Priority = "high"
	}
	if !policy.Enabled && policy.IntervalMS == 0 && policy.TimeoutMS == 0 && policy.Target == "health" {
		return policy
	}
	policy.Enabled = true
	return policy
}

func normalizeProbeEvent(event sdk.ResourceStatusEvent, resource sdk.ConnectorResource) sdk.ResourceStatusEvent {
	if event.ResourceID == "" {
		event.ResourceID = resource.ID
	}
	if event.ProviderPluginID == "" {
		event.ProviderPluginID = resource.PluginID
	}
	if event.Status == "" {
		event.Status = sdk.ResourceStatusHealthy
	}
	if event.ObservedAtUnixNano == 0 {
		event.ObservedAtUnixNano = time.Now().UnixNano()
	}
	return event
}

func providerIdentity(provider Provider, resource sdk.ConnectorResource) (string, string) {
	pluginID := resource.PluginID
	instanceID := ""
	if described, ok := provider.(providerDescriptor); ok {
		if described.ProviderPluginID() != "" {
			pluginID = described.ProviderPluginID()
		}
		if described.ProviderInstanceID() != "" {
			instanceID = described.ProviderInstanceID()
		}
	}
	return pluginID, instanceID
}

func isHighPriority(req sdk.ConnectorRequest) bool {
	if req.Priority == "high" {
		return true
	}
	switch req.Kind {
	case sdk.ConnectorRequestWrite, sdk.ConnectorRequestBatchWrite:
		return true
	default:
		return false
	}
}
