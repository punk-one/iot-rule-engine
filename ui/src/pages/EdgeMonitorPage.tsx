import React, { Suspense, useEffect, useState, useMemo, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { runtimeAPI } from '../services/api';
import { EdgeInstance } from '../types';
import { ConsoleHeader, ConsolePage, SegmentTabs } from '../components/ConsolePage';

const EdgeMetricsChart = React.lazy(async () => {
  const module = await import('./RuntimeCharts');
  return { default: module.EdgeMetricsChart };
});

/**
 * Edge Monitor 页面（通道/边监控）
 * 规范：规则引擎 UI 标准规范
 * 工程角色：数据排障 - 解决"数据卡在哪条边"
 *
 * 功能：
 * - Tabs: Metrics | Flow | Diagnosis
 * - EdgeMetrics（Published, Consumed, Dropped, Lag）
 * - 自动诊断规则
 */
export const EdgeMonitorPage: React.FC = () => {
  const { instanceId, edgeId } = useParams<{ instanceId: string; edgeId: string }>();
  const navigate = useNavigate();
  const [edgeInstance, setEdgeInstance] = useState<EdgeInstance | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [edgeUnavailable, setEdgeUnavailable] = useState(false);
  const [activeTab, setActiveTab] = useState<'metrics' | 'flow' | 'diagnosis'>('metrics');
  const [metricsHistory, setMetricsHistory] = useState<any[]>([]);
  const historyLimit = 30;

  const loadEdgeInstance = useCallback(async () => {
    if (!instanceId || !edgeId) return;

    try {
      const data = await runtimeAPI.getEdgeInstance(instanceId, edgeId);
      setEdgeInstance(data);
      setError(null);
      setEdgeUnavailable(false);

      // 更新指标历史
      setMetricsHistory(prev => {
        const now = new Date();
        const timeStr = `${now.getHours()}:${now.getMinutes().toString().padStart(2, '0')}:${now.getSeconds().toString().padStart(2, '0')}`;
        const metrics = data.metrics || { events_published: 0, events_consumed: 0, events_dropped: 0 };
        const newPoint = {
          time: timeStr,
          published: metrics.events_published,
          consumed: metrics.events_consumed,
          dropped: metrics.events_dropped,
          lag: metrics.events_published - metrics.events_consumed
        };

        const updated = [...prev, newPoint];
        if (updated.length > historyLimit) {
          return updated.slice(updated.length - historyLimit);
        }
        return updated;
      });
    } catch (err: any) {
      const message = err.message || 'Failed to load edge instance';
      setError(message);
      setEdgeUnavailable(message.toLowerCase().includes('edge instance not found'));
      console.error('Failed to load edge instance:', err);
    } finally {
      setLoading(false);
    }
  }, [edgeId, instanceId]);

  useEffect(() => {
    if (instanceId && edgeId && !edgeUnavailable) {
      loadEdgeInstance();
      const interval = setInterval(loadEdgeInstance, 3000);
      return () => clearInterval(interval);
    }
  }, [instanceId, edgeId, edgeUnavailable, loadEdgeInstance]);


  // 计算 Lag
  const lag = useMemo(() => {
    if (!edgeInstance || !edgeInstance.metrics) return 0;
    return edgeInstance.metrics.events_published - edgeInstance.metrics.events_consumed;
  }, [edgeInstance]);

  // 自动诊断
  const diagnosis = useMemo(() => {
    if (!edgeInstance || !edgeInstance.metrics) return null;

    const published = edgeInstance.metrics.events_published;
    const consumed = edgeInstance.metrics.events_consumed;
    const dropped = edgeInstance.metrics.events_dropped;
    const lagValue = lag;


    const issues: Array<{ type: 'error' | 'warning' | 'info'; message: string }> = [];

    // Lag 持续增长 → 下游消费慢
    if (lagValue > 0 && lagValue > published * 0.1) {
      issues.push({
        type: 'warning',
        message: `Lag 持续增长（${lagValue}），下游消费慢`,
      });
    }

    // Published > 0 & Consumed = 0 → 未订阅
    if (published > 0 && consumed === 0) {
      issues.push({
        type: 'error',
        message: '有事件发布但无事件消费，可能下游节点未订阅或订阅失败',
      });
    }

    // Dropped > 0 → Sink 异常
    if (dropped > 0) {
      issues.push({
        type: 'error',
        message: `有 ${dropped} 个事件被丢弃，Sink 可能异常`,
      });
    }

    // 正常情况
    if (issues.length === 0 && published > 0 && consumed > 0) {
      issues.push({
        type: 'info',
        message: '数据流正常',
      });
    }

    return issues;
  }, [edgeInstance, lag]);

  if (loading && !edgeInstance) {
    return (
      <div style={{ padding: '20px', textAlign: 'center' }}>
        <p>加载中...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ padding: '20px' }}>
        <div style={{ color: edgeUnavailable ? 'var(--color-text-secondary)' : 'red', marginBottom: '10px' }}>
          {edgeUnavailable ? '该拓扑边暂无独立运行指标。' : `错误: ${error}`}
        </div>
        {!edgeUnavailable ? <button onClick={loadEdgeInstance}>重试</button> : null}
        <button onClick={() => navigate(`/runtime/${instanceId}/dag`)} style={{ marginLeft: edgeUnavailable ? 0 : '10px' }}>
          返回 DAG
        </button>
      </div>
    );
  }

  if (!edgeInstance) {
    return (
      <div style={{ padding: '20px' }}>
        <p>边实例不存在</p>
        <button onClick={() => navigate(`/runtime/${instanceId}/dag`)}>返回 DAG</button>
      </div>
    );
  }

  return (
    <ConsolePage maxWidth={1400}>
      {/* Edge Header */}
      <ConsoleHeader
        title={`通道监控: ${edgeInstance.edge_id}`}
        description={`${edgeInstance.from_node} → ${edgeInstance.to_node}`}
        actions={
          <>
            <button onClick={loadEdgeInstance} style={toolbarButton('#2563eb')}>刷新</button>
            <button onClick={() => navigate(`/runtime/${instanceId}/dag`)} style={ghostButtonStyle}>返回 DAG</button>
          </>
        }
      />

      {/* Tabs */}
      <SegmentTabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as 'metrics' | 'flow' | 'diagnosis')}
        items={[
          { key: 'metrics', label: '指标' },
          { key: 'flow', label: '数据流' },
          { key: 'diagnosis', label: '诊断' },
        ]}
      />

      {/* Tab Content */}
      <div style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', borderRadius: '18px', padding: '20px', boxShadow: 'var(--shadow-card)' }}>
        {activeTab === 'metrics' && (
          <div>
            <h3 style={{ marginTop: 0 }}>通道指标</h3>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '16px', marginBottom: '24px' }}>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>已发布事件</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: 'var(--text-main)' }}>
                  {edgeInstance.metrics.events_published.toLocaleString()}
                </div>
              </div>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>已消费事件</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: 'var(--success-color)' }}>
                  {edgeInstance.metrics.events_consumed.toLocaleString()}
                </div>
              </div>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>丢弃事件</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: edgeInstance.metrics.events_dropped > 0 ? 'var(--error-color)' : 'var(--text-main)' }}>
                  {edgeInstance.metrics.events_dropped.toLocaleString()}
                </div>
              </div>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>Lag</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: lag > 10 ? 'var(--warning-color)' : 'var(--text-main)' }}>
                  {lag.toLocaleString()}
                </div>
              </div>
            </div>

            {/* 消费率 */}
            <div className="card" style={{ padding: '20px', marginBottom: '24px', background: 'linear-gradient(to right, var(--color-surface), var(--color-surface-subtle))' }}>
              <div style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '8px' }}>消费完成率</div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
                <div style={{ fontSize: '32px', fontWeight: 800, color: '#0369a1' }}>
                  {edgeInstance.metrics.events_published > 0
                    ? ((edgeInstance.metrics.events_consumed / edgeInstance.metrics.events_published) * 100).toFixed(2)
                    : '0.00'}
                  %
                </div>
                <div style={{ flex: 1, height: '12px', background: '#e0f2fe', borderRadius: '6px', overflow: 'hidden' }}>
                  <div
                    style={{
                      height: '100%',
                      width: `${edgeInstance.metrics.events_published > 0 ? (edgeInstance.metrics.events_consumed / edgeInstance.metrics.events_published * 100) : 0}%`,
                      background: 'linear-gradient(to right, #3b82f6, #10b981)',
                      borderRadius: '6px'
                    }}
                  />
                </div>
              </div>
            </div>

            {/* 通道趋势图 */}
            <div className="card" style={{ padding: '24px', height: '400px', marginBottom: '24px' }}>
              <h4 style={{ marginTop: 0, marginBottom: '20px' }}>通道吞吐趋势</h4>
              <div style={{ width: '100%', height: '300px' }}>
                <Suspense fallback={<div style={{ paddingTop: 120, textAlign: 'center', color: '#94a3b8' }}>图表加载中...</div>}>
                  <EdgeMetricsChart metricsHistory={metricsHistory} />
                </Suspense>
              </div>
            </div>

            {edgeInstance.metrics.last_event_at && (
              <div style={{ marginTop: '16px', fontSize: '12px', color: '#666' }}>
                最后事件时间: {new Date(edgeInstance.metrics.last_event_at).toLocaleString()}
              </div>
            )}
          </div>
        )}

        {activeTab === 'flow' && (
          <div>
            <h3 style={{ marginTop: 0 }}>数据流分析</h3>
            <div style={{ border: '1px solid #ddd', borderRadius: '8px', padding: '20px', marginBottom: '16px' }}>
              <div style={{ fontSize: '14px', lineHeight: '1.8' }}>
                <div style={{ marginBottom: '12px' }}>
                  <strong>数据流路径:</strong>{' '}
                  <span style={{ fontFamily: 'monospace', backgroundColor: '#f5f5f5', padding: '4px 8px', borderRadius: '4px' }}>
                    {edgeInstance.from_node} → NATS Topic ({edgeInstance.topic}) → {edgeInstance.to_node}
                  </span>
                </div>
                <div style={{ marginBottom: '12px' }}>
                  <strong>NATS Topic:</strong>{' '}
                  <span style={{ fontFamily: 'monospace', backgroundColor: '#f5f5f5', padding: '4px 8px', borderRadius: '4px' }}>
                    {edgeInstance.topic}
                  </span>
                </div>
                <div>
                  <strong>状态:</strong>{' '}
                  {edgeInstance.metrics.events_dropped > 0 ? (
                    <span style={{ color: '#F44336', fontWeight: 'bold' }}>异常 - 存在数据丢失</span>
                  ) : edgeInstance.metrics.events_published > 0 ? (
                    <span style={{ color: '#4CAF50', fontWeight: 'bold' }}>正常 - 数据流正常</span>
                  ) : (
                    <span style={{ color: '#666' }}>空闲 - 暂无数据流</span>
                  )}
                </div>
              </div>
            </div>

            {/* 数据流图表占位 */}
            <div style={{ border: '1px solid #ddd', borderRadius: '8px', padding: '40px', textAlign: 'center', color: '#666' }}>
              <p>数据流图表</p>
              <p style={{ fontSize: '12px' }}>未来版本将显示数据流的实时可视化</p>
            </div>
          </div>
        )}

        {activeTab === 'diagnosis' && (
          <div>
            <h3 style={{ marginTop: 0 }}>自动诊断</h3>
            {diagnosis && diagnosis.length > 0 ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {diagnosis.map((issue, index) => (
                  <div
                    key={index}
                    style={{
                      border: `2px solid ${
                        issue.type === 'error' ? '#F44336' : issue.type === 'warning' ? '#FF9800' : '#4CAF50'
                      }`,
                      borderRadius: '8px',
                      padding: '16px',
                      backgroundColor:
                        issue.type === 'error' ? '#FFEBEE' : issue.type === 'warning' ? '#FFF3E0' : '#E8F5E9',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span style={{ fontSize: '20px' }}>
                        {issue.type === 'error' ? '❌' : issue.type === 'warning' ? '⚠️' : '✅'}
                      </span>
                      <span
                        style={{
                          color:
                            issue.type === 'error' ? '#F44336' : issue.type === 'warning' ? '#FF9800' : '#4CAF50',
                          fontWeight: 'bold',
                        }}
                      >
                        {issue.message}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div style={{ textAlign: 'center', padding: '40px', color: '#666' }}>
                <p>暂无诊断信息</p>
              </div>
            )}

            {/* 诊断规则说明 */}
            <div style={{ marginTop: '24px', border: '1px solid #ddd', borderRadius: '8px', padding: '16px', backgroundColor: '#f9f9f9' }}>
              <h4 style={{ marginTop: 0 }}>诊断规则说明</h4>
              <ul style={{ margin: 0, paddingLeft: '20px', lineHeight: '1.8' }}>
                <li>
                  <strong>Lag 持续增长</strong> → 下游消费慢（建议检查下游节点性能）
                </li>
                <li>
                  <strong>Published &gt; 0 &amp; Consumed = 0</strong> → 未订阅（建议检查下游节点订阅状态）
                </li>
                <li>
                  <strong>Dropped &gt; 0</strong> → Sink 异常（建议检查 Sink 节点配置和连接）
                </li>
              </ul>
            </div>
          </div>
        )}
      </div>
    </ConsolePage>
  );
};

const toolbarButton = (backgroundColor: string): React.CSSProperties => ({
  padding: '8px 16px',
  backgroundColor,
  color: 'white',
  border: 'none',
  borderRadius: '10px',
  cursor: 'pointer',
  fontWeight: 600,
});

const ghostButtonStyle: React.CSSProperties = {
  padding: '8px 16px',
  backgroundColor: 'var(--color-surface)',
  color: 'var(--color-text)',
  border: '1px solid #cbd5e1',
  borderRadius: '10px',
  cursor: 'pointer',
  fontWeight: 600,
};
