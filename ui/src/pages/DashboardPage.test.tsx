import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { DashboardPage } from './DashboardPage';
import { rulesAPI, runtimeAPI } from '../services/api';

jest.mock('../services/api', () => ({
  rulesAPI: { list: jest.fn() },
  runtimeAPI: { listInstances: jest.fn() },
}));

jest.mock('react-router-dom', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

const mockedRulesAPI = rulesAPI as jest.Mocked<typeof rulesAPI>;
const mockedRuntimeAPI = runtimeAPI as jest.Mocked<typeof runtimeAPI>;

describe('DashboardPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders rule asset and runtime summaries together', async () => {
    mockedRulesAPI.list.mockResolvedValue([
      {
        id: 'rule-1',
        name: '温度波动检测',
        version: 3,
        enabled: true,
        debug_mode: false,
        description: 'detect changes',
        status: 'deployed',
        updated_at: '2026-04-14T10:00:00Z',
      },
    ]);
    mockedRuntimeAPI.listInstances.mockResolvedValue([
      {
        id: 'instance-1',
        rule_id: 'rule-1',
        name: '温度波动检测',
        version: 'v3',
        debug_mode: false,
        status: 'failed',
        created_at: '2026-04-14T09:00:00Z',
        started_at: '2026-04-14T09:05:00Z',
        nodes: {},
        edges: {},
        metrics: {
          events_in: 128,
          events_out: 120,
          events_failed: 3,
          last_event_at: '2026-04-14T10:06:00Z',
        },
      },
      {
        id: 'instance-2',
        rule_id: 'rule-2',
        name: '湿度聚合',
        version: 'v1',
        debug_mode: false,
        status: 'running',
        created_at: '2026-04-14T09:00:00Z',
        started_at: '2026-04-14T09:15:00Z',
        nodes: {},
        edges: {},
        metrics: {
          events_in: 266,
          events_out: 260,
          events_failed: 0,
          last_event_at: '2026-04-14T10:08:00Z',
        },
      },
    ]);

    render(<DashboardPage />);

    await waitFor(() => {
      const stableHeading = screen.getByRole('heading', { name: '稳定运行' });
      const abnormalHeading = screen.getByRole('heading', { name: '异常优先' });

      expect(screen.getByText('规则总数')).toBeInTheDocument();
      expect(screen.getByText('运行中实例')).toBeInTheDocument();
      expect(stableHeading).toBeInTheDocument();
      expect(abnormalHeading).toBeInTheDocument();
      expect(stableHeading.compareDocumentPosition(abnormalHeading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(screen.getByText(/失败数 3/)).toBeInTheDocument();
      expect(screen.getByText(/吞吐 266/)).toBeInTheDocument();
      expect(screen.getByText('规则管理')).toBeInTheDocument();
    });
  });

  test('renders empty states for both rule assets and runtime data', async () => {
    mockedRulesAPI.list.mockResolvedValue([]);
    mockedRuntimeAPI.listInstances.mockResolvedValue([]);

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText('暂无规则')).toBeInTheDocument();
      expect(screen.getByText('当前无异常实例，运行风险暂未出现明显信号')).toBeInTheDocument();
      expect(screen.getByText('尚未发现稳定运行中的实例，可先前往规则管理部署规则')).toBeInTheDocument();
    });
  });

  test('renders a runtime module error without blocking rule assets', async () => {
    mockedRulesAPI.list.mockResolvedValue([
      {
        id: 'rule-1',
        name: '压力上报',
        version: 1,
        enabled: false,
        debug_mode: false,
        description: 'pressure',
        updated_at: '2026-04-14T08:00:00Z',
      },
    ]);
    mockedRuntimeAPI.listInstances.mockRejectedValue(new Error('runtime down'));

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByText('规则总数')).toBeInTheDocument();
      expect(screen.getAllByText(/运行态数据加载失败/).length).toBeGreaterThan(0);
    });
  });
});
