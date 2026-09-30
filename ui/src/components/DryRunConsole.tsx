import React, { useState } from 'react';
import { RuleFlow } from '../types';
import { rulesAPI } from '../services/api';
import { dehydrateRule } from '../utils/ruleUtils';
import { ErrorBoundary } from './ErrorBoundary';
import { LazyCodeEditor } from './LazyCodeEditor';

interface DryRunConsoleProps {
  currentRule: RuleFlow | null;
}

export const DryRunConsole: React.FC<DryRunConsoleProps> = ({ currentRule }) => {
  const [sampleData, setSampleData] = useState<string>('[\n  {\n    "temp": 85,\n    "humidity": 60\n  }\n]');
  const [result, setResult] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleRun = async () => {
    if (!currentRule) {
      setError('请先选择一个规则');
      return;
    }

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const parsedData = JSON.parse(sampleData);
      // Dehydrate the rule to remove the visual source node before sending to backend
      const dehydratedRule = dehydrateRule(currentRule);
      const response = await rulesAPI.dryRun(dehydratedRule, Array.isArray(parsedData) ? parsedData : [parsedData]);
      setResult(response);
    } catch (err: any) {
      setError(err.message || 'DryRun 执行失败');
    } finally {
      setLoading(false);
    }
  };

  if (!currentRule) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
        <p>请选择一个规则以进行 DryRun</p>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: '20px' }}>
      <div style={{ marginBottom: '20px' }}>
        <h2 style={{ margin: 0 }}>DryRun 调试控制台</h2>
        <p style={{ color: '#666', fontSize: '14px', margin: '5px 0' }}>
          输入示例数据，模拟规则执行过程
        </p>
      </div>

      <div style={{ display: 'flex', flex: 1, gap: '20px', overflow: 'hidden' }}>
        {/* 左侧：输入区域 */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
          <div style={{ marginBottom: '10px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <label style={{ fontWeight: 'bold' }}>示例数据 (JSON)</label>
            <button
              onClick={handleRun}
              disabled={loading}
              style={{
                padding: '8px 16px',
                backgroundColor: loading ? '#ccc' : '#4CAF50',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: loading ? 'not-allowed' : 'pointer',
              }}
            >
              {loading ? '运行中...' : '▶ 运行'}
            </button>
          </div>
          <div
            style={{ flex: 1, border: '1px solid #ddd', borderRadius: '4px', overflow: 'hidden', minHeight: 0 }}
          >
            <ErrorBoundary>
              <LazyCodeEditor
                height="100%"
                mode="json"
                value={sampleData}
                onChange={(val) => setSampleData(val || '')}
              />
            </ErrorBoundary>
          </div>
        </div>

        {/* 右侧：结果区域 */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
          <label style={{ fontWeight: 'bold', marginBottom: '10px' }}>执行结果</label>
          <div
            style={{
              flex: 1,
              border: '1px solid #ddd',
              borderRadius: '4px',
              padding: '15px',
              backgroundColor: '#f9f9f9',
              overflow: 'auto',
              fontFamily: 'monospace',
              fontSize: '13px',
            }}
          >
            {error && (
              <div style={{ color: '#F44336', marginBottom: '10px' }}>
                <strong>错误：</strong>
                {error}
              </div>
            )}
            {loading && <div>执行中...</div>}
            {result && (
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>
                {JSON.stringify(result, null, 2)}
              </pre>
            )}
            {!loading && !result && !error && (
              <div style={{ color: '#666' }}>点击"运行"按钮开始 DryRun</div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
