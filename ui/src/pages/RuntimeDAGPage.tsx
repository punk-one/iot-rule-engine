import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import ReactFlow, {
  Node,
  Edge,
  Background,
  Controls,
  MiniMap,
  MarkerType,
  ReactFlowInstance,
} from 'reactflow';
import 'reactflow/dist/style.css';
import { runtimeAPI, rulesAPI } from '../services/api';
import { RuleInstance, RuleFlow, NodeConfig } from '../types';
import { nodeTypes } from '../components/NodeTypes';
import { applyStoredNodePositions, hydrateRule } from '../utils/ruleUtils';

const getFallbackPosition = (index: number) => ({
  x: 80 + (index % 4) * 240,
  y: 80 + Math.floor(index / 4) * 160,
});

const hasValidPosition = (value: unknown): value is { x: number; y: number } => (
  !!value &&
  typeof value === 'object' &&
  typeof (value as { x?: unknown }).x === 'number' &&
  typeof (value as { y?: unknown }).y === 'number'
);

const mergeRuleNodesWithRuntime = (ruleDef: RuleFlow | null, instance: RuleInstance): NodeConfig[] => {
  const runtimeEdges = Object.values(instance.edges || {});
  const mergedNodes = new Map<string, NodeConfig>();

  (ruleDef?.nodes || []).forEach((node) => {
    mergedNodes.set(node.id, node);
  });

  Object.values(instance.nodes || {}).forEach((nodeInstance) => {
    if (mergedNodes.has(nodeInstance.node_id)) {
      return;
    }

    mergedNodes.set(nodeInstance.node_id, {
      id: nodeInstance.node_id,
      type: nodeInstance.plugin_id || 'processor',
      plugin: nodeInstance.plugin_id,
      name: nodeInstance.node_id,
      config: nodeInstance.config || {},
      wires: runtimeEdges
        .filter((edge) => edge.from_node === nodeInstance.node_id)
        .map((edge) => edge.to_node),
      metadata: {},
    });
  });

  return Array.from(mergedNodes.values()).map((node, index) => {
    const currentPosition = node.metadata?.position;

    return {
      ...node,
      metadata: {
        ...(node.metadata || {}),
        position: hasValidPosition(currentPosition) ? currentPosition : getFallbackPosition(index),
      },
    };
  });
};

