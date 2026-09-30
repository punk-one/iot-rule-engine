import React, { useState, useEffect } from 'react';
import { grayReleaseAPI } from '../services/api';
import { GrayReleaseConfig } from '../types';
import { useFeedback } from './FeedbackProvider';

interface GrayReleaseConfigProps {
  ruleId: string;
}

export const GrayReleaseConfigComponent: React.FC<GrayReleaseConfigProps> = ({ ruleId }) => {
  const feedback = useFeedback();
  const [config, setConfig] = useState<GrayReleaseConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);

  const [formData, setFormData] = useState({
    version: 1,
    enabled: false,
    strategy: 'percentage' as 'percentage' | 'condition',
    percentage: 10,
    condition: {} as Record<string, string>,
    target_nodes: [] as string[],
  });

  useEffect(() => {
    loadConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ruleId]);

  const loadConfig = async () => {
    try {
      setLoading(true);
      const data = await grayReleaseAPI.get(ruleId);
      setConfig(data);
      setFormData({
        version: data.version,
        enabled: data.enabled,
        strategy: data.strategy,
        percentage: data.percentage || 10,
        condition: data.condition || {},
        target_nodes: data.target_nodes || [],
      });
      setError(null);
    } catch (err: any) {
      if (err.response?.status === 404) {
        // 配置不存在，使用默认值
        setConfig(null);
      } else {
        setError(err.message || '加载配置失败');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    try {
      setLoading(true);
      const configToSave = {
        version: formData.version,
        enabled: formData.enabled,
        strategy: formData.strategy,
        percentage: formData.strategy === 'percentage' ? formData.percentage : undefined,
        condition: formData.strategy === 'condition' ? formData.condition : undefined,
        target_nodes: formData.target_nodes,
      };

      if (config) {
        await grayReleaseAPI.update(ruleId, configToSave);
      } else {
        await grayReleaseAPI.create(ruleId, configToSave);
      }

      await loadConfig();
      setEditing(false);
      feedback.success(config ? '灰度配置已更新' : '灰度配置已创建');
    } catch (err: any) {
      setError(err.message || '保存配置失败');
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!await feedback.confirm({ title: '确定要删除灰度发布配置吗？', danger: true })) {
      return;
    }

    try {
      setLoading(true);
      await grayReleaseAPI.delete(ruleId);
      setConfig(null);
      setEditing(false);
      feedback.success('灰度配置已删除');
    } catch (err: any) {
      setError(err.message || '删除配置失败');
    } finally {
      setLoading(false);
    }
  };

  const addConditionField = () => {
    setFormData({
      ...formData,
      condition: { ...formData.condition, '': '' },
    });
  };

  const updateConditionField = (oldKey: string, newKey: string, value: string) => {
    const newCondition = { ...formData.condition };
    delete newCondition[oldKey];
    if (newKey && value) {
      newCondition[newKey] = value;
    }
    setFormData({ ...formData, condition: newCondition });
  };

  const removeConditionField = (key: string) => {
    const newCondition = { ...formData.condition };
    delete newCondition[key];
    setFormData({ ...formData, condition: newCondition });
  };

  const updateTargetNodes = (value: string) => {
    setFormData({
      ...formData,
      target_nodes: value.split(',').map(s => s.trim()).filter(s => s),
    });
  };

  if (loading && !config && !error) {
    return <div style={{ padding: '20px', textAlign: 'center' }}>加载中...</div>;
  }

  return (
    <div style={{ padding: '20px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
        <h2>灰度发布配置</h2>
        {!editing && (
          <div>
            <button
              onClick={() => setEditing(true)}
              style={{
                padding: '8px 16px',
                backgroundColor: '#2196F3',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
                marginRight: '10px',
              }}
            >
              {config ? '编辑' : '创建配置'}
            </button>
            {config && (
              <button
                onClick={handleDelete}
                style={{
                  padding: '8px 16px',
                  backgroundColor: '#f44336',
                  color: 'white',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: 'pointer',
                }}
              >
                删除
              </button>
            )}
          </div>
        )}
      </div>

      {error && (
        <div style={{ padding: '10px', backgroundColor: '#ffebee', color: '#c62828', borderRadius: '4px', marginBottom: '20px' }}>
          错误: {error}
        </div>
      )}

      {!config && !editing && (
        <div style={{ padding: '20px', textAlign: 'center', color: '#999' }}>
          暂无灰度发布配置，点击"创建配置"开始配置
        </div>
      )}

      {(config || editing) && (
        <div style={{ backgroundColor: '#f9f9f9', padding: '20px', borderRadius: '4px' }}>
          {editing ? (
            <div>
              <div style={{ marginBottom: '15px' }}>
                <label style={{ display: 'block', marginBottom: '5px', fontWeight: 'bold' }}>
                  版本号:
                </label>
                <input
                  type="number"
                  value={formData.version}
                  onChange={(e) => setFormData({ ...formData, version: parseInt(e.target.value) || 1 })}
                  style={{ width: '100%', padding: '8px', borderRadius: '4px', border: '1px solid #ddd' }}
                />
              </div>

              <div style={{ marginBottom: '15px' }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  <input
                    type="checkbox"
                    checked={formData.enabled}
                    onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
                  />
                  <span style={{ fontWeight: 'bold' }}>启用灰度发布</span>
                </label>
              </div>

              <div style={{ marginBottom: '15px' }}>
                <label style={{ display: 'block', marginBottom: '5px', fontWeight: 'bold' }}>
                  灰度策略:
                </label>
                <select
                  value={formData.strategy}
                  onChange={(e) => setFormData({ ...formData, strategy: e.target.value as 'percentage' | 'condition' })}
                  style={{ width: '100%', padding: '8px', borderRadius: '4px', border: '1px solid #ddd' }}
                >
                  <option value="percentage">比例分流 (Percentage)</option>
                  <option value="condition">条件分流 (Condition)</option>
                </select>
              </div>

              {formData.strategy === 'percentage' && (
                <div style={{ marginBottom: '15px' }}>
                  <label style={{ display: 'block', marginBottom: '5px', fontWeight: 'bold' }}>
                    灰度比例 (%): {formData.percentage}
                  </label>
                  <input
                    type="range"
                    min="0"
                    max="100"
                    value={formData.percentage}
                    onChange={(e) => setFormData({ ...formData, percentage: parseInt(e.target.value) })}
                    style={{ width: '100%' }}
                  />
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', color: '#666' }}>
                    <span>0%</span>
                    <span>100%</span>
                  </div>
                </div>
              )}

              {formData.strategy === 'condition' && (
                <div style={{ marginBottom: '15px' }}>
                  <label style={{ display: 'block', marginBottom: '5px', fontWeight: 'bold' }}>
                    条件匹配 (事件元数据):
                  </label>
                  {Object.entries(formData.condition).map(([key, value]) => (
                    <div key={key} style={{ display: 'flex', gap: '10px', marginBottom: '10px' }}>
                      <input
                        type="text"
                        placeholder="元数据键"
                        value={key}
                        onChange={(e) => updateConditionField(key, e.target.value, value)}
                        style={{ flex: 1, padding: '8px', borderRadius: '4px', border: '1px solid #ddd' }}
                      />
                      <input
                        type="text"
                        placeholder="匹配值"
                        value={value}
                        onChange={(e) => updateConditionField(key, key, e.target.value)}
                        style={{ flex: 1, padding: '8px', borderRadius: '4px', border: '1px solid #ddd' }}
                      />
                      <button
                        onClick={() => removeConditionField(key)}
                        style={{
                          padding: '8px 16px',
                          backgroundColor: '#f44336',
                          color: 'white',
                          border: 'none',
                          borderRadius: '4px',
                          cursor: 'pointer',
                        }}
                      >
                        删除
                      </button>
                    </div>
                  ))}
                  <button
                    onClick={addConditionField}
                    style={{
                      padding: '8px 16px',
                      backgroundColor: '#4CAF50',
                      color: 'white',
                      border: 'none',
                      borderRadius: '4px',
                      cursor: 'pointer',
                    }}
                  >
                    + 添加条件
                  </button>
                </div>
              )}

              <div style={{ marginBottom: '15px' }}>
                <label style={{ display: 'block', marginBottom: '5px', fontWeight: 'bold' }}>
                  目标节点ID (逗号分隔):
                </label>
                <input
                  type="text"
                  value={formData.target_nodes.join(', ')}
                  onChange={(e) => updateTargetNodes(e.target.value)}
                  placeholder="node1, node2, node3"
                  style={{ width: '100%', padding: '8px', borderRadius: '4px', border: '1px solid #ddd' }}
                />
              </div>

              <div style={{ display: 'flex', gap: '10px' }}>
                <button
                  onClick={handleSave}
                  disabled={loading}
                  style={{
                    padding: '10px 20px',
                    backgroundColor: '#4CAF50',
                    color: 'white',
                    border: 'none',
                    borderRadius: '4px',
                    cursor: loading ? 'not-allowed' : 'pointer',
                    opacity: loading ? 0.6 : 1,
                  }}
                >
                  保存
                </button>
                <button
                  onClick={() => {
                    setEditing(false);
                    loadConfig();
                  }}
                  disabled={loading}
                  style={{
                    padding: '10px 20px',
                    backgroundColor: '#9E9E9E',
                    color: 'white',
                    border: 'none',
                    borderRadius: '4px',
                    cursor: loading ? 'not-allowed' : 'pointer',
                    opacity: loading ? 0.6 : 1,
                  }}
                >
                  取消
                </button>
              </div>
            </div>
          ) : (
            <div>
              <div style={{ marginBottom: '10px' }}>
                <strong>版本:</strong> {config?.version}
              </div>
              <div style={{ marginBottom: '10px' }}>
                <strong>状态:</strong> {config?.enabled ? '✅ 已启用' : '❌ 未启用'}
              </div>
              <div style={{ marginBottom: '10px' }}>
                <strong>策略:</strong> {config?.strategy === 'percentage' ? '比例分流' : '条件分流'}
              </div>
              {config?.strategy === 'percentage' && (
                <div style={{ marginBottom: '10px' }}>
                  <strong>灰度比例:</strong> {config.percentage}%
                </div>
              )}
              {config?.strategy === 'condition' && config.condition && (
                <div style={{ marginBottom: '10px' }}>
                  <strong>条件:</strong>
                  <pre style={{ backgroundColor: 'var(--color-surface-raised)', padding: '10px', borderRadius: '4px', marginTop: '5px' }}>
                    {JSON.stringify(config.condition, null, 2)}
                  </pre>
                </div>
              )}
              <div style={{ marginBottom: '10px' }}>
                <strong>目标节点:</strong> {config?.target_nodes.join(', ')}
              </div>
              {config?.created_at && (
                <div style={{ marginBottom: '10px', fontSize: '12px', color: '#666' }}>
                  创建时间: {new Date(config.created_at).toLocaleString()}
                </div>
              )}
              {config?.updated_at && (
                <div style={{ fontSize: '12px', color: '#666' }}>
                  更新时间: {new Date(config.updated_at).toLocaleString()}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
