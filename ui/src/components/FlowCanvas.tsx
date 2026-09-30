import React, { useCallback, useMemo, useRef, useState } from 'react';
import ReactFlow, {
  Node,
  Edge,
  addEdge,
  applyEdgeChanges,
  Connection,
  useNodesState,
  useEdgesState,
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  Panel,
  ReactFlowInstance,
  Position,
  MarkerType,
  EdgeChange,
} from 'reactflow';
import 'reactflow/dist/style.css';
import dagre from 'dagre';
import { nodeTypes } from './NodeTypes';
import { NodeConfig, Position as NodePosition } from '../types';
import { useRuleStore } from '../store/ruleStore';
import { usePluginStore } from '../store/pluginStore';
import { SOURCE_NODE_ID, buildNodePositionMap } from '../utils/ruleUtils';

interface FlowCanvasProps {
  onNodeClick?: (nodeId: string | null) => void;
  selectedNodeId?: string | null;
}

const buildEdge = (sourceId: string, targetId: string, selected = false): Edge => ({
  id: `${sourceId}-${targetId}`,
  source: sourceId,
  target: targetId,
  type: 'smoothstep',
  style: {
    stroke: selected ? '#2E90FA' : '#98A2B3',
    strokeWidth: selected ? 3 : 2,
  },
  markerEnd: {
    type: MarkerType.ArrowClosed,
    color: selected ? '#2E90FA' : '#98A2B3',
  },
  focusable: true,
  interactionWidth: 32,
  selected,
});

// 自动布局函数
const getLayoutedElements = (nodes: Node[], edges: Edge[], direction = 'LR') => {
  const dagreGraph = new dagre.graphlib.Graph();
  dagreGraph.setDefaultEdgeLabel(() => ({}));

  // LR = Left to Right (Dify style)
  dagreGraph.setGraph({ rankdir: direction, nodesep: 60, ranksep: 120 });

  const isHorizontal = direction === 'LR';

  nodes.forEach((node) => {
    // Adjusted for new Card size (approx 180x80 + padding)
    dagreGraph.setNode(node.id, { width: 180, height: 80 });
  });

  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target);
  });

  dagre.layout(dagreGraph);

  const layoutedNodes = nodes.map((node) => {
    const nodeWithPosition = dagreGraph.node(node.id);
    return {
      ...node,
      targetPosition: isHorizontal ? Position.Left : Position.Top,
      sourcePosition: isHorizontal ? Position.Right : Position.Bottom,
      position: {
        x: nodeWithPosition.x - 180 / 2,
        y: nodeWithPosition.y - 80 / 2,
      },
    };
  });

  return { nodes: layoutedNodes, edges };
};

