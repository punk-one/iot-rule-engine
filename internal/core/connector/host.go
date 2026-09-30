package connector

import (
	"context"
	"fmt"
	"sync"

	coreplugin "github.com/punk-one/iot-rule-engine/internal/core/plugin"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type Host struct {
	loader       *coreplugin.Loader
	mu           sync.Mutex
	providers    map[string][]*pluginBackedProvider
	nextInstance map[string]int
}

func NewHost(loader *coreplugin.Loader) *Host {
	return &Host{
		loader:       loader,
		providers:    make(map[string][]*pluginBackedProvider),
		nextInstance: make(map[string]int),
	}
}

func (h *Host) Resolve(pluginID string) (Provider, error) {
	return h.ResolveForResource(sdk.ConnectorResource{PluginID: pluginID})
}

func (h *Host) ResolveForResource(resource sdk.ConnectorResource) (Provider, error) {
	if resource.PluginID == "" {
		return nil, fmt.Errorf("connector resource plugin_id is required")
	}

	if h.loader == nil {
		return nil, fmt.Errorf("connector host loader is nil")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	policy := normalizeProviderPolicy(resource.ProviderPolicy)
	if err := h.ensurePreferredProvidersLocked(resource.PluginID, policy); err != nil {
		return nil, err
	}

	provider, err := h.selectProviderLocked(resource.PluginID, policy)
	if err != nil {
		return nil, err
	}
	provider.assignedResources++
	return provider, nil
}

func (h *Host) ensurePreferredProvidersLocked(pluginID string, policy sdk.ProviderPolicy) error {
	_ = policy // v1 single-process contract: provider scaling is resource-level, not process-level.
	if len(h.providers[pluginID]) == 0 {
		provider, err := h.spawnProviderLocked(pluginID)
		if err != nil {
			return err
		}
		h.providers[pluginID] = append(h.providers[pluginID], provider)
	}
	return nil
}

func (h *Host) selectProviderLocked(pluginID string, policy sdk.ProviderPolicy) (*pluginBackedProvider, error) {
	_ = policy
	pool := h.providers[pluginID]
	if len(pool) == 0 {
		provider, err := h.spawnProviderLocked(pluginID)
		if err != nil {
			return nil, err
		}
		h.providers[pluginID] = append(h.providers[pluginID], provider)
		return provider, nil
	}

	var selected *pluginBackedProvider
	for _, candidate := range pool {
		if candidate == nil {
			continue
		}
		if selected == nil || candidate.assignedResources < selected.assignedResources {
			selected = candidate
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("connector provider pool %s is empty", pluginID)
	}

	return selected, nil
}

func (h *Host) spawnProviderLocked(pluginID string) (*pluginBackedProvider, error) {
	client, err := h.acquirePluginClientLocked(pluginID)
	if err != nil {
		return nil, err
	}
	if client.Info.Type != sdk.PluginTypeConnector || client.ConnectorRPCClient == nil {
		if client.Client != nil {
			client.Client.Kill()
		}
		return nil, fmt.Errorf("plugin %s is not a connector plugin", pluginID)
	}

	return &pluginBackedProvider{
		host:             h,
		client:           client,
		pluginID:         pluginID,
		instanceID:       h.nextProviderInstanceIDLocked(pluginID),
		handleToResource: make(map[string]string),
		resourcePolicies: make(map[string]sdk.ProviderPolicy),
	}, nil
}

func (h *Host) acquirePluginClientLocked(pluginID string) (*coreplugin.PluginClient, error) {
	if len(h.providers[pluginID]) == 0 {
		client, err := h.loader.GetConnectorPluginClient(pluginID)
		if err == nil && client != nil && client.Client != nil && !client.Client.Exited() {
			return client, nil
		}
	}

	path, err := h.loader.ResolvePluginPath(pluginID)
	if err != nil {
		return nil, err
	}
	return h.loader.SpawnConnectorPlugin(path)
}

func (h *Host) nextProviderInstanceIDLocked(pluginID string) string {
	h.nextInstance[pluginID]++
	return fmt.Sprintf("%s#%d", pluginID, h.nextInstance[pluginID])
}

func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for pluginID, providers := range h.providers {
		for _, provider := range providers {
			if provider == nil || provider.client == nil || provider.client.Client == nil {
				continue
			}
			if provider.client.ConnectorRPCClient != nil {
				_ = provider.client.ConnectorRPCClient.Stop()
			}
			provider.client.Client.Kill()
		}
		delete(h.providers, pluginID)
	}
}

type pluginBackedProvider struct {
	host              *Host
	client            *coreplugin.PluginClient
	pluginID          string
	instanceID        string
	assignedResources int
	mu                sync.Mutex
	cond              *sync.Cond
	inflight          int
	handleToResource  map[string]string
	resourcePolicies  map[string]sdk.ProviderPolicy
}

func (p *pluginBackedProvider) CreateResource(ctx context.Context, resource sdk.ConnectorResource) (string, error) {
	handle, err := p.client.ConnectorRPCClient.CreateResource(resource)
	if err != nil {
		p.releaseAssignment("")
		return "", err
	}
	p.mu.Lock()
	if p.cond == nil {
		p.cond = sync.NewCond(&p.mu)
	}
	p.handleToResource[handle] = resource.ID
	p.resourcePolicies[resource.ID] = resource.ProviderPolicy
	p.mu.Unlock()
	return handle, nil
}

func (p *pluginBackedProvider) DestroyResource(ctx context.Context, providerHandle string) error {
	defer p.releaseAssignment(p.resourceIDByHandle(providerHandle))
	return p.client.ConnectorRPCClient.DestroyResource(providerHandle)
}

func (p *pluginBackedProvider) Execute(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ConnectorResponse, error) {
	return p.client.ConnectorRPCClient.Execute(providerHandle, req)
}

func (p *pluginBackedProvider) OpenStream(ctx context.Context, providerHandle string, req sdk.StreamOpenRequest) (sdk.StreamOpenResponse, error) {
	return p.client.ConnectorRPCClient.OpenStream(providerHandle, req)
}

func (p *pluginBackedProvider) ReceiveStream(ctx context.Context, providerHandle string, req sdk.StreamReceiveRequest) (sdk.StreamReceiveResponse, error) {
	return p.client.ConnectorRPCClient.ReceiveStream(providerHandle, req)
}

func (p *pluginBackedProvider) AckStream(ctx context.Context, providerHandle string, req sdk.StreamAckRequest) error {
	return p.client.ConnectorRPCClient.AckStream(providerHandle, req)
}

func (p *pluginBackedProvider) NackStream(ctx context.Context, providerHandle string, req sdk.StreamNackRequest) error {
	return p.client.ConnectorRPCClient.NackStream(providerHandle, req)
}

func (p *pluginBackedProvider) GrantCredits(ctx context.Context, providerHandle string, req sdk.StreamGrantCreditsRequest) error {
	return p.client.ConnectorRPCClient.GrantCredits(providerHandle, req)
}

func (p *pluginBackedProvider) CloseStream(ctx context.Context, providerHandle string, req sdk.StreamCloseRequest) error {
	return p.client.ConnectorRPCClient.CloseStream(providerHandle, req)
}

func (p *pluginBackedProvider) Probe(ctx context.Context, providerHandle string, req sdk.ConnectorRequest) (sdk.ResourceStatusEvent, error) {
	return p.client.ConnectorRPCClient.Probe(providerHandle, req)
}

func (p *pluginBackedProvider) ProviderPluginID() string {
	return p.pluginID
}

func (p *pluginBackedProvider) ProviderInstanceID() string {
	return p.instanceID
}

func (p *pluginBackedProvider) AcquireProviderSlot(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cond == nil {
		p.cond = sync.NewCond(&p.mu)
	}

	for {
		limit := p.maxInflightLocked()
		if limit <= 0 || p.inflight < limit {
			p.inflight++
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		waitDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				p.mu.Lock()
				if p.cond != nil {
					p.cond.Broadcast()
				}
				p.mu.Unlock()
			case <-waitDone:
			}
		}()
		p.cond.Wait()
		close(waitDone)
	}
}

func (p *pluginBackedProvider) ReleaseProviderSlot() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inflight > 0 {
		p.inflight--
	}
	if p.cond != nil {
		p.cond.Broadcast()
	}
}

