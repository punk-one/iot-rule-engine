import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Input, Select, Table, Tag, type TableProps } from 'antd';
import { useNavigate } from 'react-router-dom';
import { ConsolePage } from '../components/ConsolePage';
import { AsyncState, FilterBar } from '../components/ui/ConsoleUI';
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

export const RuleOverviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [instances, setInstances] = useState<RuleInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | RuleStatus>('all');

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

  const filteredInstances = useMemo(() => {
    const normalizedSearch = searchTerm.trim().toLowerCase();
    return instances.filter((item) => {
      const matchesSearch = !normalizedSearch
        || (item.name || '').toLowerCase().includes(normalizedSearch)
        || item.rule_id.toLowerCase().includes(normalizedSearch)
        || item.id.toLowerCase().includes(normalizedSearch);
      return matchesSearch && (statusFilter === 'all' || item.status === statusFilter);
    });
  }, [instances, searchTerm, statusFilter]);

  const runtimeSummary = useMemo(() => ({
    total: instances.length,
    running: instances.filter((item) => item.status === 'running').length,
    failed: instances.filter((item) => item.status === 'failed').length,
    eventsIn: instances.reduce((sum, item) => sum + item.metrics.events_in, 0),
    eventsFailed: instances.reduce((sum, item) => sum + item.metrics.events_failed, 0),
  }), [instances]);

  const columns = useMemo<NonNullable<TableProps<RuleInstance>['columns']>>(() => [
    {
      title: '规则',
      key: 'rule',
      ellipsis: true,
      render: (_, item) => (
        <div>
          <div style={{ fontWeight: 650 }}>{item.name || item.rule_id}</div>
          <div className="plugin-manager__id">{item.rule_id}</div>
        </div>
      ),
    },
    { title: '版本', dataIndex: 'version', width: 90, align: 'center' },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      align: 'center',
      render: (status: RuleStatus) => <Tag color={statusColor(status)}>{statusLabel(status)}</Tag>,
    },
    {
      title: '节点',
      width: 80,
      align: 'right',
      render: (_, item) => Object.keys(item.nodes).length,
    },
    {
      title: '输入',
      width: 110,
      align: 'right',
      render: (_, item) => item.metrics.events_in,
    },
    {
      title: '输出',
      width: 110,
      align: 'right',
      render: (_, item) => item.metrics.events_out,
    },
    {
      title: '错误',
      width: 90,
      align: 'right',
      render: (_, item) => (
        <span style={{ color: item.metrics.events_failed > 0 ? '#dc2626' : undefined }}>
          {item.metrics.events_failed}
        </span>
      ),
    },
    {
      title: '操作',
      width: 118,
      align: 'center',
      render: (_, item) => (
        <Button type="primary" size="small" onClick={() => navigate(`/runtime/${item.id}/dag`)}>
          查看运行态
        </Button>
      ),
    },
  ], [navigate]);

  return (
    <ConsolePage maxWidth={1400}>
      <h1 className="sr-only">运行监控</h1>
      <section className="runtime-summary-strip" aria-label="运行概览">
        <RuntimeMetric label="实例" value={runtimeSummary.total} />
        <RuntimeMetric label="运行中" value={runtimeSummary.running} tone="success" />
        <RuntimeMetric label="失败" value={runtimeSummary.failed} tone={runtimeSummary.failed ? 'danger' : 'default'} />
        <RuntimeMetric label="累计输入" value={runtimeSummary.eventsIn.toLocaleString()} />
        <RuntimeMetric label="失败事件" value={runtimeSummary.eventsFailed.toLocaleString()} tone={runtimeSummary.eventsFailed ? 'danger' : 'default'} />
      </section>
      <FilterBar
        summary={`共 ${filteredInstances.length} / ${instances.length} 个实例`}
        actions={<Button onClick={() => void loadInstances()} loading={loading}>刷新</Button>}
      >
        <Input.Search
          allowClear
          aria-label="搜索运行实例"
          placeholder="搜索规则名称、规则 ID 或实例 ID"
          value={searchTerm}
          onChange={(event) => setSearchTerm(event.target.value)}
          style={{ flex: 1, minWidth: 280 }}
        />
        <Select
          aria-label="筛选运行状态"
          value={statusFilter}
          onChange={setStatusFilter}
          style={{ minWidth: 150 }}
          options={[
            { label: '全部状态', value: 'all' },
            { label: '运行中', value: 'running' },
            { label: '失败', value: 'failed' },
            { label: '已暂停', value: 'paused' },
            { label: '已停止', value: 'stopped' },
            { label: '已创建', value: 'created' },
          ]}
        />
      </FilterBar>

      <AsyncState
        loading={loading && instances.length === 0}
        error={error || undefined}
        onRetry={() => void loadInstances()}
        empty={!loading && !error && filteredInstances.length === 0}
        emptyDescription={instances.length ? '没有匹配的运行实例，请调整筛选条件。' : '暂无运行实例；部署规则后会在这里出现。'}
      >
        <section className="console-surface">
          <Table<RuleInstance>
            rowKey="id"
            size="middle"
            columns={columns}
            dataSource={filteredInstances}
            loading={loading}
            pagination={{
              defaultPageSize: 20,
              showSizeChanger: true,
              pageSizeOptions: [10, 20, 50, 100],
              showTotal: (total) => `共 ${total} 条`,
            }}
            scroll={{ x: 980 }}
          />
        </section>
      </AsyncState>
    </ConsolePage>
  );
};

const RuntimeMetric: React.FC<{
  label: string;
  value: React.ReactNode;
  tone?: 'default' | 'success' | 'danger';
}> = ({ label, value, tone = 'default' }) => (
  <div className={`runtime-summary-strip__item runtime-summary-strip__item--${tone}`}>
    <span>{label}</span>
    <strong>{value}</strong>
  </div>
);
