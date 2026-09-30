import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { ConnectorResourceView, NodeConfig, PluginInfo } from '../types';
import { useRuleStore } from '../store/ruleStore';
import { usePluginStore } from '../store/pluginStore';
import { LazyJSONSchemaRenderer } from './LazyJSONSchemaRenderer';
import { connectorsAPI } from '../services/api';

interface PropertiesPanelProps {
  nodeId: string | null;
}

type FallbackConnectorBinding = {
  required: boolean;
  accepted_families?: string[];
  required_capabilities?: string[];
};

const FALLBACK_CONNECTOR_BINDINGS: Record<string, FallbackConnectorBinding> = {
  'source-s7': {
    required: true,
    accepted_families: ['s7'],
    required_capabilities: ['read'],
  },
  'sink-s7': {
    required: true,
    accepted_families: ['s7'],
    required_capabilities: ['write'],
  },
  'sink-timeseries-timescaledb': {
    required: true,
    accepted_families: ['timescaledb'],
    required_capabilities: ['write'],
  },
};

const CONNECTOR_PREVIEW_LABELS: Record<string, string> = {
  host: 'IP / Host',
  port: 'Port',
  rack: 'Rack',
  slot: 'Slot',
  database: 'Database',
  user: 'User',
};

const normalizePluginIdentity = (value?: string | null): string =>
  String(value || '')
    .trim()
    .toLowerCase()
    .replace(/_/g, '-');

const inferFallbackConnectorBinding = (pluginIdentity?: string | null): FallbackConnectorBinding | null => {
  return FALLBACK_CONNECTOR_BINDINGS[normalizePluginIdentity(pluginIdentity)] || null;
};

const stripConnectorRefField = (schema: any): any => {
  if (!schema || typeof schema !== 'object') {
    return schema;
  }

  if (Array.isArray(schema)) {
    return schema.map((item) => stripConnectorRefField(item));
  }

  const next: Record<string, any> = {};
  for (const [key, value] of Object.entries(schema)) {
    if (key === 'properties' && value && typeof value === 'object' && !Array.isArray(value)) {
      const props = { ...(value as Record<string, any>) };
      delete props.connector_ref;
      next[key] = Object.fromEntries(
        Object.entries(props).map(([propKey, propValue]) => [propKey, stripConnectorRefField(propValue)])
      );
      continue;
    }

    if (key === 'required' && Array.isArray(value)) {
      next[key] = value.filter((field) => field !== 'connector_ref');
      continue;
    }

    next[key] = stripConnectorRefField(value);
  }

  return next;
};

const formatConnectorOptionLabel = (connector: ConnectorResourceView): string => {
  const base = connector.name?.trim()
    ? `${connector.name} (${connector.id})`
    : connector.id;
  if (!connector.connection_summary) {
    return base;
  }
  return `${base} - ${connector.connection_summary}`;
};

const buildConnectorPreviewEntries = (connector: ConnectorResourceView | null): Array<{ key: string; label: string; value: string }> => {
  if (!connector?.connection_preview) {
    return [];
  }

  const preview = connector.connection_preview;
  const preferredKeys = connector.plugin_info?.connector_descriptor?.display_fields || [];
  const seen = new Set<string>();
  const orderedKeys: string[] = [];

  for (const key of preferredKeys) {
    if (preview[key]) {
      orderedKeys.push(key);
      seen.add(key);
    }
  }
  for (const key of Object.keys(preview).sort()) {
    if (!seen.has(key) && preview[key]) {
      orderedKeys.push(key);
    }
  }

  return orderedKeys.map((key) => ({
    key,
    label: CONNECTOR_PREVIEW_LABELS[key] || key,
    value: preview[key],
  }));
};

