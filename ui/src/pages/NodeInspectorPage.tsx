import React, { Suspense, useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { runtimeAPI } from '../services/api';
import { NodeInstance, NodeStatus, TraceEvent } from '../types';
import { ConsoleHeader, ConsolePage, SegmentTabs } from '../components/ConsolePage';

const NodeMetricsChart = React.lazy(async () => {
  const module = await import('./RuntimeCharts');
  return { default: module.NodeMetricsChart };
});

const tryParseEmbeddedJSON = (value: string): unknown | null => {
  const trimmed = value.trim();
  if (!trimmed) return null;

  const looksLikeJSON =
    (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
    (trimmed.startsWith('[') && trimmed.endsWith(']'));

  if (!looksLikeJSON) {
    return null;
  }

  try {
    return JSON.parse(trimmed);
  } catch {
    return null;
  }
};

const renderConfigValue = (value: unknown, path: string): React.ReactNode => {
  if (Array.isArray(value)) {
    if (value.length === 0) {
      return <div style={configPrimitiveStyle}>[]</div>;
    }

    return (
      <div style={configCollectionStyle}>
        {value.map((item, index) => (
          <div key={`${path}[${index}]`} style={configItemStyle}>
            <div style={configKeyStyle}>[{index}]</div>
            <div>{renderConfigValue(item, `${path}[${index}]`)}</div>
          </div>
        ))}
      </div>
    );
  }

  if (value && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) {
      return <div style={configPrimitiveStyle}>{'{}'}</div>;
    }

    return (
      <div style={configCollectionStyle}>
        {entries.map(([key, nestedValue]) => (
          <div key={`${path}.${key}`} style={configItemStyle}>
            <div style={configKeyStyle}>{key}</div>
            <div>{renderConfigValue(nestedValue, `${path}.${key}`)}</div>
          </div>
        ))}
      </div>
    );
  }

  if (typeof value === 'string') {
    const embeddedJSON = tryParseEmbeddedJSON(value);
    if (embeddedJSON !== null) {
      return (
        <div style={embeddedJSONCardStyle}>
          <div style={embeddedJSONLabelStyle}>内嵌 JSON 字符串</div>
          <pre style={embeddedJSONPreStyle}>{JSON.stringify(embeddedJSON, null, 2)}</pre>
        </div>
      );
    }

    return <div style={configPrimitiveStyle}>{value || '""'}</div>;
  }

  return <div style={configPrimitiveStyle}>{JSON.stringify(value)}</div>;
};

/**
 * Node Inspector 页面（节点运行详情）
 * 规范：规则引擎 UI 标准规范
 * 工程角色：插件排障 - 解决"插件为何异常"
 *
 * 功能：
 * - Tabs: Info | Config | Metrics | Errors | Trace
 * - 基本信息、配置信息、实时指标、最近错误、Trace 事件
 */
export const NodeInspectorPage: React.FC = () => {
  const { instanceId, nodeId } = useParams<{ instanceId: string; nodeId: string }>();
  const navigate = useNavigate();
  const [nodeInstance, setNodeInstance] = useState<NodeInstance | null>(null);
  const [traceEvents, setTraceEvents] = useState<TraceEvent[]>([]);
  const [traceDebugEnabled, setTraceDebugEnabled] = useState(true);
  const [traceMessage, setTraceMessage] = useState('');
  const [loading, setLoading] = useState(false);
  const [traceLoading, setTraceLoading] = useState(false);
  const [copiedTraceToken, setCopiedTraceToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [runtimeEdgeIDs, setRuntimeEdgeIDs] = useState<Set<string>>(new Set());
  const [activeTab, setActiveTab] = useState<'info' | 'config' | 'metrics' | 'errors' | 'trace'>('info');
  const [metricsHistory, setMetricsHistory] = useState<any[]>([]);
  const historyLimit = 30;


  const loadNodeInstance = useCallback(async () => {
    if (!instanceId || !nodeId) return;

    try {
      const data = await runtimeAPI.getNodeInstance(instanceId, nodeId);
      setNodeInstance(data);

      try {
        const runtimeInstance = await runtimeAPI.getInstance(instanceId);
        const edgeIDs = new Set<string>();
        Object.entries(runtimeInstance.edges || {}).forEach(([key, edge]) => {
          edgeIDs.add(key);
          if (edge.edge_id) edgeIDs.add(edge.edge_id);
        });
        setRuntimeEdgeIDs(edgeIDs);
      } catch (instanceError) {
        // Node details remain useful even when the surrounding instance cannot be refreshed.
        // In that case, fail closed: topology edge IDs are shown as text, not broken links.
        setRuntimeEdgeIDs(new Set());
        console.error('Failed to load runtime edges:', instanceError);
      }

      // 更新指标历史
      setMetricsHistory(prev => {
        const now = new Date();
        const timeStr = `${now.getHours()}:${now.getMinutes().toString().padStart(2, '0')}:${now.getSeconds().toString().padStart(2, '0')}`;
        const metrics = data.metrics || { events_in: 0, events_out: 0, events_failed: 0 };
        const newPoint = {
          time: timeStr,
          in: metrics.events_in,
          out: metrics.events_out,
          err: metrics.events_failed
        };

        const updated = [...prev, newPoint];
        if (updated.length > historyLimit) {
          return updated.slice(updated.length - historyLimit);
        }
        return updated;
      });
    } catch (err: any) {
      setError(err.message || 'Failed to load node instance');
      console.error('Failed to load node instance:', err);
    } finally {
      setLoading(false);
    }
  }, [instanceId, nodeId]);


  const loadTraceEvents = useCallback(async () => {
    if (!instanceId || !nodeId) return;

    setTraceLoading(true);
    try {
      const response = await runtimeAPI.getTraceEvents(instanceId, nodeId, 100);
      setTraceEvents(response.events);
      setTraceDebugEnabled(response.debug_enabled);
      setTraceMessage(response.message || '');
    } catch (err: any) {
      setTraceDebugEnabled(true);
      setTraceMessage('');
      console.error('Failed to load trace events:', err);
    } finally {
      setTraceLoading(false);
    }
  }, [instanceId, nodeId]);

  useEffect(() => {
    if (instanceId && nodeId) {
      loadNodeInstance();
      const interval = setInterval(loadNodeInstance, 3000);
      return () => clearInterval(interval);
    }
  }, [instanceId, nodeId, loadNodeInstance]);

  useEffect(() => {
    if (activeTab === 'trace' && instanceId && nodeId && traceEvents.length === 0) {
      loadTraceEvents();
    }
  }, [activeTab, instanceId, nodeId, traceEvents.length, loadTraceEvents]);

  const getStatusColor = (status: NodeStatus): string => {
    switch (status) {
      case 'running':
        return '#4CAF50';
      case 'error':
        return '#F44336';
      case 'idle':
        return '#FF9800';
      case 'stopped':
        return '#9E9E9E';
      case 'init':
        return '#2196F3';
      default:
        return '#9E9E9E';
    }
  };

  const getStatusText = (status: NodeStatus): string => {
    switch (status) {
      case 'running':
        return '运行中';
      case 'error':
        return '错误';
      case 'idle':
        return '空闲';
      case 'stopped':
        return '已停止';
      case 'init':
        return '初始化';
      default:
        return status;
    }
  };

  const getTracePhaseColor = (phase?: string): string => {
    switch (phase) {
      case 'receive':
        return '#1E88E5';
      case 'done':
        return '#43A047';
      case 'emit':
        return '#8E24AA';
      case 'ingress':
        return '#FB8C00';
      default:
        return '#757575';
    }
  };

  const formatTraceTime = (value?: string): string => {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString();
  };

  const copyTraceValue = useCallback(async (value: string, token: string) => {
    if (!value) return;

    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = value;
        textarea.setAttribute('readonly', '');
        textarea.style.position = 'absolute';
        textarea.style.left = '-9999px';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
      }
      setCopiedTraceToken(token);
      window.setTimeout(() => {
        setCopiedTraceToken((current) => (current === token ? null : current));
      }, 1500);
    } catch (copyError) {
      console.error('Failed to copy trace value:', copyError);
    }
  }, []);

  const errorHistory = nodeInstance?.error_history ?? [];
  const nodeConfig = nodeInstance?.config;
  const renderEdgeReference = (edgeId: string) => (
    runtimeEdgeIDs.has(edgeId) ? (
      <button
        onClick={() => navigate(`/runtime/${instanceId}/edges/${edgeId}`)}
        style={{
          background: 'none',
          border: 'none',
          color: '#2196F3',
          cursor: 'pointer',
          padding: 0,
          textDecoration: 'underline',
        }}
      >
        {edgeId}
      </button>
    ) : (
      <span title="该拓扑边暂无独立运行指标">{edgeId}</span>
    )
  );

  if (loading && !nodeInstance) {
    return (
      <div style={{ padding: '20px', textAlign: 'center' }}>
        <p>加载中...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ padding: '20px' }}>
        <div style={{ color: 'red', marginBottom: '10px' }}>错误: {error}</div>
        <button onClick={loadNodeInstance}>重试</button>
        <button onClick={() => navigate(`/runtime/${instanceId}/dag`)} style={{ marginLeft: '10px' }}>
          返回 DAG
        </button>
      </div>
    );
  }

  if (!nodeInstance) {
    return (
      <div style={{ padding: '20px' }}>
        <p>节点实例不存在</p>
        <button onClick={() => navigate(`/runtime/${instanceId}/dag`)}>返回 DAG</button>
      </div>
    );
  }

  return (
    <ConsolePage maxWidth={1400}>
      {/* Node Header */}
      <ConsoleHeader
        title={`节点详情: ${nodeInstance.node_id}`}
        description={`Plugin: ${nodeInstance.plugin_id} | Version: ${nodeInstance.version}`}
        actions={
          <>
            <button onClick={loadNodeInstance} style={toolbarButton('#2563eb')}>刷新</button>
            <button onClick={() => navigate(`/runtime/${instanceId}/dag`)} style={ghostButtonStyle}>返回 DAG</button>
          </>
        }
      />

      {/* Tabs */}
      <SegmentTabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as 'info' | 'config' | 'metrics' | 'errors' | 'trace')}
        items={[
          { key: 'info', label: '基本信息' },
          { key: 'config', label: '配置信息' },
          { key: 'metrics', label: '实时指标' },
          { key: 'errors', label: '错误日志' },
          {
            key: 'trace',
            label: (
              <>
                Trace
                {traceEvents.length > 0 ? (
                  <span
                    style={{
                      marginLeft: '8px',
                      padding: '2px 6px',
                      borderRadius: '999px',
                      backgroundColor: '#2563eb',
                      color: 'white',
                      fontSize: '11px',
                      fontWeight: 'bold',
                    }}
                  >
                    {traceEvents.length}
                  </span>
                ) : null}
              </>
            ),
          },
        ]}
      />

      {/* Tab Content */}
      <div style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', borderRadius: '18px', padding: '20px', boxShadow: 'var(--shadow-card)' }}>
        {activeTab === 'info' && (
          <div>
            <h3 style={{ marginTop: 0 }}>基本信息</h3>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '16px' }}>
              <div>
                <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>Plugin ID</div>
                <div style={{ fontSize: '16px', fontWeight: 'bold' }}>{nodeInstance.plugin_id}</div>
              </div>
              <div>
                <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>Version</div>
                <div style={{ fontSize: '16px', fontWeight: 'bold' }}>{nodeInstance.version}</div>
              </div>
              <div>
                <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>Status</div>
                <span
                  style={{
                    padding: '4px 12px',
                    borderRadius: '4px',
                    backgroundColor: getStatusColor(nodeInstance.status),
                    color: 'white',
                    fontSize: '14px',
                    fontWeight: 'bold',
                    display: 'inline-block',
                  }}
                >
                  {getStatusText(nodeInstance.status)}
                </span>
              </div>
              <div>
                <div style={{ fontSize: '12px', color: '#666', marginBottom: '4px' }}>Started At</div>
                <div>{new Date(nodeInstance.started_at).toLocaleString()}</div>
              </div>
            </div>

            {/* 连接信息 */}
            <div style={{ marginTop: '24px' }}>
              <h4>连接信息</h4>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '16px' }}>
                <div>
                  <div style={{ fontSize: '14px', fontWeight: 'bold', marginBottom: '8px' }}>
                    输入边 ({nodeInstance.input_edges.length})
                  </div>
                  {nodeInstance.input_edges.length > 0 ? (
                    <ul style={{ margin: 0, paddingLeft: '20px' }}>
                      {nodeInstance.input_edges.map((edgeId) => (
                        <li key={edgeId}>
                          {renderEdgeReference(edgeId)}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <div style={{ color: '#666', fontSize: '12px' }}>无输入边（Source 节点）</div>
                  )}
                </div>
                <div>
                  <div style={{ fontSize: '14px', fontWeight: 'bold', marginBottom: '8px' }}>
                    输出边 ({nodeInstance.output_edges.length})
                  </div>
                  {nodeInstance.output_edges.length > 0 ? (
                    <ul style={{ margin: 0, paddingLeft: '20px' }}>
                      {nodeInstance.output_edges.map((edgeId) => (
                        <li key={edgeId}>
                          {renderEdgeReference(edgeId)}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <div style={{ color: '#666', fontSize: '12px' }}>无输出边（Sink 节点）</div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        {activeTab === 'config' && (
          <div>
            <h3 style={{ marginTop: 0 }}>配置信息</h3>
            {nodeConfig ? (
              <>
                <div style={{ marginBottom: '16px', fontSize: '13px', color: 'var(--color-text-secondary)' }}>
                  当前展示的是该节点运行实例启动时加载的配置快照。
                </div>
                <div
                  style={{
                    border: '1px solid #dbe3f0',
                    borderRadius: '14px',
                    overflow: 'hidden',
                    backgroundColor: 'var(--color-surface)',
                  }}
                >
                  <div
                    style={{
                      padding: '12px 16px',
                      borderBottom: '1px solid #dbe3f0',
                      backgroundColor: '#f8fafc',
                      color: 'var(--color-text)',
                      fontSize: '12px',
                      fontWeight: 700,
                      letterSpacing: '0.04em',
                      textTransform: 'uppercase',
                    }}
                  >
                    格式化视图
                  </div>
                  <div style={{ padding: '16px' }}>
                    {renderConfigValue(nodeConfig, 'config')}
                  </div>
                </div>
              </>
            ) : (
              <div
                style={{
                  border: '1px dashed #cbd5e1',
                  borderRadius: '14px',
                  padding: '40px',
                  textAlign: 'center',
                  color: 'var(--color-text-secondary)',
                  backgroundColor: '#f8fafc',
                }}
              >
                当前节点没有可展示的运行配置。
              </div>
            )}
          </div>
        )}

        {activeTab === 'metrics' && (
          <div>
            <h3 style={{ marginTop: 0 }}>实时指标</h3>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '16px', marginBottom: '24px' }}>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>输入事件</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: 'var(--text-main)' }}>{nodeInstance.metrics.events_in.toLocaleString()}</div>
              </div>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>输出事件</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: 'var(--success-color)' }}>
                  {nodeInstance.metrics.events_out.toLocaleString()}
                </div>
              </div>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>失败事件</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: 'var(--error-color)' }}>
                  {nodeInstance.metrics.events_failed.toLocaleString()}
                </div>
              </div>
              <div className="card" style={{ padding: '20px' }}>
                <div style={{ fontSize: '13px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>平均延迟</div>
                <div style={{ fontSize: '32px', fontWeight: 700, color: 'var(--primary-color)' }}>
                  {nodeInstance.metrics.avg_latency_ms.toFixed(2)} <span style={{ fontSize: '16px' }}>ms</span>
                </div>
              </div>
            </div>

            {/* 指标趋势图 */}
            <div className="card" style={{ padding: '24px', height: '400px' }}>
              <h4 style={{ marginTop: 0, marginBottom: '20px' }}>处理性能趋势</h4>
              <div style={{ width: '100%', height: '300px' }}>
                <Suspense fallback={<div style={{ paddingTop: 120, textAlign: 'center', color: '#94a3b8' }}>图表加载中...</div>}>
                  <NodeMetricsChart metricsHistory={metricsHistory} />
                </Suspense>
              </div>
            </div>


            {nodeInstance.metrics.last_event_at && (
              <div style={{ marginTop: '16px', fontSize: '12px', color: '#666' }}>
                最后事件时间: {new Date(nodeInstance.metrics.last_event_at).toLocaleString()}
              </div>
            )}
          </div>
        )}

        {activeTab === 'errors' && (
          <div>
            <h3 style={{ marginTop: 0 }}>最近错误</h3>
            {nodeInstance.last_error ? (
              <div
                style={{
                  border: '2px solid #F44336',
                  borderRadius: '8px',
                  padding: '16px',
                  backgroundColor: '#FFEBEE',
                }}
              >
                <div style={{ marginBottom: '8px' }}>
                  <strong>时间:</strong> {new Date(nodeInstance.last_error.time).toLocaleString()}
                </div>
                <div style={{ marginBottom: '8px' }}>
                  <strong>消息:</strong> {nodeInstance.last_error.message}
                </div>
                {nodeInstance.last_error.event_id && (
                  <div>
                    <strong>事件ID:</strong> {nodeInstance.last_error.event_id}
                  </div>
                )}
              </div>
            ) : (
              <div style={{ textAlign: 'center', padding: '40px', color: '#666' }}>
                <p>暂无错误</p>
              </div>
            )}

            <div style={{ marginTop: '24px' }}>
              <h3 style={{ marginTop: 0, marginBottom: '12px' }}>错误历史列表</h3>
              {errorHistory.length === 0 ? (
                <div style={{ border: '1px solid #ddd', borderRadius: '8px', padding: '40px', textAlign: 'center', color: '#666' }}>
                  <p>暂无历史错误</p>
                </div>
              ) : (
                <div style={{ border: '1px solid #ddd', borderRadius: '8px', overflow: 'hidden' }}>
                  <div style={{ padding: '12px 16px', borderBottom: '1px solid #eee', backgroundColor: '#fafafa', fontSize: '12px', color: '#666' }}>
                    共 {errorHistory.length} 条，列表按最新在前显示，顶部卡片同步展示最近一条
                  </div>
                  <div style={{ maxHeight: '420px', overflowY: 'auto' }}>
                    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                      <thead style={{ position: 'sticky', top: 0, backgroundColor: '#f5f5f5', zIndex: 1 }}>
                        <tr>
                          <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>时间</th>
                          <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>消息</th>
                          <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>事件ID</th>
                        </tr>
                      </thead>
                      <tbody>
                        {errorHistory.slice().reverse().map((item, index) => (
                          <tr
                            key={`${item.time}-${item.event_id || 'no-event'}-${index}`}
                            style={{
                              borderBottom: index < errorHistory.length - 1 ? '1px solid #eee' : 'none',
                              backgroundColor: index === 0 ? 'color-mix(in srgb, var(--error-color) 10%, var(--color-surface))' : 'var(--color-surface)',
                            }}
                          >
                            <td style={{ padding: '12px', fontSize: '12px', fontFamily: 'monospace', verticalAlign: 'top' }}>
                              {formatTraceTime(item.time)}
                            </td>
                            <td style={{ padding: '12px', fontSize: '13px', lineHeight: 1.6 }}>
                              {item.message || '-'}
                            </td>
                            <td style={{ padding: '12px', fontSize: '12px', fontFamily: 'monospace', color: '#666', verticalAlign: 'top' }}>
                              {item.event_id || '-'}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {activeTab === 'trace' && (
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
              <h3 style={{ margin: 0 }}>Trace 事件</h3>
              <button
                onClick={loadTraceEvents}
                disabled={traceLoading}
                style={{
                  padding: '8px 16px',
                  backgroundColor: '#2196F3',
                  color: 'white',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: traceLoading ? 'not-allowed' : 'pointer',
                  fontSize: '14px',
                }}
              >
                {traceLoading ? '加载中...' : '刷新'}
              </button>
            </div>

            {traceEvents.length === 0 ? (
              <div style={{ border: '1px solid #ddd', borderRadius: '8px', padding: '40px', textAlign: 'center', color: '#666' }}>
                <p>暂无 Trace 事件</p>
                <p style={{ fontSize: '12px' }}>
                  {traceLoading
                    ? '正在加载...'
                    : traceDebugEnabled
                      ? '当节点处理事件时，会记录 Trace 事件'
                      : traceMessage || '当前规则未开启 Debug mode，因此不会记录详细 Trace'}
                </p>
              </div>
            ) : (
              <div style={{ border: '1px solid #ddd', borderRadius: '8px', overflow: 'hidden' }}>
                <div style={{ maxHeight: '500px', overflowY: 'auto', overflowX: 'auto' }}>
                  <table style={{ width: '100%', borderCollapse: 'collapse', tableLayout: 'fixed' }}>
                    <colgroup>
                      <col style={{ width: '160px' }} />
                      <col style={{ width: '160px' }} />
                      <col style={{ width: '92px' }} />
                      <col style={{ width: '260px' }} />
                      <col style={{ width: '96px' }} />
                      <col />
                    </colgroup>
                    <thead style={{ position: 'sticky', top: 0, backgroundColor: '#f5f5f5', zIndex: 1 }}>
                      <tr>
                        <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>记录时间</th>
                        <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>事件时间</th>
                        <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>阶段</th>
                        <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>事件ID</th>
                        <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>状态</th>
                        <th style={{ padding: '12px', textAlign: 'left', fontWeight: 'bold', borderBottom: '2px solid #ddd' }}>消息</th>
                      </tr>
                    </thead>
                    <tbody>
                      {traceEvents.map((event, index) => (
                        <tr
                          key={`${event.trace_id}-${index}`}
                          style={{
                            borderBottom: index < traceEvents.length - 1 ? '1px solid #eee' : 'none',
                          }}
                        >
                          <td style={{ padding: '12px', fontSize: '12px', fontFamily: 'monospace', verticalAlign: 'top' }}>
                            {formatTraceTime(event.observed_at || event.timestamp)}
                          </td>
                          <td style={{ padding: '12px', fontSize: '12px', fontFamily: 'monospace', verticalAlign: 'top' }}>
                            {formatTraceTime(event.event_timestamp)}
                          </td>
                          <td style={{ padding: '12px', verticalAlign: 'top' }}>
                            <span
                              style={{
                                padding: '4px 8px',
                                borderRadius: '4px',
                                backgroundColor: getTracePhaseColor(event.phase),
                                color: 'white',
                                fontSize: '11px',
                                fontWeight: 'bold',
                              }}
                            >
                              {event.phase || '-'}
                            </span>
                          </td>
                          <td style={{ padding: '12px', verticalAlign: 'top' }}>
                            <TraceTextCell
                              value={event.event_id}
                              copyLabel="事件ID"
                              copyToken={`event_id:${event.event_id}`}
                              copiedToken={copiedTraceToken}
                              onCopy={copyTraceValue}
                              monospace
                            />
                          </td>
                          <td style={{ padding: '12px', verticalAlign: 'top' }}>
                            <span
                              style={{
                                padding: '4px 8px',
                                borderRadius: '4px',
                                backgroundColor:
                                  event.status === 'success'
                                    ? '#4CAF50'
                                    : event.status === 'failed'
                                    ? '#F44336'
                                    : '#FF9800',
                                color: 'white',
                                fontSize: '11px',
                                fontWeight: 'bold',
                              }}
                            >
                              {event.status}
                            </span>
                          </td>
                          <td style={{ padding: '12px', verticalAlign: 'top' }}>
                            <TraceTextCell
                              value={event.message}
                              copyLabel="消息"
                              copyToken={`message:${event.event_id}`}
                              copiedToken={copiedTraceToken}
                              onCopy={copyTraceValue}
                              collapseThreshold={240}
                            />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div style={{ padding: '12px', backgroundColor: '#f5f5f5', fontSize: '12px', color: '#666', textAlign: 'center' }}>
                  共 {traceEvents.length} 条事件（最近 100 条）
                </div>
              </div>
            )}
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

const configCollectionStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
};

const configItemStyle: React.CSSProperties = {
  padding: '12px 14px',
  borderRadius: '12px',
  border: '1px solid #e2e8f0',
  backgroundColor: '#f8fafc',
};

const configKeyStyle: React.CSSProperties = {
  marginBottom: '8px',
  fontSize: '12px',
  fontWeight: 700,
  color: 'var(--color-text-secondary)',
  letterSpacing: '0.02em',
};

const configPrimitiveStyle: React.CSSProperties = {
  fontSize: '13px',
  lineHeight: 1.7,
  color: 'var(--color-text)',
  fontFamily: '"SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace',
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-word',
};

const embeddedJSONCardStyle: React.CSSProperties = {
  borderRadius: '12px',
  overflow: 'hidden',
  border: '1px solid #dbe3f0',
  backgroundColor: 'var(--color-surface)',
};

const embeddedJSONLabelStyle: React.CSSProperties = {
  padding: '8px 12px',
  borderBottom: '1px solid #dbe3f0',
  backgroundColor: '#eef4ff',
  color: '#1d4ed8',
  fontSize: '11px',
  fontWeight: 700,
  letterSpacing: '0.04em',
  textTransform: 'uppercase',
};

const embeddedJSONPreStyle: React.CSSProperties = {
  margin: 0,
  padding: '12px 14px',
  overflowX: 'auto',
  fontSize: '13px',
  lineHeight: 1.7,
  color: 'var(--color-text)',
  fontFamily: '"SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace',
};

const traceCopyButtonStyle: React.CSSProperties = {
  padding: '4px 8px',
  borderRadius: '8px',
  border: '1px solid #dbeafe',
  backgroundColor: '#eff6ff',
  color: '#1d4ed8',
  cursor: 'pointer',
  fontSize: '12px',
  fontWeight: 600,
};

const traceToggleButtonStyle: React.CSSProperties = {
  padding: 0,
  border: 'none',
  background: 'none',
  color: '#2563eb',
  cursor: 'pointer',
  fontSize: '12px',
  fontWeight: 600,
};

const TraceTextCell: React.FC<{
  value?: string;
  copyLabel: string;
  copyToken: string;
  copiedToken: string | null;
  onCopy: (value: string, token: string) => Promise<void>;
  monospace?: boolean;
  collapseThreshold?: number;
}> = ({ value, copyLabel, copyToken, copiedToken, onCopy, monospace = false, collapseThreshold = 0 }) => {
  const [collapsed, setCollapsed] = useState(false);

  if (!value) {
    return <span style={{ color: '#94a3b8' }}>-</span>;
  }

  const canCollapse = collapseThreshold > 0 && value.length > collapseThreshold;
  const isCopied = copiedToken === copyToken;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', alignItems: 'flex-start' }}>
      <div
        style={{
          width: '100%',
          fontSize: '13px',
          lineHeight: 1.6,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-all',
          overflowWrap: 'anywhere',
          fontFamily: monospace ? 'monospace' : undefined,
          ...(collapsed
            ? {
                display: '-webkit-box',
                WebkitLineClamp: 4,
                WebkitBoxOrient: 'vertical',
                overflow: 'hidden',
              }
            : {}),
        }}
      >
        {value}
      </div>
      <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' }}>
        <button
          type="button"
          aria-label={`复制${copyLabel} ${copyToken.startsWith('message:') ? copyToken.slice('message:'.length) : value}`}
          style={traceCopyButtonStyle}
          onClick={() => void onCopy(value, copyToken)}
        >
          {isCopied ? '已复制' : `复制${copyLabel}`}
        </button>
        {canCollapse ? (
          <button
            type="button"
            style={traceToggleButtonStyle}
            onClick={() => setCollapsed((prev) => !prev)}
          >
            {collapsed ? '展开' : '收起'}
          </button>
        ) : null}
      </div>
    </div>
  );
};
