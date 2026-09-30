package dispatcher

import "fmt"

// ErrPluginNotFound 插件未找到
var ErrPluginNotFound = fmt.Errorf("plugin not found")

// ErrDispatcherNotStarted Dispatcher 未启动
var ErrDispatcherNotStarted = fmt.Errorf("dispatcher not started")

// ErrRateLimited 限流错误
var ErrRateLimited = fmt.Errorf("rate limited")

// ErrSchedulerQueueFull Scheduler 队列已满
var ErrSchedulerQueueFull = fmt.Errorf("scheduler queue full")

// ErrPayloadTooLarge Payload 过大
var ErrPayloadTooLarge = fmt.Errorf("payload too large")

// ErrMissingTraceID Event 缺失 trace_id
var ErrMissingTraceID = fmt.Errorf("missing trace_id")

// BackpressureError indicates event submission was rejected by dispatcher queue policy.
type BackpressureError struct {
	RuleID    string
	FromNode  string
	ToNode    string
	Strategy  string
	QueueHint string
}

func (e *BackpressureError) Error() string {
	return fmt.Sprintf(
		"backpressure (rule=%s from=%s to=%s strategy=%s): %s",
		e.RuleID,
		e.FromNode,
		e.ToNode,
		e.Strategy,
		e.QueueHint,
	)
}
