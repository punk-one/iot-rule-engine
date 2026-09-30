import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Table, Tag, type TableProps } from 'antd';
import { useNavigate } from 'react-router-dom';
import { ConsoleHeader, ConsolePage, StatCard } from '../components/ConsolePage';
import { AsyncState } from '../components/ui/ConsoleUI';
import { runtimeAPI } from '../services/api';
import type { RuleInstance, RuleStatus } from '../types';

const statusColor = (status: RuleStatus): string => {
  if (status === 'running') return 'green';
  if (status === 'failed') return 'red';
  if (status === 'paused') return 'orange';
  if (status === 'created') return 'blue';
  return 'default';
};

const statusLabel = (status: RuleStatus): string => ({
  created: '已创建',
  running: '运行中',
  paused: '已暂停',
  failed: '失败',
  stopped: '已停止',
}[status]);

export const RuntimeOverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [instances, setInstances] = useState<RuleInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadInstances = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setInstances(await runtimeAPI.listInstances());
    } catch (loadError: any) {
      setError(loadError.message || '无法加载运行实例');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadInstances();
    const timer = window.setInterval(() => void loadInstances(), 5000);
    return () => window.clearInterval(timer);
  }, [loadInstances]);

  const summary = useMemo(() => {
    const running = instances.filter((item) => item.status === 'running').length;
    const failed = instances.filter((item) => item.status === 'failed').length;
    const totalThroughput = instances.reduce((sum, item) => sum + item.metrics.events_in, 0);
    const failedEvents = instances.reduce((sum, item) => sum + item.metrics.events_failed, 0);
    const lastEventTimes = instances
      .map((item) => item.metrics.last_event_at)
      .filter((value): value is string => Boolean(value))
      .map((value) => new Date(value).getTime())
      .filter(Number.isFinite);
    return {
      running,
      failed,
      totalThroughput,
      failedEvents,
      lastEvent: lastEventTimes.length ? new Date(Math.max(...lastEventTimes)) : null,
    };
  }, [instances]);

  const statusSummary = useMemo(() => {
    const counts = new Map<RuleStatus, number>();
    instances.forEach((item) => counts.set(item.status, (counts.get(item.status) || 0) + 1));
    return Array.from(counts.entries()).sort((left, right) => right[1] - left[1]);
  }, [instances]);

  const recentInstances = useMemo(() => [...instances]
    .sort((left, right) => {
      const leftTime = new Date(left.metrics.last_event_at || left.started_at || left.created_at).getTime();
      const rightTime = new Date(right.metrics.last_event_at || right.started_at || right.created_at).getTime();
      return rightTime - leftTime;
    })
    .slice(0, 8), [instances]);

  const columns = useMemo<NonNullable<TableProps<RuleInstance>['columns']>>(() => [
    {
      title: '规则',
      key: 'name',
      ellipsis: true,
      render: (_, item) => (
        <div>
          <div style={{ fontWeight: 650 }}>{item.name || item.rule_id}</div>
          <div className="plugin-manager__id">{item.rule_id}</div>
        </div>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (status: RuleStatus) => <Tag color={statusColor(status)}>{statusLabel(status)}</Tag>,
    },
    {
      title: '输入 / 输出',
      width: 150,
      render: (_, item) => `${item.metrics.events_in} / ${item.metrics.events_out}`,
    },
    {
      title: '最后事件',
      width: 190,
      render: (_, item) => item.metrics.last_event_at ? new Date(item.metrics.last_event_at).toLocaleString() : '—',
    },
    {
      title: '操作',
      width: 110,
      render: (_, item) => <Button size="small" onClick={() => navigate(`/runtime/${item.id}/dag`)}>查看拓扑</Button>,
    },
  ], [navigate]);

  return (
    <ConsolePage maxWidth={1400}>
      <ConsoleHeader
        eyebrow="运行监控"
        title="运行概览"
        description="用于判断引擎健康度；实例检索和逐条查看请进入运行实例。"
        actions={
          <>
            <Button onClick={() => void loadInstances()} loading={loading}>刷新</Button>
            <Button type="primary" onClick={() => navigate('/runtime')}>查看实例</Button>
          </>
        }
      />

      <AsyncState loading={loading && instances.length === 0} error={error || undefined} onRetry={() => void loadInstances()}>
        <div className="console-overview-grid">
          <StatCard label="实例总数" value={instances.length} hint="当前运行态实例" accent="#1d4ed8" />
          <StatCard label="运行中" value={summary.running} hint="正常处理事件" accent="#15803d" />
          <StatCard label="失败实例" value={summary.failed} hint="需要立即处理" accent={summary.failed ? '#dc2626' : 'var(--color-text-secondary)'} />
          <StatCard label="累计输入" value={summary.totalThroughput.toLocaleString()} hint="所有实例输入事件" accent="var(--color-text-secondary)" />
          <StatCard label="失败事件" value={summary.failedEvents.toLocaleString()} hint="累计处理失败" accent={summary.failedEvents ? '#dc2626' : 'var(--color-text-secondary)'} />
        </div>

        <div className="console-overview-panels">
          <section className="console-surface console-overview-panel">
            <h2>状态分布</h2>
            <div className="console-overview-tags">
              {statusSummary.length
                ? statusSummary.map(([status, count]) => <Tag key={status} color={statusColor(status)}>{statusLabel(status)} {count}</Tag>)
                : <span className="console-filter-bar__summary">暂无运行实例</span>}
            </div>
          </section>
          <section className="console-surface console-overview-panel">
            <h2>最新事件</h2>
            <div style={{ color: 'var(--color-text)', fontSize: 20, fontWeight: 750 }}>
              {summary.lastEvent ? summary.lastEvent.toLocaleString() : '暂无事件'}
            </div>
          </section>
        </div>

        {recentInstances.length > 0 ? (
          <section className="console-surface" style={{ marginTop: 12 }}>
            <Table<RuleInstance>
              rowKey="id"
              size="middle"
              columns={columns}
              dataSource={recentInstances}
              pagination={false}
            />
          </section>
        ) : null}
      </AsyncState>
    </ConsolePage>
  );
};
