package handlers

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/internal/core/rulesanitize"
	pipelinesvc "github.com/punk-one/iot-rule-engine/internal/infra/pipeline"
	executorsvc "github.com/punk-one/iot-rule-engine/internal/server/executor"
)

func (m *Manager) ExecutorHealthHandler(c *gin.Context) {
	if !executorsvc.AvailableLocal() && m.executorURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": "executor service not available"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (m *Manager) ExecutorRulesHandler(c *gin.Context) {
	rules, err := executorsvc.ListRulesLocal()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	sort.Strings(rules)
	c.JSON(http.StatusOK, gin.H{
		"rules": rules,
		"count": len(rules),
	})
}

func (m *Manager) ExecutorStatsHandler(c *gin.Context) {
	stats, err := executorsvc.StatsLocal()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (m *Manager) ExecutorSyncRuleHandler(c *gin.Context) {
	ruleID := c.Param("rule_id")

	var ruleFlow schema.RuleFlow
	if err := c.ShouldBindJSON(&ruleFlow); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rulesanitize.CleanupVirtualSourceNodes(&ruleFlow)
	if ruleFlow.ID != "" && ruleFlow.ID != ruleID {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "rule_id mismatch",
			"path_id": ruleID,
			"body_id": ruleFlow.ID,
		})
		return
	}
	ruleFlow.ID = ruleID

	if err := m.syncRuleToExecutor(&ruleFlow); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "synced",
		"rule_id": ruleID,
		"enabled": ruleFlow.Enabled,
		"version": ruleFlow.Version,
	})
}

func (m *Manager) PipelineHealthHandler(c *gin.Context) {
	if !pipelinesvc.AvailableLocal() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": "pipeline service not available"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (m *Manager) PipelineMetricsHandler(c *gin.Context) {
	metrics, err := pipelinesvc.MetricsLocal()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}