export const RuntimeDAGPage: React.FC = () => {
  const { instanceId } = useParams<{ instanceId: string }>();
  const navigate = useNavigate();
  const [instance, setInstance] = useState<RuleInstance | null>(null);
  const [ruleDef, setRuleDef] = useState<RuleFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [ruleLoadWarning, setRuleLoadWarning] = useState<string | null>(null);
  const [reactFlowInstance, setReactFlowInstance] = useState<ReactFlowInstance | null>(null);
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const [canvasHeight, setCanvasHeight] = useState(0);
  const measuredCanvasHeight = canvasHeight > 0 ? `${canvasHeight}px` : '100%';

  const syncCanvasMetrics = () => {
    const canvas = canvasRef.current;
    if (!canvas) {
      return;
    }

    const rect = canvas.getBoundingClientRect();
    const nextHeight = Math.round(rect.height || canvas.clientHeight || 0);
    setCanvasHeight((current) => (current === nextHeight ? current : nextHeight));
  };

  useEffect(() => {
    if (!instanceId) {
      return;
    }

    let cancelled = false;

    const loadInstance = async () => {
      try {
        const inst = await runtimeAPI.getInstance(instanceId);
        if (cancelled) {
          return;
        }
        setInstance(inst);
        setError(null);
      } catch (err: any) {
        if (cancelled) {
          return;
        }
        console.error('Failed to load runtime data:', err);
        setError(err.message || 'Failed to load data');
      }
    };

    void loadInstance();

    return () => {
      cancelled = true;
    };
  }, [instanceId]);

  useEffect(() => {
    const ruleID = instance?.rule_id;
    if (!ruleID || ruleDef?.id === ruleID) {
      return;
    }

    let cancelled = false;

    const loadRuleDefinition = async () => {
      try {
        const rule = hydrateRule(applyStoredNodePositions(await rulesAPI.get(ruleID)));
        if (cancelled) {
          return;
        }
        setRuleDef(rule);
        setRuleLoadWarning(null);
      } catch (ruleErr: any) {
        if (cancelled) {
          return;
        }
        console.error('Failed to load rule definition for runtime DAG:', ruleErr);
        setRuleDef((current) => (current?.id === ruleID ? current : null));
        setRuleLoadWarning(ruleErr?.message || '规则定义加载失败，已回退到运行态拓扑');
      }
    };

    void loadRuleDefinition();

    return () => {
      cancelled = true;
    };
  }, [instance?.rule_id, ruleDef?.id]);

  const { flowNodes, flowEdges } = useMemo(() => {
    if (!instance) return { flowNodes: [], flowEdges: [] };

    const nodeInstancesByCanvasId = Object.values(instance.nodes || {}).reduce<Record<string, typeof instance.nodes[string]>>((acc, nodeInstance) => {
      acc[nodeInstance.node_id] = nodeInstance;
      return acc;
    }, {});

    const resolvedNodes = mergeRuleNodesWithRuntime(ruleDef, instance);

    const nodes: Node[] = resolvedNodes.map((nodeConfig) => {
      const nodeInstance = nodeInstancesByCanvasId[nodeConfig.id];
      const position = nodeConfig.metadata?.position || getFallbackPosition(0);

      return {
        id: nodeConfig.id,
        type: 'custom',
        position,
        data: {
          label: nodeConfig.name || nodeConfig.id,
          nodeType: nodeConfig.type,
          nodeConfig,
          runtimeStatus: nodeInstance?.status || 'stopped',
          metrics: nodeInstance?.metrics,
        },
        draggable: false, // Read-only
        selectable: true,
      };
    });

    const edges: Edge[] = [];
    const runtimeEdges = Object.values(instance.edges || {});

    if (runtimeEdges.length > 0) {
      runtimeEdges.forEach((edgeInst) => {
        const published = edgeInst.metrics.events_published;
        const consumed = edgeInst.metrics.events_consumed;
        const dropped = edgeInst.metrics.events_dropped;
        const lag = published - consumed;

        const isWarning = dropped > 0 || (lag > (published * 0.1) && published > 10);

        edges.push({
            id: edgeInst.edge_id,
            source: edgeInst.from_node,
            target: edgeInst.to_node,
            type: 'smoothstep',
            animated: published > 0 && !isWarning,
            style: {
                stroke: isWarning ? (dropped > 0 ? '#F04438' : '#F79009') : '#98A2B3',
                strokeWidth: isWarning ? 3 : 2
            },
            markerEnd: {
                 type: MarkerType.ArrowClosed,
                 color: isWarning ? (dropped > 0 ? '#F04438' : '#F79009') : '#98A2B3',
            },
        });
      });
    } else {
      resolvedNodes.forEach((nodeConfig) => {
        (nodeConfig.wires || []).forEach((targetId) => {
          edges.push({
            id: `${nodeConfig.id}-${targetId}`,
            source: nodeConfig.id,
            target: targetId,
            type: 'smoothstep',
            style: {
              stroke: '#98A2B3',
              strokeWidth: 2,
            },
            markerEnd: {
              type: MarkerType.ArrowClosed,
              color: '#98A2B3',
            },
          });
        });
      });
    }

    return { flowNodes: nodes, flowEdges: edges };
  }, [instance, ruleDef]);

  useEffect(() => {
    if (!reactFlowInstance || flowNodes.length === 0) {
      return;
    }

    const fitViewport = () => {
      syncCanvasMetrics();
      reactFlowInstance.fitView({
        padding: 0.2,
        duration: 0,
      });
      window.requestAnimationFrame(syncCanvasMetrics);
    };

    const frameId = window.requestAnimationFrame(fitViewport);
    const timeoutId = window.setTimeout(fitViewport, 120);
    const warmupTimeouts = [320, 640, 900].map((delay, index) => (
      window.setTimeout(fitViewport, delay)
    ));
    const resizeObserver = typeof ResizeObserver !== 'undefined' && canvasRef.current
      ? new ResizeObserver(() => {
          syncCanvasMetrics();
          fitViewport();
        })
      : null;

    if (resizeObserver && canvasRef.current) {
      resizeObserver.observe(canvasRef.current);
    }

    const handleWindowResize = () => {
      syncCanvasMetrics();
      fitViewport();
    };

    window.addEventListener('resize', handleWindowResize);

    return () => {
      window.cancelAnimationFrame(frameId);
      window.clearTimeout(timeoutId);
      warmupTimeouts.forEach((timerId) => window.clearTimeout(timerId));
      resizeObserver?.disconnect();
      window.removeEventListener('resize', handleWindowResize);
    };
  }, [reactFlowInstance, flowNodes.length, flowEdges.length]);

  if (error && !instance) {
    return <div style={{ padding: '20px', color: 'red' }}>Error: {error}</div>;
  }

  if (!instance) {
    return <div style={{ padding: '20px' }}>Loading runtime view...</div>;
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: 'calc(100vh - 140px)',
        backgroundColor: '#F9FAFB',
      }}
    >
      {/* 顶部状态栏 */}
      <div
        style={{
          padding: '16px 20px',
          backgroundColor: 'var(--color-surface)',
          borderBottom: '1px solid #EAECF0',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '16px',
          flexWrap: 'wrap',
          boxShadow: '0 10px 26px rgba(15, 23, 42, 0.05)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
          <button onClick={() => navigate('/runtime')} style={ghostButtonStyle}>←</button>
          <div>
            <div style={{ fontSize: '18px', fontWeight: 700 }}>{ruleDef?.name || instance.name || instance.rule_id}</div>
            <div style={{ fontSize: '13px', color: '#667085' }}>实例 ID: {instance.id} | 版本: V{instance.version}</div>
          </div>
        </div>
        <div style={{ display: 'flex', gap: '12px' }}>
          <div style={{ padding: '8px 14px', backgroundColor: '#ECFDF3', color: '#027A48', borderRadius: '999px', fontSize: '12px', fontWeight: 700 }}>
            ● {instance.status.toUpperCase()}
          </div>
        </div>
      </div>

      {ruleLoadWarning && (
        <div
          style={{
            margin: '16px 16px 0',
            padding: '12px 14px',
            borderRadius: '14px',
            backgroundColor: '#FFFAEB',
            border: '1px solid #FEDF89',
            color: '#B54708',
            fontSize: '13px',
            fontWeight: 500,
          }}
        >
          已回退到运行态拓扑：{ruleLoadWarning}
        </div>
      )}

      <div style={{ flex: 1, minHeight: 560, position: 'relative', padding: '16px' }}>
        <div
          className="card"
          style={{
            height: '100%',
            minHeight: 560,
            overflow: 'hidden',
            borderRadius: '20px',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <div
            style={{
              padding: '12px 16px',
              borderBottom: '1px solid #EAECF0',
              backgroundColor: '#FCFCFD',
              display: 'flex',
              gap: '12px',
              flexWrap: 'wrap',
              alignItems: 'center',
              fontSize: '12px',
              color: '#667085',
              fontWeight: 600,
            }}
          >
            <span>节点数：{flowNodes.length}</span>
            <span>边数：{flowEdges.length}</span>
            <span>规则 ID：{instance.rule_id}</span>
          </div>
          {flowNodes.length === 0 ? (
            <div
              style={{
                flex: 1,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#667085',
                fontSize: '14px',
              }}
            >
              当前运行实例暂无可展示的拓扑节点
            </div>
          ) : (
            <div
              data-testid="runtime-dag-canvas"
              ref={canvasRef}
              style={{
                width: '100%',
                height: 'calc(100vh - 320px)',
                minHeight: 520,
                flex: 1,
              }}
            >
              <div
                data-testid="runtime-dag-inner"
                style={{
                  width: '100%',
                  height: measuredCanvasHeight,
                  minHeight: measuredCanvasHeight,
                }}
              >
                <ReactFlow
                  nodes={flowNodes}
                  edges={flowEdges}
                  nodeTypes={nodeTypes}
                  onInit={(instance) => {
                    setReactFlowInstance(instance);
                    syncCanvasMetrics();
                  }}
                  onNodeClick={(_, node) => navigate(`/runtime/${instanceId}/nodes/${node.id}`)}
                  fitView
                  style={{ width: '100%', height: measuredCanvasHeight }}
                >
                  <Background color="#F2F4F7" />
                  <Controls />
                  <MiniMap />
                </ReactFlow>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

const ghostButtonStyle: React.CSSProperties = {
  padding: '8px 12px',
  background: 'var(--color-surface)',
  border: '1px solid #D0D5DD',
  borderRadius: '10px',
  cursor: 'pointer',
  fontWeight: 700,
};
