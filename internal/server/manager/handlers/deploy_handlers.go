package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-engine/internal/core/rulesanitize"
	executorsvc "github.com/punk-one/iot-rule-engine/internal/server/executor"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// DeployRuleHandler 部署/激活规则
// 部署前强制校验规则
func (m *Manager) DeployRuleHandler(c *gin.Context) {
	ruleID := c.Param("id")

	rule, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 强制校验规则（校验失败禁止部署）
	validationErrors := m.validateRule(rule)
	if len(validationErrors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "rule validation failed",
			"details": validationErrors,
		})
		return
	}
	if deploymentErrors := m.validateRuleDeployment(rule); len(deploymentErrors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "rule deployment validation failed",
			"details": deploymentErrors,
		})
		return
	}

	// 启用规则
	rule.Enabled = true

	// 直接保存到 rules.db
	if err := m.db.SaveRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 同步推送到 Executor，避免仅依赖轮询导致部署状态与 runtime 展示不一致。
	if err := m.syncRuleToExecutor(rule); err != nil {
		m.rollbackRuleActivation(rule)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "rule activation failed and was rolled back",
			"details": validationErrorsFromRuntimeError(err, "EXECUTOR_SYNC_FAILED"),
			"rule":    rule,
		})
		return
	}
	if err := m.waitForExecutorRuntimeState(rule.ID, true); err != nil {
		m.rollbackRuleActivation(rule)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "runtime activation failed and was rolled back",
			"details": validationErrorsFromRuntimeError(err, "RUNTIME_STATE_NOT_READY"),
			"rule":    rule,
		})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// EnableRuleHandler 启用规则
func (m *Manager) EnableRuleHandler(c *gin.Context) {
	ruleID := c.Param("id")

	rule, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	rule.Enabled = true

	if deploymentErrors := m.validateRuleDeployment(rule); len(deploymentErrors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "rule deployment validation failed",
			"details": deploymentErrors,
		})
		return
	}

	// 直接保存到 rules.db
	if err := m.db.SaveRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := m.syncRuleToExecutor(rule); err != nil {
		m.rollbackRuleActivation(rule)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "rule activation failed and was rolled back",
			"details": validationErrorsFromRuntimeError(err, "EXECUTOR_SYNC_FAILED"),
			"rule":    rule,
		})
		return
	}
	if err := m.waitForExecutorRuntimeState(rule.ID, true); err != nil {
		m.rollbackRuleActivation(rule)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "runtime activation failed and was rolled back",
			"details": validationErrorsFromRuntimeError(err, "RUNTIME_STATE_NOT_READY"),
			"rule":    rule,
		})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// DisableRuleHandler 禁用规则
func (m *Manager) DisableRuleHandler(c *gin.Context) {
	ruleID := c.Param("id")

	rule, err := m.db.GetRule(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	rule.Enabled = false

	// 直接保存到 rules.db
	if err := m.db.SaveRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := m.syncRuleToExecutor(rule); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "rule saved but failed to sync to executor",
			"details": validationErrorsFromRuntimeError(err, "EXECUTOR_SYNC_FAILED"),
			"rule":    rule,
		})
		return
	}
	if err := m.waitForExecutorRuntimeState(rule.ID, false); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "rule synced but runtime state is not ready",
			"details": validationErrorsFromRuntimeError(err, "RUNTIME_STATE_NOT_READY"),
			"rule":    rule,
		})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// DryRunHandler 模拟运行（完全隔离，不产生副作用）
