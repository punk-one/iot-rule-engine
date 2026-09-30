package instance

import (
	"errors"
	"sync"
	"time"
)

// MemoryRegistry 基于内存的注册表实现
type MemoryRegistry struct {
	mu        sync.RWMutex
	instances map[string]*PluginInstance
	runner    InstanceRunner
}

// NewMemoryRegistry 创建内存注册表
func NewMemoryRegistry(runner InstanceRunner) *MemoryRegistry {
	return &MemoryRegistry{
		instances: make(map[string]*PluginInstance),
		runner:    runner,
	}
}

func (r *MemoryRegistry) Register(inst *PluginInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.instances[inst.ID]; ok {
		return errors.New("instance already exists")
	}

	inst.Status = InstanceCreated
	inst.CreatedAt = time.Now()
	inst.UpdatedAt = inst.CreatedAt

	r.instances[inst.ID] = inst
	return nil
}

func (r *MemoryRegistry) Start(id string) error {
	r.mu.Lock()
	inst, ok := r.instances[id]
	r.mu.Unlock()

	if !ok {
		return errors.New("instance not found")
	}

	// Double check status if needed, but for now just run
	if err := r.runner.Start(inst); err != nil {
		r.mu.Lock()
		inst.Status = InstanceFailed
		inst.UpdatedAt = time.Now()
		r.mu.Unlock()
		return err
	}

	r.mu.Lock()
	inst.Status = InstanceRunning
	inst.UpdatedAt = time.Now()
	r.mu.Unlock()
	return nil
}

func (r *MemoryRegistry) Stop(id string) error {
	r.mu.Lock()
	inst, ok := r.instances[id]
	r.mu.Unlock()

	if !ok {
		return errors.New("instance not found")
	}

	if err := r.runner.Stop(inst); err != nil {
		return err
	}

	r.mu.Lock()
	inst.Status = InstanceStopped
	inst.UpdatedAt = time.Now()
	r.mu.Unlock()
	return nil
}

func (r *MemoryRegistry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Optionally check if stopped?
	delete(r.instances, id)
	return nil
}

func (r *MemoryRegistry) Get(id string) (*PluginInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[id]
	return inst, ok
}

func (r *MemoryRegistry) ListByRule(ruleID string) []*PluginInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*PluginInstance
	for _, inst := range r.instances {
		if inst.RuleID == ruleID {
			result = append(result, inst)
		}
	}
	return result
}

func (r *MemoryRegistry) ListAll() []*PluginInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*PluginInstance
	for _, inst := range r.instances {
		result = append(result, inst)
	}
	return result
}