func (p *pluginBackedProvider) releaseAssignment(resourceID string) {
	if p == nil || p.host == nil {
		return
	}

	p.mu.Lock()
	if resourceID != "" {
		delete(p.resourcePolicies, resourceID)
		for handle, assignedResourceID := range p.handleToResource {
			if assignedResourceID == resourceID {
				delete(p.handleToResource, handle)
			}
		}
	}
	if p.cond != nil {
		p.cond.Broadcast()
	}
	p.mu.Unlock()

	p.host.mu.Lock()
	defer p.host.mu.Unlock()
	if p.assignedResources > 0 {
		p.assignedResources--
	}
}

func (p *pluginBackedProvider) resourceIDByHandle(handle string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.handleToResource[handle]
}

func (p *pluginBackedProvider) maxInflightLocked() int {
	limit := 0
	for _, policy := range p.resourcePolicies {
		if policy.MaxInflightPerProvider <= 0 {
			continue
		}
		if limit == 0 || policy.MaxInflightPerProvider < limit {
			limit = policy.MaxInflightPerProvider
		}
	}
	return limit
}

func normalizeProviderPolicy(policy sdk.ProviderPolicy) sdk.ProviderPolicy {
	if policy.PreferredProviderCount <= 0 {
		policy.PreferredProviderCount = 1
	}
	if policy.MaxResourcesPerProvider < 0 {
		policy.MaxResourcesPerProvider = 0
	}
	if policy.MaxInflightPerProvider < 0 {
		policy.MaxInflightPerProvider = 0
	}
	return policy
}