func (m *Manager) DryRunHandler(c *gin.Context) {
	var req struct {
		RuleFlow   *schema.RuleFlow         `json:"rule_flow"`
		SampleData []map[string]interface{} `json:"sample_data"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rulesanitize.CleanupVirtualSourceNodes(req.RuleFlow)

	if req.RuleFlow == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule_flow is required"})
		return
	}
	if validationErrors := m.validateRule(req.RuleFlow); len(validationErrors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule validation failed", "details": validationErrors})
		return
	}
	// A real dry run requires an SDK-level side-effect contract honored by every
	// sink. Returning success before that contract exists is unsafe and misleading.
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "dry-run execution is unavailable until plugin side-effect isolation is configured",
	})
}

// ValidateRuleHandler 验证规则
func (m *Manager) ValidateRuleHandler(c *gin.Context) {
	var ruleFlow schema.RuleFlow
	if err := c.ShouldBindJSON(&ruleFlow); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rulesanitize.CleanupVirtualSourceNodes(&ruleFlow)

	errors := m.validateRule(&ruleFlow)
	warnings := []ValidationWarning{}

	// 将某些错误降级为警告（用于验证接口）
	for i := len(errors) - 1; i >= 0; i-- {
		if errors[i].Code == "NO_SINK_NODE" {
			warnings = append(warnings, ValidationWarning{
				Message: errors[i].Message,
			})
			errors = append(errors[:i], errors[i+1:]...)
		}
	}

	c.JSON(http.StatusOK, ValidationResult{
		Valid:    len(errors) == 0,
		Errors:   errors,
		Warnings: warnings,
	})
}

// validateRule 验证规则（内部方法，返回错误列表）
func (m *Manager) validateRule(rule *schema.RuleFlow) []ValidationError {
	errors := []ValidationError{}

	// 1. 验证节点ID唯一性
	nodeIDs := make(map[string]bool)
	for _, node := range rule.Nodes {
		if nodeIDs[node.ID] {
			errors = append(errors, ValidationError{
				NodeID:  node.ID,
				Message: "duplicate node ID",
				Code:    "DUPLICATE_NODE_ID",
			})
		}
		nodeIDs[node.ID] = true
	}

	// 2. 验证DAG是否有环
	if hasCycle, cycleNodes := m.checkCycle(rule.Nodes); hasCycle {
		errors = append(errors, ValidationError{
			Message: fmt.Sprintf("cycle detected: %v", cycleNodes),
			Code:    "CYCLE_DETECTED",
		})
	}

	// 3. 验证Sink节点数量
	sinkCount := 0
	for _, node := range rule.Nodes {
		if node.Type == "sink" {
			sinkCount++
		}
	}
	if sinkCount == 0 {
		errors = append(errors, ValidationError{
			Message: "no sink node found",
			Code:    "NO_SINK_NODE",
		})
	}

	// 4. 验证节点配置完整性
	for _, node := range rule.Nodes {
		if err := m.validateNodeConfig(node); err != nil {
			errors = append(errors, ValidationError{
				NodeID:  node.ID,
				Message: err.Error(),
				Code:    "INVALID_NODE_CONFIG",
			})
		}
	}

	// 5. 验证插件是否启用
	for _, node := range rule.Nodes {
		if node.Plugin != "" {
			if !m.pluginLoader.IsPluginLoaded(node.Plugin) {
				errors = append(errors, ValidationError{
					NodeID:  node.ID,
					Field:   "plugin",
					Message: fmt.Sprintf("plugin %s is not loaded", node.Plugin),
					Code:    "PLUGIN_NOT_LOADED",
				})
			}
		}
	}

	return errors
}

// checkCycle 检查DAG是否有环
func (m *Manager) checkCycle(nodes []*schema.NodeConfig) (bool, []string) {
	// 构建邻接表
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, node := range nodes {
		graph[node.ID] = node.Wires
		inDegree[node.ID] = 0
	}

	// 计算入度
	for _, node := range nodes {
		for _, target := range node.Wires {
			inDegree[target]++
		}
	}

	// 拓扑排序检测环
	queue := []string{}
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	count := 0
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		count++

		for _, v := range graph[u] {
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	if count != len(nodes) {
		// 有环，找出环中的节点
		cycleNodes := []string{}
		for id, degree := range inDegree {
			if degree > 0 {
				cycleNodes = append(cycleNodes, id)
			}
		}
		return true, cycleNodes
	}

	return false, nil
}

func (m *Manager) syncRuleToExecutor(rule *schema.RuleFlow) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}
	if m.executorURL == "" {
		return executorsvc.SyncRuleLocal(rule)
	}

	payload, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("failed to marshal rule for executor sync: %w", err)
	}

	url := fmt.Sprintf("%s/rules/%s/sync", m.executorURL, rule.ID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to build sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call executor sync API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("executor sync failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (m *Manager) rollbackRuleActivation(rule *schema.RuleFlow) {
	if rule == nil || !rule.Enabled {
		return
	}
	rule.Enabled = false
	if err := m.db.SaveRule(rule); err != nil {
		return
	}
	_ = m.syncRuleToExecutor(rule)
}

func (m *Manager) waitForExecutorRuntimeState(ruleID string, shouldExist bool) error {
	if m.executorURL == "" {
		return m.waitForLocalRuntimeState(ruleID, shouldExist)
	}

	url := fmt.Sprintf("%s/runtime/rules/%s/instance", m.executorURL, ruleID)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(6 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if shouldExist && resp.StatusCode == http.StatusOK {
				return nil
			}
			if !shouldExist && resp.StatusCode == http.StatusNotFound {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	if shouldExist {
		return fmt.Errorf("runtime running instance not found for rule %s within timeout", ruleID)
	}
	return fmt.Errorf("runtime instance for rule %s still exists within timeout", ruleID)
}

func (m *Manager) waitForLocalRuntimeState(ruleID string, shouldExist bool) error {
	if m.runtimeMgr == nil {
		return fmt.Errorf("runtime manager is not available")
	}

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		_, err := m.runtimeMgr.GetRuleInstanceByRuleID(ruleID)
		exists := err == nil
		if shouldExist && exists {
			return nil
		}
		if !shouldExist && !exists {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	if shouldExist {
		return fmt.Errorf("runtime running instance not found for rule %s within timeout", ruleID)
	}
	return fmt.Errorf("runtime instance for rule %s still exists within timeout", ruleID)
}
