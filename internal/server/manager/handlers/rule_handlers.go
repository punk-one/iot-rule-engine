package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/core/rulesanitize"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// ListRulesHandler 列出所有规则
func (m *Manager) ListRulesHandler(c *gin.Context) {
	rules, err := m.db.ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 检查是否需要完整数据
	full := c.Query("full") == "true"
	if !full {
		// 返回摘要列表
		summaries := make([]RuleSummary, 0, len(rules))
		for _, rule := range rules {
			summaries = append(summaries, m.toRuleSummary(rule))
		}
		c.JSON(http.StatusOK, gin.H{
			"rules": summaries,
			"count": len(summaries),
		})
		return
	}

	// 返回完整数据
	c.JSON(http.StatusOK, gin.H{
		"rules": rules,
		"count": len(rules),
	})
}

// GetRuleHandler 获取单个规则
func (m *Manager) GetRuleHandler(c *gin.Context) {
	ruleID := c.Param("id")
	rule, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// CreateRuleHandler 创建规则
func (m *Manager) CreateRuleHandler(c *gin.Context) {
	var ruleFlow schema.RuleFlow
	if err := c.ShouldBindJSON(&ruleFlow); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rulesanitize.CleanupVirtualSourceNodes(&ruleFlow)

	// 验证规则
	if ruleFlow.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule id is required"})
		return
	}

	// 检查是否已存在
	exists, err := m.db.RuleExists(ruleFlow.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "rule already exists"})
		return
	}

	// 设置默认生命周期状态为 draft
	if ruleFlow.LifecycleStatus == "" {
		ruleFlow.LifecycleStatus = string(runtime.RuleLifecycleDraft)
	}
	// A rule is only enabled through the deploy endpoint after validation and
	// successful runtime activation.
	ruleFlow.Enabled = false
	ruleFlow.Version = 0

	// 直接写入 rules.db
	if err := m.db.SaveRule(&ruleFlow); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save rule to database"})
		return
	}

	c.JSON(http.StatusCreated, ruleFlow)
}

// UpdateRuleHandler 更新规则
func (m *Manager) UpdateRuleHandler(c *gin.Context) {
	ruleID := c.Param("id")

	var ruleFlow schema.RuleFlow
	if err := c.ShouldBindJSON(&ruleFlow); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rulesanitize.CleanupVirtualSourceNodes(&ruleFlow)

	// 确保 ID 匹配
	ruleFlow.ID = ruleID

	current, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	if current.Enabled {
		c.JSON(http.StatusConflict, gin.H{"error": "deployed rules must be undeployed before editing"})
		return
	}
	ruleFlow.Enabled = false
	ruleFlow.CreatedAt = current.CreatedAt

	// 直接写入 rules.db
	if err := m.db.SaveRule(&ruleFlow); err != nil {
		if errors.Is(err, rulesdb.ErrRuleVersionConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "rule was changed by another request; reload before saving"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save rule to database"})
		return
	}

	c.JSON(http.StatusOK, ruleFlow)
}

// DeleteRuleHandler 删除规则
func (m *Manager) DeleteRuleHandler(c *gin.Context) {
	ruleID := c.Param("id")

	// 检查是否存在
	exists, err := m.db.RuleExists(ruleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	// 直接从数据库删除
	if err := m.db.DeleteRule(ruleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}

// toRuleSummary 转换为规则摘要
func (m *Manager) toRuleSummary(rule *schema.RuleFlow) RuleSummary {
	return RuleSummary{
		ID:          rule.ID,
		Name:        rule.Name,
		Version:     rule.Version,
		Enabled:     rule.Enabled,
		DebugMode:   rule.DebugMode,
		Description: rule.Description,
		UpdatedAt:   rule.UpdatedAt,
		CreatedAt:   rule.CreatedAt,
		Status:      m.getRuleStatus(rule),
		NodeCount:   len(rule.Nodes),
	}
}

// getRuleStatus 获取规则状态
func (m *Manager) getRuleStatus(rule *schema.RuleFlow) string {
	if !rule.Enabled {
		return "not_deployed"
	}
	if m.runtimeMgr == nil {
		return "error"
	}
	instance, err := m.runtimeMgr.GetRuleInstanceByRuleID(rule.ID)
	if err != nil || instance == nil {
		return "error"
	}
	if instance.Status == runtime.RuleStatusRunning && instance.Version == strconv.Itoa(rule.Version) {
		return "running"
	}
	return "error"
}
