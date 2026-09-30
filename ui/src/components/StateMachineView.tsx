import React, { useState, useEffect } from 'react';
import { stateMachineAPI, rulesAPI } from '../services/api';
import { LifecycleStatus, RuleFlow } from '../types';
import { useFeedback } from './FeedbackProvider';

interface StateMachineViewProps {
  ruleId: string;
  onStateChange?: (newStatus: LifecycleStatus) => void;
}

const STATUS_COLORS: Record<LifecycleStatus, string> = {
  draft: '#9E9E9E',      // 灰色
  reviewed: '#2196F3',   // 蓝色
  active: '#4CAF50',     // 绿色
  archived: '#FF9800',   // 橙色
};

const STATUS_LABELS: Record<LifecycleStatus, string> = {
  draft: '草稿',
  reviewed: '已审核',
  active: '已激活',
  archived: '已归档',
};

// TRANSITIONS 已移除，使用 API 返回的允许转换列表

export const StateMachineView: React.FC<StateMachineViewProps> = ({ ruleId, onStateChange }) => {
  const feedback = useFeedback();
  const [rule, setRule] = useState<RuleFlow | null>(null);
  const [currentStatus, setCurrentStatus] = useState<LifecycleStatus>('draft');
  const [allowedTransitions, setAllowedTransitions] = useState<LifecycleStatus[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ruleId]);

  const loadData = async () => {
    try {
      setLoading(true);
      const ruleData = await rulesAPI.get(ruleId);
      setRule(ruleData);
      const status = (ruleData.lifecycle_status || 'draft') as LifecycleStatus;
      setCurrentStatus(status);

      const transitions = await stateMachineAPI.getAllowedTransitions(ruleId);
      setAllowedTransitions(transitions.allowed_transitions as LifecycleStatus[]);
      setError(null);
    } catch (err: any) {
      setError(err.message || '加载数据失败');
    } finally {
      setLoading(false);
    }
  };

  const handleTransition = async (toStatus: LifecycleStatus) => {
    if (!await feedback.confirm({
      title: '确认状态转换',
      content: `确定要将规则状态从 "${STATUS_LABELS[currentStatus]}" 转换为 "${STATUS_LABELS[toStatus]}" 吗？`,
    })) {
      return;
    }

    try {
      setLoading(true);
      await stateMachineAPI.transition(ruleId, toStatus);
      await loadData();
      feedback.success(`规则状态已更新为“${STATUS_LABELS[toStatus]}”`);
      if (onStateChange) {
        onStateChange(toStatus);
      }
    } catch (err: any) {
      setError(err.message || '状态转换失败');
    } finally {
      setLoading(false);
    }
  };

  if (loading && !rule) {
    return <div style={{ padding: '20px', textAlign: 'center' }}>加载中...</div>;
  }

  if (error) {
    return <div style={{ padding: '20px', color: 'red' }}>错误: {error}</div>;
  }

  return (
    <div style={{ padding: '20px' }}>
      <h2 style={{ marginBottom: '20px' }}>规则生命周期状态机</h2>

      {/* 状态机可视化 */}
      <div style={{
        display: 'flex',
        justifyContent: 'space-around',
        alignItems: 'center',
        marginBottom: '40px',
        padding: '20px',
        backgroundColor: '#f5f5f5',
        borderRadius: '8px',
      }}>
        {(['draft', 'reviewed', 'active', 'archived'] as LifecycleStatus[]).map((status, index) => {
          const isActive = status === currentStatus;
          const canTransition = allowedTransitions.includes(status);

          return (
            <React.Fragment key={status}>
              <div
                style={{
                  position: 'relative',
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                }}
              >
                <div
                  style={{
                    width: '80px',
                    height: '80px',
                    borderRadius: '50%',
                    backgroundColor: isActive ? STATUS_COLORS[status] : '#E0E0E0',
                    color: 'white',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontWeight: 'bold',
                    fontSize: '14px',
                    textAlign: 'center',
                    border: isActive ? '3px solid #333' : 'none',
                    boxShadow: isActive ? '0 4px 8px rgba(0,0,0,0.2)' : 'none',
                    cursor: canTransition && !isActive ? 'pointer' : 'default',
                    opacity: isActive ? 1 : 0.6,
                  }}
                  onClick={() => canTransition && !isActive && handleTransition(status)}
                  title={isActive ? '当前状态' : canTransition ? `点击转换为 ${STATUS_LABELS[status]}` : ''}
                >
                  {STATUS_LABELS[status]}
                </div>
                <div style={{ marginTop: '8px', fontSize: '12px', color: '#666' }}>
                  {status}
                </div>
              </div>
              {index < 3 && (
                <div
                  style={{
                    width: '60px',
                    height: '2px',
                    backgroundColor: canTransition && index < allowedTransitions.length ? STATUS_COLORS[currentStatus] : '#E0E0E0',
                    margin: '0 10px',
                  }}
                />
              )}
            </React.Fragment>
          );
        })}
      </div>

      {/* 当前状态信息 */}
      <div style={{
        padding: '15px',
        backgroundColor: STATUS_COLORS[currentStatus],
        color: 'white',
        borderRadius: '4px',
        marginBottom: '20px',
      }}>
        <strong>当前状态:</strong> {STATUS_LABELS[currentStatus]} ({currentStatus})
      </div>

      {/* 允许的转换操作 */}
      <div>
        <h3>可执行的状态转换:</h3>
        <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
          {allowedTransitions.length === 0 ? (
            <div style={{ color: '#999' }}>当前状态无法进行转换</div>
          ) : (
            allowedTransitions.map((toStatus) => (
              <button
                key={toStatus}
                onClick={() => handleTransition(toStatus)}
                disabled={loading}
                style={{
                  padding: '10px 20px',
                  backgroundColor: STATUS_COLORS[toStatus],
                  color: 'white',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: loading ? 'not-allowed' : 'pointer',
                  fontWeight: 'bold',
                  opacity: loading ? 0.6 : 1,
                }}
              >
                {STATUS_LABELS[currentStatus]} → {STATUS_LABELS[toStatus]}
              </button>
            ))
          )}
        </div>
      </div>

      {/* 状态转换规则说明 */}
      <div style={{ marginTop: '30px', padding: '15px', backgroundColor: '#f9f9f9', borderRadius: '4px' }}>
        <h4>状态转换规则:</h4>
        <ul style={{ margin: '10px 0', paddingLeft: '20px' }}>
          <li><strong>草稿 (Draft)</strong> → 已审核 (Reviewed) / 已归档 (Archived)</li>
          <li><strong>已审核 (Reviewed)</strong> → 已激活 (Active) / 草稿 (Draft) / 已归档 (Archived)</li>
          <li><strong>已激活 (Active)</strong> → 已归档 (Archived)</li>
          <li><strong>已归档 (Archived)</strong> → 草稿 (Draft)</li>
        </ul>
      </div>
    </div>
  );
};
