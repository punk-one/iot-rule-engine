import React from 'react';
import { Alert, Button, Popconfirm, Space } from 'antd';
import { ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import { configAPI, SystemConfig } from '../services/api';
import { ConfigForm } from '../components/ConfigForm';
import { ActionButton, AsyncState, PageHeader, PageLayout } from '../components/ui/ConsoleUI';

export const ConfigPage: React.FC = () => {
  const [config, setConfig] = React.useState<SystemConfig | null>(null);
  const [savedSnapshot, setSavedSnapshot] = React.useState('');
  const [loading, setLoading] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [loadError, setLoadError] = React.useState('');
  const [message, setMessage] = React.useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [source, setSource] = React.useState('');

  const isDirty = React.useMemo(
    () => Boolean(config && savedSnapshot && JSON.stringify(config) !== savedSnapshot),
    [config, savedSnapshot],
  );

  const fetchConfig = React.useCallback(async (preserveMessage = false) => {
    setLoading(true);
    setLoadError('');
    if (!preserveMessage) setMessage(null);
    try {
      const response = await configAPI.get();
      setConfig(response.config);
      setSavedSnapshot(JSON.stringify(response.config));
      setSource(response.source);
    } catch (error: any) {
      console.error(error);
      setLoadError(error?.message || '服务暂时不可用，请检查后端状态后重试。');
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void fetchConfig();
  }, [fetchConfig]);

  React.useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!isDirty) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [isDirty]);

  const handleSave = async () => {
    if (!config || saving) return;

    setSaving(true);
    setMessage(null);
    try {
      const response = await configAPI.update(config);
      const latest = await configAPI.get();
      setConfig(latest.config);
      setSavedSnapshot(JSON.stringify(latest.config));
      setSource(latest.source || response.source);
      setMessage({ type: 'success', text: response.message || '配置已保存' });
    } catch (error: any) {
      console.error(error);
      setMessage({ type: 'error', text: `保存配置失败：${error?.message || '未知错误'}` });
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageLayout maxWidth={1600} className="config-page">
      <PageHeader
        title="配置管理"
        description={`来源：${source === 'database' ? 'SQLite 数据库' : '默认开发配置'} · 系统级配置保存后可能需要重启后端`}
        eyebrow={isDirty ? '有未保存修改' : '系统设置'}
        actions={
          <Space wrap>
            <Popconfirm
              title={isDirty ? '重新加载会丢弃未保存修改，是否继续？' : '重新加载当前配置？'}
              okText="重新加载"
              cancelText="取消"
              onConfirm={() => void fetchConfig()}
            >
              <Button size="small" icon={<ReloadOutlined />} disabled={saving || loading}>重新加载</Button>
            </Popconfirm>
            <ActionButton
              type="primary"
              size="small"
              icon={<SaveOutlined />}
              loading={saving}
              pendingText="保存中"
              disabled={!config || !isDirty}
              onClick={() => void handleSave()}
            >
              保存配置
            </ActionButton>
          </Space>
        }
      />

      <div>
        {message ? (
          <Alert
            showIcon
            closable
            type={message.type}
            message={message.type === 'success' ? '保存成功' : '保存失败'}
            description={message.text}
            onClose={() => setMessage(null)}
            style={{ marginBottom: 8 }}
          />
        ) : null}

        <AsyncState
          loading={loading && !config}
          error={loadError || undefined}
          onRetry={() => void fetchConfig()}
        >
          {config ? (
            <section className="console-surface" aria-label="系统配置表单">
              <div style={{ background: 'var(--color-surface-subtle)' }}>
                <ConfigForm config={config} onChange={setConfig} />
              </div>
            </section>
          ) : null}
        </AsyncState>
      </div>
    </PageLayout>
  );
};
