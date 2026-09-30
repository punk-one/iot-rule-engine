package bus

import (
	"fmt"

	"github.com/nats-io/nats.go"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// PublishRuleInput 发布规则输入事件到 RULE_PIPE Stream
//
// 规范：
// - Producer: Dispatcher（或 Pipeline Executor）
// - Consumer: Pipeline Executor
// - Subject 格式: rule.input.{ruleID}
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
func PublishRuleInput(ruleID string, payload []byte) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	subject := fmt.Sprintf("rule.input.%s", ruleID)
	if err := js.Publish(subject, payload); err != nil {
		return fmt.Errorf("failed to publish rule input to %s: %w", subject, err)
	}

	pkglog.Debug("Published rule input to %s", subject)
	return nil
}

// SubscribeRuleInput 订阅规则输入事件
//
// 规范：
// - Producer: Dispatcher（或 Pipeline Executor）
// - Consumer: Pipeline Executor
// - Subject 格式: rule.input.{ruleID}
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
func SubscribeRuleInput(ruleID string, queueGroup string, handler func([]byte) error) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	subject := fmt.Sprintf("rule.input.%s", ruleID)
	if err := js.SubscribeQueue(subject, queueGroup, handler); err != nil {
		return fmt.Errorf("failed to subscribe rule input from %s: %w", subject, err)
	}

	pkglog.Info("Subscribed to rule input: %s (queue: %s)", subject, queueGroup)
	return nil
}

// SubscribeRuleInputManualAck 订阅规则输入事件（支持手动 Ack）
//
// 规范：
// - Producer: Dispatcher（或 Pipeline Executor）
// - Consumer: Pipeline Executor
// - Subject 格式: rule.input.{ruleID}
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
func SubscribeRuleInputManualAck(ruleID string, queueGroup string, handler func(*nats.Msg)) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	subject := fmt.Sprintf("rule.input.%s", ruleID)
	if err := js.SubscribeQueueManualAck(subject, queueGroup, handler); err != nil {
		return fmt.Errorf("failed to subscribe rule input from %s: %w", subject, err)
	}

	pkglog.Info("Subscribed to rule input (manual ack): %s (queue: %s)", subject, queueGroup)
	return nil
}

// SubscribeAllRuleInputs 订阅所有规则输入事件（使用通配符）
//
// 规范：
// - Producer: Dispatcher（或 Pipeline Executor）
// - Consumer: Pipeline Executor
// - Subject 格式: rule.input.>
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
func SubscribeAllRuleInputs(queueGroup string, handler func([]byte) error) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	subject := "rule.input.>"
	if err := js.SubscribeQueue(subject, queueGroup, handler); err != nil {
		return fmt.Errorf("failed to subscribe all rule inputs: %w", err)
	}

	pkglog.Info("Subscribed to all rule inputs: %s (queue: %s)", subject, queueGroup)
	return nil
}

// SubscribeAllRuleInputsManualAck 订阅所有规则输入事件（使用通配符，支持手动 Ack）
//
// 规范：
// - Producer: Dispatcher（或 Pipeline Executor）
// - Consumer: Pipeline Executor
// - Subject 格式: rule.input.>
//
// ⚠️ 禁止业务层直接使用 subject string，必须通过此 API
func SubscribeAllRuleInputsManualAck(queueGroup string, handler func(*nats.Msg)) error {
	_, err := SubscribeAllRuleInputsManualAckManaged(queueGroup, handler)
	return err
}

func SubscribeAllRuleInputsManualAckManaged(queueGroup string, handler func(*nats.Msg)) (*SubscriptionHandle, error) {
	js := GetJetStream()
	if js == nil {
		return nil, fmt.Errorf("JetStream not initialized")
	}

	subject := "rule.input.>"
	handle, err := js.SubscribeQueueManualAckManaged(subject, queueGroup, handler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe all rule inputs: %w", err)
	}

	pkglog.Info("Subscribed to all rule inputs (manual ack): %s (queue: %s)", subject, queueGroup)
	return handle, nil
}
