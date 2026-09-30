import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { RuntimeDAGPage } from './RuntimeDAGPage';

const mockNavigate = jest.fn();
const mockGetInstance = jest.fn();
const mockGetRule = jest.fn();
const mockFitView = jest.fn();
const mockGetViewport = jest.fn();
const mockGetNodes = jest.fn();

jest.mock('react-router-dom', () => ({
  useParams: () => ({ instanceId: 'inst-1' }),
  useNavigate: () => mockNavigate,
}));

jest.mock('reactflow', () => ({
  __esModule: true,
  default: ({
    nodes,
    edges,
    style,
    onInit,
  }: {
    nodes?: Array<{ id: string; data?: { label?: string } }>;
    edges?: Array<{ id: string }>;
    style?: React.CSSProperties;
    onInit?: (instance: {
      fitView: (...args: unknown[]) => void;
      getViewport: (...args: unknown[]) => { x: number; y: number; zoom: number };
      getNodes: (...args: unknown[]) => Array<{
        id: string;
        width?: number;
        height?: number;
        position?: { x: number; y: number };
        positionAbsolute?: { x: number; y: number };
      }>;
    }) => void;
  }) => {
    const mockReact = require('react');

    mockReact.useEffect(() => {
      onInit?.({
        fitView: mockFitView,
        getViewport: mockGetViewport,
        getNodes: mockGetNodes,
      });
    }, []);

    return (
      <div data-testid="runtime-reactflow" style={style}>
        <div data-testid="runtime-node-labels">
          {(nodes || []).map((node) => (
            <span key={node.id}>{node.data?.label || node.id}</span>
          ))}
        </div>
        <div data-testid="runtime-edge-ids">
          {(edges || []).map((edge) => (
            <span key={edge.id}>{edge.id}</span>
          ))}
        </div>
      </div>
    );
  },
  Background: () => null,
  Controls: () => null,
  MiniMap: () => null,
  MarkerType: {
    ArrowClosed: 'arrowclosed',
  },
}));

jest.mock('../services/api', () => ({
  runtimeAPI: {
    getInstance: (...args: unknown[]) => mockGetInstance(...args),
  },
  rulesAPI: {
    get: (...args: unknown[]) => mockGetRule(...args),
  },
}));

