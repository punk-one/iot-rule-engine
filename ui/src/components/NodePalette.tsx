import React, { useEffect } from 'react';
import { usePluginStore } from '../store/pluginStore';

interface NodePaletteProps {
  onDragStart?: (nodeType: string) => void;
}

export const NodePalette: React.FC<NodePaletteProps> = ({ onDragStart }) => {
  const { pluginInfos, loadPluginInfos, loading, error } = usePluginStore();

  // 加载插件信息
  useEffect(() => {
    console.log('[NodePalette] Loading plugin infos...');
    loadPluginInfos().catch(err => {
      console.error('[NodePalette] Failed to load plugin infos:', err);
    });
  }, [loadPluginInfos]);

  const handleDragStart = (e: React.DragEvent, nodeType: string) => {
    e.dataTransfer.setData('application/reactflow', nodeType);
    e.dataTransfer.effectAllowed = 'move';
    onDragStart?.(nodeType);
  };

  // 移除硬编码的插件 ID，完全依赖后端返回的 pluginInfos
  // 所有插件都通过 pluginInfos 获取，不再区分"内置"和"外部"

  // 分组获取节点类型（只使用 pluginInfos，不再使用硬编码的内置节点定义）
  const groupedNodes = React.useMemo(() => {
    const sources: any[] = [];
    const processors: any[] = [];
    const sinks: any[] = [];

    // 只使用 pluginInfos，不再使用 BUILT_IN_NODE_DEFINITIONS
    pluginInfos.forEach(p => {
      if (!p) return;

      // 所有插件直接使用 ID，不再区分内置和外部
      const nodeType = p.id;

      const nodeItem = {
        type: nodeType,
        label: p.name,
        color: p.type === 'source' ? '#E91E63' :
               p.type === 'processor' ? '#FF9800' : '#607D8B',
        icon: p.type === 'source' ? '🔌' :
              p.type === 'processor' ? '⚡' : '📤',
        description: p.description
      };

      if (p.type === 'source') {
        sources.push(nodeItem);
      } else if (p.type === 'processor') {
        processors.push(nodeItem);
      } else if (p.type === 'sink') {
        sinks.push(nodeItem);
      }
    });

    return { sources, processors, sinks };
  }, [pluginInfos]);

  // 调试：打印插件信息（在 groupedNodes 定义之后）
  useEffect(() => {
    console.log('[NodePalette] pluginInfos:', pluginInfos.length, pluginInfos);
    console.log('[NodePalette] groupedNodes:', groupedNodes);
  }, [pluginInfos, groupedNodes]);

  const renderNodeItem = (nodeType: any) => (
      <div
        key={nodeType.type}
        draggable
        onDragStart={(e) => handleDragStart(e, nodeType.type)}
        style={{
            padding: '10px',
            backgroundColor: 'var(--color-surface)',
            border: `2px solid ${nodeType.color}`,
            borderRadius: '6px',
            cursor: 'grab',
            marginBottom: '8px',
            userSelect: 'none',
            transition: 'transform 0.2s',
        }}
        onMouseEnter={(e) => e.currentTarget.style.transform = 'translateX(5px)'}
        onMouseLeave={(e) => e.currentTarget.style.transform = 'translateX(0)'}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
             <span style={{ fontSize: '16px' }}>{nodeType.icon}</span>
             <span style={{ fontWeight: '500', fontSize: '13px' }}>{nodeType.label}</span>
        </div>
      </div>
  );

  // 显示加载状态或错误
  if (loading) {
    return (
      <div
        style={{
          width: '220px',
          backgroundColor: '#f5f5f5',
          borderRight: '1px solid #ddd',
          padding: '15px',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          height: '100%'
        }}
      >
        <div style={{ fontSize: '12px', color: '#666' }}>加载中...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div
        style={{
          width: '220px',
          backgroundColor: '#f5f5f5',
          borderRight: '1px solid #ddd',
          padding: '15px',
          display: 'flex',
          flexDirection: 'column',
          height: '100%'
        }}
      >
        <div style={{ fontSize: '12px', color: '#F04438', marginBottom: '10px' }}>加载失败: {error}</div>
        <button
          onClick={() => loadPluginInfos()}
          style={{
            padding: '6px 12px',
            backgroundColor: '#2E90FA',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
            fontSize: '12px'
          }}
        >
          重试
        </button>
      </div>
    );
  }

  // 如果没有插件信息，显示提示
  if (pluginInfos.length === 0) {
    return (
      <div
        style={{
          width: '220px',
          backgroundColor: '#f5f5f5',
          borderRight: '1px solid #ddd',
          padding: '15px',
          display: 'flex',
          flexDirection: 'column',
          height: '100%'
        }}
      >
        <div style={{ fontSize: '12px', color: '#999', marginBottom: '10px' }}>暂无可用插件</div>
        <button
          onClick={() => loadPluginInfos()}
          style={{
            padding: '6px 12px',
            backgroundColor: '#2E90FA',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
            fontSize: '12px'
          }}
        >
          刷新
        </button>
      </div>
    );
  }

  return (
    <div
      style={{
        width: '220px',
        backgroundColor: '#f5f5f5',
        borderRight: '1px solid #ddd',
        padding: '15px',
        overflowY: 'auto',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        gap: '20px'
      }}
    >
      <div>
        <h4 style={{ margin: '0 0 10px 0', color: '#666', fontSize: '12px', textTransform: 'uppercase' }}>Sources (Input)</h4>
        {groupedNodes.sources.length > 0 ? groupedNodes.sources.map(renderNodeItem) : <div style={{fontSize:'12px', color:'#999'}}>暂无</div>}
      </div>

      <div>
        <h4 style={{ margin: '0 0 10px 0', color: '#666', fontSize: '12px', textTransform: 'uppercase' }}>Processors</h4>
        {groupedNodes.processors.length > 0 ? groupedNodes.processors.map(renderNodeItem) : <div style={{fontSize:'12px', color:'#999'}}>暂无</div>}
      </div>

      <div>
        <h4 style={{ margin: '0 0 10px 0', color: '#666', fontSize: '12px', textTransform: 'uppercase' }}>Sinks (Output)</h4>
        {groupedNodes.sinks.length > 0 ? groupedNodes.sinks.map(renderNodeItem) : <div style={{fontSize:'12px', color:'#999'}}>暂无</div>}
      </div>

       <div style={{ marginTop: 'auto', fontSize: '11px', color: '#999', textAlign: 'center' }}>
        Drag nodes to canvas
      </div>
    </div>
  );
};
