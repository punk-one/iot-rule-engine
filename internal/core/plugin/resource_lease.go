package plugin

import (
	"fmt"
	"sync"
)

// RetainResource pins a connector process before its CreateResource RPC. The
// same mutex arbitrates retirement, preventing unload during creation/use.
func (c *PluginClient) RetainResource() (func(), error) {
	c.mutex.Lock()
	if c.retired || (c.Client != nil && c.Client.Exited()) {
		c.mutex.Unlock()
		return nil, fmt.Errorf("connector process retired")
	}
	c.ActiveResources++
	c.mutex.Unlock()
	var once sync.Once
	return func() { once.Do(func() { c.mutex.Lock(); c.ActiveResources--; c.mutex.Unlock() }) }, nil
}
