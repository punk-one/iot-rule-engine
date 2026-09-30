package runtime

import (
	"fmt"
)

// RuleLifecycleStatus 规则生命周期状态
// 用于配置态的状态管理，与运行时状态（RuleStatus）分离
type RuleLifecycleStatus string

const (
	// RuleLifecycleDraft 草稿状态：规则正在编辑中
	RuleLifecycleDraft RuleLifecycleStatus = "draft"
	// RuleLifecycleReviewed 已审核状态：规则已审核，等待发布
	RuleLifecycleReviewed RuleLifecycleStatus = "reviewed"
	// RuleLifecycleActive 激活状态：规则已发布并运行
	RuleLifecycleActive RuleLifecycleStatus = "active"
	// RuleLifecycleArchived 归档状态：规则已停用并归档
	RuleLifecycleArchived RuleLifecycleStatus = "archived"
)

// StateMachine 规则状态机
type StateMachine struct {
	// 状态转换表：from -> []to
	transitions map[RuleLifecycleStatus][]RuleLifecycleStatus
}

// NewStateMachine 创建状态机实例
func NewStateMachine() *StateMachine {
	sm := &StateMachine{
		transitions: make(map[RuleLifecycleStatus][]RuleLifecycleStatus),
	}

	// 定义状态转换规则
	sm.transitions[RuleLifecycleDraft] = []RuleLifecycleStatus{
		RuleLifecycleReviewed, // Draft -> Reviewed（提交审核）
		RuleLifecycleArchived, // Draft -> Archived（直接归档）
	}

	sm.transitions[RuleLifecycleReviewed] = []RuleLifecycleStatus{
		RuleLifecycleActive,   // Reviewed -> Active（发布）
		RuleLifecycleDraft,    // Reviewed -> Draft（退回修改）
		RuleLifecycleArchived, // Reviewed -> Archived（归档）
	}

	sm.transitions[RuleLifecycleActive] = []RuleLifecycleStatus{
		RuleLifecycleArchived, // Active -> Archived（停用）
	}

	sm.transitions[RuleLifecycleArchived] = []RuleLifecycleStatus{
		RuleLifecycleDraft, // Archived -> Draft（恢复编辑）
	}

	return sm
}

// CanTransition 检查是否可以从 from 状态转换到 to 状态
func (sm *StateMachine) CanTransition(from, to RuleLifecycleStatus) bool {
	if from == to {
		return true // 允许保持当前状态
	}

	allowed, exists := sm.transitions[from]
	if !exists {
		return false
	}

	for _, allowedStatus := range allowed {
		if allowedStatus == to {
			return true
		}
	}

	return false
}

// Transition 执行状态转换
// 返回新状态和错误（如果转换不合法）
func (sm *StateMachine) Transition(from, to RuleLifecycleStatus) (RuleLifecycleStatus, error) {
	if !sm.CanTransition(from, to) {
		return from, fmt.Errorf("invalid transition from %s to %s", from, to)
	}

	return to, nil
}

// GetAllowedTransitions 获取允许的状态转换列表
func (sm *StateMachine) GetAllowedTransitions(from RuleLifecycleStatus) []RuleLifecycleStatus {
	allowed, exists := sm.transitions[from]
	if !exists {
		return []RuleLifecycleStatus{}
	}

	// 返回副本，避免外部修改
	result := make([]RuleLifecycleStatus, len(allowed))
	copy(result, allowed)
	return result
}

// IsValidStatus 检查状态是否有效
func IsValidStatus(status RuleLifecycleStatus) bool {
	switch status {
	case RuleLifecycleDraft, RuleLifecycleReviewed, RuleLifecycleActive, RuleLifecycleArchived:
		return true
	default:
		return false
	}
}
