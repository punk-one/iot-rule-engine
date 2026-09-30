package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// Manager 负责将 data.ingest.* 路由到 rule.input.*
type Manager struct {
	rules        map[string]*schema.RuleFlow // ruleID -> ruleFlow
	sourceIndex  map[string][]string         // sourceTopic -> []ruleID (倒排索引)
	mutex        sync.RWMutex
	msgReceived  int64 // 接收消息总数（atomic）
	msgRouted    int64 // 路由成功总数（atomic）
	msgDropped   int64 // 无法匹配规则的消息数（atomic）
	subscription *bus.SubscriptionHandle
}

// NewManager 创建新的 Pipeline Manager
func NewManager() *Manager {
	return &Manager{
		rules:       make(map[string]*schema.RuleFlow),
		sourceIndex: make(map[string][]string),
	}
}

// RegisterRule 注册规则（更新倒排索引）
func (m *Manager) RegisterRule(ruleFlow *schema.RuleFlow) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.rules[ruleFlow.ID] = ruleFlow

	// 更新倒排索引
	if ruleFlow.Source != nil && ruleFlow.Source.Topic != "" {
		sourceTopic := ruleFlow.Source.Topic
		// 检查是否已存在
		exists := false
		for _, ruleID := range m.sourceIndex[sourceTopic] {
			if ruleID == ruleFlow.ID {
				exists = true
				break
			}
		}
		if !exists {
			m.sourceIndex[sourceTopic] = append(m.sourceIndex[sourceTopic], ruleFlow.ID)
			pkglog.Debug("Updated source index: %s -> %s", sourceTopic, ruleFlow.ID)
		}
	}

	pkglog.Info("Registered rule in Pipeline Manager: %s", ruleFlow.ID)
}

// UnregisterRule 取消注册规则
func (m *Manager) UnregisterRule(ruleID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	rule, exists := m.rules[ruleID]
	if !exists {
		return
	}

	// 从倒排索引中删除
	if rule.Source != nil && rule.Source.Topic != "" {
		sourceTopic := rule.Source.Topic
		rules := m.sourceIndex[sourceTopic]
		newRules := make([]string, 0, len(rules))
		for _, id := range rules {
			if id != ruleID {
				newRules = append(newRules, id)
			}
		}
		if len(newRules) == 0 {
			delete(m.sourceIndex, sourceTopic)
		} else {
			m.sourceIndex[sourceTopic] = newRules
		}
	}

	delete(m.rules, ruleID)
	pkglog.Info("Unregistered rule from Pipeline Manager: %s", ruleID)
}

// Start 启动 Pipeline Manager（订阅 data.ingest.* 并路由）
func (m *Manager) Start() error {
	js := bus.GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	// 订阅 data.ingest.* 主题
	subject := "data.ingest.>"
	queueGroup := "pipeline-manager"

	subscription, err := js.SubscribeQueueManaged(subject, queueGroup, func(data []byte) error {
		atomic.AddInt64(&m.msgReceived, 1)

		// 解析消息
		var msg schema.RuleMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			pkglog.Error("Failed to unmarshal message: %v", err)
			return err
		}

		// 路由消息
		if err := m.routeMessage(&msg); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	m.mutex.Lock()
	m.subscription = subscription
	m.mutex.Unlock()
	return nil
}

func (m *Manager) Stop() error {
	m.mutex.Lock()
	subscription := m.subscription
	m.subscription = nil
	m.mutex.Unlock()
	if subscription == nil {
		return nil
	}
	return subscription.Close()
}

// routeMessage 路由消息到对应的规则（使用倒排索引优化）
func (m *Manager) routeMessage(msg *schema.RuleMessage) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	routed := false
	var routeErrors []error

	// 如果消息中已经指定了 RuleID，直接路由（O(1)）
	if msg.RuleID != "" {
		if rule, exists := m.rules[msg.RuleID]; exists && rule.Enabled {
			if err := m.sendToRule(msg.RuleID, msg); err != nil {
				return err
			}
			atomic.AddInt64(&m.msgRouted, 1)
			routed = true
		} else {
			pkglog.Debug("Rule %s not found or disabled, dropping message", msg.RuleID)
		}
	} else {
		// 使用倒排索引匹配（O(1)）
		// 构建 sourceTopic（从消息的 source_type 和 source_id）
		sourceTopic := fmt.Sprintf("data.ingest.%s.%s",
			msg.Metadata["source_type"], msg.SourceID)
		if msg.SourceID == "" {
			sourceTopic = "data.ingest.http.default"
		}

		// 查找匹配的规则
		ruleIDs, exists := m.sourceIndex[sourceTopic]
		if exists {
			for _, ruleID := range ruleIDs {
				rule, ok := m.rules[ruleID]
				if !ok || !rule.Enabled {
					continue
				}
				if err := m.sendToRule(ruleID, msg); err != nil {
					pkglog.Error("Failed to route message to rule %s: %v", ruleID, err)
					routeErrors = append(routeErrors, err)
					continue
				}
				atomic.AddInt64(&m.msgRouted, 1)
				routed = true
			}
		}
	}

	if !routed {
		atomic.AddInt64(&m.msgDropped, 1)
	}

	return errors.Join(routeErrors...)
}

// sendToRule 发送消息到指定规则
// 使用语义级 API，禁止裸 subject
func (m *Manager) sendToRule(ruleID string, msg *schema.RuleMessage) error {
	// 每个路由分支使用独立值，避免修改调用方消息并污染后续分支。
	owned := *msg
	owned.RuleID = ruleID

	data, err := json.Marshal(&owned)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 使用语义级 API 发布规则输入事件
	if err := bus.PublishRuleInput(ruleID, data); err != nil {
		return fmt.Errorf("failed to publish rule input for rule %s: %w", ruleID, err)
	}

	pkglog.Debug("Routed message to rule.input.%s", ruleID)
	return nil
}

// UpdateRuleFromJSON 从 JSON 更新规则
func (m *Manager) UpdateRuleFromJSON(jsonData []byte) error {
	var ruleFlow schema.RuleFlow

	// 尝试直接反序列化
	if err := json.Unmarshal(jsonData, &ruleFlow); err != nil {
		// 如果失败，可能是双重序列化的字符串，尝试先解析字符串
		var str string
		if err2 := json.Unmarshal(jsonData, &str); err2 == nil {
			// 成功解析为字符串，再次反序列化
			if err3 := json.Unmarshal([]byte(str), &ruleFlow); err3 != nil {
				return fmt.Errorf("failed to unmarshal RuleFlow (double-encoded string): %w", err3)
			}
		} else {
			return fmt.Errorf("failed to unmarshal RuleFlow: %w", err)
		}
	}

	m.RegisterRule(&ruleFlow)
	return nil
}

// GetMetrics 获取统计指标
func (m *Manager) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"msg_received_total": atomic.LoadInt64(&m.msgReceived),
		"msg_routed_total":   atomic.LoadInt64(&m.msgRouted),
		"msg_dropped_total":  atomic.LoadInt64(&m.msgDropped),
		"rules_count":        len(m.rules),
	}
}
