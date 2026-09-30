import React from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { RuleListPage } from './RuleListPage';
import { rulesAPI, runtimeAPI } from '../services/api';

// Ant Design's table and motion cleanup can take longer under CI/JSDOM than
// Jest's five-second default, especially on the pagination cases.
jest.setTimeout(15000);

jest.mock('../services/api', () => ({
  rulesAPI: {
    list: jest.fn(),
	deploy: jest.fn(),
	undeploy: jest.fn(),
  },
  runtimeAPI: {
    getInstanceByRuleID: jest.fn(),
  },
}));

const mockNavigate = jest.fn();
const mockFeedback = {
  success: jest.fn(),
  error: jest.fn(),
  info: jest.fn(),
  warning: jest.fn(),
  confirm: jest.fn(),
  prompt: jest.fn(),
};

jest.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}));

jest.mock('../components/FeedbackProvider', () => ({
  useFeedback: () => mockFeedback,
}));

const mockedRulesAPI = rulesAPI as jest.Mocked<typeof rulesAPI>;

const buildRule = (id: string) => ({
  id,
  name: `Name ${id}`,
  version: 1,
  enabled: false,
  debug_mode: false,
  description: `Description ${id}`,
  updated_at: '2026-04-16T09:00:00Z',
  status: 'not_deployed' as const,
});

describe('RuleListPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedRulesAPI.list.mockResolvedValue(
      Array.from({ length: 25 }, (_, index) => buildRule(`rule-${String(25 - index).padStart(2, '0')}`)),
    );
    (runtimeAPI.getInstanceByRuleID as jest.Mock).mockResolvedValue({ id: 'instance-1' });
  });

  test('sorts rules by id ascending and paginates with a default page size of 20', async () => {
    render(<RuleListPage />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: '规则列表' })).toBeInTheDocument();
      expect(screen.getByLabelText('搜索规则')).toBeInTheDocument();
      expect(screen.getByLabelText('每页条数')).toHaveValue('20');
      expect(screen.getByText('第 1 / 2 页')).toBeInTheDocument();
    });

    expect(screen.getAllByTestId('rule-row').map((row) => row.getAttribute('data-rule-id'))).toEqual([
      'rule-01',
      'rule-02',
      'rule-03',
      'rule-04',
      'rule-05',
      'rule-06',
      'rule-07',
      'rule-08',
      'rule-09',
      'rule-10',
      'rule-11',
      'rule-12',
      'rule-13',
      'rule-14',
      'rule-15',
      'rule-16',
      'rule-17',
      'rule-18',
      'rule-19',
      'rule-20',
    ]);
    expect(screen.queryByText('rule-21')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '下一页' }));

    await waitFor(() => {
      expect(screen.getByText('第 2 / 2 页')).toBeInTheDocument();
    });

    expect(screen.getAllByTestId('rule-row').map((row) => row.getAttribute('data-rule-id'))).toEqual([
      'rule-21',
      'rule-22',
      'rule-23',
      'rule-24',
      'rule-25',
    ]);
  });

  test('supports custom page size changes', async () => {
    render(<RuleListPage />);

    await waitFor(() => {
      expect(screen.getByLabelText('每页条数')).toHaveValue('20');
    });

    fireEvent.change(screen.getByLabelText('每页条数'), { target: { value: '10' } });

    await waitFor(() => {
      expect(screen.getByText('第 1 / 3 页')).toBeInTheDocument();
    });

    expect(screen.getAllByTestId('rule-row')).toHaveLength(10);
    expect(screen.queryByText('rule-11')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '下一页' }));

    await waitFor(() => {
      expect(screen.getByText('第 2 / 3 页')).toBeInTheDocument();
    });

    expect(screen.getAllByTestId('rule-row').map((row) => row.getAttribute('data-rule-id'))).toEqual([
      'rule-11',
      'rule-12',
      'rule-13',
      'rule-14',
      'rule-15',
      'rule-16',
      'rule-17',
      'rule-18',
      'rule-19',
      'rule-20',
    ]);
  });

  test('locks deploy immediately, submits once, and refreshes the list after success', async () => {
	const draftRule = buildRule('rule-deploy');
	const deployedRule = { ...draftRule, enabled: true, version: 2, status: 'deployed' as const };
	let resolveDeploy!: (value: any) => void;
	const deployPromise = new Promise<any>((resolve) => {
		resolveDeploy = resolve;
	});

	mockedRulesAPI.list.mockReset();
	mockedRulesAPI.list.mockResolvedValueOnce([draftRule]).mockResolvedValue([deployedRule]);
	mockedRulesAPI.deploy.mockReturnValue(deployPromise);
	mockFeedback.confirm.mockResolvedValue(true);

	render(<RuleListPage />);
	const deployButton = await screen.findByRole('button', { name: '部署' });
	await act(async () => {
		fireEvent.click(deployButton);
		fireEvent.click(deployButton);
		await Promise.resolve();
	});

	expect(mockFeedback.confirm).toHaveBeenCalledTimes(1);
	expect(mockedRulesAPI.deploy).toHaveBeenCalledTimes(1);
	expect(screen.getByRole('button', { name: '部署中...' })).toBeDisabled();

	await act(async () => {
		resolveDeploy(deployedRule);
		await deployPromise;
	});

	await waitFor(() => {
		expect(mockedRulesAPI.list).toHaveBeenCalledTimes(2);
		expect(screen.getByText('已部署')).toBeInTheDocument();
		expect(screen.getByRole('button', { name: '取消部署' })).toBeEnabled();
	});
  });

	test('locks undeploy immediately, submits once, and refreshes the list after success', async () => {
	const deployedRule = { ...buildRule('rule-undeploy'), enabled: true, version: 2, status: 'deployed' as const };
	const draftRule = { ...deployedRule, enabled: false, status: 'not_deployed' as const };
	let resolveUndeploy!: (value: any) => void;
	const undeployPromise = new Promise<any>((resolve) => {
		resolveUndeploy = resolve;
	});

	mockedRulesAPI.list.mockReset();
	mockedRulesAPI.list.mockResolvedValueOnce([deployedRule]).mockResolvedValue([draftRule]);
	mockedRulesAPI.undeploy.mockReturnValue(undeployPromise);
	mockFeedback.confirm.mockResolvedValue(true);

	render(<RuleListPage />);
	const undeployButton = await screen.findByRole('button', { name: '取消部署' });
	await act(async () => {
		fireEvent.click(undeployButton);
		fireEvent.click(undeployButton);
		await Promise.resolve();
	});

	expect(mockFeedback.confirm).toHaveBeenCalledTimes(1);
	expect(mockedRulesAPI.undeploy).toHaveBeenCalledTimes(1);
	expect(screen.getByRole('button', { name: '取消中...' })).toBeDisabled();

	await act(async () => {
		resolveUndeploy(draftRule);
		await undeployPromise;
	});

	await waitFor(() => {
		expect(mockedRulesAPI.list).toHaveBeenCalledTimes(2);
		expect(screen.getByText('未部署')).toBeInTheDocument();
		expect(screen.getByRole('button', { name: '部署' })).toBeEnabled();
	});
	});
});
