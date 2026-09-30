import React from 'react';
import { Button, Tag } from 'antd';
import { Link } from 'react-router-dom';
import { rulesAPI, runtimeAPI } from '../services/api';
import type { RuleInstance, RuleSummary } from '../types';
import { ConsoleHeader, ConsolePage } from '../components/ConsolePage';

type DashboardRule = RuleSummary & {
  lifecycle_status?: 'draft' | 'reviewed' | 'active' | 'archived';
};

interface DataState<T> {
  data: T;
  loading: boolean;
  error: string | null;
}

const initialRulesState: DataState<DashboardRule[]> = { data: [], loading: true, error: null };
const initialRuntimeState: DataState<RuleInstance[]> = { data: [], loading: true, error: null };

export const DashboardPage: React.FC = () => {
  const [rulesState, setRulesState] = React.useState(initialRulesState);
  const [runtimeState, setRuntimeState] = React.useState(initialRuntimeState);

  const loadRules = React.useCallback(async () => {
    setRulesState((current) => ({ ...current, loading: true, error: null }));
    try {
      setRulesState({ data: await rulesAPI.list() as DashboardRule[], loading: false, error: null });
    } catch (error: any) {
      setRulesState({ data: [], loading: false, error: error?.message || '规则数据加载失败' });
    }
  }, []);

  const loadRuntime = React.useCallback(async () => {
    setRuntimeState((current) => ({ ...current, loading: true, error: null }));
    try {
      setRuntimeState({ data: await runtimeAPI.listInstances(), loading: false, error: null });
    } catch (error: any) {
      setRuntimeState({ data: [], loading: false, error: error?.message || '运行态数据加载失败' });
    }
  }, []);

  React.useEffect(() => {
    void loadRules();
    void loadRuntime();
  }, [loadRules, loadRuntime]);

  const ruleSummary = React.useMemo(() => {
    const deployed = rulesState.data.filter((rule) => rule.enabled || rule.status === 'deployed' || rule.status === 'running').length;
    const drafts = rulesState.data.filter((rule) => rule.lifecycle_status === 'draft' || (!rule.enabled && rule.status !== 'deployed' && rule.status !== 'running')).length;
    return { total: rulesState.data.length, deployed, drafts };
  }, [rulesState.data]);

  const runtimeSummary = React.useMemo(() => ({
    running: runtimeState.data.filter((instance) => instance.status === 'running').length,
    failed: runtimeState.data.filter((instance) => instance.status === 'failed').length,
    throughput: runtimeState.data.reduce((sum, instance) => sum + instance.metrics.events_in, 0),
  }), [runtimeState.data]);

  const stableInstances = React.useMemo(() => [...runtimeState.data]
    .filter((instance) => instance.status === 'running')
    .sort((left, right) => right.metrics.events_in - left.metrics.events_in)
    .slice(0, 3), [runtimeState.data]);

  const riskInstances = React.useMemo(() => [...runtimeState.data]
    .filter((instance) => instance.status === 'failed' || instance.metrics.events_failed > 0)
    .sort((left, right) => right.metrics.events_failed - left.metrics.events_failed)
    .slice(0, 3), [runtimeState.data]);

  const refresh = () => {
    void loadRules();
    void loadRuntime();
  };

  return (
    <ConsolePage maxWidth={1600}>
      <div className="dashboard-viewport">
        <ConsoleHeader
          title="工作台"
          description="规则资产与运行状态集中概览"
          actions={
            <>
              <Link to="/rules"><Button size="small">规则管理</Button></Link>
              <Button size="small" type="primary" loading={rulesState.loading || runtimeState.loading} onClick={refresh}>刷新</Button>
            </>
          }
        />

        <section className="dashboard-kpi-grid" aria-label="核心指标">
          <DashboardMetric label="规则总数" value={ruleSummary.total} tone="info" />
          <DashboardMetric label="已部署" value={ruleSummary.deployed} tone="success" />
          <DashboardMetric label="草稿" value={ruleSummary.drafts} tone="warning" />
          <DashboardMetric label="运行中实例" value={runtimeSummary.running} tone="success" />
          <DashboardMetric label="失败实例" value={runtimeSummary.failed} tone={runtimeSummary.failed ? 'danger' : 'neutral'} />
          <DashboardMetric label="累计吞吐" value={runtimeSummary.throughput.toLocaleString()} tone="info" />
        </section>

        {rulesState.error ? <DashboardError text={`规则数据加载失败：${rulesState.error}`} onRetry={loadRules} /> : null}
        {!rulesState.loading && !rulesState.error && rulesState.data.length === 0
          ? <div className="dashboard-inline-hint">暂无规则</div>
          : null}

        <div className="dashboard-status-grid">
          <DashboardPanel title="稳定运行" action="/runtime">
            {runtimeState.loading ? <DashboardHint text="运行态数据加载中..." /> : null}
            {runtimeState.error ? <DashboardError text={`运行态数据加载失败：${runtimeState.error}`} onRetry={loadRuntime} /> : null}
            {!runtimeState.loading && !runtimeState.error && stableInstances.length === 0
              ? <DashboardHint text="尚未发现稳定运行中的实例，可先前往规则管理部署规则" />
              : null}
            {stableInstances.map((instance) => (
              <div className="dashboard-instance-row" key={instance.id}>
                <div className="dashboard-instance-copy">
                  <strong>{instance.name || instance.rule_id}</strong>
                  <span>输入 {instance.metrics.events_in} · 输出 {instance.metrics.events_out}</span>
                </div>
                <div className="dashboard-instance-value">吞吐 {instance.metrics.events_in}</div>
              </div>
            ))}
          </DashboardPanel>

          <DashboardPanel title="异常优先" action="/runtime">
            {runtimeState.loading ? <DashboardHint text="运行态数据加载中..." /> : null}
            {runtimeState.error ? <DashboardError text={`运行态数据加载失败：${runtimeState.error}`} onRetry={loadRuntime} /> : null}
            {!runtimeState.loading && !runtimeState.error && riskInstances.length === 0
              ? <DashboardHint text="当前无异常实例，运行风险暂未出现明显信号" />
              : null}
            {riskInstances.map((instance) => (
              <div className="dashboard-instance-row dashboard-instance-row--risk" key={instance.id}>
                <div className="dashboard-instance-copy">
                  <strong>{instance.name || instance.rule_id}</strong>
                  <span>失败数 {instance.metrics.events_failed} · 输入 {instance.metrics.events_in} · 输出 {instance.metrics.events_out}</span>
                </div>
                <Tag color={instance.status === 'failed' ? 'error' : 'warning'}>
                  {instance.status === 'failed' ? '异常' : '需关注'}
                </Tag>
              </div>
            ))}
          </DashboardPanel>
        </div>
      </div>
    </ConsolePage>
  );
};

const DashboardMetric: React.FC<{
  label: string;
  value: React.ReactNode;
  tone: 'neutral' | 'info' | 'success' | 'warning' | 'danger';
}> = ({ label, value, tone }) => (
  <div className={`dashboard-metric dashboard-metric--${tone}`}>
    <span>{label}</span>
    <strong>{value}</strong>
  </div>
);

const DashboardPanel: React.FC<{ title: string; action: string; children: React.ReactNode }> = ({ title, action, children }) => (
  <section className="console-surface dashboard-panel">
    <header>
      <h2>{title}</h2>
      <Link to={action}>查看全部</Link>
    </header>
    <div className="dashboard-panel__body">{children}</div>
  </section>
);

const DashboardHint: React.FC<{ text: string }> = ({ text }) => (
  <div className="dashboard-panel__hint">{text}</div>
);

const DashboardError: React.FC<{ text: string; onRetry: () => void }> = ({ text, onRetry }) => (
  <div className="dashboard-inline-error">
    <span>{text}</span>
    <Button size="small" danger onClick={onRetry}>重试</Button>
  </div>
);
