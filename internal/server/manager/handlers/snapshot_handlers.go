package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// Snapshot API Handlers

// ListSnapshotsHandler 列出指定规则的所有快照
func (m *Manager) ListSnapshotsHandler(c *gin.Context) {
	ruleID := c.Param("id")

	snapshots, err := m.db.ListSnapshots(ruleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rule_id":   ruleID,
		"snapshots": snapshots,
		"count":     len(snapshots),
	})
}

// GetSnapshotHandler 获取指定规则的指定版本快照
func (m *Manager) GetSnapshotHandler(c *gin.Context) {
	ruleID := c.Param("id")
	versionStr := c.Param("version")

	var version int
	if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid version format"})
		return
	}

	snapshot, err := m.db.GetSnapshot(ruleID, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

// GetLatestSnapshotHandler 获取指定规则的最新快照
func (m *Manager) GetLatestSnapshotHandler(c *gin.Context) {
	ruleID := c.Param("id")

	snapshot, err := m.db.GetLatestSnapshot(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}

// CreateSnapshotHandler 手动创建规则快照
func (m *Manager) CreateSnapshotHandler(c *gin.Context) {
	ruleID := c.Param("id")

	// 获取当前规则
	rule, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 创建快照
	snapshot := &schema.RuleSnapshot{
		ID:        fmt.Sprintf("%s-v%d-%d", ruleID, rule.Version, time.Now().Unix()),
		RuleID:    ruleID,
		Version:   rule.Version,
		RuleFlow:  *rule,
		CreatedAt: time.Now(),
	}

	if err := m.db.SaveSnapshot(snapshot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, snapshot)
}
