import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Descriptions, Input, Modal, Select, Space, Table, Tabs, Tag, Typography, type TableProps } from 'antd';
import { CloudDownloadOutlined, DeleteOutlined, InfoCircleOutlined, PlusOutlined, ReloadOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { usePluginStore } from '../store/pluginStore';
import { connectorsAPI, pluginsAPI, rulesAPI } from '../services/api';
import { ConsolePage } from '../components/ConsolePage';
import { ActionButton, FilterBar, StatusTag } from '../components/ui/ConsoleUI';
import { useFeedback } from '../components/FeedbackProvider';
import { ResourceTabs } from '../components/ResourceTabs';
import { ConnectorResourceView, PluginMeta, RegistryCatalogItem, RuleFlow } from '../types';

type PluginUsage = Pick<RuleFlow, 'id' | 'name' | 'enabled' | 'status'>;
type DependencyWarning = {
  plugin: PluginMeta;
  action: '禁用' | '删除';
  usages: PluginUsage[];
  resources: ConnectorResourceView[];
};

export const PluginManagerPage: React.FC = () => {
  const feedback = useFeedback();
  const { plugins, loading, loadPlugins, enablePlugin, disablePlugin, deletePlugin, uploadPlugin } = usePluginStore();
  const [searchTerm, setSearchTerm] = useState('');
  const [filterType, setFilterType] = useState<'all' | 'sink' | 'source' | 'processor' | 'connector'>('all');
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadDescriptor, setUploadDescriptor] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [detailPlugin, setDetailPlugin] = useState<PluginMeta | null>(null);
  const [dependencyWarning, setDependencyWarning] = useState<DependencyWarning | null>(null);
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const [activeView, setActiveView] = useState<'installed' | 'marketplace'>('installed');
  const [registryPlugins, setRegistryPlugins] = useState<RegistryCatalogItem[]>([]);
  const [registryLoading, setRegistryLoading] = useState(false);
  const pendingActionRef = useRef<string | null>(null);

  useEffect(() => {
    void loadPlugins();
  }, [loadPlugins]);

  const startAction = (key: string) => {
    if (pendingActionRef.current) return false;
    pendingActionRef.current = key;
    setPendingAction(key);
    return true;
  };

  const finishAction = () => {
    pendingActionRef.current = null;
    setPendingAction(null);
  };

  const loadRegistry = async () => {
    setRegistryLoading(true);
    try {
      setRegistryPlugins(await pluginsAPI.listRegistry());
    } catch (error: any) {
      feedback.error(`插件市场加载失败：${error.message || '未知错误'}`);
    } finally {
      setRegistryLoading(false);
    }
  };

  const handleRegistryInstall = async (item: RegistryCatalogItem) => {
    const key = `registry:${item.plugin.plugin}`;
    if (!startAction(key)) return;
    try {
      await pluginsAPI.installFromRegistry(item.plugin.plugin, item.plugin.version, item.plugin.os, item.plugin.arch);
      await loadPlugins();
      feedback.success(`插件 ${item.plugin.plugin} ${item.plugin.version} 已完成签名校验并安装`);
    } catch (error: any) {
      feedback.error(`安装失败：${error.message || '未知错误'}`);
    } finally {
      finishAction();
    }
  };

  const findPluginDependencies = async (pluginId: string) => {
    const [rules, resources] = await Promise.all([rulesAPI.listFull(), connectorsAPI.list()]);
    const pluginResources = resources.filter((resource) => resource.plugin_id === pluginId);
    const resourceIDs = new Set(pluginResources.map((resource) => resource.id));
    const usages = rules
      .filter((rule) => (rule.nodes || []).some((node) =>
        node.plugin === pluginId ||
        (typeof node.config?.connector_ref === 'string' && resourceIDs.has(node.config.connector_ref)),
      ))
      .map(({ id, name, enabled, status }) => ({ id, name, enabled, status }));
    return { usages, resources: pluginResources };
  };

  const handleUpload = async () => {
    if (!uploadFile) {
      feedback.warning('请选择插件文件');
      return;
    }
    setUploading(true);
    try {
      await uploadPlugin(uploadFile, uploadDescriptor || undefined);
      setShowUploadModal(false);
      setUploadFile(null);
      setUploadDescriptor(null);
      await loadPlugins();
      feedback.success('插件上传成功');
    } catch (error: any) {
      feedback.error(`上传失败：${error.message || '未知错误'}`);
    } finally {
      setUploading(false);
    }
  };

  const handleEnable = async (plugin: PluginMeta) => {
    const key = `enable:${plugin.id}`;
    if (!startAction(key)) return;
    try {
      await enablePlugin(plugin.id);
      feedback.success(`插件 ${plugin.name} 已启用`);
    } catch (error: any) {
      feedback.error(`启用失败：${error.message || '未知错误'}`);
    } finally {
      finishAction();
    }
  };

  const handleDisable = async (plugin: PluginMeta) => {
    const key = `disable:${plugin.id}`;
    if (!startAction(key)) return;
    try {
      const dependencies = await findPluginDependencies(plugin.id);
      if (dependencies.usages.length > 0 || dependencies.resources.length > 0) {
        setDependencyWarning({ plugin, action: '禁用', ...dependencies });
        return;
      }
      if (!await feedback.confirm({
        title: `确定要禁用插件 “${plugin.name}” 吗？`,
        content: '禁用后，新规则将不能启动该插件。',
        danger: true,
        okText: '禁用',
      })) return;
      await disablePlugin(plugin.id);
      feedback.success(`插件 ${plugin.name} 已禁用`);
    } catch (error: any) {
      feedback.error(`禁用失败：${error.message || '未知错误'}`);
    } finally {
      finishAction();
    }
  };

  const handleDelete = async (plugin: PluginMeta) => {
    const key = `delete:${plugin.id}`;
    if (!startAction(key)) return;
    try {
      const dependencies = await findPluginDependencies(plugin.id);
      if (dependencies.usages.length > 0 || dependencies.resources.length > 0) {
        setDependencyWarning({ plugin, action: '删除', ...dependencies });
        return;
      }
      if (!await feedback.confirm({
        title: `确定要删除插件 “${plugin.name}” 吗？`,
        content: '插件文件会从引擎中删除，此操作不可撤销。',
        danger: true,
        okText: '删除',
      })) return;
      await deletePlugin(plugin.id);
      feedback.success(`插件 ${plugin.name} 已删除`);
      setDetailPlugin((current) => current?.id === plugin.id ? null : current);
    } catch (error: any) {
      feedback.error(`删除失败：${error.message || '未知错误'}`);
    } finally {
      finishAction();
    }
  };

  const filteredPlugins = useMemo(
    () => [...plugins]
      .filter((plugin) => {
        const query = searchTerm.toLowerCase();
        return (
          (plugin.name.toLowerCase().includes(query) ||
            plugin.id.toLowerCase().includes(query) ||
            plugin.description.toLowerCase().includes(query)) &&
          (filterType === 'all' || plugin.type === filterType)
        );
      })
      .sort((left, right) => left.id.localeCompare(right.id, 'en', { sensitivity: 'base' })),
    [filterType, plugins, searchTerm],
  );

  const columns: NonNullable<TableProps<PluginMeta>['columns']> = [
    {
      title: '插件',
      dataIndex: 'name',
      ellipsis: true,
      render: (_, plugin) => (
        <div>
          <div className="plugin-manager__name" title={plugin.name}>{plugin.name}</div>
          <div className="plugin-manager__id" title={plugin.id}>{plugin.id}</div>
        </div>
      ),
    },
    { title: '版本', dataIndex: 'version', width: 90, align: 'center', render: (_, plugin) => `v${plugin.version}` },
    {
      title: '类型', dataIndex: 'type', width: 110, align: 'center',
      render: (_, plugin) => <Tag color={getTypeColor(plugin.type)}>{getTypeText(plugin.type)}</Tag>,
    },
    { title: '描述', dataIndex: 'description', ellipsis: true, render: (value) => value || '-' },
    {
      title: 'Manifest', dataIndex: 'info', width: 120, align: 'center',
      render: (_, plugin) => {
        const issues = getManifestIssues(plugin);
        return <StatusTag tone={issues.length === 0 ? 'success' : 'warning'}>{issues.length === 0 ? '完整' : '待完善'}</StatusTag>;
      },
    },
    {
      title: '状态', dataIndex: 'enabled', width: 96, align: 'center',
      render: (_, plugin) => <StatusTag tone={plugin.enabled ? 'success' : 'neutral'}>{plugin.enabled ? '已启用' : '已禁用'}</StatusTag>,
    },
    {
      title: '操作', dataIndex: '__actions__', width: 260, align: 'center', fixed: 'right',
      render: (_, plugin) => {
        const rowBusy = pendingAction?.endsWith(`:${plugin.id}`) === true;
        return (
          <Space size={6}>
            <ActionButton
              size="small"
              type={plugin.enabled ? 'default' : 'primary'}
              danger={plugin.enabled}
              disabled={pendingAction !== null}
              loading={rowBusy && (pendingAction?.startsWith('enable:') || pendingAction?.startsWith('disable:'))}
              onClick={() => plugin.enabled ? void handleDisable(plugin) : void handleEnable(plugin)}
            >
              {plugin.enabled ? '禁用' : '启用'}
            </ActionButton>
            <Button size="small" icon={<InfoCircleOutlined />} data-testid={`plugin-detail-open-${plugin.id}`} onClick={() => setDetailPlugin(plugin)}>详情</Button>
            <ActionButton
              size="small"
              danger
              icon={<DeleteOutlined />}
              disabled={pendingAction !== null}
              loading={pendingAction === `delete:${plugin.id}`}
              onClick={() => void handleDelete(plugin)}
            >
              删除
            </ActionButton>
          </Space>
        );
      },
    },
  ];

  return (
    <ConsolePage maxWidth={1600}>
      <ResourceTabs />
      <h1 className="sr-only">插件管理</h1>
      <Tabs
        activeKey={activeView}
        onChange={(key) => {
          const next = key as 'installed' | 'marketplace';
          setActiveView(next);
          if (next === 'marketplace' && registryPlugins.length === 0) void loadRegistry();
        }}
        items={[
          { key: 'installed', label: '已安装插件' },
          { key: 'marketplace', label: '插件市场' },
        ]}
      />
      {activeView === 'installed' ? (
      <>
      <FilterBar
        summary={`共 ${filteredPlugins.length} / ${plugins.length} 个插件`}
        actions={
          <Space wrap>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadPlugins()}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setShowUploadModal(true)}>上传插件</Button>
          </Space>
        }
      >
        <Input
          aria-label="搜索插件"
          allowClear
          placeholder="搜索插件名称、标识或描述"
          value={searchTerm}
          onChange={(event) => setSearchTerm(event.target.value)}
          style={{ width: 320 }}
        />
        <Select
          value={filterType}
          onChange={setFilterType}
          style={{ width: 180 }}
          options={[
            { label: '全部类型', value: 'all' },
            { label: '输出插件', value: 'sink' },
            { label: '输入插件', value: 'source' },
            { label: '处理插件', value: 'processor' },
            { label: 'Connector 插件', value: 'connector' },
          ]}
        />
      </FilterBar>

      <section className="console-surface">
        <Table<PluginMeta>
          columns={columns}
          dataSource={filteredPlugins}
          loading={loading}
          locale={{ emptyText: searchTerm || filterType !== 'all' ? '没有匹配的插件' : '暂无插件' }}
          pagination={{ defaultPageSize: 20, showSizeChanger: true, pageSizeOptions: [10, 20, 50, 100] }}
          rowKey="id"
          scroll={{ x: 1180 }}
        />
      </section>
      </>
      ) : (
        <PluginMarketplace
          items={registryPlugins}
          installed={plugins}
          loading={registryLoading}
          pendingAction={pendingAction}
          onReload={() => void loadRegistry()}
          onInstall={(item) => void handleRegistryInstall(item)}
        />
      )}

      <PluginDetailModal plugin={detailPlugin} onClose={() => setDetailPlugin(null)} />

      <Modal
        title={dependencyWarning ? `无法${dependencyWarning.action}插件` : ''}
        open={Boolean(dependencyWarning)}
        data-testid="plugin-dependency-modal"
        footer={<Button type="primary" onClick={() => setDependencyWarning(null)}>知道了</Button>}
        onCancel={() => setDependencyWarning(null)}
      >
        {dependencyWarning ? (
          <div className="plugin-manager__dependency-warning">
            <Alert
              type="warning"
              showIcon
              message={`“${dependencyWarning.plugin.name}” 存在使用依赖`}
              description="请先移除相关规则节点和 Connector 资源，再执行插件操作。运行中与未部署的配置都会受到保护。"
            />
            {dependencyWarning.resources.length > 0 ? (
              <>
                <h3>Connector 资源（{dependencyWarning.resources.length}）</h3>
                <ul>
                  {dependencyWarning.resources.map((resource) => (
                    <li key={resource.id}>
                      <strong>{resource.name || resource.id}</strong>
                      <span>{resource.id}</span>
                      <StatusTag tone={resource.enabled ? 'success' : 'neutral'}>{resource.enabled ? '已启用' : '已禁用'}</StatusTag>
                    </li>
                  ))}
                </ul>
              </>
            ) : null}
            {dependencyWarning.usages.length > 0 ? <h3>规则（{dependencyWarning.usages.length}）</h3> : null}
            <ul>
              {dependencyWarning.usages.map((rule) => (
                <li key={rule.id}>
                  <strong>{rule.name || rule.id}</strong>
                  <span>{rule.id}</span>
                  <StatusTag tone={isActiveRule(rule) ? 'success' : 'neutral'}>{isActiveRule(rule) ? '已部署' : '未部署'}</StatusTag>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </Modal>

      <Modal
        title="上传插件"
        open={showUploadModal}
        onCancel={() => {
          if (uploading) return;
          setShowUploadModal(false);
          setUploadFile(null);
          setUploadDescriptor(null);
        }}
        onOk={() => void handleUpload()}
        okText={uploading ? '上传中...' : '上传'}
        cancelText="取消"
        confirmLoading={uploading}
        okButtonProps={{ disabled: !uploadFile }}
        destroyOnHidden
      >
        <div className="plugin-manager__upload-form">
          <label>
            <strong>插件文件 *</strong>
            <input type="file" accept=".so,.dll" onChange={(event) => setUploadFile(event.target.files?.[0] || null)} />
          </label>
          <label>
            <strong>Manifest 描述文件</strong>
            <input type="file" accept=".json" onChange={(event) => setUploadDescriptor(event.target.files?.[0] || null)} />
            <span>每个插件必须独立声明 Manifest，并遵循 SDK v1 的统一规范。未上传时，引擎会从插件二进制中读取内嵌描述。</span>
          </label>
        </div>
      </Modal>
    </ConsolePage>
  );
};

const PluginMarketplace: React.FC<{
  items: RegistryCatalogItem[];
  installed: PluginMeta[];
  loading: boolean;
  pendingAction: string | null;
  onReload: () => void;
  onInstall: (item: RegistryCatalogItem) => void;
}> = ({ items, installed, loading, pendingAction, onReload, onInstall }) => {
  const installedVersions = new Map(installed.map((plugin) => [plugin.id, plugin.version]));
  const columns: NonNullable<TableProps<RegistryCatalogItem>['columns']> = [
    { title: '插件', render: (_, item) => <><strong>{item.plugin.plugin}</strong><div className="plugin-manager__id">{item.plugin.filename}</div></> },
    { title: '版本', width: 100, align: 'center', render: (_, item) => item.plugin.version },
    { title: '类型', width: 110, align: 'center', render: (_, item) => <Tag color={getTypeColor(item.plugin.type)}>{getTypeText(item.plugin.type)}</Tag> },
    { title: '平台', width: 140, align: 'center', render: (_, item) => `${item.plugin.os}/${item.plugin.arch}` },
    { title: '供应链', width: 130, align: 'center', render: () => <StatusTag tone="success"><SafetyCertificateOutlined /> Sigstore</StatusTag> },
    {
      title: '操作', width: 140, align: 'center',
      render: (_, item) => {
        const installedVersion = installedVersions.get(item.plugin.plugin);
        const busy = pendingAction === `registry:${item.plugin.plugin}`;
        return (
          <ActionButton type="primary" icon={<CloudDownloadOutlined />} disabled={pendingAction !== null || installedVersion === item.plugin.version.replace(/^v/, '')} loading={busy} onClick={() => onInstall(item)}>
            {installedVersion ? (installedVersion === item.plugin.version.replace(/^v/, '') ? '已安装' : '升级') : '安装'}
          </ActionButton>
        );
      },
    },
  ];
  return (
    <>
      <FilterBar summary={`共 ${items.length} 个已验证制品`} actions={<Button icon={<ReloadOutlined />} loading={loading} onClick={onReload}>刷新</Button>}>
        <Typography.Text type="secondary">制品将在下载后再次校验 SHA-256、Sigstore 身份和插件握手信息。</Typography.Text>
      </FilterBar>
      <section className="console-surface">
        <Table<RegistryCatalogItem> columns={columns} dataSource={items} loading={loading} rowKey={(item) => `${item.plugin.plugin}:${item.plugin.version}:${item.plugin.os}:${item.plugin.arch}`} pagination={{ defaultPageSize: 20 }} scroll={{ x: 980 }} />
      </section>
    </>
  );
};

const PluginDetailModal: React.FC<{ plugin: PluginMeta | null; onClose: () => void }> = ({ plugin, onClose }) => {
  if (!plugin) return null;
  const info = plugin.info;
  const issues = getManifestIssues(plugin);
  const capabilities = info?.capabilities;

  return (
    <Modal
      title="插件详情"
      open
      width={760}
      footer={<Button type="primary" onClick={onClose}>关闭</Button>}
      onCancel={onClose}
      data-testid="plugin-detail-modal"
    >
      <div className="plugin-manager__detail-heading">
        <div>
          <h2>{plugin.name}</h2>
          <Typography.Text code>{plugin.id}</Typography.Text>
        </div>
        <Space wrap>
          <Tag color={getTypeColor(plugin.type)}>{getTypeText(plugin.type)}</Tag>
          <StatusTag tone={plugin.enabled ? 'success' : 'neutral'}>{plugin.enabled ? '已启用' : '已禁用'}</StatusTag>
        </Space>
      </div>

      {issues.length > 0 ? (
        <Alert type="warning" showIcon message="Manifest 信息不完整" description={`缺少：${issues.join('、')}`} />
      ) : (
        <Alert type="success" showIcon message="Manifest 已通过完整性检查" description="插件拥有独立声明，并使用 SDK v1 统一字段。" />
      )}

      <Descriptions className="plugin-manager__descriptions" bordered size="small" column={2}>
        <Descriptions.Item label="版本">v{plugin.version}</Descriptions.Item>
        <Descriptions.Item label="类型">{plugin.type}</Descriptions.Item>
        <Descriptions.Item label="分类">{info?.category || '-'}</Descriptions.Item>
        <Descriptions.Item label="运行平面">{info?.plane || '-'}</Descriptions.Item>
        <Descriptions.Item label="输入端口">{capabilities?.input_ports ?? '-'}</Descriptions.Item>
        <Descriptions.Item label="输出端口">{capabilities?.output_ports ?? '-'}</Descriptions.Item>
        <Descriptions.Item label="批处理">{capabilities ? (capabilities.support_batch ? '支持' : '不支持') : '-'}</Descriptions.Item>
        <Descriptions.Item label="确认机制">{capabilities ? (capabilities.support_ack ? '支持' : '不支持') : '-'}</Descriptions.Item>
        <Descriptions.Item label="Connector Family" span={2}>{info?.connector_descriptor?.family || '-'}</Descriptions.Item>
        <Descriptions.Item label="描述" span={2}>{plugin.description || '-'}</Descriptions.Item>
      </Descriptions>

      <div className="plugin-manager__schema">
        <h3>配置 Schema</h3>
        <pre>{formatSchema(capabilities?.config_schema)}</pre>
      </div>
    </Modal>
  );
};

const getManifestIssues = (plugin: PluginMeta) => {
  const info = plugin.info;
  const issues: string[] = [];
  if (!info) return ['完整 Manifest'];
  if (!info.id) issues.push('id');
  if (!info.name) issues.push('name');
  if (!info.version) issues.push('version');
  if (!info.type) issues.push('type');
  if (!info.description) issues.push('description');
  if (!info.category) issues.push('category');
  if (!info.capabilities) issues.push('capabilities');
  if (!info.capabilities?.config_schema) issues.push('config_schema');
  return issues;
};

const formatSchema = (schema?: string) => {
  if (!schema) return '未声明';
  try {
    return JSON.stringify(JSON.parse(schema), null, 2);
  } catch {
    return schema;
  }
};

const isActiveRule = (rule: PluginUsage) => Boolean(rule.enabled || rule.status === 'deployed' || rule.status === 'running');

const getTypeColor = (type: string) => {
  switch (type) {
    case 'source': return 'cyan';
    case 'processor':
    case 'function': return 'purple';
    case 'connector': return 'blue';
    default: return 'default';
  }
};

const getTypeText = (type: string) => {
  switch (type) {
    case 'sink': return '输出';
    case 'source': return '输入';
    case 'function': return '函数';
    case 'processor': return '处理';
    case 'connector': return '连接器';
    default: return type;
  }
};
