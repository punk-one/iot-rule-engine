package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
)

// State Machine API Handlers

// TransitionStateHandler 执行状态转换
func (m *Manager) TransitionStateHandler(c *gin.Context) {
	ruleID := c.Param("id")

	var req struct {
		ToStatus string `json:"to_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前规则
	rule, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 解析状态
	fromStatus := runtime.RuleLifecycleStatus(rule.LifecycleStatus)
	if fromStatus == "" {
		fromStatus = runtime.RuleLifecycleDraft // 默认状态
	}
	toStatus := runtime.RuleLifecycleStatus(req.ToStatus)

	// 验证状态
	if !runtime.IsValidStatus(toStatus) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid status: %s", req.ToStatus)})
		return
	}

	// 执行状态转换
	sm := runtime.NewStateMachine()
	newStatus, err := sm.Transition(fromStatus, toStatus)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新规则状态
	rule.LifecycleStatus = string(newStatus)
	if err := m.db.SaveRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rule_id":          ruleID,
		"from_status":      string(fromStatus),
		"to_status":        string(newStatus),
		"lifecycle_status": rule.LifecycleStatus,
	})
}

// GetAllowedTransitionsHandler 获取允许的状态转换列表
func (m *Manager) GetAllowedTransitionsHandler(c *gin.Context) {
	ruleID := c.Param("id")

	// 获取当前规则
	rule, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 获取当前状态
	currentStatus := runtime.RuleLifecycleStatus(rule.LifecycleStatus)
	if currentStatus == "" {
		currentStatus = runtime.RuleLifecycleDraft // 默认状态
	}

	// 获取允许的转换
	sm := runtime.NewStateMachine()
	allowed := sm.GetAllowedTransitions(currentStatus)

	// 转换为字符串数组
	allowedStr := make([]string, len(allowed))
	for i, s := range allowed {
		allowedStr[i] = string(s)
	}

	c.JSON(http.StatusOK, gin.H{
		"rule_id":             ruleID,
		"current_status":      string(currentStatus),
		"allowed_transitions": allowedStr,
	})
}
