import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Table, Tag, type TableProps } from 'antd';
import { useNavigate } from 'react-router-dom';
import { ConsoleHeader, ConsolePage, StatCard } from '../components/ConsolePage';
import { AsyncState } from '../components/ui/ConsoleUI';
import { ResourceTabs } from '../components/ResourceTabs';
import { connectorsAPI } from '../services/api';
import type { ConnectorResourceView, ResourceStatus } from '../types';

const statusLabel = (status?: ResourceStatus): string => {
  const labels: Record<ResourceStatus, string> = {
    unknown: 'Unknown',
    healthy: 'Healthy',
    degraded: 'Degraded',
    congested: 'Congested',
    disconnected: 'Disconnected',
    readonly: 'Readonly',
    blocked: 'Blocked',
    recovering: 'Recovering',
  };
  return labels[status || 'unknown'];
};

const statusColor = (status?: ResourceStatus): string => {
  if (status === 'healthy') return 'green';
  if (status === 'degraded' || status === 'recovering') return 'gold';
  if (status === 'congested' || status === 'readonly') return 'orange';
  if (status === 'disconnected' || status === 'blocked') return 'red';
  return 'default';
};

export const ConnectorOverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [connectors, setConnectors] = useState<ConnectorResourceView[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadData = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setConnectors(await connectorsAPI.list());
    } catch (loadError: any) {
      setError(loadError.message || 'Connector 服务暂时不可用');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const summary = useMemo(() => {
    const enabled = connectors.filter((item) => item.enabled).length;
    const healthy = connectors.filter((item) => item.status?.status === 'healthy').length;
    const attention = connectors.filter((item) => {
      const status = item.status?.status;
      return Boolean(status && status !== 'healthy' && status !== 'unknown');
    });
    const referenced = connectors.filter((item) => (item.ref_count || 0) > 0).length;
    return { enabled, healthy, attention, referenced };
  }, [connectors]);

  const statusSummary = useMemo(() => {
    const counts = new Map<string, number>();
    connectors.forEach((item) => {
      const key = item.status?.status || 'unknown';
      counts.set(key, (counts.get(key) || 0) + 1);
    });
    return Array.from(counts.entries()).sort((left, right) => right[1] - left[1]);
  }, [connectors]);

  const familySummary = useMemo(() => {
    const counts = new Map<string, number>();
    connectors.forEach((item) => {
      const key = item.plugin_info?.connector_descriptor?.family || item.plugin_id || 'unknown';
      counts.set(key, (counts.get(key) || 0) + 1);
    });
    return Array.from(counts.entries()).sort((left, right) => right[1] - left[1]);
  }, [connectors]);

  const columns = useMemo<NonNullable<TableProps<ConnectorResourceView>['columns']>>(() => [
    {
      title: '资源',
      key: 'resource',
      ellipsis: true,
      render: (_, item) => (
        <div>
          <div style={{ fontWeight: 650 }}>{item.name || item.id}</div>
          <div className="plugin-manager__id">{item.id}</div>
        </div>
      ),
    },
    {
      title: '插件',
      dataIndex: 'plugin_id',
      ellipsis: true,
    },
    {
      title: '状态',
      width: 130,
      render: (_, item) => (
        <Tag color={statusColor(item.status?.status)}>{statusLabel(item.status?.status)}</Tag>
      ),
    },
    {
      title: '原因',
      width: 240,
      ellipsis: true,
      render: (_, item) => item.status?.message || item.status?.reason_code || '—',
    },
  ], []);

  return (
    <ConsolePage maxWidth={1400}>
      <ResourceTabs />
      <ConsoleHeader
        eyebrow="资源管理"
        title="Connector 概览"
        description="集中查看资源健康度和协议分布；创建、搜索与批量操作请进入 Connector 资源。"
        actions={
          <>
            <Button onClick={() => void loadData()} loading={loading}>刷新</Button>
            <Button type="primary" onClick={() => navigate('/connectors')}>管理资源</Button>
          </>
        }
      />

      <AsyncState loading={loading && connectors.length === 0} error={error || undefined} onRetry={() => void loadData()}>
        <div className="console-overview-grid">
          <StatCard label="资源总数" value={connectors.length} hint="当前纳管实例" accent="#1d4ed8" />
          <StatCard label="已启用" value={summary.enabled} hint="允许规则使用" accent="#15803d" />
          <StatCard label="Healthy" value={summary.healthy} hint="最近状态健康" accent="#16a34a" />
          <StatCard label="需关注" value={summary.attention.length} hint="非 Healthy 状态" accent={summary.attention.length ? '#b45309' : 'var(--color-text-secondary)'} />
          <StatCard label="已被引用" value={summary.referenced} hint="至少一个规则引用" accent="var(--color-text-secondary)" />
        </div>

        <div className="console-overview-panels">
          <section className="console-surface console-overview-panel">
            <h2>状态分布</h2>
            <div className="console-overview-tags">
              {statusSummary.length
                ? statusSummary.map(([status, count]) => (
                    <Tag key={status} color={statusColor(status as ResourceStatus)}>
                      {statusLabel(status as ResourceStatus)} {count}
                    </Tag>
                  ))
                : <span className="console-filter-bar__summary">暂无资源</span>}
            </div>
          </section>
          <section className="console-surface console-overview-panel">
            <h2>协议族分布</h2>
            <div className="console-overview-tags">
              {familySummary.length
                ? familySummary.map(([family, count]) => <Tag key={family}>{family} {count}</Tag>)
                : <span className="console-filter-bar__summary">暂无资源</span>}
            </div>
          </section>
        </div>

        {summary.attention.length > 0 ? (
          <section className="console-surface" style={{ marginTop: 12 }}>
            <Table<ConnectorResourceView>
              rowKey="id"
              size="middle"
              columns={columns}
              dataSource={summary.attention}
              pagination={{ defaultPageSize: 10, showSizeChanger: false }}
            />
          </section>
        ) : null}
      </AsyncState>
    </ConsolePage>
  );
};
