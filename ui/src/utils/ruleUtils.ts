// Helper utilities for Rule Source <-> Node conversion

import { RuleFlow, NodeConfig, SourceConfig, Position } from '../types';

export const SOURCE_NODE_ID = 'rule_source_node';
export const NODE_POSITIONS_METADATA_KEY = 'node_positions';

type NodePositionMap = Record<string, Position>;

const isValidPosition = (value: any): value is Position => (
  value &&
  typeof value === 'object' &&
  typeof value.x === 'number' &&
  typeof value.y === 'number'
);

const getStoredNodePositions = (rule: RuleFlow): NodePositionMap => {
  const raw = rule.metadata?.[NODE_POSITIONS_METADATA_KEY];
  if (!raw || typeof raw !== 'object') {
    return {};
  }

  return Object.entries(raw as Record<string, unknown>).reduce<NodePositionMap>((acc, [nodeId, position]) => {
    if (isValidPosition(position)) {
      acc[nodeId] = position;
    }
    return acc;
  }, {});
};

export const buildNodePositionMap = (nodes: NodeConfig[] = []): NodePositionMap => (
  nodes.reduce<NodePositionMap>((acc, node) => {
    const position = node.metadata?.position;
    if (isValidPosition(position)) {
      acc[node.id] = position;
    }
    return acc;
  }, {})
);

export const applyStoredNodePositions = (rule: RuleFlow): RuleFlow => {
  const storedNodePositions = getStoredNodePositions(rule);
  if (Object.keys(storedNodePositions).length === 0) {
    return rule;
  }

  let changed = false;
  const nodes = (rule.nodes || []).map((node) => {
    const storedPosition = storedNodePositions[node.id];
    if (!storedPosition) {
      return node;
    }

    const currentPosition = node.metadata?.position;
    if (isValidPosition(currentPosition) &&
      currentPosition.x === storedPosition.x &&
      currentPosition.y === storedPosition.y) {
      return node;
    }

    changed = true;
    return {
      ...node,
      metadata: {
        ...(node.metadata || {}),
        position: storedPosition,
      },
    };
  });

  if (!changed) {
    return rule;
  }

  return {
    ...rule,
    nodes,
  };
};

export const persistNodePositions = (rule: RuleFlow): RuleFlow => {
  const normalizedRule = applyStoredNodePositions(rule);
  const nodePositions = buildNodePositionMap(normalizedRule.nodes || []);
  const previousNodePositions = getStoredNodePositions(normalizedRule);

  const sameKeys =
    Object.keys(nodePositions).length === Object.keys(previousNodePositions).length &&
    Object.keys(nodePositions).every((key) => {
      const nextPosition = nodePositions[key];
      const prevPosition = previousNodePositions[key];
      return prevPosition && prevPosition.x === nextPosition.x && prevPosition.y === nextPosition.y;
    });

  if (sameKeys) {
    return normalizedRule;
  }

  return {
    ...normalizedRule,
    metadata: {
      ...(normalizedRule.metadata || {}),
      [NODE_POSITIONS_METADATA_KEY]: nodePositions,
    },
  };
};

const normalizeSourceNodeType = (type: string): string => {
  if (!type.includes('-')) {
    return `source-${type}-ingress`;
  }
  if (type.endsWith('-source') && !type.startsWith('source-')) {
    const baseType = type.replace(/-source$/, '');
    return `source-${baseType}-ingress`;
  }
  return type;
};

const isLegacyIngressSourceType = (nodeType: string): boolean => (
  nodeType === 'source-http-ingress' ||
  nodeType === 'source-mqtt-ingress' ||
  nodeType === 'source-command-ingress' ||
  nodeType === 'http-source' ||
  nodeType === 'mqtt-source' ||
  nodeType === 'command-source' ||
  nodeType === 'http' ||
  nodeType === 'mqtt' ||
  nodeType === 'command'
);

const isPluginSourceNode = (node: NodeConfig | null | undefined): boolean => {
  if (!node || node.id === SOURCE_NODE_ID) {
    return false;
  }

  if (node.type === 'source') {
    return true;
  }

  return typeof node.plugin === 'string' && node.plugin.startsWith('source-');
};

/**
 * Hydrates a RuleFlow by converting its SourceConfig into a visible SourceNode.
 * Returns a new RuleFlow object with the Source Node added to the nodes list.
 */