export const FlowCanvas: React.FC<FlowCanvasProps> = ({ onNodeClick, selectedNodeId }) => {
  const { currentRule, updateCurrentRule } = useRuleStore();
  const { pluginInfos } = usePluginStore();
  const reactFlowInstance = useRef<ReactFlowInstance | null>(null);
  const reactFlowWrapper = useRef<HTMLDivElement | null>(null);
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);

  // 将 RuleFlow 转换为 React Flow 的节点和边
  const { initialNodes, initialEdges } = useMemo(() => {
    if (!currentRule || !currentRule.nodes) {
      return { initialNodes: [], initialEdges: [] };
    }

    const nodes: Node[] = currentRule.nodes.map((nodeConfig: NodeConfig) => {
      const position = nodeConfig.metadata?.position || { x: 50, y: 50 };

      // Default to LR handles
      return {
        id: nodeConfig.id,
        type: 'custom',
        position,
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
        data: {
          label: nodeConfig.name || nodeConfig.id,
          nodeType: nodeConfig.type,
          nodeConfig,
        },
        selected: selectedNodeId === nodeConfig.id,
      };
    });

    const edges: Edge[] = [];
    currentRule.nodes.forEach((nodeConfig: NodeConfig) => {
      nodeConfig.wires.forEach((targetId: string) => {
        const edgeId = `${nodeConfig.id}-${targetId}`;
        edges.push(buildEdge(nodeConfig.id, targetId, selectedEdgeId === edgeId));
      });
    });

    return { initialNodes: nodes, initialEdges: edges };
  }, [currentRule, selectedNodeId, selectedEdgeId]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges] = useEdgesState(initialEdges);

  const onEdgesChange = useCallback(
    (changes: EdgeChange[]) => {
      let nextSelectedEdgeId: string | null | undefined;

      changes.forEach((change) => {
        if (change.type === 'select') {
          nextSelectedEdgeId = change.selected ? change.id : null;
        }
      });

      if (nextSelectedEdgeId !== undefined) {
        setSelectedEdgeId(nextSelectedEdgeId);
        if (nextSelectedEdgeId) {
          onNodeClick?.(null);
        }
      }

      setEdges((currentEdges) => applyEdgeChanges(changes, currentEdges));
    },
    [onNodeClick, setEdges]
  );

  // 保存当前 React Flow 中的节点位置到 currentRule
  const syncNodePositionsToRule = useCallback(() => {
    if (!currentRule || !nodes.length) return;

    const updatedNodes = currentRule.nodes.map((nodeConfig) => {
      const flowNode = nodes.find((n) => n.id === nodeConfig.id);
      if (flowNode) {
        return {
          ...nodeConfig,
          metadata: {
            ...nodeConfig.metadata,
            position: flowNode.position as NodePosition,
          },
        };
      }
      return nodeConfig;
    });

    updateCurrentRule({
      nodes: updatedNodes,
      metadata: {
        ...(currentRule.metadata || {}),
        node_positions: buildNodePositionMap(updatedNodes),
      },
    });
  }, [currentRule, nodes, updateCurrentRule]);

  const lastPositionsRef = React.useRef<Map<string, { x: number; y: number }>>(new Map());

  React.useEffect(() => {
    if (!currentRule || nodes.length === 0) return;

    let hasPositionChange = false;
    for (const node of nodes) {
      const lastPos = lastPositionsRef.current.get(node.id);
      if (!lastPos || lastPos.x !== node.position.x || lastPos.y !== node.position.y) {
        hasPositionChange = true;
        lastPositionsRef.current.set(node.id, { x: node.position.x, y: node.position.y });
      }
    }

    if (hasPositionChange) {
      const timeoutId = setTimeout(() => {
        syncNodePositionsToRule();
      }, 50);

      return () => clearTimeout(timeoutId);
    }
  }, [nodes, currentRule, syncNodePositionsToRule]);

  // 当 currentRule 变化时更新节点和边
  const previousNodesRef = React.useRef<Map<string, { x: number; y: number }>>(new Map());
  const previousRuleIdRef = React.useRef<string | null>(null);
  const previousNodeIdsRef = React.useRef<Set<string>>(new Set());
  const previousEdgeIdsRef = React.useRef<Set<string>>(new Set());

  React.useEffect(() => {
    if (!currentRule) return;

    const ruleIdChanged = previousRuleIdRef.current !== currentRule.id;
    const currentNodeIds = new Set(currentRule.nodes.map((n) => n.id));
    const nodeIdsChanged =
      previousNodeIdsRef.current.size !== currentNodeIds.size ||
      Array.from(previousNodeIdsRef.current).some((id) => !currentNodeIds.has(id)) ||
      Array.from(currentNodeIds).some((id) => !previousNodeIdsRef.current.has(id));

    const currentEdgeIds = new Set<string>();
    currentRule.nodes.forEach((nodeConfig) => {
      nodeConfig.wires.forEach((targetId) => {
        currentEdgeIds.add(`${nodeConfig.id}-${targetId}`);
      });
    });
    const edgeIdsChanged =
      previousEdgeIdsRef.current.size !== currentEdgeIds.size ||
      Array.from(previousEdgeIdsRef.current).some((id) => !currentEdgeIds.has(id)) ||
      Array.from(currentEdgeIds).some((id) => !previousEdgeIdsRef.current.has(id));

    if (ruleIdChanged || nodeIdsChanged || edgeIdsChanged) {
      const newNodes: Node[] = currentRule.nodes.map((nodeConfig: NodeConfig) => {
        const savedPosition = previousNodesRef.current.get(nodeConfig.id);
        const position = nodeConfig.metadata?.position || savedPosition || { x: 50, y: 50 };

        previousNodesRef.current.set(nodeConfig.id, position);

        return {
          id: nodeConfig.id,
          type: 'custom',
          position,
          sourcePosition: Position.Right,
          targetPosition: Position.Left,
          data: {
            label: nodeConfig.name || nodeConfig.id,
            nodeType: nodeConfig.type,
            nodeConfig,
          },
          selected: selectedNodeId === nodeConfig.id,
        };
      });

      const newEdges: Edge[] = [];
      currentRule.nodes.forEach((nodeConfig: NodeConfig) => {
        nodeConfig.wires.forEach((targetId: string) => {
          const edgeId = `${nodeConfig.id}-${targetId}`;
          newEdges.push(buildEdge(nodeConfig.id, targetId, selectedEdgeId === edgeId));
        });
      });

      setNodes((prevNodes) => {
        const mergedNodes = newNodes.map((newNode) => {
          const existingNode = prevNodes.find((n) => n.id === newNode.id);
          if (existingNode) {
            return {
              ...newNode,
              position: existingNode.position,
            };
          }
          return newNode;
        });
        return mergedNodes;
      });
      setEdges(newEdges);

      if (selectedEdgeId && !currentEdgeIds.has(selectedEdgeId)) {
        setSelectedEdgeId(null);
      }

      previousRuleIdRef.current = currentRule.id;
      previousNodeIdsRef.current = currentNodeIds;
      previousEdgeIdsRef.current = currentEdgeIds;
    } else {
      setNodes((prevNodes) =>
        prevNodes.map((node) => {
          const newNodeConfig = currentRule.nodes.find((n) => n.id === node.id);
          if (!newNodeConfig) return node;

          return {
            ...node,
            selected: selectedNodeId === node.id,
            data: {
              ...node.data,
              nodeConfig: newNodeConfig,
            },
          };
        })
      );
    }
  }, [currentRule, selectedNodeId, selectedEdgeId, setNodes, setEdges]);

  React.useEffect(() => {
    if (!selectedEdgeId || !currentRule) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Delete' && event.key !== 'Backspace') {
        return;
      }

      const target = event.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable)
      ) {
        return;
      }

      const selectedEdge = edges.find((edge) => edge.id === selectedEdgeId);
      if (!selectedEdge) {
        return;
      }

      event.preventDefault();
      setSelectedEdgeId(null);
      setEdges((currentEdges) => currentEdges.filter((edge) => edge.id !== selectedEdgeId));

      const updatedNodes = currentRule.nodes.map((node) => {
        if (node.id !== selectedEdge.source) {
          return node;
        }

        return {
          ...node,
          wires: node.wires.filter((wire) => wire !== selectedEdge.target),
        };
      });

      updateCurrentRule({
        nodes: updatedNodes,
        metadata: {
          ...(currentRule.metadata || {}),
          node_positions: buildNodePositionMap(updatedNodes),
        },
      });
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [currentRule, edges, selectedEdgeId, setEdges, updateCurrentRule]);

  const onConnect = useCallback(
    (params: Connection) => {
      if (!currentRule || !params.source || !params.target) return;
      const sourceId = params.source;
      const targetId = params.target;

      const updatedNodes = currentRule.nodes.map((node) => {
        if (node.id === sourceId) {
          const wires = node.wires || [];
          if (!wires.includes(targetId)) {
            return { ...node, wires: [...wires, targetId] };
          }
        }
        return node;
      });

      updateCurrentRule({
        nodes: updatedNodes,
        metadata: {
          ...(currentRule.metadata || {}),
          node_positions: buildNodePositionMap(updatedNodes),
        },
      });

      setEdges((eds) => addEdge({
          ...params,
          ...buildEdge(sourceId, targetId),
      }, eds));
    },
    [currentRule, updateCurrentRule, setEdges]
  );

  const onNodeDragStop = useCallback(
    (_: React.MouseEvent, node: Node) => {
      if (!currentRule) return;

      const updatedNodes = currentRule.nodes.map((n) => {
        if (n.id === node.id) {
          return {
            ...n,
            metadata: {
              ...n.metadata,
              position: node.position as NodePosition,
            },
          };
        }
        return n;
      });

      updateCurrentRule({
        nodes: updatedNodes,
        metadata: {
          ...(currentRule.metadata || {}),
          node_positions: buildNodePositionMap(updatedNodes),
        },
      });
    },
    [currentRule, updateCurrentRule]
  );

  const onNodeClickHandler = useCallback(
    (_: React.MouseEvent, node: Node) => {
      setSelectedEdgeId(null);
      onNodeClick?.(node.id);
    },
    [onNodeClick]
  );

  const onEdgeClickHandler = useCallback(
    (event: React.MouseEvent, edge: Edge) => {
      event.stopPropagation();
      setSelectedEdgeId(edge.id);
      onNodeClick?.(null);
      setEdges((currentEdges) =>
        currentEdges.map((currentEdge) => ({
          ...currentEdge,
          selected: currentEdge.id === edge.id,
        }))
      );
    },
    [onNodeClick, setEdges]
  );

  const onPaneClickHandler = useCallback(() => {
    setSelectedEdgeId(null);
    setEdges((currentEdges) =>
      currentEdges.map((currentEdge) => ({
        ...currentEdge,
        selected: false,
      }))
    );
  }, [setEdges]);

  const isValidConnection = useCallback(
    (connection: Connection): boolean => {
      if (!connection.source || !connection.target || !currentRule) return false;

      const sourceNode = currentRule.nodes.find((n) => n.id === connection.source);
      const targetNode = currentRule.nodes.find((n) => n.id === connection.target);

      if (!sourceNode || !targetNode) return false;
      // 检查所有 Sink 类型（包括新 ID）
      const sinkTypes = ['sink', 'mock-sink', 'postgres-sink', 'sink-timeseries-timescaledb',
                         'sink-relational-postgres',
                         'sink-debug-log', 'timescaledb-sink'];
      if (sinkTypes.includes(sourceNode.type)) return false;

      if (connection.source === connection.target) return false;
      const existingEdge = edges.find(
        (e) => e.source === connection.source && e.target === connection.target
      );
      if (existingEdge) return false;

      return true;
    },
    [currentRule, edges]
  );

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();

      if (!currentRule || !reactFlowInstance.current) return;

      const nodeType = event.dataTransfer.getData('application/reactflow');
      if (!nodeType) return;

      const reactFlowBounds = reactFlowWrapper.current?.getBoundingClientRect();
      if (!reactFlowBounds) return;

      const position = reactFlowInstance.current.screenToFlowPosition({
        x: event.clientX - reactFlowBounds.left,
        y: event.clientY - reactFlowBounds.top,
      });

      let finalType = nodeType;
      let pluginName: string | undefined;

      // 如果是从插件系统拖拽的（旧格式，带前缀）
      if (nodeType.startsWith('sink:')) {
        finalType = 'sink';
        pluginName = nodeType.split(':')[1];
      } else if (nodeType.startsWith('source:')) {
        finalType = 'source';
        pluginName = nodeType.split(':')[1];
      } else if (nodeType.startsWith('processor:')) {
        finalType = 'processor';
        pluginName = nodeType.split(':')[1];
      } else {
        // 新格式中，nodeType 直接是插件 ID
        // 从 pluginInfos 中查找插件信息以确定类型
        const pluginInfo = pluginInfos.find(p => p.id === nodeType);
        if (pluginInfo) {
          pluginName = nodeType;
          finalType = pluginInfo.type; // 'source', 'processor', 或 'sink'
        } else {
          // 如果找不到插件信息，尝试从 ID 推断类型
          if (nodeType.startsWith('source-')) {
            finalType = 'source';
            pluginName = nodeType;
          } else if (nodeType.startsWith('sink-')) {
            finalType = 'sink';
            pluginName = nodeType;
          } else if (nodeType.startsWith('process-') || nodeType.includes('-')) {
            finalType = 'processor';
            pluginName = nodeType;
          }
        }
      }

      // 不再使用硬编码的节点定义，默认配置为空
      // 节点配置将通过 JSON Schema 从后端获取
      const defaultConfig: Record<string, any> = {};

      const newNodeId = `${finalType}_${Date.now()}`;
      const newNode: NodeConfig = {
        id: newNodeId,
        type: finalType,
        plugin: pluginName,
        name: `${finalType}_${Date.now()}`,
        config: defaultConfig, // 使用默认配置
        wires: [],
        metadata: {
          position: position as NodePosition,
        },
      };

      const updatedNodes = [...currentRule.nodes, newNode];
      updateCurrentRule({
        nodes: updatedNodes,
        metadata: {
          ...(currentRule.metadata || {}),
          node_positions: buildNodePositionMap(updatedNodes),
        },
      });
    },
    [currentRule, updateCurrentRule, pluginInfos]
  );

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  const handleAutoLayout = useCallback(() => {
    if (!currentRule || nodes.length === 0) return;

    const { nodes: layoutedNodes } = getLayoutedElements(nodes, edges);

    const updatedNodes = currentRule.nodes.map((nodeConfig) => {
      const layoutedNode = layoutedNodes.find((n) => n.id === nodeConfig.id);
      if (layoutedNode) {
        return {
          ...nodeConfig,
          metadata: {
            ...nodeConfig.metadata,
            position: layoutedNode.position as NodePosition,
          },
        };
      }
      return nodeConfig;
    });

    updateCurrentRule({
      nodes: updatedNodes,
      metadata: {
        ...(currentRule.metadata || {}),
        node_positions: buildNodePositionMap(updatedNodes),
      },
    });
    setNodes(layoutedNodes);
  }, [currentRule, nodes, edges, updateCurrentRule, setNodes]);

  const onNodesDelete = useCallback(
    (deleted: Node[]) => {
      if (!currentRule) return;

      const deletedIds = new Set(deleted.map((n) => n.id));
      const updatedNodes = currentRule.nodes
        .filter((n) => !deletedIds.has(n.id))
        .map((n) => ({
          ...n,
          wires: n.wires.filter((w) => !deletedIds.has(w)),
        }));

      let updatedSource = currentRule.source;
      if (deletedIds.has(SOURCE_NODE_ID)) {
          updatedSource = undefined;
      }

      updateCurrentRule({
        nodes: updatedNodes,
        source: updatedSource,
        metadata: {
          ...(currentRule.metadata || {}),
          node_positions: buildNodePositionMap(updatedNodes),
        },
      });
    },
    [currentRule, updateCurrentRule]
  );

  const onEdgesDelete = useCallback(
    (deleted: Edge[]) => {
      if (!currentRule) return;
      setSelectedEdgeId((currentSelectedEdgeId) =>
        deleted.some((edge) => edge.id === currentSelectedEdgeId) ? null : currentSelectedEdgeId
      );

      const updatedNodes = currentRule.nodes.map(node => {
          const deletedWires = deleted
              .filter(e => e.source === node.id)
              .map(e => e.target);

          if (deletedWires.length > 0) {
              return {
                  ...node,
                  wires: node.wires.filter(w => !deletedWires.includes(w))
              };
          }
          return node;
      });

      updateCurrentRule({
        nodes: updatedNodes,
        metadata: {
          ...(currentRule.metadata || {}),
          node_positions: buildNodePositionMap(updatedNodes),
        },
      });
    },
    [currentRule, updateCurrentRule]
  );

  if (!currentRule) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
        <p>请选择一个规则或创建新规则</p>
      </div>
    );
  }

  return (
    <div
      ref={reactFlowWrapper}
      style={{ width: '100%', height: '100%' }}
      onDrop={onDrop}
      onDragOver={onDragOver}
    >
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      onNodeDragStop={onNodeDragStop}
      onNodeClick={onNodeClickHandler}
        onEdgeClick={onEdgeClickHandler}
        onPaneClick={onPaneClickHandler}
        onNodesDelete={onNodesDelete}
        onEdgesDelete={onEdgesDelete}
        deleteKeyCode={['Backspace', 'Delete']}
        onInit={(instance) => {
          reactFlowInstance.current = instance;
        }}
      nodeTypes={nodeTypes}
        isValidConnection={isValidConnection}
        elevateEdgesOnSelect
      fitView
    >
        <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#E4E7EC" />
      <Controls />
        <MiniMap
            nodeColor={(n) => {
                switch(n.data.nodeType) {
                    // 支持新插件 ID
                    case 'filter-by-expression':
                    case 'filter': return '#F79009';
                    case 'source-http-ingress':
                    case 'source-mqtt-ingress':
                    case 'source-command-ingress':
                    case 'source': return '#12B76A';
                    case 'sink':
                    case 'sink-debug-log':
                    case 'mock-sink': return '#475467';
                    case 'sink-relational-postgres':
                    case 'sink-timeseries-timescaledb':
                    case 'postgres-sink': return '#336791';
                    default: return '#2E90FA';
                }

            }}
            style={{ borderRadius: 8, boxShadow: '0 2px 4px rgba(0,0,0,0.1)' }}
        />
      <Panel position="top-left">
          <div style={{ background: 'var(--color-surface)', padding: '10px 16px', borderRadius: '8px', boxShadow: 'var(--shadow-card)', border: '1px solid var(--color-border)' }}>
            <strong style={{ color: '#101828' }}>{currentRule.name || currentRule.id}</strong>
          {currentRule.debug_mode && (
              <span style={{ marginLeft: '10px', color: '#12B76A', fontSize: '12px', background: '#ECFDF3', padding: '2px 8px', borderRadius: '10px' }}>● Debug Mode</span>
          )}
        </div>
      </Panel>
        <Panel position="top-right">
          <button
            onClick={handleAutoLayout}
            style={{
              padding: '8px 12px',
              backgroundColor: 'var(--color-surface)',
              color: '#344054',
              border: '1px solid #D0D5DD',
              borderRadius: '8px',
              cursor: 'pointer',
              fontSize: '14px',
              fontWeight: 500,
              boxShadow: '0 1px 2px rgba(16, 24, 40, 0.05)',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            🔄 自动布局
          </button>
        </Panel>
    </ReactFlow>
    </div>
  );
};
