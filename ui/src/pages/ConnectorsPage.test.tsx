import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ConnectorsPage } from './ConnectorsPage';
import { connectorsAPI, pluginsAPI } from '../services/api';
import {
  connectorStatusLabel,
  defaultConnectorSessionPolicy,
  effectiveConnectorSessionPolicy,
} from '../utils/connectorLifecycle';

jest.mock('../services/api', () => ({
  connectorsAPI: {
    list: jest.fn(),
    create: jest.fn(),
    update: jest.fn(),
    delete: jest.fn(),
  },
  pluginsAPI: {
    listPluginInfos: jest.fn(),
  },
}));

const mockFeedback = {
  success: jest.fn(),
  error: jest.fn(),
  info: jest.fn(),
  warning: jest.fn(),
  confirm: jest.fn(),
  prompt: jest.fn(),
};

jest.mock('../components/FeedbackProvider', () => ({
  useFeedback: () => mockFeedback,
}));

jest.mock('../components/LazyJSONSchemaRenderer', () => ({
  LazyJSONSchemaRenderer: () => <div>Schema Renderer</div>,
}));

jest.mock('../components/ResourceTabs', () => ({
  ResourceTabs: () => <div>资源页签</div>,
}));

const mockedConnectorsAPI = connectorsAPI as jest.Mocked<typeof connectorsAPI>;
const mockedPluginsAPI = pluginsAPI as jest.Mocked<typeof pluginsAPI>;

describe('ConnectorsPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders connector cards with summary details', async () => {
    mockedPluginsAPI.listPluginInfos.mockResolvedValue([
      {
        id: 'connector-timescaledb',
        name: 'Timescale Connector',
        version: '1.0.0',
        type: 'connector',
        category: 'db',
        description: 'timescaledb connector',
        capabilities: { input_ports: 0, output_ports: 0, support_batch: false, support_ack: false, config_schema: '{}' },
        connector_descriptor: {
          family: 'timescaledb',
          label: 'TimescaleDB',
          display_fields: ['host', 'database'],
        },
      },
    ] as any);

    mockedConnectorsAPI.list.mockResolvedValue([
      {
        id: 'ts-main',
        name: '主库',
        plugin_id: 'connector-timescaledb',
        enabled: true,
        connection: { host: '127.0.0.1', database: 'punk' },
        connection_summary: '127.0.0.1 / punk',
        connection_preview: { host: '127.0.0.1', database: 'punk' },
        status: { status: 'healthy', message: 'ok' },
        ref_count: 2,
        pending: 0,
        inflight_weight: 0,
        plugin_info: {
          connector_descriptor: {
            family: 'timescaledb',
            label: 'TimescaleDB',
            display_fields: ['host', 'database'],
          },
        },
      },
    ] as any);

    render(<ConnectorsPage />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Connector 资源' })).toBeInTheDocument();
      expect(screen.getByLabelText('搜索连接资源')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: '新建 Connector' })).toBeInTheDocument();
      expect(screen.getByText('主库')).toBeInTheDocument();
      expect(screen.getByText('连接摘要')).toBeInTheDocument();
      expect(screen.getByText('127.0.0.1 / punk')).toBeInTheDocument();
      expect(screen.getByText('引用数')).toBeInTheDocument();
    });
    expect(screen.queryByText('资源总数')).not.toBeInTheDocument();
  });

  test('paginates connector cards with 20 items per page by default', async () => {
    mockedPluginsAPI.listPluginInfos.mockResolvedValue([] as any);
    mockedConnectorsAPI.list.mockResolvedValue(Array.from({ length: 21 }, (_, index) => ({
      id: `connector-${String(index + 1).padStart(2, '0')}`,
      name: `Connector ${String(index + 1).padStart(2, '0')}`,
      plugin_id: 'connect-redis',
      enabled: true,
      connection: {},
    })) as any);

    render(<ConnectorsPage />);

    expect(await screen.findByText('Connector 01')).toBeInTheDocument();
    expect(screen.getByText('Connector 20')).toBeInTheDocument();
    expect(screen.queryByText('Connector 21')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTitle('2'));
    expect(await screen.findByText('Connector 21')).toBeInTheDocument();
    expect(screen.queryByText('Connector 01')).not.toBeInTheDocument();
  });

	test('uses lifecycle capabilities for connector defaults and on-demand status labels', () => {
	const s7Plugin = {
		connector_descriptor: { family: 's7', capabilities: ['read', 'session:scheduled_reuse', 'session:per_operation'] },
	} as any;
	expect(defaultConnectorSessionPolicy(s7Plugin)).toBe('scheduled_reuse');
	expect(effectiveConnectorSessionPolicy({
		id: 's7-legacy', plugin_id: 'connect-s7', enabled: true,
		session_policy: 'shared_duplex', plugin_info: s7Plugin,
	} as any)).toBe('scheduled_reuse');
	expect(connectorStatusLabel({
		id: 's7-cnc107', plugin_id: 'connect-s7', enabled: true,
		session_policy: 'scheduled_reuse', status: { status: 'healthy', reason_code: 'OPERATION_OK' },
		plugin_info: s7Plugin,
	} as any)).toBe('Ready');
	expect(connectorStatusLabel({
		id: 's7-cnc107', plugin_id: 'connect-s7', enabled: true,
		session_policy: 'per_operation', status: { status: 'disconnected' }, plugin_info: s7Plugin,
	} as any)).toBe('Unreachable');
  });
});
