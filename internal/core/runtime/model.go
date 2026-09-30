package runtime

import (
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

// RuleStatus 规则状态
type RuleStatus string

const (
	RuleStatusCreated RuleStatus = "created"
	RuleStatusRunning RuleStatus = "running"
	RuleStatusPaused  RuleStatus = "paused"
	RuleStatusFailed  RuleStatus = "failed"
	RuleStatusStopped RuleStatus = "stopped"
)

// NodeStatus 节点状态
type NodeStatus string

const (
	NodeStatusInit    NodeStatus = "init"
	NodeStatusRunning NodeStatus = "running"
	NodeStatusIdle    NodeStatus = "idle"
	NodeStatusError   NodeStatus = "error"
	NodeStatusStopped NodeStatus = "stopped"
)

// RuleInstance 规则运行实例
type RuleInstance struct {
	ID        string                   `json:"id"`
	RuleID    string                   `json:"rule_id"`
	Version   string                   `json:"version"`
	DebugMode bool                     `json:"debug_mode"`
	Status    RuleStatus               `json:"status"`
	CreatedAt time.Time                `json:"created_at"`
	StartedAt time.Time                `json:"started_at"`
	StoppedAt *time.Time               `json:"stopped_at,omitempty"`
	Nodes     map[string]*NodeInstance `json:"nodes"`
	Edges     map[string]*EdgeInstance `json:"edges"`
	Metrics   RuleMetrics              `json:"metrics"`
}

type RuleMetrics struct {
	EventsIn     int64      `json:"events_in"`
	EventsOut    int64      `json:"events_out"`
	EventsFailed int64      `json:"events_failed"`
	LastEventAt  *time.Time `json:"last_event_at"`
}

// NodeInstance 节点运行实例
type NodeInstance struct {
	ID           string                 `json:"id"`
	NodeID       string                 `json:"node_id"`
	PluginID     string                 `json:"plugin_id"`
	Version      string                 `json:"version"`
	Config       map[string]interface{} `json:"config"`
	Status       NodeStatus             `json:"status"`
	StartedAt    time.Time              `json:"started_at"`
	InputEdges   []string               `json:"input_edges"`
	OutputEdges  []string               `json:"output_edges"`
	Metrics      NodeMetrics            `json:"metrics"`
	Health       NodeHealth             `json:"health"`
	LastError    *NodeError             `json:"last_error,omitempty"`
	ErrorHistory []*NodeError           `json:"error_history,omitempty"`
}

// NodeHealth 节点业务健康状态。
type NodeHealth struct {
	Status          sdk.HealthStatus  `json:"status"`
	Code            string            `json:"code,omitempty"`
	Message         string            `json:"message,omitempty"`
	Details         map[string]string `json:"details,omitempty"`
	ObservedAt      time.Time         `json:"observed_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	LastHeartbeatAt *time.Time        `json:"last_heartbeat_at,omitempty"`
	LastHealthyAt   *time.Time        `json:"last_healthy_at,omitempty"`
	Sequence        uint64            `json:"sequence,omitempty"`
}

type NodeMetrics struct {
	EventsIn     int64      `json:"events_in"`
	EventsOut    int64      `json:"events_out"`
	EventsFailed int64      `json:"events_failed"`
	AvgLatencyMs float64    `json:"avg_latency_ms"`
	LastEventAt  *time.Time `json:"last_event_at"`
}

type NodeError struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	EventID string    `json:"event_id,omitempty"`
}

type EdgeInstance struct {
	ID       string      `json:"id"`
	EdgeID   string      `json:"edge_id"`
	FromNode string      `json:"from_node"`
	ToNode   string      `json:"to_node"`
	Topic    string      `json:"topic"`
	Metrics  EdgeMetrics `json:"metrics"`
}

type EdgeMetrics struct {
	EventsPublished int64      `json:"events_published"`
	EventsConsumed  int64      `json:"events_consumed"`
	EventsDropped   int64      `json:"events_dropped"`
	LastEventAt     *time.Time `json:"last_event_at"`
}

type ExecutionEvent struct {
	RuleID    string      `json:"rule_id"`
	NodeID    string      `json:"node_id"`
	EventID   string      `json:"event_id"`
	TraceID   string      `json:"trace_id"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload,omitempty"`
	Message   string      `json:"message,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

const (
	ExecutionEventTypeIn    = "in"
	ExecutionEventTypeOut   = "out"
	ExecutionEventTypeExec  = "exec"
	ExecutionEventTypeError = "error"
)

type TraceEvent struct {
	InstanceID     string      `json:"instance_id"`
	TraceID        string      `json:"trace_id"`
	RuleID         string      `json:"rule_id"`
	NodeID         string      `json:"node_id"`
	EventID        string      `json:"event_id"`
	Phase          string      `json:"phase,omitempty"`
	Status         string      `json:"status"`
	Message        string      `json:"message,omitempty"`
	Payload        interface{} `json:"payload,omitempty"`
	Timestamp      time.Time   `json:"timestamp"`
	EventTimestamp time.Time   `json:"event_timestamp"`
	ObservedAt     time.Time   `json:"observed_at"`
}

type CustomMetric struct {
	Key       string            `json:"key"`
	RuleID    string            `json:"rule_id"`
	NodeID    string            `json:"node_id"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Count     int64             `json:"count"`
	Sum       float64           `json:"sum"`
	LastValue float64           `json:"last_value"`
	UpdatedAt time.Time         `json:"updated_at"`
}