export const PropertiesPanel: React.FC<PropertiesPanelProps> = ({ nodeId }) => {
  const { currentRule, updateCurrentRule } = useRuleStore();
  const { pluginInfos, loadPluginInfos } = usePluginStore();
  const [nodeConfig, setNodeConfig] = useState<NodeConfig | null>(null);
  const [formData, setFormData] = useState<Record<string, any>>({});
  const [compatibleConnectors, setCompatibleConnectors] = useState<ConnectorResourceView[]>([]);
  const [connectorsLoading, setConnectorsLoading] = useState(false);
  const [connectorsError, setConnectorsError] = useState<string | null>(null);

  useEffect(() => {
    if (pluginInfos.length === 0) {
      loadPluginInfos();
    }
  }, [loadPluginInfos, pluginInfos.length]);

  useEffect(() => {
    if (!nodeId || !currentRule) {
      setNodeConfig(null);
      return;
    }

    const node = currentRule.nodes.find((n) => n.id === nodeId);
    setNodeConfig(node || null);
  }, [nodeId, currentRule]);

  const updateNode = useCallback((updates: Partial<NodeConfig>) => {
    if (!currentRule || !nodeConfig) return;

    const updatedNodes = currentRule.nodes.map((n) =>
      n.id === nodeConfig.id ? { ...n, ...updates } : n
    );

    updateCurrentRule({ nodes: updatedNodes });
    setNodeConfig((prev) => prev ? { ...prev, ...updates } : null);
  }, [currentRule, nodeConfig, updateCurrentRule]);

  const lastSyncedNodeIdRef = useRef<string | null>(null);
  const lastSyncedConfigRef = useRef<string>('');

  const updateConfig = useCallback((formData: Record<string, any>) => {
    if (!nodeConfig) return;

    setFormData(formData);
    lastSyncedConfigRef.current = JSON.stringify(formData || {});
    updateNode({ config: formData });
  }, [nodeConfig, updateNode]);

  useEffect(() => {
    if (!nodeConfig) {
      setFormData({});
      lastSyncedNodeIdRef.current = null;
      lastSyncedConfigRef.current = '';
      return;
    }

    const nextConfig = nodeConfig.config || {};
    const nextConfigStr = JSON.stringify(nextConfig);
    const nodeChanged = lastSyncedNodeIdRef.current !== nodeConfig.id;
    const configChangedExternally = nextConfigStr !== lastSyncedConfigRef.current;

    if (nodeChanged || configChangedExternally) {
      setFormData(nextConfig);
      lastSyncedNodeIdRef.current = nodeConfig.id;
      lastSyncedConfigRef.current = nextConfigStr;
    }
  }, [nodeConfig]);

  // 获取插件信息（如果是插件节点）
  const pluginIdentifier = useMemo(
    () => nodeConfig?.plugin || nodeConfig?.type || '',
    [nodeConfig]
  );

  const pluginInfo = useMemo<PluginInfo | null>(() => {
    if (!pluginIdentifier) return null;
    const normalizedIdentifier = normalizePluginIdentity(pluginIdentifier);
    return pluginInfos.find((p) =>
      normalizePluginIdentity(p.id) === normalizedIdentifier ||
      normalizePluginIdentity(p.name) === normalizedIdentifier
    ) || null;
  }, [pluginIdentifier, pluginInfos]);

  const hasConfigSchema = useMemo(() => {
    const schema = pluginInfo?.capabilities?.config_schema;
    return typeof schema === 'string' && schema.trim().length > 0;
  }, [pluginInfo]);

  const connectorBinding = useMemo(
    () => pluginInfo?.connector_binding || inferFallbackConnectorBinding(pluginIdentifier),
    [pluginIdentifier, pluginInfo?.connector_binding]
  );

  const connectorPluginID = useMemo(
    () => pluginInfo?.id || pluginIdentifier,
    [pluginIdentifier, pluginInfo?.id]
  );

  const resolvedFormData = useMemo(() => {
    if (!nodeConfig) {
      return {};
    }

    if (lastSyncedNodeIdRef.current !== nodeConfig.id) {
      return nodeConfig.config || {};
    }

    return formData;
  }, [nodeConfig, formData]);

  useEffect(() => {
    if (!connectorPluginID || !connectorBinding) {
      setCompatibleConnectors([]);
      setConnectorsError(null);
      return;
    }

    let alive = true;
    setConnectorsLoading(true);
    setConnectorsError(null);
    connectorsAPI.listCompatible(connectorPluginID)
      .then((items) => {
        if (!alive) return;
        setCompatibleConnectors(items);
      })
      .catch((error: any) => {
        if (!alive) return;
        setCompatibleConnectors([]);
        setConnectorsError(error.message || 'Failed to load compatible connectors');
      })
      .finally(() => {
        if (!alive) return;
        setConnectorsLoading(false);
      });

    return () => {
      alive = false;
    };
  }, [connectorBinding, connectorPluginID]);

  const currentConnectorRef = useMemo(() => {
    const value = resolvedFormData?.connector_ref;
    return typeof value === 'string' ? value : '';
  }, [resolvedFormData]);

  const selectedConnector = useMemo(
    () => compatibleConnectors.find((item) => item.id === currentConnectorRef) || null,
    [compatibleConnectors, currentConnectorRef]
  );

  const selectedConnectorPreview = useMemo(
    () => buildConnectorPreviewEntries(selectedConnector),
    [selectedConnector]
  );

  const connectorAwareSchema = useMemo(() => {
    const rawSchema = pluginInfo?.capabilities?.config_schema;
    if (!connectorBinding || !rawSchema) {
      return rawSchema || '';
    }

    try {
      const parsed = typeof rawSchema === 'string' ? JSON.parse(rawSchema) : rawSchema;
      return stripConnectorRefField(parsed);
    } catch {
      return rawSchema;
    }
  }, [pluginInfo?.capabilities?.config_schema, connectorBinding]);

  const isJavaScriptTransform = normalizePluginIdentity(pluginInfo?.id || pluginIdentifier) === 'javascript-transform';
  const javaScriptExample = useMemo(() => {
    if (!isJavaScriptTransform || !connectorAwareSchema) return '';
    try {
      const parsed = typeof connectorAwareSchema === 'string' ? JSON.parse(connectorAwareSchema) : connectorAwareSchema;
      return typeof parsed?.properties?.code?.default === 'string' ? parsed.properties.code.default : '';
    } catch {
      return '';
    }
  }, [connectorAwareSchema, isJavaScriptTransform]);

  const handleConnectorSelect = useCallback((connectorRef: string) => {
    const nextConfig = {
      ...(resolvedFormData || {}),
      connector_ref: connectorRef,
    };
    updateConfig(nextConfig);
  }, [resolvedFormData, updateConfig]);

  useEffect(() => {
    if (!connectorBinding || currentConnectorRef || connectorsLoading || connectorsError) {
      return;
    }
    if (compatibleConnectors.length === 1) {
      handleConnectorSelect(compatibleConnectors[0].id);
    }
  }, [
    compatibleConnectors,
    connectorBinding,
    connectorsError,
    connectorsLoading,
    currentConnectorRef,
    handleConnectorSelect,
  ]);


  // 早期返回必须在所有 hooks 之后
  if (!nodeId || !nodeConfig) {
    return null;
  }

  // 判断是否使用 JSON Schema 渲染器（插件节点）
  const useJSONSchema = !!pluginInfo && hasConfigSchema;

  return (
    <div style={{ padding: '16px', height: '100%', overflow: 'auto' }}>
      <h3 style={{ marginTop: 0, marginBottom: '12px' }}>节点属性</h3>

      {/* 基本信息 */}
      <div style={{ marginBottom: '12px' }}>
        <label style={{ display: 'block', marginBottom: '4px', fontWeight: 'bold', fontSize: '13px' }}>
          节点ID
        </label>
        <input
          type="text"
          value={nodeConfig.id}
          disabled
          style={{ width: '100%', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', fontSize: '13px' }}
        />
      </div>

      <div style={{ marginBottom: '12px' }}>
        <label style={{ display: 'block', marginBottom: '4px', fontWeight: 'bold', fontSize: '13px' }}>
          节点名称
        </label>
        <input
          type="text"
          value={nodeConfig.name || ''}
          onChange={(e) => updateNode({ name: e.target.value })}
          style={{ width: '100%', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', fontSize: '13px' }}
        />
      </div>

      <div style={{ marginBottom: '12px' }}>
        <label style={{ display: 'block', marginBottom: '4px', fontWeight: 'bold', fontSize: '13px' }}>
          节点类型
        </label>
        <input
          type="text"
          value={nodeConfig.type}
          disabled
          style={{ width: '100%', padding: '6px 8px', border: '1px solid #ddd', borderRadius: '4px', fontSize: '13px' }}
        />
      </div>

      {/* 节点配置 */}
      {useJSONSchema ? (
        // 使用 JSON Schema 渲染器（所有插件节点，包括内置和外部）
        <div style={{ marginTop: '4px', marginBottom: '4px' }}>
          <h4 style={{ marginBottom: '6px', fontSize: '14px', fontWeight: 600 }}>节点配置</h4>
          {isJavaScriptTransform && (
            <div style={{ marginBottom: 14, padding: 14, border: '1px solid #bfdbfe', borderRadius: 12, background: '#eff6ff', color: '#1e3a5f' }}>
              <div style={{ fontWeight: 700, marginBottom: 6 }}>JavaScript 主函数</div>
              <div style={{ fontSize: 12, lineHeight: 1.7 }}>
                引擎调用 <code>main(input)</code>。使用 <code>console.log</code> 输出调试信息，
                在示例的“在这里编写业务处理代码”注释下修改逻辑，并返回包含 <code>payload</code> 的对象。
              </div>
              <div style={{ marginTop: 8, fontSize: 12, lineHeight: 1.7, color: 'var(--color-text-secondary)' }}>
                内置标准 JavaScript 对象及 console；不提供 require、npm、文件、网络或浏览器 API。
                第三方库应封装成独立、签名并固定版本的插件，不允许运行时在线安装。
              </div>
              {javaScriptExample && (
                <button
                  type="button"
                  onClick={() => updateConfig({ ...(resolvedFormData || {}), code: javaScriptExample })}
                  style={{ marginTop: 10, padding: '7px 12px', border: '1px solid #93c5fd', borderRadius: 8, background: 'var(--color-surface)', color: '#1d4ed8', cursor: 'pointer', fontWeight: 600 }}
                >
                  载入标准示例
                </button>
              )}
            </div>
          )}
          {connectorBinding && (
            <div style={{ marginBottom: '14px', padding: '12px', border: '1px solid #dbeafe', borderRadius: '10px', backgroundColor: '#f8fbff' }}>
              <div style={{ fontSize: '13px', fontWeight: 700, color: '#1d4ed8', marginBottom: '8px' }}>
                Connector 绑定
              </div>
              <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)', marginBottom: '8px', lineHeight: 1.6 }}>
                当前节点要求的 Connector Family: {(connectorBinding.accepted_families || []).join(', ') || '未指定'}
              </div>
              {!!connectorBinding.required_capabilities?.length && (
                <div style={{ fontSize: '12px', color: 'var(--color-text-secondary)', marginBottom: '8px', lineHeight: 1.6 }}>
                  必需能力: {connectorBinding.required_capabilities.join(', ')}
                </div>
              )}
              <select
                value={currentConnectorRef}
                onChange={(event) => handleConnectorSelect(event.target.value)}
                style={{ width: '100%', padding: '8px 10px', borderRadius: '8px', border: '1px solid var(--color-border)', fontSize: '13px', backgroundColor: 'var(--color-surface)', color: 'var(--color-text)' }}
              >
                <option value="">{connectorsLoading ? '加载兼容 Connector...' : '请选择 Connector 实例'}</option>
                {compatibleConnectors.map((connector) => (
                  <option key={connector.id} value={connector.id}>
                    {formatConnectorOptionLabel(connector)}
                  </option>
                ))}
              </select>
              {!connectorsLoading && compatibleConnectors.length > 0 && (
                <div style={{ marginTop: '8px', fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                  已维护 {compatibleConnectors.length} 个兼容 Connector 实例，优先通过下拉选择，避免手工录入。
                </div>
              )}
              {!connectorsLoading && !connectorsError && compatibleConnectors.length === 0 && (
                <div style={{ marginTop: '8px', color: '#b45309', fontSize: '12px', lineHeight: 1.6 }}>
                  当前没有可用的兼容 Connector 实例。请先到 Connector 资源页面创建该协议的实例，再返回选择。
                </div>
              )}
              {connectorsError && (
                <div style={{ marginTop: '8px', color: '#dc2626', fontSize: '12px' }}>
                  {connectorsError}
                </div>
              )}
              {currentConnectorRef && !selectedConnector && !connectorsLoading && (
                <div style={{ marginTop: '8px', color: '#dc2626', fontSize: '12px', lineHeight: 1.6 }}>
                  当前 connector_ref 不存在，或与该节点插件不兼容。
                </div>
              )}
              {selectedConnector && (
                <div style={{ marginTop: '10px', display: 'grid', gap: '8px', fontSize: '12px', color: 'var(--color-text-secondary)' }}>
                  <div><strong>实例:</strong> {selectedConnector.name || selectedConnector.id} ({selectedConnector.id})</div>
                  <div><strong>插件:</strong> {selectedConnector.plugin_info?.name || selectedConnector.plugin_id}</div>
                  <div><strong>状态:</strong> {selectedConnector.status?.status || 'unknown'}</div>
                  <div><strong>连接摘要:</strong> {selectedConnector.connection_summary || '未生成摘要'}</div>
                  {selectedConnectorPreview.length > 0 && (
                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: '6px' }}>
                      {selectedConnectorPreview.map((item) => (
                        <div
                          key={item.key}
                          style={{
                            padding: '8px 10px',
                            borderRadius: '8px',
                            border: '1px solid #dbeafe',
                            backgroundColor: 'var(--color-surface)',
                          }}
                        >
                          <div style={{ fontSize: '11px', color: 'var(--color-text-secondary)', marginBottom: '2px' }}>{item.label}</div>
                          <div style={{ fontSize: '12px', color: 'var(--color-text)', fontWeight: 600 }}>{item.value}</div>
                        </div>
                      ))}
                    </div>
                  )}
                  {selectedConnector.session_policy && <div><strong>会话策略:</strong> {selectedConnector.session_policy}</div>}
                </div>
              )}
            </div>
          )}
          <LazyJSONSchemaRenderer
            key={`${nodeConfig.id}-${pluginIdentifier}`}
            schema={connectorAwareSchema}
            formData={resolvedFormData}
            onChange={updateConfig}
            pluginId={pluginInfo?.id || pluginIdentifier}
            pluginName={pluginInfo.name}
          />
        </div>
      ) : pluginInfo && pluginIdentifier ? (
        <div style={{ padding: '12px', backgroundColor: '#fff7e6', borderRadius: '4px', color: '#ad6800', fontSize: '13px' }}>
          <strong>⚠️ 提示：</strong>插件 "{pluginIdentifier}" 已加载，但未声明 `config_schema`，因此当前无法渲染配置表单。
        </div>
      ) : !pluginInfo && nodeConfig && pluginIdentifier ? (
        // 如果找不到插件信息，显示提示
        <div style={{ padding: '12px', backgroundColor: '#fff3cd', borderRadius: '4px', color: '#856404', fontSize: '13px' }}>
          <strong>⚠️ 警告：</strong>未找到节点类型 "{pluginIdentifier}" 的插件信息。请检查插件是否正确加载。
        </div>
      ) : null}

      {/* 插件信息显示 - 移到最下方 */}
      {pluginInfo && (
        <div style={{ marginTop: '16px', padding: '10px', backgroundColor: '#f5f5f5', borderRadius: '4px', borderTop: '1px solid #EAECF0' }}>
          <label style={{ display: 'block', marginBottom: '6px', fontWeight: 'bold', fontSize: '13px' }}>
            插件信息
          </label>
          <div style={{ fontSize: '12px', color: '#666', lineHeight: '1.6' }}>
            <div style={{ marginBottom: '4px' }}><strong>名称:</strong> {pluginInfo.name}</div>
            <div style={{ marginBottom: '4px' }}><strong>版本:</strong> {pluginInfo.version}</div>
            <div style={{ marginBottom: '4px' }}><strong>类型:</strong> {pluginInfo.type}</div>
            {pluginInfo.description && (
              <div><strong>描述:</strong> {pluginInfo.description}</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
