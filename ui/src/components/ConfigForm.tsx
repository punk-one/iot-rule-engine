import React from 'react';
import { configAPI, SystemConfig } from '../services/api';

interface ConfigFormProps {
  config: SystemConfig;
  onChange: (config: SystemConfig) => void;
}

export const ConfigForm: React.FC<ConfigFormProps> = ({ config, onChange }) => {
  // 确保配置对象有完整的结构
  const safeConfig = React.useMemo(() => {
    return {
      ...config,
      common: {
        ...config.common,
        nats: {
          ...config.common?.nats,
          mode: config.common?.nats?.mode || 'internal',
          host: config.common?.nats?.host || '127.0.0.1',
          port: config.common?.nats?.port ?? 0,
          store_dir: config.common?.nats?.store_dir || './data/nats',
          url: config.common?.nats?.url || '',
          auth_mode: config.common?.nats?.auth_mode || 'none',
          max_reconnects: config.common?.nats?.max_reconnects ?? -1,
          reconnect_wait_ms: config.common?.nats?.reconnect_wait_ms || 2000,
          username: config.common?.nats?.username || '',
          password: config.common?.nats?.password || '',
          token: config.common?.nats?.token || '',
          cert_file: config.common?.nats?.cert_file || '',
          key_file: config.common?.nats?.key_file || '',
          ca_file: config.common?.nats?.ca_file || '',
          creds_file: config.common?.nats?.creds_file || '',
        },
      },
      security: {
        ...config.security,
        plugin: {
          ...config.security?.plugin,
          max_emit_qps: config.security?.plugin?.max_emit_qps || 10000,
          rpc_timeout_ms: config.security?.plugin?.rpc_timeout_ms || 5000,
          max_payload_kb: config.security?.plugin?.max_payload_kb || 256,
        },
        dispatcher: {
          ...config.security?.dispatcher,
          max_queue: config.security?.dispatcher?.max_queue || 100000,
          rule_qps: config.security?.dispatcher?.rule_qps || 10000,
          node_qps: config.security?.dispatcher?.node_qps || 1000,
          deduplication_window_ms: config.security?.dispatcher?.deduplication_window_ms || 1000,
        },
        manager: {
          ...config.security?.manager,
          auth_enabled: config.security?.manager?.auth_enabled ?? true,
          allow_insecure: config.security?.manager?.allow_insecure ?? false,
          allowed_origins: config.security?.manager?.allowed_origins || [],
          allow_runtime_plugin_install: config.security?.manager?.allow_runtime_plugin_install ?? false,
          trusted_plugin_manifest: config.security?.manager?.trusted_plugin_manifest || '',
          max_plugin_upload_mb: config.security?.manager?.max_plugin_upload_mb || 64,
        },
      },
      manager: {
        ...config.manager,
        port: config.manager?.port || 11011,
        db_path: config.manager?.db_path || './data',
      },
      executor: {
        ...config.executor,
        plugins_dir: config.executor?.plugins_dir || './plugins',
        worker_count: config.executor?.worker_count || 1,
        node_error_history_limit: config.executor?.node_error_history_limit || 100,
      },
      pipeline: { ...config.pipeline },
    };
  }, [config]);

  const updateNestedField = (path: string[], field: string, value: any) => {
    const newConfig = { ...safeConfig };
    let current: any = newConfig;
    for (const key of path) {
      current[key] = { ...current[key] };
      current = current[key];
    }
    current[field] = value;
    onChange(newConfig);
  };

  const [activeTab, setActiveTab] = React.useState<'common' | 'services' | 'security'>('common');

  return (
    <div className="config-form">
      {/* Tab Selector */}
      <div className="config-form__tabs" style={{
        display: 'flex',
        padding: '6px 12px',
        backgroundColor: 'var(--color-surface)',
        borderBottom: '1px solid var(--color-border)',
        gap: '8px'
      }}>
        <TabButton
          active={activeTab === 'common'}
          onClick={() => setActiveTab('common')}
          label="基础配置"
          icon="🔌"
        />
        <TabButton
          active={activeTab === 'services'}
          onClick={() => setActiveTab('services')}
          label="服务组件"
          icon="⚙️"
        />
        <TabButton
          active={activeTab === 'security'}
          onClick={() => setActiveTab('security')}
          label="安全配置"
          icon="🔒"
        />
      </div>

      <div className="config-form__content" style={{ padding: '12px' }}>
        <div className="config-form__sections">
          {activeTab === 'common' && (
            <ConfigSection title="NATS 消息总线" icon="🔌">
              <div style={{ gridColumn: '1 / -1', padding: '8px 10px', borderRadius: '6px', background: 'color-mix(in srgb, var(--color-primary) 10%, var(--color-surface))', color: 'var(--color-text-secondary)', fontSize: 12 }}>
                默认由规则引擎启动内置 NATS；需要连接独立集群时可切换为 external。
              </div>
              <ConfigField
                label="运行模式"
                value={safeConfig.common.nats.mode}
                onChange={(v) => updateNestedField(['common', 'nats'], 'mode', v)}
                type="select"
                options={['internal', 'external']}
                hint="internal 为内置服务，external 为独立 NATS 集群"
              />
              {safeConfig.common.nats.mode === 'internal' ? (
                <>
                  <ConfigField
                    label="监听地址"
                    value={safeConfig.common.nats.host}
                    onChange={(v) => updateNestedField(['common', 'nats'], 'host', v)}
                    type="text"
                    hint="生产环境建议保持 127.0.0.1"
                  />
                  <ConfigField
                    label="监听端口"
                    value={safeConfig.common.nats.port}
                    onChange={(v) => updateNestedField(['common', 'nats'], 'port', Number(v))}
                    type="number"
                    hint="0 表示自动选择随机可用端口"
                  />
                  <ConfigField
                    label="JetStream 数据目录"
                    value={safeConfig.common.nats.store_dir}
                    onChange={(v) => updateNestedField(['common', 'nats'], 'store_dir', v)}
                    type="text"
                    hint="用于持久化内置消息数据"
                  />
                </>
              ) : (
                <>
                  <ConfigField
                    label="NATS URL"
                    value={safeConfig.common.nats.url}
                    onChange={(v) => updateNestedField(['common', 'nats'], 'url', v)}
                    type="text"
                    hint="例如 nats://127.0.0.1:4222"
                  />
                  <ConfigField
                    label="认证模式"
                    value={safeConfig.common.nats.auth_mode}
                    onChange={(v) => updateNestedField(['common', 'nats'], 'auth_mode', v)}
                    type="select"
                    options={['none', 'user', 'token', 'tls', 'creds']}
                  />
                  {safeConfig.common.nats.auth_mode === 'user' && (
                    <>
                      <ConfigField label="用户名" value={safeConfig.common.nats.username} onChange={(v) => updateNestedField(['common', 'nats'], 'username', v)} type="text" />
                      <div style={{ color: '#92400e', fontSize: '12px' }}>密码请通过部署环境变量 IOT_RULE_NATS_PASSWORD 注入。</div>
                    </>
                  )}
                  {safeConfig.common.nats.auth_mode === 'token' && (
                    <div style={{ color: '#92400e', fontSize: '12px' }}>Token 请通过部署环境变量 IOT_RULE_NATS_TOKEN 注入。</div>
                  )}
                  {safeConfig.common.nats.auth_mode === 'tls' && (
                    <>
                      <CertFileField label="证书文件" value={safeConfig.common.nats.cert_file} onChange={(v) => updateNestedField(['common', 'nats'], 'cert_file', v)} />
                      <CertFileField label="密钥文件" value={safeConfig.common.nats.key_file} onChange={(v) => updateNestedField(['common', 'nats'], 'key_file', v)} />
                      <CertFileField label="CA 文件" value={safeConfig.common.nats.ca_file} onChange={(v) => updateNestedField(['common', 'nats'], 'ca_file', v)} />
                    </>
                  )}
                  {safeConfig.common.nats.auth_mode === 'creds' && (
                    <ConfigField
                      label="NATS creds 文件路径"
                      value={safeConfig.common.nats.creds_file}
                      onChange={(v) => updateNestedField(['common', 'nats'], 'creds_file', v)}
                      type="text"
                      hint="填写已安全部署到服务主机的 .creds 路径；该文件不是 PEM 证书，不通过证书上传接口传输。"
                    />
                  )}
                </>
              )}
            </ConfigSection>
          )}

          {activeTab === 'services' && (
            <>
              <ConfigSection title="Manager 管理器" icon="⚙️">
                <ConfigField
                  label="API 端口"
                  value={safeConfig.manager.port}
                  onChange={(v) => updateNestedField(['manager'], 'port', Number(v))}
                  type="number"
                  disabled
                  hint="启动监听项，不能在当前连接中在线修改。开发模式通过 REACT_APP_API_URL 指向 Manager；生产模式使用同源 /api/v1，由反向代理或单文件服务自动跟随监听端口。"
                />
                <ConfigField
                  label="数据库路径"
                  value={safeConfig.manager.db_path}
                  onChange={(v) => updateNestedField(['manager'], 'db_path', v)}
                  type="text"
                  disabled
                  hint="启动存储位置，需在启动 YAML 或环境配置中修改并重启。"
                />
              </ConfigSection>

              <ConfigSection title="Executor 执行器" icon="⚡">
                <ConfigField
                  label="插件目录"
                  value={safeConfig.executor.plugins_dir}
                  onChange={(v) => updateNestedField(['executor'], 'plugins_dir', v)}
                  type="text"
                />
                <ConfigField
                  label="工作线程数"
                  value={safeConfig.executor.worker_count}
                  onChange={(v) => updateNestedField(['executor'], 'worker_count', Number(v))}
                  type="number"
                  hint="并发执行规则的工作线程数量"
                />
                <ConfigField
                  label="节点错误历史保留条数"
                  value={safeConfig.executor.node_error_history_limit}
                  onChange={(v) => updateNestedField(['executor'], 'node_error_history_limit', Number(v))}
                  type="number"
                  hint="每个节点内存中保留的历史错误数量，默认 100"
                />
                <div style={{ color: '#666', fontSize: '13px', lineHeight: 1.6 }}>
                  运维接口已合并到 Manager 端口，访问路径为 `/api/v1/executor/*` 和 `/api/v1/runtime/*`。
                </div>
              </ConfigSection>

              <ConfigSection title="Pipeline 管道" icon="🔄">
                <div style={{ color: '#666', fontSize: '13px', lineHeight: 1.6 }}>
                  运维接口已合并到 Manager 端口，访问路径为 `/api/v1/pipeline/*`。
                </div>
              </ConfigSection>
            </>
          )}

          {activeTab === 'security' && (
            <>
              <ConfigSection title="插件安全" icon="🔌">
                <ConfigField
                  label="最大 Emit QPS"
                  value={safeConfig.security.plugin.max_emit_qps}
                  onChange={(v) => updateNestedField(['security', 'plugin'], 'max_emit_qps', Number(v))}
                  type="number"
                  hint="插件每秒最大事件发送数"
                />
                <ConfigField
                  label="RPC 超时 (毫秒)"
                  value={safeConfig.security.plugin.rpc_timeout_ms}
                  onChange={(v) => updateNestedField(['security', 'plugin'], 'rpc_timeout_ms', Number(v))}
                  type="number"
                  hint="RPC 调用超时时间"
                />
                <ConfigField
                  label="最大 Payload (KB)"
                  value={safeConfig.security.plugin.max_payload_kb}
                  onChange={(v) => updateNestedField(['security', 'plugin'], 'max_payload_kb', Number(v))}
                  type="number"
                  hint="事件 Payload 最大大小（KB）"
                />
              </ConfigSection>

              <ConfigSection title="Dispatcher 安全" icon="📦">
                <ConfigField
                  label="最大队列大小"
                  value={safeConfig.security.dispatcher.max_queue}
                  onChange={(v) => updateNestedField(['security', 'dispatcher'], 'max_queue', Number(v))}
                  type="number"
                  hint="Scheduler 队列最大大小"
                />
                <ConfigField
                  label="Rule 级 QPS"
                  value={safeConfig.security.dispatcher.rule_qps}
                  onChange={(v) => updateNestedField(['security', 'dispatcher'], 'rule_qps', Number(v))}
                  type="number"
                  hint="每个规则每秒最大事件数"
                />
                <ConfigField
                  label="Node 级 QPS"
                  value={safeConfig.security.dispatcher.node_qps}
                  onChange={(v) => updateNestedField(['security', 'dispatcher'], 'node_qps', Number(v))}
                  type="number"
                  hint="每个节点每秒最大事件数"
                />
                <ConfigField
                  label="去重时间窗口 (毫秒)"
                  value={safeConfig.security.dispatcher.deduplication_window_ms}
                  onChange={(v) => updateNestedField(['security', 'dispatcher'], 'deduplication_window_ms', Number(v))}
                  type="number"
                  hint="防循环去重的时间窗口"
                />
              </ConfigSection>

              <ConfigSection title="Manager 安全" icon="🔐">
                <ConfigField
                  label="启用鉴权"
                  value={safeConfig.security.manager.auth_enabled}
                  onChange={(v) => updateNestedField(['security', 'manager'], 'auth_enabled', v)}
                  type="checkbox"
                  hint="生产环境必须启用 Manager 用户会话鉴权"
                />
                <ConfigField
                  label="允许不安全开发模式"
                  value={safeConfig.security.manager.allow_insecure}
                  onChange={(v) => updateNestedField(['security', 'manager'], 'allow_insecure', v)}
                  type="checkbox"
                  hint="仅本地开发使用；生产环境必须关闭"
                />
                <ConfigField
                  label="允许的前端来源"
                  value={safeConfig.security.manager.allowed_origins.join(', ')}
                  onChange={(v) => updateNestedField(['security', 'manager'], 'allowed_origins', String(v).split(',').map((origin) => origin.trim()).filter(Boolean))}
                  type="text"
                  hint="多个 Origin 以逗号分隔；例如 http://localhost:3000"
                />
                <ConfigField
                  label="允许在线安装插件"
                  value={safeConfig.security.manager.allow_runtime_plugin_install}
                  onChange={(v) => updateNestedField(['security', 'manager'], 'allow_runtime_plugin_install', v)}
                  type="checkbox"
                  hint="默认关闭；生产环境优先离线发布"
                />
                {safeConfig.security.manager.allow_runtime_plugin_install && (
                  <>
                    <ConfigField
                      label="可信发布清单"
                      value={safeConfig.security.manager.trusted_plugin_manifest}
                      onChange={(v) => updateNestedField(['security', 'manager'], 'trusted_plugin_manifest', v)}
                      type="text"
                      hint="应为已在引擎外验证签名的只读 release-manifest.json"
                    />
                    <ConfigField
                      label="插件上传上限 (MB)"
                      value={safeConfig.security.manager.max_plugin_upload_mb}
                      onChange={(v) => updateNestedField(['security', 'manager'], 'max_plugin_upload_mb', Number(v))}
                      type="number"
                    />
                  </>
                )}
              </ConfigSection>
            </>
          )}
        </div>
      </div>
    </div>
  );
};

