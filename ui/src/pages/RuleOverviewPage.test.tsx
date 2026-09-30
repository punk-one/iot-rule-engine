import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { RuleOverviewPage } from './RuleOverviewPage';
import { runtimeAPI } from '../services/api';

jest.mock('../services/api', () => ({
  runtimeAPI: {
    listInstances: jest.fn(),
  },
}));

const mockNavigate = jest.fn();

jest.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}));

const mockedRuntimeAPI = runtimeAPI as jest.Mocked<typeof runtimeAPI>;

describe('RuleOverviewPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders runtime rows and navigates to dag view', async () => {
    mockedRuntimeAPI.listInstances.mockResolvedValue([
      {
        id: 'instance-1',
        rule_id: 'rule-1',
        name: '温度规则',
        version: 'v3',
        status: 'running',
        created_at: '2026-04-16T08:00:00Z',
        started_at: '2026-04-16T08:10:00Z',
        nodes: { n1: {} },
        edges: {},
        metrics: {
          events_in: 120,
          events_out: 118,
          events_failed: 0,
          last_event_at: '2026-04-16T09:00:00Z',
        },
      },
    ] as any);

    render(<RuleOverviewPage />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: '运行监控' })).toBeInTheDocument();
      expect(screen.getByLabelText('运行概览')).toBeInTheDocument();
      expect(screen.getByText('温度规则')).toBeInTheDocument();
      expect(screen.getByLabelText('搜索运行实例')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: '查看运行态' })).toBeInTheDocument();
    });
    expect(screen.queryByText('总吞吐量')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '查看运行态' }));

    expect(mockNavigate).toHaveBeenCalledWith('/runtime/instance-1/dag');
  });
});
