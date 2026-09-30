import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Card, Checkbox, Empty, Input, Modal, Pagination, Select, Space, Switch, Tag } from 'antd';
import { ConsolePage } from '../components/ConsolePage';
import { LazyJSONSchemaRenderer } from '../components/LazyJSONSchemaRenderer';
import { ResourceTabs } from '../components/ResourceTabs';
import { useFeedback } from '../components/FeedbackProvider';
import { ActionButton, AsyncState, FilterBar } from '../components/ui/ConsoleUI';
import { connectorsAPI, pluginsAPI } from '../services/api';
import {
  connectorStatusLabel,
  defaultConnectorSessionPolicy,
  effectiveConnectorSessionPolicy,
  sessionPoliciesForPlugin,
} from '../utils/connectorLifecycle';
import {
  ConnectorHealthPolicy,
  ConnectorProviderPolicy,
  ConnectorQuotaPolicy,
  ConnectorResource,
  ConnectorResourceView,
  PluginInfo,
  ResourceStatus,
} from '../types';

type ConnectorDraft = ConnectorResource;
type TimescaleDSNFields = {
  host: string;
  port: string;
  database: string;
  user: string;
  password: string;
  sslmode: string;
};

const CONNECTOR_PREVIEW_LABELS: Record<string, string> = {
  host: 'IP / Host',
  port: 'Port',
  rack: 'Rack',
  slot: 'Slot',
  database: 'Database',
  user: 'User',
};

const defaultHealthPolicy = (): ConnectorHealthPolicy => ({
  enabled: false,
  interval_ms: 5000,
  timeout_ms: 3000,
  target: '',
  priority: 'normal',
});

const defaultQuotaPolicy = (): ConnectorQuotaPolicy => ({
  max_inflight_requests: 64,
  max_inflight_weight: 128,
  default_usage_weight: 1,
});

const defaultProviderPolicy = (): ConnectorProviderPolicy => ({
  max_resources_per_provider: 0,
  max_inflight_per_provider: 0,
  preferred_provider_count: 1,
});

const emptyDraft = (): ConnectorDraft => ({
  id: '',
  name: '',
  plugin_id: '',
  connection: {},
  session_policy: '',
  warmup_policy: 'lazy',
  health_policy: defaultHealthPolicy(),
  quota_policy: defaultQuotaPolicy(),
  provider_policy: defaultProviderPolicy(),
  enabled: true,
  metadata: {},
});

const normalizeDraft = (resource?: ConnectorResourceView | null): ConnectorDraft => {
  if (!resource) {
    return emptyDraft();
  }
  return {
    id: resource.id || '',
    name: resource.name || '',
    plugin_id: resource.plugin_id || '',
    connection: resource.connection || {},
    session_policy: effectiveConnectorSessionPolicy(resource),
    warmup_policy: resource.warmup_policy || 'lazy',
    health_policy: {
      ...defaultHealthPolicy(),
      ...(resource.health_policy || {}),
    },
    quota_policy: {
      ...defaultQuotaPolicy(),
      ...(resource.quota_policy || {}),
    },
    provider_policy: {
      ...defaultProviderPolicy(),
      ...(resource.provider_policy || {}),
    },
    enabled: resource.enabled !== false,
    metadata: resource.metadata || {},
  };
};

const statusColor = (status?: ResourceStatus): string => {
  switch (status) {
    case 'healthy':
      return 'green';
    case 'degraded':
    case 'recovering':
      return 'gold';
    case 'congested':
    case 'readonly':
      return 'orange';
    case 'disconnected':
    case 'blocked':
      return 'red';
    default:
      return 'default';
  }
};

const statusLabel = (status?: ResourceStatus): string => {
  switch (status) {
    case 'healthy':
      return 'Healthy';
    case 'degraded':
      return 'Degraded';
    case 'recovering':
      return 'Recovering';
    case 'congested':
      return 'Congested';
    case 'readonly':
      return 'Readonly';
    case 'disconnected':
      return 'Disconnected';
    case 'blocked':
      return 'Blocked';
    default:
      return 'Unknown';
  }
};

const enabledLabel = (enabled: boolean): string => (enabled ? 'Enabled' : 'Disabled');

const isS7Connector = (plugin?: PluginInfo | null) => plugin?.connector_descriptor?.family === 's7';
const isTimescaleConnector = (plugin?: PluginInfo | null) => plugin?.connector_descriptor?.family === 'timescaledb';

const isPlainObject = (value: unknown): value is Record<string, any> =>
  value !== null && typeof value === 'object' && !Array.isArray(value);

