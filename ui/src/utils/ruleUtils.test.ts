import { applyStoredNodePositions, dehydrateRule, hydrateRule, NODE_POSITIONS_METADATA_KEY, SOURCE_NODE_ID } from './ruleUtils';
import { RuleFlow } from '../types';

describe('ruleUtils', () => {
  test('hydrateRule strips stale visual source node for plugin-source rules', () => {
    const rule: RuleFlow = {
      id: 'rule-1',
      name: 'rule-1',
      version: 1,
      enabled: false,
      debug_mode: false,
      description: '',
      source: {
        type: 's7',
        topic: 'data.ingest.s7.default',
        data_format: 'json',
        config: { host: '127.0.0.1' },
      },
      metadata: {
        source_wires: [],
        source_position: { x: 50, y: 50 },
      },
      nodes: [
        {
          id: SOURCE_NODE_ID,
          type: 'source-s7-ingress',
          name: 'Data Source',
          config: {},
          wires: [],
        },
        {
          id: 'src_s7',
          type: 'source',
          plugin: 'source-s7',
          name: 'source-s7 acm006',
          config: { connection: { host: '127.0.0.1' } },
          wires: ['sink_mqtt'],
        },
        {
          id: 'sink_mqtt',
          type: 'sink',
          plugin: 'sink-mqtt',
          name: 'sink-mqtt telemetry',
          config: {},
          wires: [],
        },
      ],
    };

    const hydrated = hydrateRule(rule);

    expect(hydrated.source).toBeUndefined();
    expect(hydrated.nodes.map((node) => node.id)).toEqual(['src_s7', 'sink_mqtt']);
  });

  test('dehydrateRule removes stale non-legacy visual source node', () => {
    const rule: RuleFlow = {
      id: 'rule-2',
      name: 'rule-2',
      version: 1,
      enabled: false,
      debug_mode: false,
      description: '',
      source: {
        type: 's7',
        topic: 'data.ingest.s7.default',
        data_format: 'json',
        config: {},
      },
      nodes: [
        {
          id: SOURCE_NODE_ID,
          type: 'source-s7-ingress',
          name: 'Data Source',
          config: {},
          wires: [],
        },
        {
          id: 'src_s7',
          type: 'source',
          plugin: 'source-s7',
          name: 'source-s7',
          config: {},
          wires: [],
        },
      ],
    };

    const dehydrated = dehydrateRule(rule);

    expect(dehydrated.nodes.map((node) => node.id)).toEqual(['src_s7']);
    expect(dehydrated.source).toEqual(rule.source);
  });

  test('dehydrateRule keeps legacy ingress source in source field and removes visual node', () => {
    const rule: RuleFlow = {
      id: 'rule-3',
      name: 'rule-3',
      version: 1,
      enabled: false,
      debug_mode: false,
      description: '',
      metadata: {
        source_wires: ['sink_1'],
        source_position: { x: 10, y: 20 },
      },
      nodes: [
        {
          id: SOURCE_NODE_ID,
          type: 'source-http-ingress',
          name: 'Data Source',
          config: { path: '/demo' },
          wires: ['sink_1'],
        },
        {
          id: 'sink_1',
          type: 'sink',
          plugin: 'sink-mqtt',
          name: 'sink-mqtt',
          config: {},
          wires: [],
        },
      ],
    };

    const dehydrated = dehydrateRule(rule);

    expect(dehydrated.source).toEqual({
      type: 'http',
      topic: 'data.ingest.http.default',
      data_format: 'json',
      config: { path: '/demo' },
    });
    expect(dehydrated.nodes.map((node) => node.id)).toEqual(['sink_1']);
  });

  test('applyStoredNodePositions restores node layout from metadata', () => {
    const rule: RuleFlow = {
      id: 'rule-4',
      name: 'rule-4',
      version: 1,
      enabled: false,
      debug_mode: false,
      description: '',
      metadata: {
        [NODE_POSITIONS_METADATA_KEY]: {
          src_s7: { x: 120, y: 48 },
          sink_mqtt: { x: 420, y: 64 },
        },
      },
      nodes: [
        {
          id: 'src_s7',
          type: 'source',
          plugin: 'source-s7',
          name: 'source-s7',
          config: {},
          wires: ['sink_mqtt'],
        },
        {
          id: 'sink_mqtt',
          type: 'sink',
          plugin: 'sink-mqtt',
          name: 'sink-mqtt',
          config: {},
          wires: [],
        },
      ],
    };

    const restored = applyStoredNodePositions(rule);

    expect(restored.nodes[0].metadata?.position).toEqual({ x: 120, y: 48 });
    expect(restored.nodes[1].metadata?.position).toEqual({ x: 420, y: 64 });
  });

  test('dehydrateRule persists node positions into metadata', () => {
    const rule: RuleFlow = {
      id: 'rule-5',
      name: 'rule-5',
      version: 1,
      enabled: false,
      debug_mode: false,
      description: '',
      metadata: {},
      nodes: [
        {
          id: 'src_s7',
          type: 'source',
          plugin: 'source-s7',
          name: 'source-s7',
          config: {},
          metadata: {
            position: { x: 88, y: 96 },
          },
          wires: ['sink_mqtt'],
        },
        {
          id: 'sink_mqtt',
          type: 'sink',
          plugin: 'sink-mqtt',
          name: 'sink-mqtt',
          metadata: {
            position: { x: 388, y: 102 },
          },
          config: {},
          wires: [],
        },
      ],
    };

    const dehydrated = dehydrateRule(rule);

    expect(dehydrated.metadata?.[NODE_POSITIONS_METADATA_KEY]).toEqual({
      src_s7: { x: 88, y: 96 },
      sink_mqtt: { x: 388, y: 102 },
    });
  });
});