describe('RuntimeDAGPage', () => {
  let consoleErrorSpy: jest.SpyInstance;
  let clientWidthDescriptor: PropertyDescriptor | undefined;
  let clientHeightDescriptor: PropertyDescriptor | undefined;
  let rectSpy: jest.SpyInstance;
  let resizeObserverCallback: ResizeObserverCallback | null;
  let originalResizeObserver: typeof ResizeObserver | undefined;

  beforeEach(() => {
    jest.clearAllMocks();
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    clientWidthDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth');
    clientHeightDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientHeight');
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true,
      get: () => 1280,
    });
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
      configurable: true,
      get: () => 720,
    });
    rectSpy = jest.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(() => ({
      width: 1280,
      height: 720,
      top: 0,
      left: 0,
      right: 1280,
      bottom: 720,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    }));
    mockGetViewport.mockReturnValue({ x: 93.5, y: 27, zoom: 2 });
    mockGetNodes.mockReturnValue([
      {
        id: 'node_a',
        width: 130,
        height: 63,
        position: { x: 80, y: 80 },
        positionAbsolute: { x: 80, y: 80 },
      },
    ]);
    resizeObserverCallback = null;
    originalResizeObserver = global.ResizeObserver;
    global.ResizeObserver = class ResizeObserver {
      constructor(callback: ResizeObserverCallback) {
        resizeObserverCallback = callback;
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    } as typeof ResizeObserver;
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    rectSpy.mockRestore();
    if (clientWidthDescriptor) {
      Object.defineProperty(HTMLElement.prototype, 'clientWidth', clientWidthDescriptor);
    }
    if (clientHeightDescriptor) {
      Object.defineProperty(HTMLElement.prototype, 'clientHeight', clientHeightDescriptor);
    }
    global.ResizeObserver = originalResizeObserver as typeof ResizeObserver;
  });

  it('falls back to runtime topology when the rule definition cannot be loaded', async () => {
    mockGetInstance.mockResolvedValue({
      id: 'inst-1',
      rule_id: 'rule-1',
      version: '3',
      status: 'running',
      nodes: {
        'inst-1-source_1': {
          id: 'inst-1-source_1',
          node_id: 'source_1',
          plugin_id: 'source',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: [],
          output_edges: ['source_1-sink_1'],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
        'inst-1-sink_1': {
          id: 'inst-1-sink_1',
          node_id: 'sink_1',
          plugin_id: 'sink',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: ['source_1-sink_1'],
          output_edges: [],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
      },
      edges: {
        'inst-1-source_1-sink_1': {
          id: 'inst-1-source_1-sink_1',
          edge_id: 'source_1-sink_1',
          from_node: 'source_1',
          to_node: 'sink_1',
          topic: 'rule.edge.rule-1.source_1.sink_1',
          metrics: { events_published: 5, events_consumed: 5, events_dropped: 0 },
        },
      },
      metrics: { events_in: 5, events_out: 5, events_failed: 0 },
    });
    mockGetRule.mockRejectedValue(new Error('rule not found'));

    render(<RuntimeDAGPage />);

    await waitFor(() => expect(mockGetInstance).toHaveBeenCalledWith('inst-1'));
    await waitFor(() => expect(screen.getByText('source_1')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText(/已回退到运行态拓扑/i)).toBeInTheDocument());

    expect(screen.getByText('sink_1')).toBeInTheDocument();
    expect(screen.getByText('source_1-sink_1')).toBeInTheDocument();
    expect(screen.getByText('节点数：2')).toBeInTheDocument();
    expect(screen.getByText('边数：1')).toBeInTheDocument();
    expect(screen.getByTestId('runtime-dag-canvas')).toHaveStyle({ width: '100%', height: 'calc(100vh - 320px)' });
    expect(screen.getByTestId('runtime-reactflow')).toHaveStyle({ width: '100%', height: '720px' });
    expect(screen.queryByText(/正在初始化画布/i)).not.toBeInTheDocument();
  });

  it('still renders the DAG when the canvas host initially measures as zero-sized', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true,
      get: () => 0,
    });
    Object.defineProperty(HTMLElement.prototype, 'clientHeight', {
      configurable: true,
      get: () => 0,
    });
    rectSpy.mockImplementation(() => ({
      width: 0,
      height: 0,
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    }));

    mockGetInstance.mockResolvedValue({
      id: 'inst-1',
      rule_id: 'rule-1',
      version: '3',
      status: 'running',
      nodes: {
        'inst-1-node_a': {
          id: 'inst-1-node_a',
          node_id: 'node_a',
          plugin_id: 'source',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: [],
          output_edges: ['node_a-node_b'],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
        'inst-1-node_b': {
          id: 'inst-1-node_b',
          node_id: 'node_b',
          plugin_id: 'sink',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: ['node_a-node_b'],
          output_edges: [],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
      },
      edges: {
        'inst-1-node_a-node_b': {
          id: 'inst-1-node_a-node_b',
          edge_id: 'node_a-node_b',
          from_node: 'node_a',
          to_node: 'node_b',
          topic: 'rule.edge.rule-1.node_a.node_b',
          metrics: { events_published: 5, events_consumed: 5, events_dropped: 0 },
        },
      },
      metrics: { events_in: 5, events_out: 5, events_failed: 0 },
    });
    mockGetRule.mockResolvedValue({
      id: 'rule-1',
      name: 'Rule 1',
      version: 3,
      enabled: true,
      debug_mode: false,
      description: '',
      nodes: [
        {
          id: 'node_a',
          type: 'source',
          name: 'Node A',
          config: {},
          wires: ['node_b'],
          metadata: { position: { x: 80, y: 80 } },
        },
        {
          id: 'node_b',
          type: 'sink',
          name: 'Node B',
          config: {},
          wires: [],
          metadata: { position: { x: 320, y: 80 } },
        },
      ],
      metadata: {},
    });

    render(<RuntimeDAGPage />);

    await waitFor(() => expect(mockGetInstance).toHaveBeenCalledWith('inst-1'));
    await waitFor(() => expect(screen.getByTestId('runtime-reactflow')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText('Node A')).toBeInTheDocument());

    expect(screen.getByText('Node B')).toBeInTheDocument();
    expect(screen.queryByText(/正在初始化画布/i)).not.toBeInTheDocument();
  });

  it('loads runtime data once instead of polling the instance endpoint repeatedly', async () => {
    jest.useFakeTimers();

    mockGetInstance.mockResolvedValue({
      id: 'inst-1',
      rule_id: 'rule-1',
      version: '3',
      status: 'running',
      nodes: {
        'inst-1-node_a': {
          id: 'inst-1-node_a',
          node_id: 'node_a',
          plugin_id: 'source',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: [],
          output_edges: ['node_a-node_b'],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
      },
      edges: {},
      metrics: { events_in: 5, events_out: 5, events_failed: 0 },
    });
    mockGetRule.mockResolvedValue({
      id: 'rule-1',
      name: 'Rule 1',
      version: 3,
      enabled: true,
      debug_mode: false,
      description: '',
      nodes: [
        {
          id: 'node_a',
          type: 'source',
          name: 'Node A',
          config: {},
          wires: [],
          metadata: { position: { x: 80, y: 80 } },
        },
      ],
      metadata: {},
    });

    render(<RuntimeDAGPage />);

    await waitFor(() => expect(mockGetInstance).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(mockGetRule).toHaveBeenCalledTimes(1));

    jest.advanceTimersByTime(12000);

    expect(mockGetInstance).toHaveBeenCalledTimes(1);
    expect(mockGetRule).toHaveBeenCalledTimes(1);

    jest.useRealTimers();
  });

  it('re-fits the viewport after the DAG nodes are rendered', async () => {
    mockGetInstance.mockResolvedValue({
      id: 'inst-1',
      rule_id: 'rule-1',
      version: '3',
      status: 'running',
      nodes: {
        'inst-1-node_a': {
          id: 'inst-1-node_a',
          node_id: 'node_a',
          plugin_id: 'source',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: [],
          output_edges: ['node_a-node_b'],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
        'inst-1-node_b': {
          id: 'inst-1-node_b',
          node_id: 'node_b',
          plugin_id: 'sink',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: ['node_a-node_b'],
          output_edges: [],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
      },
      edges: {
        'inst-1-node_a-node_b': {
          id: 'inst-1-node_a-node_b',
          edge_id: 'node_a-node_b',
          from_node: 'node_a',
          to_node: 'node_b',
          topic: 'rule.edge.rule-1.node_a.node_b',
          metrics: { events_published: 5, events_consumed: 5, events_dropped: 0 },
        },
      },
      metrics: { events_in: 5, events_out: 5, events_failed: 0 },
    });
    mockGetRule.mockResolvedValue({
      id: 'rule-1',
      name: 'Rule 1',
      version: 3,
      enabled: true,
      debug_mode: false,
      description: '',
      nodes: [
        {
          id: 'node_a',
          type: 'source',
          name: 'Node A',
          config: {},
          wires: ['node_b'],
          metadata: { position: { x: 80, y: 80 } },
        },
        {
          id: 'node_b',
          type: 'sink',
          name: 'Node B',
          config: {},
          wires: [],
          metadata: { position: { x: 320, y: 80 } },
        },
      ],
      metadata: {},
    });

    render(<RuntimeDAGPage />);

    await waitFor(() => expect(screen.getByText('Node A')).toBeInTheDocument());
    await waitFor(() => expect(mockFitView).toHaveBeenCalled());
  });

  it('re-fits the viewport when the DAG canvas container resizes after first render', async () => {
    mockGetInstance.mockResolvedValue({
      id: 'inst-1',
      rule_id: 'rule-1',
      version: '3',
      status: 'running',
      nodes: {
        'inst-1-node_a': {
          id: 'inst-1-node_a',
          node_id: 'node_a',
          plugin_id: 'source',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: [],
          output_edges: ['node_a-node_b'],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
        'inst-1-node_b': {
          id: 'inst-1-node_b',
          node_id: 'node_b',
          plugin_id: 'sink',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: ['node_a-node_b'],
          output_edges: [],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
      },
      edges: {
        'inst-1-node_a-node_b': {
          id: 'inst-1-node_a-node_b',
          edge_id: 'node_a-node_b',
          from_node: 'node_a',
          to_node: 'node_b',
          topic: 'rule.edge.rule-1.node_a.node_b',
          metrics: { events_published: 5, events_consumed: 5, events_dropped: 0 },
        },
      },
      metrics: { events_in: 5, events_out: 5, events_failed: 0 },
    });
    mockGetRule.mockResolvedValue({
      id: 'rule-1',
      name: 'Rule 1',
      version: 3,
      enabled: true,
      debug_mode: false,
      description: '',
      nodes: [
        {
          id: 'node_a',
          type: 'source',
          name: 'Node A',
          config: {},
          wires: ['node_b'],
          metadata: { position: { x: 80, y: 80 } },
        },
        {
          id: 'node_b',
          type: 'sink',
          name: 'Node B',
          config: {},
          wires: [],
          metadata: { position: { x: 320, y: 80 } },
        },
      ],
      metadata: {},
    });

    render(<RuntimeDAGPage />);

    await waitFor(() => expect(screen.getByText('Node A')).toBeInTheDocument());
    expect(resizeObserverCallback).not.toBeNull();

    mockFitView.mockClear();
    resizeObserverCallback?.([], {} as ResizeObserver);

    await waitFor(() => expect(mockFitView).toHaveBeenCalled());
  });

  it('does not render the temporary debug panel after the fix is stabilized', async () => {
    mockGetInstance.mockResolvedValue({
      id: 'inst-1',
      rule_id: 'rule-1',
      version: '3',
      status: 'running',
      nodes: {
        'inst-1-node_a': {
          id: 'inst-1-node_a',
          node_id: 'node_a',
          plugin_id: 'source',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: [],
          output_edges: ['node_a-node_b'],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
      },
      edges: {},
      metrics: { events_in: 5, events_out: 5, events_failed: 0 },
    });
    mockGetRule.mockResolvedValue({
      id: 'rule-1',
      name: 'Rule 1',
      version: 3,
      enabled: true,
      debug_mode: false,
      description: '',
      nodes: [
        {
          id: 'node_a',
          type: 'source',
          name: 'Node A',
          config: {},
          wires: [],
          metadata: { position: { x: 80, y: 80 } },
        },
      ],
      metadata: {},
    });

    render(<RuntimeDAGPage />);

    await waitFor(() => expect(screen.getByText('Node A')).toBeInTheDocument());
    expect(screen.queryByText(/Debug View/i)).not.toBeInTheDocument();
    expect(screen.getByTestId('runtime-dag-inner')).toHaveStyle({ height: '720px' });
    await waitFor(() => expect(screen.getByTestId('runtime-reactflow')).toHaveStyle({ height: '720px' }));
  });

  it('keeps retrying fitView during the initial layout warmup window', async () => {
    jest.useFakeTimers();

    mockGetInstance.mockResolvedValue({
      id: 'inst-1',
      rule_id: 'rule-1',
      version: '3',
      status: 'running',
      nodes: {
        'inst-1-node_a': {
          id: 'inst-1-node_a',
          node_id: 'node_a',
          plugin_id: 'source',
          version: '3',
          config: {},
          status: 'running',
          started_at: '2026-04-15T12:00:00Z',
          input_edges: [],
          output_edges: ['node_a-node_b'],
          metrics: { events_in: 5, events_out: 5, events_failed: 0, avg_latency_ms: 0 },
        },
      },
      edges: {},
      metrics: { events_in: 5, events_out: 5, events_failed: 0 },
    });
    mockGetRule.mockResolvedValue({
      id: 'rule-1',
      name: 'Rule 1',
      version: 3,
      enabled: true,
      debug_mode: false,
      description: '',
      nodes: [
        {
          id: 'node_a',
          type: 'source',
          name: 'Node A',
          config: {},
          wires: [],
          metadata: { position: { x: 80, y: 80 } },
        },
      ],
      metadata: {},
    });

    render(<RuntimeDAGPage />);

    await waitFor(() => expect(screen.getByText('Node A')).toBeInTheDocument());

    mockFitView.mockClear();
    jest.advanceTimersByTime(900);

    expect(mockFitView.mock.calls.length).toBeGreaterThanOrEqual(3);

    jest.useRealTimers();
  });
});
