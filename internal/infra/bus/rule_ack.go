package bus

import (
	"fmt"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

// PublishRuleAck 发布业务 ACK 到 RULE_ACK Stream。
func PublishRuleAck(ruleID string, ack sdk.AckMessage) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}
	subject := fmt.Sprintf("rule.ack.%s", ruleID)
	return js.Publish(subject, ack)
}

// SubscribeRuleAck 订阅指定规则的业务 ACK。
func SubscribeRuleAck(ruleID string, handler func([]byte) error) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}
	subject := fmt.Sprintf("rule.ack.%s", ruleID)
	return js.Subscribe(subject, handler)
}
