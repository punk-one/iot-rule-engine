import React, { memo } from 'react';
import { Handle, Position, NodeProps } from 'reactflow';
import { NodeConfig } from '../types';

interface CustomNodeData {
  label: string;
  nodeType: string;
  nodeConfig: NodeConfig;
  runtimeStatus?: string;
  metrics?: {
    events_published: number;
    events_consumed: number;
    events_dropped: number;
  };
}

// Dify-like Color Palette
const nodeStyles: Record<string, { color: string; icon: string; bg: string }> = {
  // Source 插件
  'source-http-ingress': { color: '#E91E63', icon: '🌐', bg: '#FCE4EC' },
  'source-mqtt-ingress': { color: '#4CAF50', icon: '📡', bg: '#E8F5E9' },
  'source-command-ingress': { color: '#FF9800', icon: '🎯', bg: '#FFF3E0' },
  // Processor 插件
  'filter-by-expression': { color: '#F79009', icon: '🔍', bg: '#FFFAEB' },
  'transform-field-map': { color: '#7A5AF8', icon: '🗺️', bg: '#F9F5FF' },
  'transform-template-expand': { color: '#9C27B0', icon: '📝', bg: '#F3E5F5' },
  'transform-script-js': { color: '#F04438', icon: '📜', bg: '#FEF3F2' },
  'route-by-condition': { color: '#2E90FA', icon: '🔀', bg: '#EFF8FF' },
  'detect-change': { color: '#FF5722', icon: '📈', bg: '#FFEBEE' },
  'aggregate-window': { color: '#EE46BC', icon: '⏱️', bg: '#FDF2FA' },
  'window': { color: '#EE46BC', icon: '⏱️', bg: '#FDF2FA' },
  // Sink 插件
  'sink-timeseries-timescaledb': { color: '#336791', icon: '📊', bg: '#E8F4F8' },
  'sink-relational-postgres': { color: '#336791', icon: '🗄️', bg: '#E8F4F8' },
  'sink-debug-log': { color: '#475467', icon: '📤', bg: '#F2F4F7' },
  default: { color: '#667085', icon: '📦', bg: '#F9FAFB' },
};


const getNodeStyle = (nodeType: string) => {
  return nodeStyles[nodeType] || nodeStyles.default;
};

// Node Status Badge
const NodeStatusBadge = memo(({ status }: { status: 'error' | 'active' | 'warning' | null }) => {
  if (!status) return null;
  const colors = {
    error: '#F04438',
    active: '#12B76A',
    warning: '#F79009',
  };
  return (
    <div
      style={{
        position: 'absolute',
        top: '-4px',
        right: '-4px',
        width: '12px',
        height: '12px',
        borderRadius: '50%',
        backgroundColor: colors[status],
        border: '2px solid white',
        boxShadow: '0 0 0 1px rgba(0,0,0,0.05)',
        zIndex: 10,
      }}
    />
  );
});

NodeStatusBadge.displayName = 'NodeStatusBadge';

export const CustomNode: React.FC<NodeProps<CustomNodeData>> = memo(({ data, selected, id, sourcePosition = Position.Right, targetPosition = Position.Left }) => {
  const style = getNodeStyle(data.nodeType);

  const getNodeStatus = () => {
    if (data.runtimeStatus) {
      if (data.runtimeStatus === 'running') return 'active';
      if (data.runtimeStatus === 'error') return 'error';
      return 'warning';
    }
    // 检查 filter-by-expression 的配置
    if (data.nodeType === 'filter-by-expression' || data.nodeType === 'filter') {
      const expr = data.nodeConfig.config?.process?.condition?.expr || data.nodeConfig.config?.expr;
      if (!expr) return 'warning';
    }
    return null;
  };

  const status = getNodeStatus();

  return (
    <div
      className="dify-node-card"
      style={{
        width: '130px',
        backgroundColor: 'var(--color-surface)',
        borderRadius: '8px',
        border: `2px solid ${selected ? '#2E90FA' : 'transparent'}`,
        boxShadow: selected
          ? '0 0 0 4px #D1E9FF, 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)'
          : '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)',
        transition: 'all 0.2s ease',
        overflow: 'hidden',
        position: 'relative',
      }}
    >
      <NodeStatusBadge status={status} />

      {data.nodeType !== 'source' && (
        <Handle
            type="target"
            position={targetPosition}
            style={{
                width: '6px',
                height: '6px',
                background: '#98A2B3',
                left: '-3px',
                border: '1px solid white',
            }}
        />
      )}

      <div style={{ display: 'flex', padding: '5px 8px', gap: '6px', alignItems: 'center' }}>
        <div
          style={{
            width: '22px',
            height: '22px',
            borderRadius: '4px',
            backgroundColor: style.bg,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontSize: '12px',
            color: style.color,
            flexShrink: 0,
            boxShadow: 'inset 0 0 0 1px rgba(0,0,0,0.05)'
          }}
        >
          {style.icon}
        </div>

        <div style={{ flex: 1, minWidth: 0 }}>
          <div
            style={{
                fontSize: '11px',
                fontWeight: 600,
                color: '#101828',
                lineHeight: '14px',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              marginBottom: '0px'
            }}
            title={data.label}
          >
            {data.label}
          </div>
          <div style={{ fontSize: '9px', color: '#667085', lineHeight: '12px', display: 'flex', alignItems: 'center', gap: '4px' }}>
             <span style={{ textTransform: 'uppercase', fontSize: '8px', fontWeight: 700, color: style.color }}>{data.nodeType}</span>
          </div>
        </div>
      </div>

       {data.metrics ? (
          <div style={{
              padding: '4px 8px',
              backgroundColor: '#F2F4F7',
              borderTop: '1px solid #EAECF0',
              fontSize: '9px',
              fontFamily: 'monospace',
              color: '#475467',
              display: 'flex',
              justifyContent: 'space-between'
          }}>
             <span>P:{data.metrics.events_published} C:{data.metrics.events_consumed}</span>
             {data.metrics.events_dropped > 0 && <span style={{ color: '#F04438' }}>D:{data.metrics.events_dropped}</span>}
          </div>
       ) : data.nodeConfig.plugin && (
          <div style={{
              padding: '4px 8px',
              backgroundColor: '#F9FAFB',
              borderTop: '1px solid #EAECF0',
              fontSize: '9px',
              color: '#667085',
              display: 'flex',
              alignItems: 'center',
              gap: '4px'
          }}>
             <span>🔌</span>
             <span style={{ fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{data.nodeConfig.plugin}</span>
          </div>
       ) as any}

      {data.nodeType !== 'sink' && (
        <Handle
            type="source"
            position={sourcePosition}
            style={{
                width: '10px',
                height: '10px',
                background: '#98A2B3',
                right: '-6px',
                border: '2px solid white',
            }}
        />
      )}
    </div>
  );
});

CustomNode.displayName = 'CustomNode';

export const nodeTypes = {
  custom: CustomNode,
};
