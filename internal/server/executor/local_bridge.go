package executor

import (
	"encoding/json"
	"fmt"
	"sync"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	connectorruntime "github.com/punk-one/iot-rule-engine/internal/core/connector"
	"github.com/punk-one/iot-rule-engine/internal/core/dlq"
	coreengine "github.com/punk-one/iot-rule-engine/internal/core/engine"
)

var localExecutorBridge struct {
	mutex    sync.RWMutex
	executor *coreengine.Executor
}

func registerLocalExecutor(executor *coreengine.Executor) func() {
	localExecutorBridge.mutex.Lock()
	localExecutorBridge.executor = executor
	localExecutorBridge.mutex.Unlock()

	return func() {
		localExecutorBridge.mutex.Lock()
		if localExecutorBridge.executor == executor {
			localExecutorBridge.executor = nil
		}
		localExecutorBridge.mutex.Unlock()
	}
}

func currentLocalExecutor() *coreengine.Executor {
	localExecutorBridge.mutex.RLock()
	defer localExecutorBridge.mutex.RUnlock()
	return localExecutorBridge.executor
}

func AvailableLocal() bool {
	return currentLocalExecutor() != nil
}

func ListRulesLocal() ([]string, error) {
	executor := currentLocalExecutor()
	if executor == nil {
		return nil, fmt.Errorf("local executor not available")
	}
	return executor.ListRules(), nil
}

func StatsLocal() (map[string]interface{}, error) {
	executor := currentLocalExecutor()
	if executor == nil {
		return nil, fmt.Errorf("local executor not available")
	}
	return executor.GetStats(), nil
}

func SyncRuleLocal(rule *schema.RuleFlow) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}

	executor := currentLocalExecutor()
	if executor == nil {
		return fmt.Errorf("local executor not available")
	}

	payload, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("failed to marshal rule: %w", err)
	}

	return executor.UpdateDAGFromJSON(payload)
}

func SyncConnectorResourceLocal(resource *sdk.ConnectorResource) error {
	if resource == nil {
		return fmt.Errorf("connector resource is nil")
	}

	executor := currentLocalExecutor()
	if executor == nil {
		return fmt.Errorf("local executor not available")
	}

	return executor.RegisterConnectorResource(*resource)
}

func DeleteConnectorResourceLocal(resourceID string) error {
	if resourceID == "" {
		return fmt.Errorf("connector resource id is required")
	}

	executor := currentLocalExecutor()
	if executor == nil {
		return fmt.Errorf("local executor not available")
	}

	return executor.DeleteConnectorResource(resourceID)
}

func GetConnectorSnapshotLocal(resourceID string) (connectorruntime.ResourceSnapshot, bool) {
	executor := currentLocalExecutor()
	if executor == nil {
		return connectorruntime.ResourceSnapshot{}, false
	}
	return executor.GetConnectorResourceSnapshot(resourceID)
}

func ListConnectorSnapshotsLocal() ([]connectorruntime.ResourceSnapshot, error) {
	executor := currentLocalExecutor()
	if executor == nil {
		return nil, fmt.Errorf("local executor not available")
	}
	return executor.ListConnectorResourceSnapshots(), nil
}

func ReplayDLQMessageLocal(msg *dlq.Message) error {
	executor := currentLocalExecutor()
	if executor == nil {
		return fmt.Errorf("local executor not available")
	}
	return executor.ReplayDLQMessage(msg)
}
