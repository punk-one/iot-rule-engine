import React from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { NodeInspectorPage } from './NodeInspectorPage';
import { runtimeAPI } from '../services/api';
import { NodeInstance, TraceEvent, TraceEventsResponse } from '../types';

jest.mock('../services/api', () => ({
  runtimeAPI: {
    getNodeInstance: jest.fn(),
    getInstance: jest.fn(),
    getTraceEvents: jest.fn(),
  },
}));

jest.mock('react-router-dom');

const mockedRuntimeAPI = runtimeAPI as jest.Mocked<typeof runtimeAPI>;

const nodeInstanceFixture: NodeInstance = {
  id: 'runtime-node-map_zh_jzl',
  node_id: 'map_zh_jzl',
  plugin_id: 'transform-map',
  version: '1.0.0',
  config: {
    process: {
      expand: [],
      template:
        '{\n  "time": "{{ unixMilli .payload.time }}",\n  "device_code": "{{ .payload.device_code }}",\n  "event_type": "report",\n  "payload": "{{ json .payload.data }}",\n  "source_system": "gateway",\n  "transport_protocol": "mqtt",\n  "created_at": "{{ now }}",\n  "updated_at": "{{ now }}"\n}',
    },
  },
  status: 'running',
  started_at: '2026-04-14T17:00:00Z',
  input_edges: ['edge-in'],
  output_edges: ['edge-out'],
  metrics: {
    events_in: 10,
    events_out: 10,
    events_failed: 0,
    avg_latency_ms: 2,
    last_event_at: '2026-04-14T17:00:05Z',
  },
  error_history: [],
};

const longEventID = '01JZ8ZK4M8J4W4RWK7TTM6F5G7-TRACE-EVENT-ID-VERY-LONG';
const longMessage =
  'transform-map output payload contains a very long diagnostic message with nested context ' +
  'ZH_DYZZ_0002-1776150552946333600 and {"flow_01":123,"flow_02":456,"nested":"value"} ' +
  'so the trace table must keep the entire string visible without truncation.';

const traceEventsFixture: TraceEvent[] = [
  {
    instance_id: 'instance-1',
    trace_id: 'trace-1',
    rule_id: 'tele_zh_jzl_to_timescaledb',
    node_id: 'map_zh_jzl',
    event_id: longEventID,
    phase: 'done',
    status: 'success',
    message: longMessage,
    timestamp: '2026-04-14T17:00:05Z',
    observed_at: '2026-04-14T17:00:05Z',
    event_timestamp: '2026-04-14T17:00:04Z',
  },
];

const traceResponseFixture: TraceEventsResponse = {
  debug_enabled: true,
  events: traceEventsFixture,
  count: traceEventsFixture.length,
};

function renderPage() {
  return render(<NodeInspectorPage />);
}

describe('NodeInspectorPage trace table', () => {
  beforeEach(() => {
    mockedRuntimeAPI.getNodeInstance.mockResolvedValue(nodeInstanceFixture);
    mockedRuntimeAPI.getInstance.mockResolvedValue({
      id: 'instance-1',
      rule_id: 'rule-1',
      version: '1.0.0',
      debug_mode: false,
      status: 'running',
      created_at: '2026-04-14T17:00:00Z',
      started_at: '2026-04-14T17:00:00Z',
      nodes: {},
      edges: {
        'edge-in': { edge_id: 'edge-in' },
        'edge-out': { edge_id: 'edge-out' },
      },
      metrics: {},
    } as any);
    mockedRuntimeAPI.getTraceEvents.mockResolvedValue(traceResponseFixture);
    Object.assign(navigator, {
      clipboard: {
        writeText: jest.fn().mockResolvedValue(undefined),
      },
    });
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('renders full event id and full message in trace rows', async () => {
    renderPage();

    await screen.findByText('节点详情: map_zh_jzl');
    fireEvent.click(screen.getByRole('button', { name: /Trace/i }));

    expect(await screen.findByText(longEventID)).toBeInTheDocument();
    expect(screen.getByText(longMessage)).toBeInTheDocument();
    expect(screen.queryByText(/01JZ8ZK4\.\.\./)).not.toBeInTheDocument();
  });

  it('does not link topology edges when the runtime has no edge instances', async () => {
    mockedRuntimeAPI.getInstance.mockResolvedValue({
      id: 'instance-1',
      rule_id: 'rule-1',
      version: '1.0.0',
      debug_mode: false,
      status: 'running',
      created_at: '2026-04-14T17:00:00Z',
      started_at: '2026-04-14T17:00:00Z',
      nodes: {},
      edges: {},
      metrics: {},
    } as any);

    renderPage();

    expect(await screen.findByText('节点详情: map_zh_jzl')).toBeInTheDocument();
    expect(await screen.findByText('edge-in')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'edge-in' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'edge-out' })).not.toBeInTheDocument();
  });

  it('copies full trace values from row actions', async () => {
    const writeText = jest.fn().mockResolvedValue(undefined);
    Object.assign(navigator, {
      clipboard: { writeText },
    });

    renderPage();

    await screen.findByText('节点详情: map_zh_jzl');
    fireEvent.click(screen.getByRole('button', { name: /Trace/i }));
    const eventIDCell = await screen.findByText(longEventID);
    const row = eventIDCell.closest('tr');
    expect(row).not.toBeNull();

    const rowQueries = within(row as HTMLTableRowElement);
    fireEvent.click(rowQueries.getByRole('button', { name: `复制事件ID ${longEventID}` }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(longEventID));

    fireEvent.click(rowQueries.getByRole('button', { name: `复制消息 ${longEventID}` }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(longMessage));
  });

  it('formats nested json strings in the config tab without rendering the raw json panel', async () => {
    renderPage();

    await screen.findByText('节点详情: map_zh_jzl');
    fireEvent.click(screen.getByRole('button', { name: '配置信息' }));

    expect(await screen.findByText('格式化视图')).toBeInTheDocument();
    expect(screen.queryByText('原始 JSON')).not.toBeInTheDocument();
    expect(screen.getByText('template')).toBeInTheDocument();
    expect(screen.getByText('内嵌 JSON 字符串')).toBeInTheDocument();

    const embeddedTemplateBlock = screen.getByText('内嵌 JSON 字符串').parentElement;
    expect(embeddedTemplateBlock).not.toBeNull();
    expect(embeddedTemplateBlock).toHaveTextContent('"device_code": "{{ .payload.device_code }}"');
    expect(embeddedTemplateBlock).toHaveTextContent('"transport_protocol": "mqtt"');
  });

  it('shows debug-disabled hint when trace collection is off', async () => {
    mockedRuntimeAPI.getTraceEvents.mockResolvedValue({
      debug_enabled: false,
      events: [],
      count: 0,
      message: 'debug mode disabled',
    });

    renderPage();

    await screen.findByText('节点详情: map_zh_jzl');
    fireEvent.click(screen.getByRole('button', { name: /Trace/i }));

    expect(await screen.findByText('暂无 Trace 事件')).toBeInTheDocument();
    expect(await screen.findByText('debug mode disabled')).toBeInTheDocument();
  });
});
