package middleware

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LimiterMiddleware 限流中间件
type LimiterMiddleware struct {
	ruleQPS     int                      // 规则级 QPS 限制
	nodeQPS     int                      // 节点级 QPS 限制
	ruleLimiter map[string]*rate.Limiter // 规则级限流器
	nodeLimiter map[string]*rate.Limiter // 节点级限流器
	mutex       sync.RWMutex
}

// Name 返回中间件名称
func (m *LimiterMiddleware) Name() string {
	return "Limiter"
}

// Process 检查限流
func (m *LimiterMiddleware) Process(ctx *EventContext, next NextFunc) error {
	// 规则级限流
	if m.ruleQPS > 0 {
		limiter := m.getRuleLimiter(ctx.RuleID)
		if !limiter.Allow() {
			ctx.Set("rate_limited", true)
			ctx.Set("rate_limit_type", "rule")
			// 不停止处理，只记录
		}
	}

	// 节点级限流
	if m.nodeQPS > 0 {
		limiter := m.getNodeLimiter(ctx.FromNode)
		if !limiter.Allow() {
			ctx.Set("rate_limited", true)
			ctx.Set("rate_limit_type", "node")
		}
	}

	return next()
}

// getRuleLimiter 获取规则级限流器
func (m *LimiterMiddleware) getRuleLimiter(ruleID string) *rate.Limiter {
	m.mutex.RLock()
	limiter, ok := m.ruleLimiter[ruleID]
	m.mutex.RUnlock()

	if ok {
		return limiter
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 双重检查
	if limiter, ok = m.ruleLimiter[ruleID]; ok {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Limit(m.ruleQPS), m.ruleQPS)
	m.ruleLimiter[ruleID] = limiter
	return limiter
}

// getNodeLimiter 获取节点级限流器
func (m *LimiterMiddleware) getNodeLimiter(nodeID string) *rate.Limiter {
	m.mutex.RLock()
	limiter, ok := m.nodeLimiter[nodeID]
	m.mutex.RUnlock()

	if ok {
		return limiter
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 双重检查
	if limiter, ok = m.nodeLimiter[nodeID]; ok {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Limit(m.nodeQPS), m.nodeQPS)
	m.nodeLimiter[nodeID] = limiter
	return limiter
}

// NewLimiterMiddleware 创建限流中间件
func NewLimiterMiddleware(ruleQPS, nodeQPS int) *LimiterMiddleware {
	return &LimiterMiddleware{
		ruleQPS:     ruleQPS,
		nodeQPS:     nodeQPS,
		ruleLimiter: make(map[string]*rate.Limiter),
		nodeLimiter: make(map[string]*rate.Limiter),
	}
}

// DedupMiddleware 去重中间件
type DedupMiddleware struct {
	windowMs    int                  // 去重窗口时间（毫秒）
	seen        map[string]time.Time // 已处理事件 ID
	mutex       sync.RWMutex
	cleanupTick time.Duration // 清理周期
}

// Name 返回中间件名称
func (m *DedupMiddleware) Name() string {
	return "Dedup"
}

// Process 检查重复事件
func (m *DedupMiddleware) Process(ctx *EventContext, next NextFunc) error {
	eventID := ctx.Event.ID

	m.mutex.RLock()
	lastSeen, exists := m.seen[eventID]
	m.mutex.RUnlock()

	if exists {
		// 检查是否在去重窗口内
		if time.Since(lastSeen) < time.Duration(m.windowMs)*time.Millisecond {
			ctx.Set("deduplicated", true)
			ctx.Stop()
			return nil
		}
	}

	// 记录事件
	m.mutex.Lock()
	m.seen[eventID] = time.Now()
	m.mutex.Unlock()

	return next()
}

// cleanup 清理过期记录
func (m *DedupMiddleware) cleanup() {
	ticker := time.NewTicker(m.cleanupTick)
	defer ticker.Stop()

	for range ticker.C {
		m.mutex.Lock()
		cutoff := time.Now().Add(-time.Duration(m.windowMs) * time.Millisecond)
		for id, t := range m.seen {
			if t.Before(cutoff) {
				delete(m.seen, id)
			}
		}
		m.mutex.Unlock()
	}
}

// NewDedupMiddleware 创建去重中间件
func NewDedupMiddleware(windowMs int) *DedupMiddleware {
	m := &DedupMiddleware{
		windowMs:    windowMs,
		seen:        make(map[string]time.Time),
		cleanupTick: time.Minute,
	}

	// 启动清理协程
	go m.cleanup()

	return m
}