interface TabButtonProps {
  active: boolean;
  onClick: () => void;
  label: string;
  icon: string;
}

const TabButton: React.FC<TabButtonProps> = ({ active, onClick, label, icon }) => (
  <button
    onClick={onClick}
    style={{
      minHeight: 34,
      padding: '6px 12px',
      border: active ? '1px solid color-mix(in srgb, var(--color-primary) 45%, var(--color-border))' : '1px solid transparent',
      borderRadius: 8,
      backgroundColor: active ? 'color-mix(in srgb, var(--color-primary) 12%, var(--color-surface))' : 'transparent',
      color: active ? 'var(--color-primary)' : 'var(--color-text-secondary)',
      fontWeight: 650,
      cursor: 'pointer',
      display: 'flex',
      alignItems: 'center',
      gap: '8px',
      transition: 'all 0.2s'
    }}
  >
    <span>{icon}</span>
    {label}
  </button>
);

interface ConfigSectionProps {
  title: string;
  icon: string;
  children: React.ReactNode;
}

const ConfigSection: React.FC<ConfigSectionProps> = ({ title, icon, children }) => (
  <div className="config-form__section" style={{ border: '1px solid var(--color-border)', borderRadius: '10px', padding: '12px', backgroundColor: 'var(--color-surface)' }}>
    <h3 style={{ margin: '0 0 10px', display: 'flex', alignItems: 'center', gap: '7px', fontSize: 14 }}>
      <span>{icon}</span>
      <span>{title}</span>
    </h3>
    <div className="config-form__field-grid">
      {children}
    </div>
  </div>
);

