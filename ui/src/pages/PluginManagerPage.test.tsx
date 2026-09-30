import React from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { PluginManagerPage } from './PluginManagerPage';

const mockLoadPlugins = jest.fn();
const mockEnablePlugin = jest.fn();
const mockDisablePlugin = jest.fn();
const mockDeletePlugin = jest.fn();
const mockUploadPlugin = jest.fn();
const mockListFull = jest.fn();
const mockListConnectors = jest.fn();
const mockFeedback = {
  success: jest.fn(),
  error: jest.fn(),
  info: jest.fn(),
  warning: jest.fn(),
  confirm: jest.fn(),
  prompt: jest.fn(),
};

jest.mock('../store/pluginStore', () => ({
  usePluginStore: () => ({
    plugins: [
      {
        id: 'sink-timescaledb',
        name: 'Timescale Sink',
        version: '1.0.0',
        type: 'sink',
        description: 'write telemetry to timescaledb',
        enabled: true,
        engine_api: '/plugins/sink-timescaledb',
        info: {
          id: 'sink-timescaledb',
          name: 'Timescale Sink',
          version: '1.0.0',
          type: 'sink',
          description: 'write telemetry to timescaledb',
          category: 'timeseries',
          plane: 'rule',
          capabilities: {
            input_ports: 1,
            output_ports: 0,
            support_batch: true,
            support_ack: false,
            config_schema: '{"type":"object"}',
          },
        },
      },
    ],
    loading: false,
    loadPlugins: mockLoadPlugins,
    enablePlugin: mockEnablePlugin,
    disablePlugin: mockDisablePlugin,
    deletePlugin: mockDeletePlugin,
    uploadPlugin: mockUploadPlugin,
  }),
}));

jest.mock('../services/api', () => ({
  rulesAPI: { listFull: (...args: unknown[]) => mockListFull(...args) },
  connectorsAPI: { list: (...args: unknown[]) => mockListConnectors(...args) },
}));

jest.mock('../components/FeedbackProvider', () => ({
  useFeedback: () => mockFeedback,
}));

jest.mock('../components/ResourceTabs', () => ({
  ResourceTabs: () => <div>资源页签</div>,
}));

describe('PluginManagerPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockListFull.mockResolvedValue([]);
    mockListConnectors.mockResolvedValue([]);
    mockFeedback.confirm.mockResolvedValue(true);
  });

  test('opens plugin details in a modal with manifest information', async () => {
    render(<PluginManagerPage />);

    await waitFor(() => {
      expect(screen.getByText('Timescale Sink')).toBeInTheDocument();
      expect(screen.getByTestId('plugin-detail-open-sink-timescaledb')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTestId('plugin-detail-open-sink-timescaledb'));

    const detailModal = screen.getByTestId('plugin-detail-modal');
    expect(within(detailModal).getByText('Manifest 已通过完整性检查')).toBeInTheDocument();
    expect(within(detailModal).getAllByText('sink-timescaledb').length).toBeGreaterThan(0);
    expect(within(detailModal).getByText('timeseries')).toBeInTheDocument();
    expect(screen.queryByTestId('plugin-detail-panel')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '日志' })).not.toBeInTheDocument();
  });

  test('blocks disabling a plugin referenced by a rule', async () => {
    mockListFull.mockResolvedValue([
      {
        id: 'rule-1',
        name: '机床采集',
        enabled: true,
        status: 'deployed',
        nodes: [{ id: 'sink', plugin: 'sink-timescaledb', wires: [], config: {} }],
      },
    ]);
    render(<PluginManagerPage />);

    const detailButton = await screen.findByTestId('plugin-detail-open-sink-timescaledb');
    const pluginRow = detailButton.closest('tr');
    expect(pluginRow).not.toBeNull();
    fireEvent.click(within(pluginRow as HTMLTableRowElement).getByRole('button', { name: /禁\s*用/ }));

    await waitFor(() => expect(mockListFull).toHaveBeenCalledTimes(1));
    expect(mockFeedback.error).not.toHaveBeenCalled();
    const dependencyModal = await screen.findByTestId('plugin-dependency-modal');
    expect(within(dependencyModal).getByText('机床采集')).toBeInTheDocument();
    expect(mockDisablePlugin).not.toHaveBeenCalled();
  });
});
