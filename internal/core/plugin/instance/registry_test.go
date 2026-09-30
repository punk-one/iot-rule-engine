package instance

import (
	"errors"
	"testing"
)

type MockRunner struct {
	started map[string]bool
	fail    bool
}

func (m *MockRunner) Start(inst *PluginInstance) error {
	if m.fail {
		return errors.New("start failed")
	}
	if m.started == nil {
		m.started = make(map[string]bool)
	}
	m.started[inst.ID] = true
	return nil
}

func (m *MockRunner) Stop(inst *PluginInstance) error {
	if m.started == nil {
		return nil
	}
	delete(m.started, inst.ID)
	return nil
}

func TestMemoryRegistry(t *testing.T) {
	runner := &MockRunner{}
	registry := NewMemoryRegistry(runner)

	inst := &PluginInstance{
		ID:     "test-id",
		Plugin: "test-plugin",
		RuleID: "rule-1",
		NodeID: "node-1",
	}

	// 1. Register
	if err := registry.Register(inst); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if inst.Status != InstanceCreated {
		t.Errorf("expected status Created, got %s", inst.Status)
	}

	// 2. Start
	if err := registry.Start("test-id"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if inst.Status != InstanceRunning {
		t.Errorf("expected status Running, got %s", inst.Status)
	}
	if !runner.started["test-id"] {
		t.Error("runner should have started instance")
	}

	// 3. Get
	got, ok := registry.Get("test-id")
	if !ok {
		t.Error("Get failed")
	}
	if got != inst {
		t.Error("Get returned wrong instance")
	}

	// 4. ListByRule
	list := registry.ListByRule("rule-1")
	if len(list) != 1 {
		t.Errorf("ListByRule expected 1, got %d", len(list))
	}

	// 5. Stop
	if err := registry.Stop("test-id"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if inst.Status != InstanceStopped {
		t.Errorf("expected status Stopped, got %s", inst.Status)
	}
	if runner.started["test-id"] {
		t.Error("runner should have stopped instance")
	}

	// 6. Remove
	if err := registry.Remove("test-id"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, ok := registry.Get("test-id"); ok {
		t.Error("Remove failed, instance still exists")
	}
}

func TestMemoryRegistry_StartFail(t *testing.T) {
	runner := &MockRunner{fail: true}
	registry := NewMemoryRegistry(runner)

	inst := &PluginInstance{ID: "fail-id"}
	registry.Register(inst)

	if err := registry.Start("fail-id"); err == nil {
		t.Error("expected start error")
	}

	if inst.Status != InstanceFailed {
		t.Errorf("expected status Failed, got %s", inst.Status)
	}
}
