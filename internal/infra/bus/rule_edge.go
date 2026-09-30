package bus

import (
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// PublishRuleEdge 发布规则边事件到 RULE_EDGE Stream
//
// 规范：
// - Producer: Dispatcher（唯一允许的 Producer）
// - Consumer: Node Executor（Dispatcher 内部）
// - Subject 格式: rule.edge.{ruleID}.{fromNodeID}.{toNodeID}
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
// ⚠️ 只有 Dispatcher 可以发布到 RULE_EDGE，Plugins 禁止发布
func PublishRuleEdge(ruleID, fromNodeID, toNodeID string, payload []byte) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	subject := fmt.Sprintf("rule.edge.%s.%s.%s", ruleID, fromNodeID, toNodeID)
	if err := js.Publish(subject, payload); err != nil {
		return fmt.Errorf("failed to publish rule edge to %s: %w", subject, err)
	}

	pkglog.Debug("Published rule edge to %s", subject)
	return nil
}

// SubscribeRuleEdge 订阅规则边事件（单个规则）
//
// 规范：
// - Producer: Dispatcher（唯一允许的 Producer）
// - Consumer: Node Executor（Dispatcher 内部）
// - Subject 格式: rule.edge.{ruleID}.{fromNodeID}.{toNodeID}
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
func SubscribeRuleEdge(ruleID string, queueGroup string, handler func([]byte) error) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	subject := fmt.Sprintf("rule.edge.%s.>", ruleID)
	if err := js.SubscribeQueue(subject, queueGroup, handler); err != nil {
		return fmt.Errorf("failed to subscribe rule edge from %s: %w", subject, err)
	}

	pkglog.Info("Subscribed to rule edge: %s (queue: %s)", subject, queueGroup)
	return nil
}

// SubscribeRuleEdgeManualAck 订阅规则边事件（单个规则，支持手动 Ack）
//
// 规范：
// - Producer: Dispatcher（唯一允许的 Producer）
// - Consumer: Node Executor（Dispatcher 内部）
// - Subject 格式: rule.edge.{ruleID}.{fromNodeID}.{toNodeID}
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
func SubscribeRuleEdgeManualAck(ruleID string, queueGroup string, handler func(*nats.Msg)) error {
	_, err := SubscribeRuleEdgeManualAckManaged(ruleID, queueGroup, handler)
	return err
}

func SubscribeRuleEdgeManualAckManaged(ruleID string, queueGroup string, handler func(*nats.Msg)) (*SubscriptionHandle, error) {
	js := GetJetStream()
	if js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	subject := fmt.Sprintf("rule.edge.%s.>", ruleID)
	handle, err := js.SubscribeQueueManualAckManaged(subject, queueGroup, handler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe rule edge from %s: %w", subject, err)
	}

	pkglog.Info("Subscribed to rule edge (manual ack): %s (queue: %s)", subject, queueGroup)
	return handle, nil
}

// ExtractEdgeInfo 从 subject 中提取规则边信息
//
// 规范：
// - Subject 格式: rule.edge.{ruleID}.{fromNodeID}.{toNodeID}
//
// 返回: ruleID, fromNodeID, toNodeID, error
func ExtractEdgeInfo(subject string) (string, string, string, error) {
	// 格式: rule.edge.{ruleID}.{fromNodeID}.{toNodeID}
	parts := strings.Split(subject, ".")
	if len(parts) < 5 || parts[0] != "rule" || parts[1] != "edge" {
		return "", "", "", fmt.Errorf("invalid rule edge subject format: %s (expected rule.edge.{ruleID}.{fromNodeID}.{toNodeID})", subject)
	}

	ruleID := parts[2]
	fromNodeID := parts[3]
	toNodeID := parts[4]

	return ruleID, fromNodeID, toNodeID, nil
}
