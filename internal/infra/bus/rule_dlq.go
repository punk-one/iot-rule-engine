package bus

import (
	"fmt"
	"strings"
)

// PublishRuleDLQ publishes a failed event to DLQ stream.
// Subject format: rule.dlq.{ruleID}.{nodeID}
func PublishRuleDLQ(ruleID, nodeID string, payload []byte) error {
	js := GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	subject := fmt.Sprintf("rule.dlq.%s.%s", ruleID, nodeID)
	if err := js.Publish(subject, payload); err != nil {
		return fmt.Errorf("failed to publish dlq message to %s: %w", subject, err)
	}
	return nil
}

// ExtractDLQInfo extracts rule and node from DLQ subject.
func ExtractDLQInfo(subject string) (string, string, error) {
	parts := strings.Split(subject, ".")
	if len(parts) < 4 || parts[0] != "rule" || parts[1] != "dlq" {
		return "", "", fmt.Errorf("invalid dlq subject format: %s (expected rule.dlq.{ruleID}.{nodeID})", subject)
	}
	return parts[2], parts[3], nil
}
