package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	executorsvc "github.com/punk-one/iot-rule-engine/internal/server/executor"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func (m *Manager) ListConnectorResourcesHandler(c *gin.Context) {
	if m.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rules database is not available"})
		return
	}

	resources, err := m.db.ListConnectorResources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	views := make([]ConnectorResourceView, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		views = append(views, m.buildConnectorResourceView(resource, nil))
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Name != views[j].Name {
			return views[i].Name < views[j].Name
		}
		return views[i].ID < views[j].ID
	})

	c.JSON(http.StatusOK, gin.H{
		"connectors": views,
		"count":      len(views),
	})
}

func (m *Manager) GetConnectorResourceHandler(c *gin.Context) {
	if m.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rules database is not available"})
		return
	}

	resourceID := c.Param("id")
	resource, err := m.db.GetConnectorResource(resourceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, m.buildConnectorResourceView(resource, nil))
}

func (m *Manager) CreateConnectorResourceHandler(c *gin.Context) {
	m.upsertConnectorResource(c, true)
}

func (m *Manager) UpdateConnectorResourceHandler(c *gin.Context) {
	m.upsertConnectorResource(c, false)
}

func (m *Manager) DeleteConnectorResourceHandler(c *gin.Context) {
	if m.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rules database is not available"})
		return
	}

	resourceID := c.Param("id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connector resource id is required"})
		return
	}

	if _, err := m.db.GetConnectorResource(resourceID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if err := m.deleteConnectorResourceFromExecutor(resourceID); err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "failed to remove connector resource from executor",
			"details": err.Error(),
		})
		return
	}

	if err := m.db.DeleteConnectorResource(resourceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "connector resource deleted",
		"id":      resourceID,
	})
}

func (m *Manager) ExecutorSyncConnectorResourceHandler(c *gin.Context) {
	resourceID := c.Param("connector_id")

	var resource sdk.ConnectorResource
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if resource.ID != "" && resource.ID != resourceID {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "connector_id mismatch",
			"path_id": resourceID,
			"body_id": resource.ID,
		})
		return
	}
	resource.ID = resourceID

	if err := m.validateConnectorResource(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := m.syncConnectorResourceToExecutor(&resource); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "synced",
		"id":     resource.ID,
	})
}

func (m *Manager) ExecutorDeleteConnectorResourceHandler(c *gin.Context) {
	resourceID := c.Param("connector_id")
	if resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connector_id is required"})
		return
	}

	if err := m.deleteConnectorResourceFromExecutor(resourceID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "deleted",
		"id":     resourceID,
	})
}

func (m *Manager) upsertConnectorResource(c *gin.Context, creating bool) {
	if m.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rules database is not available"})
		return
	}

	var resource sdk.ConnectorResource
	if err := c.ShouldBindJSON(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pathID := c.Param("id")
	if pathID != "" {
		if resource.ID != "" && resource.ID != pathID {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "connector resource id mismatch",
				"path_id": pathID,
				"body_id": resource.ID,
			})
			return
		}
		resource.ID = pathID
	}

	if !creating {
		stored, err := m.db.GetConnectorResource(resource.ID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		resource.Connection = mergeConnectorSecrets(resource.Connection, stored.Connection)
	}

	if err := m.validateConnectorResource(&resource); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := m.db.SaveConnectorResource(&resource); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := m.syncConnectorResourceToExecutor(&resource); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "connector resource saved but failed to sync to executor",
			"details": err.Error(),
		})
		return
	}

	status := http.StatusOK
	if creating {
		status = http.StatusCreated
	}
	c.JSON(status, m.buildConnectorResourceView(&resource, nil))
}

func (m *Manager) validateConnectorResource(resource *sdk.ConnectorResource) error {
	if resource == nil {
		return fmt.Errorf("connector resource is nil")
	}
	if resource.ID == "" {
		return fmt.Errorf("connector resource id is required")
	}
	if resource.PluginID == "" {
		return fmt.Errorf("connector resource plugin_id is required")
	}

	path, err := m.pluginLoader.ResolvePluginPath(resource.PluginID)
	if err != nil {
		return fmt.Errorf("connector plugin %s not found: %w", resource.PluginID, err)
	}
	info, err := m.pluginLoader.VerifyPlugin(path)
	if err != nil {
		return fmt.Errorf("failed to verify connector plugin %s: %w", resource.PluginID, err)
	}
	if info.Type != sdk.PluginTypeConnector {
		return fmt.Errorf("plugin %s is not a connector plugin", resource.PluginID)
	}
	if err := m.validateConnectorResourceConnection(*info, resource.Connection); err != nil {
		return err
	}
	return nil
}

func (m *Manager) syncConnectorResourceToExecutor(resource *sdk.ConnectorResource) error {
	if resource == nil {
		return fmt.Errorf("connector resource is nil")
	}

	if m.executorURL == "" {
		if !executorsvc.AvailableLocal() {
			return nil
		}
		return executorsvc.SyncConnectorResourceLocal(resource)
	}

	payload, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal connector resource for executor sync: %w", err)
	}

	url := fmt.Sprintf("%s/connectors/%s/sync", m.executorURL, resource.ID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to build connector sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call executor connector sync API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("executor connector sync failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (m *Manager) deleteConnectorResourceFromExecutor(resourceID string) error {
	if resourceID == "" {
		return fmt.Errorf("connector resource id is required")
	}

	if m.executorURL == "" {
		if !executorsvc.AvailableLocal() {
			return nil
		}
		return executorsvc.DeleteConnectorResourceLocal(resourceID)
	}

	url := fmt.Sprintf("%s/connectors/%s", m.executorURL, resourceID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build connector delete request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call executor connector delete API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("executor connector delete failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