export const hydrateRule = (rule: RuleFlow): RuleFlow => {
  const normalizedRule = applyStoredNodePositions(rule);
  const existingNodes = normalizedRule.nodes || [];
  const cleanNodes = existingNodes.filter((node) => node && node.id !== SOURCE_NODE_ID);
  const removedVisualSourceNode = cleanNodes.length !== existingNodes.length;
  const hasPluginSource = cleanNodes.some(isPluginSourceNode);

  if (hasPluginSource) {
    if (!removedVisualSourceNode && !normalizedRule.source) {
      return normalizedRule;
    }

    return {
      ...normalizedRule,
      source: undefined,
      nodes: cleanNodes,
    };
  }

  if (!normalizedRule.source) {
    if (!removedVisualSourceNode) {
      return normalizedRule;
    }

    return {
      ...normalizedRule,
      nodes: cleanNodes,
    };
  }

  // Retrieve source wires and position from metadata if available
  const sourceWires: string[] = normalizedRule.metadata?.source_wires || [];
  const sourcePosition = normalizedRule.metadata?.source_position || { x: 50, y: 50 };

  // Backend uses plugin IDs like "source-http-ingress", "source-mqtt-ingress"
  // Support both old format (http-source) and new format (source-http-ingress)
  const nodeType = normalizeSourceNodeType(normalizedRule.source.type);

  if (!isLegacyIngressSourceType(nodeType)) {
    if (!removedVisualSourceNode) {
      return normalizedRule;
    }
    return {
      ...normalizedRule,
      nodes: cleanNodes
    };
  }

  const sourceNode: NodeConfig = {
    id: SOURCE_NODE_ID,
    type: nodeType,
    name: 'Data Source',
    config: normalizedRule.source?.config || {},
    wires: sourceWires,
    metadata: {
      position: sourcePosition
    }
  };

  // Apply defaults if config is empty (for new Source plugins)
  if (Object.keys(sourceNode.config).length === 0) {
      if (nodeType === 'source-http-ingress' || nodeType === 'http-source' || nodeType === 'http') {
          sourceNode.config = {
              process: {
                  decode: { body: 'auto' }
              }
          };
      }
  }

  return {
    ...normalizedRule,
    nodes: [sourceNode, ...cleanNodes]
  };
};

/**
 * Dehydrates a RuleFlow by extracting the SourceNode back into SourceConfig.
 */
export const dehydrateRule = (rule: RuleFlow): RuleFlow => {
  const normalizedRule = persistNodePositions(rule);
  const cleanNodes = (normalizedRule.nodes || []).filter((node) => node && node.id !== SOURCE_NODE_ID);
  // Find source node by ID
  const sourceNode = normalizedRule.nodes?.find(n => n && n.id === SOURCE_NODE_ID);

  if (!sourceNode) {
    if (cleanNodes.length === (normalizedRule.nodes || []).length) {
      return normalizedRule;
    }

    return {
      ...normalizedRule,
      nodes: cleanNodes,
    };
  }

  const normalizedType = normalizeSourceNodeType(sourceNode.type);
  if (!isLegacyIngressSourceType(normalizedType)) {
    return {
      ...normalizedRule,
      nodes: cleanNodes,
    };
  }

  // Parse type: "source-http-ingress" -> "http" (for the .source field)
  // Support both old format (http-source) and new format (source-http-ingress)
  let type = sourceNode.type;
  if (type.startsWith('source-') && type.endsWith('-ingress')) {
    // New format: "source-http-ingress" -> "http"
    type = type.replace(/^source-/, '').replace(/-ingress$/, '');
  } else if (type.endsWith('-source')) {
    // Old format: "http-source" -> "http"
    type = type.replace(/-source$/, '');
  }

  const sourceConfig: SourceConfig = {
    type: type,
    topic: (sourceNode.config.topic as string) || `data.ingest.${type}.default`,
    data_format: (sourceNode.config.data_format as string) || 'json',
    config: sourceNode.config
  };

  // Persist source wires and position in metadata
  const metadata = {
      ...normalizedRule.metadata,
      source_wires: sourceNode.wires || [],
      source_position: sourceNode.metadata?.position || { x: 50, y: 50 }
  };

  return {
    ...normalizedRule,
    metadata,
    source: sourceConfig,
    nodes: cleanNodes
  };
};