interface ConfigFieldProps {
  label: string;
  value: any;
  onChange: (value: any) => void;
  type: 'text' | 'number' | 'password' | 'checkbox' | 'select';
  options?: string[];
  hint?: string;
  disabled?: boolean;
}

const ConfigField: React.FC<ConfigFieldProps> = ({ label, value, onChange, type, options, hint, disabled = false }) => {
  return (
    <div className="config-form__field">
      <label style={{ display: 'block', marginBottom: '3px', fontWeight: 650, fontSize: '12px' }}>
        {label}
      </label>
      {hint && (
        <div className="config-form__hint">
          {hint}
        </div>
      )}
      {type === 'checkbox' ? (
        <input
          type="checkbox"
          checked={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
          style={{ width: '18px', height: '18px', cursor: 'pointer', marginTop: 5 }}
        />
      ) : type === 'select' ? (
        <select
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          style={{ width: '100%', height: 32, padding: '4px 8px', border: '1px solid var(--color-border)', borderRadius: '6px', background: 'var(--color-surface-raised)', color: 'var(--color-text)' }}
        >
          {options?.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </select>
      ) : (
        <input
          type={type}
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(type === 'number' ? Number(e.target.value) : e.target.value)}
          style={{ width: '100%', height: 32, padding: '4px 8px', border: '1px solid var(--color-border)', borderRadius: '6px', background: 'var(--color-surface-raised)', color: 'var(--color-text)' }}
        />
      )}
    </div>
  );
};

// 证书文件上传组件
const CertFileField: React.FC<{
  label: string;
  value: string;
  onChange: (v: string) => void;
}> = ({ label, value, onChange }) => {
  const [isUploading, setIsUploading] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const fileInputRef = React.useRef<HTMLInputElement>(null);

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setIsUploading(true);
    setError(null);
    try {
      const resp = await configAPI.uploadCert(file);
      onChange(resp.path);
    } catch (err) {
      console.error('File upload failed:', err);
      setError('上传失败，请重试');
    } finally {
      setIsUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  return (
    <div className="config-form__field">
      <label style={{ display: 'block', marginBottom: '3px', fontSize: '12px', fontWeight: 650, color: 'var(--color-text)' }}>
        {label}
      </label>
      <div style={{ display: 'flex', gap: '8px' }}>
        <input
          type="text"
          value={value}
          readOnly
          placeholder="未选择文件"
          style={{
            flex: 1,
            height: 32,
            padding: '4px 8px',
            border: '1px solid var(--color-border)',
            borderRadius: '6px',
            fontSize: '12px',
            backgroundColor: 'var(--color-surface-raised)',
            color: 'var(--color-text-secondary)',
            cursor: 'default'
          }}
        />
        <input
          type="file"
          hidden
          ref={fileInputRef}
          onChange={handleFileChange}
        />
        <button
          onClick={() => fileInputRef.current?.click()}
          disabled={isUploading}
          style={{
            height: 32,
            padding: '4px 10px',
            backgroundColor: isUploading ? 'var(--color-surface-subtle)' : 'var(--color-surface-raised)',
            border: '1px solid var(--color-border)',
            borderRadius: '6px',
            fontSize: '12px',
            fontWeight: 600,
            cursor: isUploading ? 'not-allowed' : 'pointer',
            transition: 'all 0.2s ease',
            color: 'var(--color-text)',
            whiteSpace: 'nowrap'
          }}
        >
          {isUploading ? '正在上传...' : '选择并上传'}
        </button>
      </div>
      {error && (
        <div style={{ marginTop: '4px', fontSize: '12px', color: '#D92D20' }}>
          {error}
        </div>
      )}
    </div>
  );
};
