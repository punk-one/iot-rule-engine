package pipeline

import (
	"fmt"
	"sync"
)

var localPipelineBridge struct {
	mutex   sync.RWMutex
	manager *Manager
}

func registerLocalManager(manager *Manager) func() {
	localPipelineBridge.mutex.Lock()
	localPipelineBridge.manager = manager
	localPipelineBridge.mutex.Unlock()

	return func() {
		localPipelineBridge.mutex.Lock()
		if localPipelineBridge.manager == manager {
			localPipelineBridge.manager = nil
		}
		localPipelineBridge.mutex.Unlock()
	}
}

func currentLocalManager() *Manager {
	localPipelineBridge.mutex.RLock()
	defer localPipelineBridge.mutex.RUnlock()
	return localPipelineBridge.manager
}

func AvailableLocal() bool {
	return currentLocalManager() != nil
}

func MetricsLocal() (map[string]interface{}, error) {
	manager := currentLocalManager()
	if manager == nil {
		return nil, fmt.Errorf("local pipeline manager not available")
	}
	return manager.GetMetrics(), nil
}
