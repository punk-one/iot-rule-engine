import React, { useState, useEffect } from 'react';
import { snapshotsAPI } from '../services/api';
import { RuleSnapshot } from '../types';
import { useFeedback } from './FeedbackProvider';

interface SnapshotManagerProps {
  ruleId: string;
}

export const SnapshotManager: React.FC<SnapshotManagerProps> = ({ ruleId }) => {
  const feedback = useFeedback();
  const [snapshots, setSnapshots] = useState<RuleSnapshot[]>([]);
  const [selectedSnapshot, setSelectedSnapshot] = useState<RuleSnapshot | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    loadSnapshots();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ruleId]);

  const loadSnapshots = async () => {
    try {
      setLoading(true);
      const data = await snapshotsAPI.list(ruleId);
      setSnapshots(data);
      setError(null);
    } catch (err: any) {
      setError(err.message || '加载快照列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateSnapshot = async () => {
    if (!await feedback.confirm('确定要创建新的快照吗？')) {
      return;
    }

    try {
      setCreating(true);
      await snapshotsAPI.create(ruleId);
      await loadSnapshots();
      feedback.success('快照创建成功');
    } catch (err: any) {
      setError(err.message || '创建快照失败');
    } finally {
      setCreating(false);
    }
  };

  const handleViewSnapshot = async (version: number) => {
    try {
      setLoading(true);
      const snapshot = await snapshotsAPI.get(ruleId, version);
      setSelectedSnapshot(snapshot);
    } catch (err: any) {
      setError(err.message || '加载快照详情失败');
    } finally {
      setLoading(false);
    }
  };

  const handleCompare = (version1: number, version2: number) => {
    // TODO: 实现快照对比功能
    feedback.info(`对比功能待实现: v${version1} vs v${version2}`);
  };

  if (loading && snapshots.length === 0) {
    return <div style={{ padding: '20px', textAlign: 'center' }}>加载中...</div>;
  }

  return (
    <div style={{ padding: '20px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
        <h2>规则快照管理</h2>
        <button
          onClick={handleCreateSnapshot}
          disabled={creating}
          style={{
            padding: '10px 20px',
            backgroundColor: '#2196F3',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: creating ? 'not-allowed' : 'pointer',
            opacity: creating ? 0.6 : 1,
            fontWeight: 'bold',
          }}
        >
          {creating ? '创建中...' : '+ 创建快照'}
        </button>
      </div>

      {error && (
        <div style={{ padding: '10px', backgroundColor: '#ffebee', color: '#c62828', borderRadius: '4px', marginBottom: '20px' }}>
          错误: {error}
        </div>
      )}

      <div style={{ display: 'flex', gap: '20px' }}>
        {/* 快照列表 */}
        <div style={{ flex: 1 }}>
          <h3>快照列表</h3>
          {snapshots.length === 0 ? (
            <div style={{ padding: '20px', textAlign: 'center', color: '#999', backgroundColor: '#f9f9f9', borderRadius: '4px' }}>
              暂无快照，点击"创建快照"创建第一个快照
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {snapshots.map((snapshot) => (
                <div
                  key={snapshot.id}
                  style={{
                    padding: '15px',
                    backgroundColor: selectedSnapshot?.version === snapshot.version ? '#e3f2fd' : '#f9f9f9',
                    borderRadius: '4px',
                    border: selectedSnapshot?.version === snapshot.version ? '2px solid #2196F3' : '1px solid #ddd',
                    cursor: 'pointer',
                  }}
                  onClick={() => handleViewSnapshot(snapshot.version)}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div>
                      <div style={{ fontWeight: 'bold', fontSize: '16px' }}>
                        版本 {snapshot.version}
                      </div>
                      <div style={{ fontSize: '12px', color: '#666', marginTop: '5px' }}>
                        {new Date(snapshot.created_at).toLocaleString()}
                      </div>
                      {snapshot.description && (
                        <div style={{ fontSize: '12px', color: '#666', marginTop: '5px' }}>
                          {snapshot.description}
                        </div>
                      )}
                    </div>
                    <div style={{ display: 'flex', gap: '10px' }}>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          handleViewSnapshot(snapshot.version);
                        }}
                        style={{
                          padding: '6px 12px',
                          backgroundColor: '#2196F3',
                          color: 'white',
                          border: 'none',
                          borderRadius: '4px',
                          cursor: 'pointer',
                          fontSize: '12px',
                        }}
                      >
                        查看
                      </button>
                      {snapshots.length > 1 && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            const prevSnapshot = snapshots.find(s => s.version < snapshot.version);
                            if (prevSnapshot) {
                              handleCompare(prevSnapshot.version, snapshot.version);
                            }
                          }}
                          style={{
                            padding: '6px 12px',
                            backgroundColor: '#4CAF50',
                            color: 'white',
                            border: 'none',
                            borderRadius: '4px',
                            cursor: 'pointer',
                            fontSize: '12px',
                          }}
                        >
                          对比
                        </button>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* 快照详情 */}
        {selectedSnapshot && (
          <div style={{ flex: 1, padding: '20px', backgroundColor: '#f9f9f9', borderRadius: '4px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '15px' }}>
              <h3>快照详情 - 版本 {selectedSnapshot.version}</h3>
              <button
                onClick={() => setSelectedSnapshot(null)}
                style={{
                  padding: '6px 12px',
                  backgroundColor: '#9E9E9E',
                  color: 'white',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: 'pointer',
                }}
              >
                关闭
              </button>
            </div>

            <div style={{ marginBottom: '10px' }}>
              <strong>快照ID:</strong> {selectedSnapshot.id}
            </div>
            <div style={{ marginBottom: '10px' }}>
              <strong>规则ID:</strong> {selectedSnapshot.rule_id}
            </div>
            <div style={{ marginBottom: '10px' }}>
              <strong>版本:</strong> {selectedSnapshot.version}
            </div>
            <div style={{ marginBottom: '10px' }}>
              <strong>创建时间:</strong> {new Date(selectedSnapshot.created_at).toLocaleString()}
            </div>
            {selectedSnapshot.created_by && (
              <div style={{ marginBottom: '10px' }}>
                <strong>创建者:</strong> {selectedSnapshot.created_by}
              </div>
            )}
            {selectedSnapshot.description && (
              <div style={{ marginBottom: '10px' }}>
                <strong>描述:</strong> {selectedSnapshot.description}
              </div>
            )}

            <div style={{ marginTop: '20px' }}>
              <strong>规则配置:</strong>
              <div style={{
                marginTop: '10px',
                padding: '15px',
                backgroundColor: 'var(--color-surface)',
                borderRadius: '4px',
                maxHeight: '400px',
                overflow: 'auto',
              }}>
                <pre style={{ margin: 0, fontSize: '12px' }}>
                  {JSON.stringify(selectedSnapshot.rule_flow, null, 2)}
                </pre>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
