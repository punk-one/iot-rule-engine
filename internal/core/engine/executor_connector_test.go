package engine

import (
	"testing"

	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
)

func TestNewExecutorWithLoaderInitializesConnectorRuntime(t *testing.T) {
	loader := plugin.NewLoader("")
	executor, err := NewExecutorWithLoader(loader, nil)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	defer executor.Close()

	if executor.resourceBus == nil {
		t.Fatal("expected resource bus to be initialized")
	}
	if executor.connectorHost == nil {
		t.Fatal("expected connector host to be initialized")
	}
}
