import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { FlowCanvas } from '../components/FlowCanvas';
import { PropertiesPanel } from '../components/PropertiesPanel';
import { NodePalette } from '../components/NodePalette';
import { DryRunConsole } from '../components/DryRunConsole';
import { useRuleStore } from '../store/ruleStore';
import { hydrateRule, dehydrateRule } from '../utils/ruleUtils';
import { SegmentTabs } from '../components/ConsolePage';
import { useFeedback } from '../components/FeedbackProvider';

export const RuleEditorPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const feedback = useFeedback();
  const { currentRule, loading, error, fetchRule, setCurrentRule, updateCurrentRule, updateRule, deployRule, undeployRule } = useRuleStore();
  const temporalStore = useRuleStore.temporal.getState();
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'canvas' | 'dryrun'>('canvas');
  const [showRuleModal, setShowRuleModal] = useState(false);
  const [runtimeAction, setRuntimeAction] = useState<'deploy' | 'undeploy' | null>(null);

  // Hydrate rule on load
  useEffect(() => {
    if (!id) return;

    const loadedRule = useRuleStore.getState().currentRule;
    if (loadedRule?.id === id) {
      return;
    }

    fetchRule(id);
  }, [id, fetchRule]);

  // Handle Hydration when currentRule is loaded from backend (no source-node)
  useEffect(() => {
      if (currentRule && currentRule.source && !currentRule.nodes.some(n => n.id === 'rule_source_node')) {
          const hydrated = hydrateRule(currentRule);
          if (hydrated !== currentRule) {
             setCurrentRule(hydrated);
          }
      }
  }, [currentRule, setCurrentRule]);

  useEffect(() => {
    if (!id || !currentRule || currentRule.id !== id) {
      return;
    }
    if (currentRule.status === 'deployed' || currentRule.status === 'running') {
      feedback.warning('已部署的规则不能编辑，请先取消部署');
      navigate('/rules', { replace: true });
    }
  }, [id, currentRule, feedback, navigate]);

  const handleSaveRule = () => {
    setShowRuleModal(true);
  };

  const executeSave = async () => {
    if (!currentRule || currentRule.id !== id) return;

    // 在保存前，等待一小段时间确保 FlowCanvas 的位置同步完成
    await new Promise(resolve => setTimeout(resolve, 100));

    // 再次获取最新的 currentRule
    const latestRule = useRuleStore.getState().currentRule;
    if (!latestRule) return;

    // Dehydrate before saving
    const finalRule = dehydrateRule(latestRule);

    try {
      await updateRule(finalRule.id, finalRule);
      feedback.success('规则已保存');
      setShowRuleModal(false);
    } catch (error: any) {
      if (error?.response?.status === 404 || error?.status === 404) {
          try {
              const { createRule } = useRuleStore.getState();
              await createRule(finalRule);
              feedback.success('规则已创建并保存');
              setShowRuleModal(false);
          } catch (createError) {
              console.error('Failed to create rule:', createError);
              feedback.error('保存失败: ' + ((createError as any)?.message || 'Unknown error'));
          }
      } else {
          console.error('Failed to save rule:', error);
          feedback.error('保存失败: ' + (error.message || 'Unknown error'));
      }
    }
  };

  const handleDeployRule = async () => {
    if (!currentRule || currentRule.id !== id || runtimeAction) return;
    if (!await feedback.confirm('确定要部署此规则吗？')) return;

    setRuntimeAction('deploy');
    try {
      await deployRule(currentRule.id);
      feedback.success('规则已部署');
    } catch (error: any) {
      console.error('Failed to deploy rule:', error);

      // 提取验证错误详情
      let errorMessage = '部署失败';
      if (error.response?.data?.details) {
        const details = error.response.data.details;
        if (Array.isArray(details)) {
          const errorList = details.map((err: any) => {
            if (err.node_id) {
              return `节点 ${err.node_id}: ${err.message}`;
            }
            return err.message;
          }).join('\n');
          errorMessage = `部署失败，验证错误：\n\n${errorList}`;
        } else if (typeof details === 'string') {
          errorMessage = `部署失败: ${details}`;
        }
      } else if (error.response?.data?.error) {
        errorMessage = `部署失败: ${error.response.data.error}`;
      } else if (error.message) {
        errorMessage = `部署失败: ${error.message}`;
      }

      feedback.error(errorMessage);
    } finally {
      setRuntimeAction(null);
    }
  };

  const handleUndeployRule = async () => {
    if (!currentRule || currentRule.id !== id || runtimeAction) return;
    if (!await feedback.confirm('确定要取消部署此规则吗？')) return;

    setRuntimeAction('undeploy');
    try {
      await undeployRule(currentRule.id);
      feedback.success('规则已取消部署');
    } catch (error) {
      console.error('Failed to undeploy rule:', error);
      feedback.error('取消部署失败');
    } finally {
      setRuntimeAction(null);
    }
  };

  if (!currentRule || currentRule.id !== id) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
        <div style={{ textAlign: 'center' }}>
          <p>{loading ? '加载中...' : (error || '规则不存在或加载失败')}</p>
          <button
            onClick={() => navigate('/rules')}
            style={ghostButtonStyle}
          >
            返回列表
          </button>
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', backgroundColor: '#F9FAFB' }}>
      {/* 顶部工具栏 */}
      <div
        style={{
          padding: '16px 20px',
          backgroundColor: 'var(--color-surface)',
          borderBottom: '1px solid #EAECF0',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '16px',
          boxShadow: '0 10px 26px rgba(15, 23, 42, 0.05)',
          flexWrap: 'wrap',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '16px', flexWrap: 'wrap' }}>
          <button
            onClick={() => navigate('/rules')}
            style={backButtonStyle}
            title="返回列表"
          >
            ←
          </button>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <h2 style={{ margin: 0, fontSize: '18px', fontWeight: 700, color: '#101828' }}>{currentRule.name || '未命名规则'}</h2>
              <span
                style={{
                  padding: '4px 10px',
                  borderRadius: '999px',
                  backgroundColor: currentRule.status === 'deployed' || currentRule.status === 'running' ? '#dcfce7' : '#e2e8f0',
                  color: currentRule.status === 'deployed' || currentRule.status === 'running' ? '#168c6a' : 'var(--color-text-secondary)',
                  fontSize: '12px',
                  fontWeight: 700,
                }}
              >
                {currentRule.status === 'deployed' || currentRule.status === 'running' ? '运行中' : '草稿'}
              </span>
            </div>
            <div style={{ fontSize: '12px', color: '#667085', fontFamily: 'JetBrains Mono, monospace' }}>{currentRule.id}</div>
          </div>
        </div>

        <div style={{ display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
          <div style={{ minWidth: '220px' }}>
            <SegmentTabs
              activeKey={activeTab}
              onChange={(key) => setActiveTab(key as 'canvas' | 'dryrun')}
              items={[
                { key: 'canvas', label: '画布' },
                { key: 'dryrun', label: 'DryRun' },
              ]}
            />
          </div>

          <button onClick={() => temporalStore.undo()} disabled={!temporalStore.pastStates.length} style={historyButtonStyle(!temporalStore.pastStates.length)}>
            Undo
          </button>
          <button onClick={() => temporalStore.redo()} disabled={!temporalStore.futureStates.length} style={historyButtonStyle(!temporalStore.futureStates.length)}>
            Redo
          </button>

          <button
            onClick={() => handleSaveRule()}
            disabled={currentRule.status === 'deployed' || currentRule.status === 'running'}
            style={primaryButtonStyle((currentRule.status === 'deployed' || currentRule.status === 'running') ? '#93c5fd' : '#2563eb', currentRule.status === 'deployed' || currentRule.status === 'running')}
          >
            保存规则
          </button>

          <div style={{ width: '1px', height: '24px', backgroundColor: '#EAECF0', alignSelf: 'center', margin: '0 4px' }} />

          {currentRule.status === 'deployed' || currentRule.status === 'running' ? (
            <button disabled={runtimeAction !== null} onClick={handleUndeployRule} style={primaryButtonStyle('#dc2626')}>
              {runtimeAction === 'undeploy' ? '停止中...' : '停止运行'}
            </button>
          ) : (
            <button disabled={runtimeAction !== null} onClick={handleDeployRule} style={primaryButtonStyle('#16a34a')}>
              {runtimeAction === 'deploy' ? '部署中...' : '部署上线'}
            </button>
          )}
        </div>
      </div>

      {/* 主内容区 */}
      <div style={{ display: 'flex', flex: 1, minHeight: 0, overflow: 'hidden', position: 'relative', padding: '16px', gap: '16px' }}>
        <NodePalette onDragStart={() => {}} />

        <div style={{ flex: 1, minWidth: 0, position: 'relative' }}>
          <div className="card" style={{ height: '100%', minWidth: 0, position: 'relative', backgroundColor: '#F2F4F7', borderRadius: '20px', overflow: 'hidden' }}>
            {activeTab === 'canvas' ? (
              <FlowCanvas onNodeClick={setSelectedNodeId} selectedNodeId={selectedNodeId} />
            ) : (
              <div style={{ height: '100%', overflow: 'auto', backgroundColor: 'var(--color-surface)' }}>
                <DryRunConsole currentRule={currentRule} />
              </div>
            )}
          </div>

          {/* 节点属性抽屉 - 以覆盖层形式显示，避免触发画布重新布局 */}
          {selectedNodeId && activeTab === 'canvas' && (
            <div
              data-testid="node-details-drawer"
              style={{
                position: 'absolute',
                top: 0,
                right: 0,
                bottom: 0,
                width: 'min(720px, calc(100% - 24px))',
                backgroundColor: 'var(--color-surface)',
                display: 'flex',
                flexDirection: 'column',
                border: '1px solid #EAECF0',
                borderRadius: '20px',
                boxShadow: '0 16px 30px rgba(15, 23, 42, 0.08)',
                zIndex: 20,
              }}
            >
              <div style={{ padding: '16px 20px', borderBottom: '1px solid #EAECF0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <h3 style={{ margin: 0, fontSize: '16px', fontWeight: 600, color: '#101828' }}>节点配置</h3>
                <button
                  onClick={() => setSelectedNodeId(null)}
                  style={{ background: 'none', border: 'none', fontSize: '18px', cursor: 'pointer', color: '#667085' }}
                >
                  ✕
                </button>
              </div>
              <div style={{ flex: 1, overflow: 'auto' }}>
                <PropertiesPanel nodeId={selectedNodeId} />
              </div>
            </div>
          )}
        </div>
      </div>

      {/* 规则属性弹窗 */}
      {showRuleModal && (
        <div style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.5)', display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 1000 }}>
          <div style={{ backgroundColor: 'var(--color-surface-raised)', color: 'var(--color-text)', padding: '32px', borderRadius: '18px', width: '480px', boxShadow: 'var(--shadow-floating)' }}>
            <h3 style={{ marginTop: 0, marginBottom: '24px', fontSize: '18px', fontWeight: 600 }}>修改规则属性</h3>

            <div style={{ marginBottom: '20px' }}>
                <label style={{ display: 'block', marginBottom: '6px', fontSize: '14px', fontWeight: 500, color: '#344054' }}>规则名称</label>
                <input
                    type="text"
                    value={currentRule.name}
                    onChange={(e) => updateCurrentRule({ name: e.target.value })}
                    style={{ width: '100%', padding: '10px 14px', border: '1px solid #D0D5DD', borderRadius: '8px', fontSize: '14px' }}
                    placeholder="例如: 设备数据清洗规则"
                />
            </div>

            <div style={{ marginBottom: '20px' }}>
                <label style={{ display: 'block', marginBottom: '6px', fontSize: '14px', fontWeight: 500, color: '#344054' }}>规则描述</label>
                <textarea
                    value={currentRule.description}
                    onChange={(e) => updateCurrentRule({ description: e.target.value })}
                    style={{ width: '100%', padding: '10px 14px', border: '1px solid #D0D5DD', borderRadius: '8px', fontSize: '14px', minHeight: '100px', resize: 'vertical' }}
                    placeholder="描述该规则的作用..."
                />
            </div>

            <div style={{ marginBottom: '24px', padding: '16px', backgroundColor: '#F9FAFB', borderRadius: '8px', border: '1px solid #EAECF0' }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '12px', cursor: 'pointer' }}>
                    <input
                        type="checkbox"
                        checked={currentRule.debug_mode || false}
                        onChange={(e) => updateCurrentRule({ debug_mode: e.target.checked })}
                        style={{ width: '18px', height: '18px', cursor: 'pointer', accentColor: '#2E90FA' }}
                    />
                    <div>
                      <div style={{ fontWeight: 600, fontSize: '14px', color: '#344054' }}>Debug 模式</div>
                      <div style={{ fontSize: '12px', color: '#667085', marginTop: '2px' }}>启用后采集详细 Trace 与调试事件；关闭时仅保留基础运行指标和错误信息</div>
                    </div>
                </label>
            </div>

            <div style={{ display: 'flex', gap: '12px', justifyContent: 'flex-end' }}>
              <button
                onClick={() => setShowRuleModal(false)}
                style={ghostButtonStyle}
              >
                取消
              </button>
              <button
                onClick={() => executeSave()}
                style={primaryButtonStyle('#2563eb')}
              >
                确认并保存
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

const backButtonStyle: React.CSSProperties = {
  padding: '8px 12px',
  backgroundColor: 'var(--color-surface)',
  color: '#667085',
  border: '1px solid #D0D5DD',
  borderRadius: '10px',
  cursor: 'pointer',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  fontWeight: 700,
};

const ghostButtonStyle: React.CSSProperties = {
  padding: '10px 18px',
  backgroundColor: 'var(--color-surface)',
  color: '#344054',
  border: '1px solid #D0D5DD',
  borderRadius: '10px',
  cursor: 'pointer',
  fontSize: '14px',
  fontWeight: 600,
};

const historyButtonStyle = (disabled: boolean): React.CSSProperties => ({
  padding: '8px 12px',
  backgroundColor: 'var(--color-surface)',
  color: disabled ? '#D0D5DD' : '#344054',
  border: '1px solid #D0D5DD',
  borderRadius: '10px',
  cursor: disabled ? 'not-allowed' : 'pointer',
  fontSize: '14px',
  fontWeight: 600,
});

const primaryButtonStyle = (backgroundColor: string, disabled = false): React.CSSProperties => ({
  padding: '8px 16px',
  backgroundColor,
  color: 'white',
  border: 'none',
  borderRadius: '10px',
  cursor: disabled ? 'not-allowed' : 'pointer',
  fontWeight: 600,
  fontSize: '14px',
  opacity: disabled ? 0.7 : 1,
});
