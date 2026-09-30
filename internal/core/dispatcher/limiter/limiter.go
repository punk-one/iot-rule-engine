package limiter

import (
	"sync"

	"golang.org/x/time/rate"
)

// LimiterConfig 限流器配置
type LimiterConfig struct {
	RuleQPS int // Rule 级 QPS
	NodeQPS int // Node 级 QPS
	Burst   int // 突发流量允许量
}

// DispatcherLimiter Dispatcher 限流器（Rule 级和 Node 级）
type DispatcherLimiter struct {
	cfg LimiterConfig

	mu        sync.Mutex
	ruleLimit map[string]*rate.Limiter // ruleID -> limiter
	nodeLimit map[string]*rate.Limiter // "ruleID:nodeID" -> limiter
}

// NewDispatcherLimiter 创建新的 Dispatcher 限流器
func NewDispatcherLimiter(cfg LimiterConfig) *DispatcherLimiter {
	if cfg.Burst <= 0 {
		cfg.Burst = cfg.RuleQPS // 默认 burst = QPS
	}
	return &DispatcherLimiter{
		cfg:       cfg,
		ruleLimit: make(map[string]*rate.Limiter),
		nodeLimit: make(map[string]*rate.Limiter),
	}
}

// ruleLimiter 获取或创建 Rule 级限流器
func (l *DispatcherLimiter) ruleLimiter(ruleID string) *rate.Limiter {
	if lim, ok := l.ruleLimit[ruleID]; ok {
		return lim
	}
	lim := rate.NewLimiter(rate.Limit(l.cfg.RuleQPS), l.cfg.Burst)
	l.ruleLimit[ruleID] = lim
	return lim
}

// nodeLimiter 获取或创建 Node 级限流器
func (l *DispatcherLimiter) nodeLimiter(ruleID, nodeID string) *rate.Limiter {
	key := ruleID + ":" + nodeID
	if lim, ok := l.nodeLimit[key]; ok {
		return lim
	}
	lim := rate.NewLimiter(rate.Limit(l.cfg.NodeQPS), l.cfg.Burst)
	l.nodeLimit[key] = lim
	return lim
}

// Allow 检查是否允许通过（Rule 级和 Node 级都要通过）
func (l *DispatcherLimiter) Allow(ruleID, nodeID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Rule 级限流
	if !l.ruleLimiter(ruleID).Allow() {
		return false
	}
	// Node 级限流
	if !l.nodeLimiter(ruleID, nodeID).Allow() {
		return false
	}
	return true
}

// Cleanup 清理不再使用的限流器（可选，用于内存管理）
func (l *DispatcherLimiter) Cleanup(ruleID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.ruleLimit, ruleID)
	// 清理该 rule 的所有 node 限流器
	prefix := ruleID + ":"
	for key := range l.nodeLimit {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(l.nodeLimit, key)
		}
	}
}