const parseDSNPreview = (dsn: string): Record<string, string> => {
  const normalized = dsn.trim();
  if (!normalized) {
    return {};
  }

  if (normalized.startsWith('postgres://') || normalized.startsWith('postgresql://')) {
    try {
      const parsed = new URL(normalized);
      const preview: Record<string, string> = {};
      if (parsed.hostname) preview.host = parsed.hostname;
      if (parsed.port) preview.port = parsed.port;
      const database = parsed.pathname.replace(/^\//, '');
      if (database) preview.database = database;
      if (parsed.username) preview.user = parsed.username;
      return preview;
    } catch {
      return {};
    }
  }

  const preview: Record<string, string> = {};
  for (const token of normalized.split(/\s+/)) {
    const [rawKey, rawValue] = token.split('=', 2);
    if (!rawKey || rawValue === undefined) continue;
    const key = rawKey.trim().toLowerCase();
    const value = rawValue.trim().replace(/^['"]|['"]$/g, '');
    switch (key) {
      case 'host':
        preview.host = value;
        break;
      case 'port':
        preview.port = value;
        break;
      case 'dbname':
      case 'database':
        preview.database = value;
        break;
      case 'user':
      case 'username':
        preview.user = value;
        break;
      default:
        break;
    }
  }
  return preview;
};

const parseTimescaleDSNFields = (dsn: string): TimescaleDSNFields => {
  const empty: TimescaleDSNFields = {
    host: '',
    port: '',
    database: '',
    user: '',
    password: '',
    sslmode: 'disable',
  };

  const normalized = dsn.trim();
  if (!normalized) {
    return empty;
  }

  if (normalized.startsWith('postgres://') || normalized.startsWith('postgresql://')) {
    try {
      const parsed = new URL(normalized);
      return {
        host: parsed.hostname || '',
        port: parsed.port || '',
        database: parsed.pathname.replace(/^\//, ''),
        user: decodeURIComponent(parsed.username || ''),
        password: decodeURIComponent(parsed.password || ''),
        sslmode: parsed.searchParams.get('sslmode') || 'disable',
      };
    } catch {
      return empty;
    }
  }

  const next = { ...empty };
  for (const token of normalized.split(/\s+/)) {
    const [rawKey, rawValue] = token.split('=', 2);
    if (!rawKey || rawValue === undefined) continue;
    const key = rawKey.trim().toLowerCase();
    const value = rawValue.trim().replace(/^['"]|['"]$/g, '');
    switch (key) {
      case 'host':
        next.host = value;
        break;
      case 'port':
        next.port = value;
        break;
      case 'dbname':
      case 'database':
        next.database = value;
        break;
      case 'user':
      case 'username':
        next.user = value;
        break;
      case 'password':
        next.password = value;
        break;
      case 'sslmode':
        next.sslmode = value || 'disable';
        break;
      default:
        break;
    }
  }
  return next;
};

const composeTimescaleDSN = (fields: TimescaleDSNFields): string => {
  const tokens: string[] = [];
  if (fields.host.trim()) tokens.push(`host=${fields.host.trim()}`);
  if (fields.port.trim()) tokens.push(`port=${fields.port.trim()}`);
  if (fields.database.trim()) tokens.push(`dbname=${fields.database.trim()}`);
  if (fields.user.trim()) tokens.push(`user=${fields.user.trim()}`);
  if (fields.password.trim()) tokens.push(`password=${fields.password}`);
  tokens.push(`sslmode=${(fields.sslmode || 'disable').trim() || 'disable'}`);
  return tokens.join(' ');
};

const sanitizeIdentifierPart = (value: string): string =>
  value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');

const buildConnectorSuggestions = (
  plugin: PluginInfo | null,
  preview: Record<string, string>
): { id: string; name: string } | null => {
  if (!plugin) {
    return null;
  }

  const family = plugin.connector_descriptor?.family || plugin.id;
  if (family === 's7') {
    const host = sanitizeIdentifierPart(preview.host || 'plc');
    const rack = sanitizeIdentifierPart(preview.rack || '0');
    const slot = sanitizeIdentifierPart(preview.slot || '1');
    const port = preview.port ? `:${preview.port}` : '';
    return {
      id: [family, host, `r${rack}`, `s${slot}`].filter(Boolean).join('-'),
      name: `S7 ${preview.host || 'PLC'}${port} R${preview.rack || '0'} S${preview.slot || '1'}`,
    };
  }

  if (family === 'timescaledb') {
    const host = sanitizeIdentifierPart(preview.host || 'db');
    const database = sanitizeIdentifierPart(preview.database || 'main');
    const port = preview.port ? `:${preview.port}` : '';
    return {
      id: [family, host, database].filter(Boolean).join('-'),
      name: `TimescaleDB ${preview.host || 'DB'}${port}/${preview.database || 'main'}`,
    };
  }

  return {
    id: sanitizeIdentifierPart(plugin.id || 'connector'),
    name: plugin.name || plugin.id,
  };
};

const buildConnectionPreview = (plugin: PluginInfo | null, connection?: Record<string, any>): Record<string, string> => {
  if (!plugin || !isPlainObject(connection)) {
    return {};
  }

  const family = plugin.connector_descriptor?.family;
  if (family === 's7') {
    const preview: Record<string, string> = {};
    if (connection.host) preview.host = String(connection.host);
    if (connection.port !== undefined && connection.port !== null && String(connection.port) !== '') preview.port = String(connection.port);
    if (connection.rack !== undefined && connection.rack !== null && String(connection.rack) !== '') preview.rack = String(connection.rack);
    if (connection.slot !== undefined && connection.slot !== null && String(connection.slot) !== '') preview.slot = String(connection.slot);
    return preview;
  }

  if (family === 'timescaledb') {
    return parseDSNPreview(String(connection.dsn || ''));
  }

  const preview: Record<string, string> = {};
  for (const key of plugin.connector_descriptor?.display_fields || []) {
    const value = connection[key];
    if (value !== undefined && value !== null && String(value) !== '') {
      preview[key] = String(value);
    }
  }
  return preview;
};

const previewEntriesFromMap = (
  preview?: Record<string, string>,
  preferredOrder?: string[]
): Array<{ key: string; label: string; value: string }> => {
  if (!preview) {
    return [];
  }
  const seen = new Set<string>();
  const orderedKeys: string[] = [];

  for (const key of preferredOrder || []) {
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

const pluginConnectionHint = (plugin: PluginInfo | null): string => {
  if (!plugin) {
    return '先选择一个 connector 插件，再配置 connection。';
  }
  const family = plugin.connector_descriptor?.family;
  switch (family) {
    case 's7':
      return '推荐直接维护 PLC 的 IP / Port / Rack / Slot。后续 source-s7 与 sink-s7 只绑定该实例，不再重复填写连接参数。';
    case 'timescaledb':
      return '推荐维护单一 DSN 与连接池参数。业务节点只选择该实例，不再重复填写数据库连接信息。';
    default:
      return `${plugin.name} 的连接配置。优先维护成可复用实例，业务节点只负责选择。`;
  }
};

type ConnectorBatchAction = 'enable' | 'disable' | 'delete';

const summarizeByStatus = (items: ConnectorResourceView[]): Array<{ key: string; label: string; count: number }> => {
  const counts = new Map<string, number>();
  for (const item of items) {
    const key = item.status?.status || 'unknown';
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return Array.from(counts.entries())
    .map(([key, count]) => ({ key, label: statusLabel(key as ResourceStatus), count }))
    .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label, 'zh-Hans-CN'));
};

export const ConnectorsPage: React.FC = () => {
  const feedback = useFeedback();
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [saving, setSaving] = useState(false);
  const [batchAction, setBatchAction] = useState<ConnectorBatchAction | null>(null);
  const [connectors, setConnectors] = useState<ConnectorResourceView[]>([]);
  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [pluginFilter, setPluginFilter] = useState<string>('all');
  const [familyFilter, setFamilyFilter] = useState<string>('all');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [enabledFilter, setEnabledFilter] = useState<string>('all');
  const [showFilters, setShowFilters] = useState(false);
  const [selectedConnectorIDs, setSelectedConnectorIDs] = useState<string[]>([]);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingResource, setEditingResource] = useState<ConnectorResourceView | null>(null);
  const [draft, setDraft] = useState<ConnectorDraft>(emptyDraft);

  const connectorPlugins = useMemo(
    () => plugins.filter((plugin) => plugin.type === 'connector'),
    [plugins]
  );

  const selectedPlugin = useMemo(
    () => connectorPlugins.find((plugin) => plugin.id === draft.plugin_id) || null,
    [connectorPlugins, draft.plugin_id]
  );

  const draftConnectionPreview = useMemo(
    () => buildConnectionPreview(selectedPlugin, draft.connection),
    [draft.connection, selectedPlugin]
  );

  const draftConnectionPreviewEntries = useMemo(
    () => previewEntriesFromMap(draftConnectionPreview, selectedPlugin?.connector_descriptor?.display_fields),
    [draftConnectionPreview, selectedPlugin]
  );

  const timescaleDSNFields = useMemo(
    () => parseTimescaleDSNFields(String(draft.connection?.dsn || '')),
    [draft.connection]
  );

  const draftSuggestions = useMemo(
    () => buildConnectorSuggestions(selectedPlugin, draftConnectionPreview),
    [draftConnectionPreview, selectedPlugin]
  );

  const loadData = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      const [pluginInfos, connectorViews] = await Promise.all([
        pluginsAPI.listPluginInfos(),
        connectorsAPI.list(),
      ]);
      setPlugins(pluginInfos);
      setConnectors(connectorViews);
    } catch (error: any) {
      setLoadError(error.message || 'Connector 服务暂时不可用');
      feedback.error(`加载 Connector 资源失败: ${error.message || '未知错误'}`);
    } finally {
      setLoading(false);
    }
  }, [feedback]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    setSelectedConnectorIDs((current) =>
      current.filter((id) => connectors.some((connector) => connector.id === id))
    );
  }, [connectors]);

  const familyOptions = useMemo(() => {
    const families = connectorPlugins
      .map((plugin) => plugin.connector_descriptor?.family)
      .filter((family): family is string => !!family);
    return Array.from(new Set(families)).sort((left, right) => left.localeCompare(right, 'zh-Hans-CN'));
  }, [connectorPlugins]);

  const filteredConnectors = useMemo(() => {
    return connectors.filter((connector) => {
      const matchesSearch =
        connector.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
        (connector.name || '').toLowerCase().includes(searchTerm.toLowerCase()) ||
        (connector.connection_summary || '').toLowerCase().includes(searchTerm.toLowerCase());
      const matchesPlugin = pluginFilter === 'all' || connector.plugin_id === pluginFilter;
      const family = connector.plugin_info?.connector_descriptor?.family || connector.plugin_id;
      const matchesFamily = familyFilter === 'all' || family === familyFilter;
      const status = connector.status?.status || 'unknown';
      const matchesStatus = statusFilter === 'all' || status === statusFilter;
      const matchesEnabled =
        enabledFilter === 'all' ||
        (enabledFilter === 'enabled' && connector.enabled) ||
        (enabledFilter === 'disabled' && !connector.enabled);
      return matchesSearch && matchesPlugin && matchesFamily && matchesStatus && matchesEnabled;
    });
  }, [connectors, enabledFilter, familyFilter, pluginFilter, searchTerm, statusFilter]);

  const paginatedConnectors = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filteredConnectors.slice(start, start + pageSize);
  }, [currentPage, filteredConnectors, pageSize]);

  useEffect(() => {
    setCurrentPage(1);
  }, [enabledFilter, familyFilter, pluginFilter, searchTerm, statusFilter]);

  useEffect(() => {
    const lastPage = Math.max(1, Math.ceil(filteredConnectors.length / pageSize));
    setCurrentPage((page) => Math.min(page, lastPage));
  }, [filteredConnectors.length, pageSize]);

  const selectedConnectorSet = useMemo(() => new Set(selectedConnectorIDs), [selectedConnectorIDs]);

  const selectedConnectors = useMemo(
    () => connectors.filter((connector) => selectedConnectorSet.has(connector.id)),
    [connectors, selectedConnectorSet]
  );

  const allFilteredSelected =
    filteredConnectors.length > 0 && filteredConnectors.every((connector) => selectedConnectorSet.has(connector.id));

  const statusSummary = useMemo(() => summarizeByStatus(connectors), [connectors]);

  const openCreateModal = () => {
    setEditingResource(null);
    setDraft(emptyDraft());
    setEditorOpen(true);
  };

  const openEditModal = (resource: ConnectorResourceView) => {
    setEditingResource(resource);
    setDraft(normalizeDraft(resource));
    setEditorOpen(true);
  };

  const closeEditor = () => {
    if (saving) return;
    setEditorOpen(false);
    setEditingResource(null);
    setDraft(emptyDraft());
  };

  const updateDraft = (patch: Partial<ConnectorDraft>) => {
    setDraft((current) => ({ ...current, ...patch }));
  };

  const updateHealthPolicy = (patch: Partial<ConnectorHealthPolicy>) => {
    setDraft((current) => ({
      ...current,
      health_policy: {
        ...defaultHealthPolicy(),
        ...(current.health_policy || {}),
        ...patch,
      },
    }));
  };

  const updateQuotaPolicy = (patch: Partial<ConnectorQuotaPolicy>) => {
    setDraft((current) => ({
      ...current,
      quota_policy: {
        ...defaultQuotaPolicy(),
        ...(current.quota_policy || {}),
        ...patch,
      },
    }));
  };

  const updateProviderPolicy = (patch: Partial<ConnectorProviderPolicy>) => {
    setDraft((current) => ({
      ...current,
      provider_policy: {
        ...defaultProviderPolicy(),
        ...(current.provider_policy || {}),
        ...patch,
      },
    }));
  };

	const handlePluginChange = (pluginID: string) => {
    const plugin = connectorPlugins.find((item) => item.id === pluginID) || null;
    setDraft((current) => ({
      ...current,
      plugin_id: pluginID,
		session_policy: defaultConnectorSessionPolicy(plugin),
      connection: {},
    }));
  };

  const updateTimescaleDSNField = useCallback((key: keyof TimescaleDSNFields, value: string) => {
    setDraft((current) => {
      const currentFields = parseTimescaleDSNFields(String(current.connection?.dsn || ''));
      const nextFields: TimescaleDSNFields = {
        ...currentFields,
        [key]: value,
      };
      return {
        ...current,
        connection: {
          ...(current.connection || {}),
          dsn: composeTimescaleDSN(nextFields),
        },
      };
    });
  }, []);

  useEffect(() => {
    if (editingResource || !draftSuggestions) {
      return;
    }
    setDraft((current) => {
      const patch: Partial<ConnectorDraft> = {};
      if (!current.id.trim() && draftSuggestions.id) {
        patch.id = draftSuggestions.id;
      }
      if (!String(current.name || '').trim() && draftSuggestions.name) {
        patch.name = draftSuggestions.name;
      }
      if (Object.keys(patch).length === 0) {
        return current;
      }
      return {
        ...current,
        ...patch,
      };
    });
  }, [draftSuggestions, editingResource]);

  const handleSave = async () => {
    if (!draft.id.trim()) {
      feedback.warning('请输入 Connector ID');
      return;
    }
    if (!draft.plugin_id.trim()) {
      feedback.warning('请选择 Connector 插件');
      return;
    }

    setSaving(true);
    try {
      const payload: ConnectorResource = {
        ...draft,
        id: draft.id.trim(),
        name: draft.name?.trim(),
      };
      if (editingResource) {
        await connectorsAPI.update(editingResource.id, payload);
        feedback.success(`Connector ${payload.id} 已更新`);
      } else {
        await connectorsAPI.create(payload);
        feedback.success(`Connector ${payload.id} 已创建`);
      }
      closeEditor();
      await loadData();
    } catch (error: any) {
      feedback.error(`保存 Connector 失败: ${error.message || '未知错误'}`);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (resource: ConnectorResourceView) => {
    if (!await feedback.confirm({ title: `确定要删除 Connector "${resource.name || resource.id}" 吗？`, danger: true })) {
      return;
    }
    try {
      await connectorsAPI.delete(resource.id);
      feedback.success(`Connector ${resource.id} 已删除`);
      await loadData();
    } catch (error: any) {
      feedback.error(`删除失败: ${error.message || '未知错误'}`);
    }
  };

  const toggleConnectorSelection = (connectorID: string, checked: boolean) => {
    setSelectedConnectorIDs((current) => {
      if (checked) {
        if (current.includes(connectorID)) {
          return current;
        }
        return [...current, connectorID];
      }
      return current.filter((id) => id !== connectorID);
    });
  };

  const toggleFilteredSelection = (checked: boolean) => {
    if (!checked) {
      const filteredIDs = new Set(filteredConnectors.map((connector) => connector.id));
      setSelectedConnectorIDs((current) => current.filter((id) => !filteredIDs.has(id)));
      return;
    }
    setSelectedConnectorIDs((current) => {
      const next = new Set(current);
      for (const connector of filteredConnectors) {
        next.add(connector.id);
      }
      return Array.from(next);
    });
  };

  const handleBatchAction = async (action: ConnectorBatchAction) => {
    if (batchAction) return;
    if (selectedConnectors.length === 0) {
      feedback.warning('请先选择至少一个 Connector 资源');
      return;
    }

    const actionLabel =
      action === 'enable' ? '批量启用' : action === 'disable' ? '批量停用' : '批量删除';
    const actionVerb =
      action === 'enable' ? '启用' : action === 'disable' ? '停用' : '删除';

    if (action === 'delete') {
      const confirmed = await feedback.confirm({
        title: `确定要批量删除 ${selectedConnectors.length} 个 Connector 吗？`,
        content: selectedConnectors.map((connector) => connector.id).join('、'),
        danger: true,
      });
      if (!confirmed) {
        return;
      }
    }

    setBatchAction(action);
    try {
      const results = await Promise.allSettled(
        selectedConnectors.map(async (connector) => {
          if (action === 'delete') {
            await connectorsAPI.delete(connector.id);
            return;
          }
          const payload: ConnectorResource = {
            ...normalizeDraft(connector),
            id: connector.id,
            name: connector.name,
            plugin_id: connector.plugin_id,
            enabled: action === 'enable',
          };
          await connectorsAPI.update(connector.id, payload);
        })
      );

      const failed = results.filter((result) => result.status === 'rejected');
      if (failed.length > 0) {
        const firstError = failed[0] as PromiseRejectedResult;
        feedback.error(`${actionLabel}失败 ${failed.length} 个: ${firstError.reason?.message || '未知错误'}`);
      } else {
        feedback.success(`${actionVerb} ${selectedConnectors.length} 个 Connector 完成`);
      }
      setSelectedConnectorIDs([]);
      await loadData();
    } catch (error: any) {
      feedback.error(`${actionLabel}失败: ${error.message || '未知错误'}`);
    } finally {
      setBatchAction(null);
    }
  };

  const sessionOptions = useMemo(() => {
    const policies = sessionPoliciesForPlugin(selectedPlugin);
    if (draft.session_policy && !policies.includes(draft.session_policy)) {
      policies.push(draft.session_policy);
    }
    return policies.map((policy) => ({ label: policy, value: policy }));
  }, [draft.session_policy, selectedPlugin]);

  return (
    <ConsolePage maxWidth={1400}>
      <ResourceTabs />
      <h1 className="sr-only">Connector 资源</h1>
      <FilterBar
        summary={`共 ${filteredConnectors.length} / ${connectors.length} 个连接资源`}
        actions={
          <Space wrap>
            <Button onClick={() => setShowFilters((value) => !value)}>
              {showFilters ? '收起筛选' : '更多筛选'}
            </Button>
            <Button onClick={() => void loadData()} loading={loading}>刷新</Button>
            <Button type="primary" onClick={openCreateModal}>新建 Connector</Button>
          </Space>
        }
      >
        <Input.Search
          allowClear
          aria-label="搜索连接资源"
          placeholder="搜索 ID、名称或连接摘要"
          value={searchTerm}
          onChange={(event) => setSearchTerm(event.target.value)}
          style={{ flex: 1, minWidth: 260 }}
        />
      </FilterBar>

      {showFilters ? (
        <section className="console-advanced-filters" aria-label="更多筛选条件">
          <Select
            aria-label="筛选 Connector 插件"
            value={pluginFilter}
            onChange={setPluginFilter}
            style={{ minWidth: 220 }}
            options={[
              { label: '全部 Connector 插件', value: 'all' },
              ...connectorPlugins.map((plugin) => ({
                label: `${plugin.name} (${plugin.id})`,
                value: plugin.id,
              })),
            ]}
          />
          <Select
            aria-label="筛选协议族"
            value={familyFilter}
            onChange={setFamilyFilter}
            style={{ minWidth: 180 }}
            options={[
              { label: '全部协议族', value: 'all' },
              ...familyOptions.map((family) => ({
                label: family,
                value: family,
              })),
            ]}
          />
          <Select
            aria-label="筛选连接状态"
            value={statusFilter}
            onChange={setStatusFilter}
            style={{ minWidth: 180 }}
            options={[
              { label: '全部状态', value: 'all' },
              ...statusSummary.map((item) => ({
                label: `${item.label} (${item.count})`,
                value: item.key,
              })),
            ]}
          />
          <Select
            aria-label="筛选启停状态"
            value={enabledFilter}
            onChange={setEnabledFilter}
            style={{ minWidth: 160 }}
            options={[
              { label: '全部启停状态', value: 'all' },
              { label: '仅已启用', value: 'enabled' },
              { label: '仅已停用', value: 'disabled' },
            ]}
          />
        </section>
      ) : null}

      {selectedConnectorIDs.length > 0 ? (
        <section className="console-selection-bar" aria-label="批量操作">
          <Space wrap>
            <Checkbox checked={allFilteredSelected} indeterminate={!allFilteredSelected && selectedConnectorIDs.length > 0} onChange={(event) => toggleFilteredSelection(event.target.checked)}>
              选择当前筛选结果
            </Checkbox>
            <Tag>{selectedConnectorIDs.length} 个已选</Tag>
          </Space>
          <Space wrap>
            <ActionButton loading={batchAction === 'enable'} pendingText="启用中" disabled={selectedConnectorIDs.length === 0 || Boolean(batchAction)} onClick={() => void handleBatchAction('enable')}>
              批量启用
            </ActionButton>
            <ActionButton loading={batchAction === 'disable'} pendingText="停用中" disabled={selectedConnectorIDs.length === 0 || Boolean(batchAction)} onClick={() => void handleBatchAction('disable')}>
              批量停用
            </ActionButton>
            <ActionButton danger loading={batchAction === 'delete'} pendingText="删除中" disabled={selectedConnectorIDs.length === 0 || Boolean(batchAction)} onClick={() => void handleBatchAction('delete')}>
              批量删除
            </ActionButton>
            <Button disabled={selectedConnectorIDs.length === 0 || Boolean(batchAction)} onClick={() => setSelectedConnectorIDs([])}>
              清空已选
            </Button>
          </Space>
        </section>
      ) : null}

      <AsyncState
        loading={loading && connectors.length === 0}
        error={loadError || undefined}
        onRetry={() => void loadData()}
        empty={!loading && !loadError && filteredConnectors.length === 0}
        emptyDescription={searchTerm || pluginFilter !== 'all' ? '没有匹配的连接资源，请调整筛选条件。' : '还没有连接资源，可以先创建一个。'}
        emptyAction={!searchTerm && pluginFilter === 'all' ? <Button type="primary" onClick={openCreateModal}>新建 Connector</Button> : undefined}
      >
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: 16 }}>
          {paginatedConnectors.map((connector) => (
            <Card
              key={connector.id}
              style={{
                borderRadius: 18,
                boxShadow: selectedConnectorSet.has(connector.id)
                  ? '0 14px 36px rgba(37, 99, 235, 0.14)'
                  : '0 10px 30px rgba(15, 23, 42, 0.05)',
                borderColor: selectedConnectorSet.has(connector.id) ? '#93c5fd' : undefined,
              }}
              title={
                <div style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                  <Checkbox
                    aria-label={`选择连接资源 ${connector.name || connector.id}`}
                    checked={selectedConnectorSet.has(connector.id)}
                    onChange={(event) => toggleConnectorSelection(connector.id, event.target.checked)}
                    style={{ marginTop: 2 }}
                  />
                  <div style={{ display: 'grid', gap: 4 }}>
                    <div style={{ fontWeight: 700 }}>{connector.name || connector.id}</div>
                    <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 12, color: 'var(--color-text-secondary)' }}>{connector.id}</div>
                  </div>
                </div>
              }
              extra={
                <Space size={6} wrap>
                  <Tag>{enabledLabel(connector.enabled)}</Tag>
				  <Tag color={statusColor(connector.status?.status)}>{connectorStatusLabel(connector)}</Tag>
                </Space>
              }
              actions={[
                <Button type="link" key="edit" onClick={() => openEditModal(connector)}>编辑</Button>,
                <Button type="link" danger key="delete" onClick={() => void handleDelete(connector)}>删除</Button>,
              ]}
            >
              <div style={{ display: 'grid', gap: 10 }}>
                <div>
                  <Tag color="blue">{connector.plugin_info?.connector_descriptor?.label || connector.plugin_id}</Tag>
                  {connector.plugin_info?.connector_descriptor?.family ? <Tag>{connector.plugin_info.connector_descriptor.family}</Tag> : null}
                  {effectiveConnectorSessionPolicy(connector) ? <Tag>{effectiveConnectorSessionPolicy(connector)}</Tag> : null}
                  {connector.warmup_policy ? <Tag>{connector.warmup_policy}</Tag> : null}
                </div>
                <InfoRow label="连接摘要" value={connector.connection_summary || '未生成摘要'} />
                <InfoRow label="引用数" value={String(connector.ref_count || 0)} />
                <details className="connector-card-details">
                  <summary>更多连接与运行信息</summary>
                  <div className="connector-card-details__content">
                    {!!connector.connection_preview && Object.keys(connector.connection_preview).length > 0 ? (
                      <PreviewGrid
                        entries={previewEntriesFromMap(connector.connection_preview, connector.plugin_info?.connector_descriptor?.display_fields)}
                      />
                    ) : null}
                    <InfoRow label="队列" value={`${connector.pending || 0} pending / ${connector.inflight_weight || 0} weight`} />
                    {connector.status?.message ? <InfoRow label="状态信息" value={connector.status.message} /> : null}
                  </div>
                </details>
              </div>
            </Card>
          ))}
        </div>
        {filteredConnectors.length > 0 ? (
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 20 }}>
            <Pagination
              current={currentPage}
              pageSize={pageSize}
              total={filteredConnectors.length}
              pageSizeOptions={[20, 50, 100]}
              showSizeChanger
              showTotal={(total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`}
              onChange={(page, nextPageSize) => {
                if (nextPageSize !== pageSize) {
                  setPageSize(nextPageSize);
                  setCurrentPage(1);
                  return;
                }
                setCurrentPage(page);
              }}
            />
          </div>
        ) : null}
      </AsyncState>

      <Modal
        title={editingResource ? `编辑 Connector: ${editingResource.id}` : '新建 Connector'}
        open={editorOpen}
        onCancel={closeEditor}
        onOk={() => void handleSave()}
        okText={editingResource ? '保存修改' : '创建'}
        cancelText="取消"
        confirmLoading={saving}
        width={960}
        destroyOnHidden
      >
        <div style={{ display: 'grid', gap: 20 }}>
          <Card style={sectionStyle} styles={{ body: { padding: 0 } }}>
            <SectionTitle title="基础信息" description="Connector 实例是可复用的资源记录，规则节点绑定的是实例 ID。" />
            <div style={gridStyle}>
              <Field label="Connector ID" required>
                <Input value={draft.id} disabled={!!editingResource} onChange={(event) => updateDraft({ id: event.target.value })} />
              </Field>
              <Field label="显示名称">
                <Input value={draft.name} onChange={(event) => updateDraft({ name: event.target.value })} />
              </Field>
              <Field label="Connector 插件" required>
                <Select
                  value={draft.plugin_id || undefined}
                  disabled={!!editingResource}
                  onChange={handlePluginChange}
                  options={connectorPlugins.map((plugin) => ({
                    label: `${plugin.name} (${plugin.id})`,
                    value: plugin.id,
                  }))}
                />
              </Field>
              <Field label="启用">
                <Switch checked={draft.enabled} onChange={(checked) => updateDraft({ enabled: checked })} />
              </Field>
              <Field label="Session Policy">
                <Select
                  value={draft.session_policy || undefined}
                  onChange={(value) => updateDraft({ session_policy: value })}
                  options={sessionOptions}
                />
              </Field>
              <Field label="Warm-up">
                <Select
                  value={draft.warmup_policy || 'lazy'}
                  onChange={(value) => updateDraft({ warmup_policy: value })}
                  options={[
                    { label: 'lazy', value: 'lazy' },
                    { label: 'preload', value: 'preload' },
                    { label: 'always_on', value: 'always_on' },
                  ]}
                />
              </Field>
            </div>
            {!editingResource && draftSuggestions && (
              <div style={{ marginTop: 12, fontSize: 12, color: 'var(--color-text-secondary)', lineHeight: 1.7 }}>
                已按当前连接信息自动建议:
                <span style={{ marginLeft: 8, fontFamily: 'JetBrains Mono, monospace' }}>ID={draftSuggestions.id || '-'}</span>
                <span style={{ marginLeft: 12 }}>名称={draftSuggestions.name || '-'}</span>
              </div>
            )}
          </Card>

          <Card style={sectionStyle} styles={{ body: { padding: 0 } }}>
            <SectionTitle
              title="连接参数"
              description={pluginConnectionHint(selectedPlugin)}
            />
            {selectedPlugin ? (
              <div style={{ display: 'grid', gap: 14 }}>
                {isTimescaleConnector(selectedPlugin) && (
                  <div style={previewPanelStyle}>
                    <div style={{ fontSize: 13, fontWeight: 700, color: '#1d4ed8', marginBottom: 8 }}>
                      TimescaleDB DSN 快速填写
                    </div>
                    <div style={{ color: 'var(--color-text-secondary)', fontSize: 12, lineHeight: 1.7, marginBottom: 10 }}>
                      可先按主机、端口、数据库、账号快速填写，系统会自动回写到下方 `dsn` 字段。
                    </div>
                    <div style={gridStyle}>
                      <Field label="Host">
                        <Input value={timescaleDSNFields.host} onChange={(event) => updateTimescaleDSNField('host', event.target.value)} />
                      </Field>
                      <Field label="Port">
                        <Input value={timescaleDSNFields.port} onChange={(event) => updateTimescaleDSNField('port', event.target.value)} />
                      </Field>
                      <Field label="Database">
                        <Input value={timescaleDSNFields.database} onChange={(event) => updateTimescaleDSNField('database', event.target.value)} />
                      </Field>
                      <Field label="User">
                        <Input value={timescaleDSNFields.user} onChange={(event) => updateTimescaleDSNField('user', event.target.value)} />
                      </Field>
                      <Field label="Password">
                        <Input.Password value={timescaleDSNFields.password} onChange={(event) => updateTimescaleDSNField('password', event.target.value)} />
                      </Field>
                      <Field label="SSL Mode">
                        <Select
                          value={timescaleDSNFields.sslmode || 'disable'}
                          onChange={(value) => updateTimescaleDSNField('sslmode', value)}
                          options={[
                            { label: 'disable', value: 'disable' },
                            { label: 'require', value: 'require' },
                            { label: 'verify-ca', value: 'verify-ca' },
                            { label: 'verify-full', value: 'verify-full' },
                          ]}
                        />
                      </Field>
                    </div>
                  </div>
                )}
                <LazyJSONSchemaRenderer
                  schema={selectedPlugin.capabilities.config_schema}
                  formData={draft.connection || {}}
                  onChange={(connection) => updateDraft({ connection })}
                  pluginId={selectedPlugin.id}
                  pluginName={selectedPlugin.name}
                />
                {draftConnectionPreviewEntries.length > 0 && (
                  <div style={previewPanelStyle}>
                    <div style={{ fontSize: 13, fontWeight: 700, color: '#1d4ed8', marginBottom: 8 }}>
                      当前连接预览
                    </div>
                    <PreviewGrid entries={draftConnectionPreviewEntries} />
                  </div>
                )}
              </div>
            ) : (
              <Empty description="请选择 Connector 插件" />
            )}
          </Card>

          <Card style={sectionStyle} styles={{ body: { padding: 0 } }}>
            <SectionTitle title="健康探针与配额" description="这些策略会直接影响共享连接的健康管理、背压和公平性。" />
            <div style={gridStyle}>
              <Field label="Health Enabled">
                <Switch checked={!!draft.health_policy?.enabled} onChange={(checked) => updateHealthPolicy({ enabled: checked })} />
              </Field>
              <Field label="Probe Interval (ms)">
                <Input
                  value={String(draft.health_policy?.interval_ms ?? '')}
                  onChange={(event) => updateHealthPolicy({ interval_ms: Number(event.target.value) || 0 })}
                />
              </Field>
              <Field label="Probe Timeout (ms)">
                <Input
                  value={String(draft.health_policy?.timeout_ms ?? '')}
                  onChange={(event) => updateHealthPolicy({ timeout_ms: Number(event.target.value) || 0 })}
                />
              </Field>
              <Field label="Probe Target">
                <Input value={draft.health_policy?.target || ''} onChange={(event) => updateHealthPolicy({ target: event.target.value })} />
              </Field>
              <Field label="Probe Priority">
                <Select
                  value={draft.health_policy?.priority || 'normal'}
                  onChange={(value) => updateHealthPolicy({ priority: value })}
                  options={[
                    { label: 'high', value: 'high' },
                    { label: 'normal', value: 'normal' },
                    { label: 'low', value: 'low' },
                  ]}
                />
              </Field>
              <Field label="Max Inflight Requests">
                <Input
                  value={String(draft.quota_policy?.max_inflight_requests ?? '')}
                  onChange={(event) => updateQuotaPolicy({ max_inflight_requests: Number(event.target.value) || 0 })}
                />
              </Field>
              <Field label="Max Inflight Weight">
                <Input
                  value={String(draft.quota_policy?.max_inflight_weight ?? '')}
                  onChange={(event) => updateQuotaPolicy({ max_inflight_weight: Number(event.target.value) || 0 })}
                />
              </Field>
              <Field label="Default Usage Weight">
                <Input
                  value={String(draft.quota_policy?.default_usage_weight ?? '')}
                  onChange={(event) => updateQuotaPolicy({ default_usage_weight: Number(event.target.value) || 0 })}
                />
              </Field>
              <Field label="Preferred Provider Count">
                <Input
                  value={String(draft.provider_policy?.preferred_provider_count ?? '')}
                  onChange={(event) => updateProviderPolicy({ preferred_provider_count: Number(event.target.value) || 0 })}
                />
              </Field>
            </div>
          </Card>

          {isS7Connector(selectedPlugin) ? (
            <div style={{ color: 'var(--color-text-secondary)', fontSize: 13, lineHeight: 1.7 }}>
              `connect-s7` 按 PLC 实例建资源并使用 scheduled reuse：一个批次最多获取一个设备会话，高频连续采集按复用窗口保留会话，无近期需求时快速释放。
            </div>
          ) : null}
        </div>
      </Modal>
    </ConsolePage>
  );
};

const gridStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
  gap: 16,
};

const sectionStyle: React.CSSProperties = {
  border: '1px solid #e2e8f0',
  borderRadius: 16,
  padding: 18,
  background: 'var(--color-surface)',
};

const previewPanelStyle: React.CSSProperties = {
  padding: '12px 14px',
  borderRadius: 12,
  border: '1px solid #dbeafe',
  background: '#f8fbff',
};

const SectionTitle: React.FC<{ title: string; description?: string }> = ({ title, description }) => (
  <div style={{ marginBottom: 14 }}>
    <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--color-text)' }}>{title}</div>
    {description ? <div style={{ marginTop: 6, color: 'var(--color-text-secondary)', fontSize: 13, lineHeight: 1.6 }}>{description}</div> : null}
  </div>
);

const Field: React.FC<{ label: string; required?: boolean; children: React.ReactNode }> = ({ label, required, children }) => (
  <label style={{ display: 'grid', gap: 8 }}>
    <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--color-text)' }}>
      {label} {required ? <span style={{ color: '#dc2626' }}>*</span> : null}
    </span>
    {children}
  </label>
);

const InfoRow: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <div style={{ display: 'grid', gap: 4 }}>
    <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-text-secondary)' }}>{label}</div>
    <div style={{ color: 'var(--color-text)', lineHeight: 1.6 }}>{value}</div>
  </div>
);

const PreviewGrid: React.FC<{ entries: Array<{ key: string; label: string; value: string }> }> = ({ entries }) => {
  if (!entries.length) {
    return null;
  }
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 8 }}>
      {entries.map((entry) => (
        <div
          key={entry.key}
          style={{
            padding: '8px 10px',
            borderRadius: 10,
            border: '1px solid #dbeafe',
            background: 'var(--color-surface)',
          }}
        >
          <div style={{ fontSize: 11, color: 'var(--color-text-secondary)', marginBottom: 2 }}>{entry.label}</div>
          <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-text)', wordBreak: 'break-all' }}>{entry.value}</div>
        </div>
      ))}
    </div>
  );
};
