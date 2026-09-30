package dlq

import (
	"time"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

// Message is the canonical dead-letter payload.
type Message struct {
	ID         string            `json:"id"`
	RuleID     string            `json:"rule_id"`
	FromNodeID string            `json:"from_node_id"`
	NodeID     string            `json:"node_id"`
	Event      sdk.Event         `json:"event"`
	Error      string            `json:"error"`
	FailedAt   time.Time         `json:"failed_at"`
	Attempt    int               `json:"attempt"`
	TraceID    string            `json:"trace_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
