import React, { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { rulesAPI } from '../services/api';
import { RuleFlow } from '../types';
import { StateMachineView } from '../components/StateMachineView';
import { GrayReleaseConfigComponent } from '../components/GrayReleaseConfig';
import { SnapshotManager } from '../components/SnapshotManager';
import { ConsoleHeader, ConsolePage, SegmentTabs } from '../components/ConsolePage';

export const RuleLifecyclePage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [rule, setRule] = useState<RuleFlow | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'state' | 'gray' | 'snapshot'>('state');

  const loadRule = useCallback(async () => {
    if (!id) return;
    try {
      setLoading(true);
      const ruleData = await rulesAPI.get(id);
      setRule(ruleData);
      setError(null);
    } catch (err: any) {
      setError(err.message || '加载规则失败');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    if (id) {
      loadRule();
    }
  }, [id, loadRule]);

  const handleStateChange = async (newStatus: string) => {
    // 重新加载规则以获取最新状态
    await loadRule();
  };

  if (loading && !rule) {
    return <div style={{ padding: '20px', textAlign: 'center' }}>加载中...</div>;
  }

  if (error) {
    return (
      <div style={{ padding: '20px' }}>
        <div style={{ padding: '15px', backgroundColor: '#ffebee', color: '#c62828', borderRadius: '4px' }}>
          错误: {error}
        </div>
        <button
          onClick={() => navigate('/rules')}
          style={{
            marginTop: '20px',
            padding: '10px 20px',
            backgroundColor: '#2196F3',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          返回规则列表
        </button>
      </div>
    );
  }

  if (!rule) {
    return (
      <div style={{ padding: '20px', textAlign: 'center' }}>
        <div>规则不存在</div>
        <button
          onClick={() => navigate('/rules')}
          style={{
            marginTop: '20px',
            padding: '10px 20px',
            backgroundColor: '#2196F3',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          返回规则列表
        </button>
      </div>
    );
  }

  return (
    <ConsolePage>
      <ConsoleHeader
        title={rule.name}
        description={`ID: ${rule.id} | 版本: ${rule.version} | 状态: ${rule.lifecycle_status || 'draft'}`}
      />

      <SegmentTabs
        activeKey={activeTab}
        onChange={(key) => setActiveTab(key as 'state' | 'gray' | 'snapshot')}
        items={[
          { key: 'state', label: '🔄 状态机' },
          { key: 'gray', label: '🎯 灰度发布' },
          { key: 'snapshot', label: '📸 快照管理' },
        ]}
      />

      <div className="card" style={{ flex: 1, minHeight: 0, overflow: 'auto', borderRadius: '20px' }}>
        {activeTab === 'state' && (
          <StateMachineView ruleId={rule.id} onStateChange={handleStateChange} />
        )}
        {activeTab === 'gray' && (
          <GrayReleaseConfigComponent ruleId={rule.id} />
        )}
        {activeTab === 'snapshot' && (
          <SnapshotManager ruleId={rule.id} />
        )}
      </div>
    </ConsolePage>
  );
};
