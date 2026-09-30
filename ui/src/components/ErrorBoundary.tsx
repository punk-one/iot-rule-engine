import React, { Component, ErrorInfo, ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    // 检查是否是 ResizeObserver 错误
    if (
      error.message?.includes('ResizeObserver') ||
      error.message?.includes('ResizeObserver loop')
    ) {
      // 忽略 ResizeObserver 错误，不显示错误 UI
      return { hasError: false, error: null };
    }
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // 检查是否是 ResizeObserver 错误
    if (
      error.message?.includes('ResizeObserver') ||
      error.message?.includes('ResizeObserver loop')
    ) {
      // 忽略 ResizeObserver 错误，不记录
      return;
    }
    console.error('ErrorBoundary caught an error:', error, errorInfo);
    // #region agent log
    fetch('http://127.0.0.1:7243/ingest/43878644-48dd-42d7-ba67-10a6b9401ea0',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'ErrorBoundary.tsx:31',message:'ErrorBoundary caught error',data:{errorMessage:error.message,errorStack:error.stack,componentStack:errorInfo.componentStack},timestamp:Date.now(),sessionId:'debug-session',runId:'run1',hypothesisId:'E'})}).catch(()=>{});
    // #endregion
    // 将错误信息保存到 state，以便在 render 中显示
    this.setState({ error });
  }

  render() {
    if (this.state.hasError && this.state.error) {
      const errorMessage = this.state.error.message || '未知错误';
      const errorStack = this.state.error.stack || '';

      // 如果提供了自定义 fallback，使用它
      if (this.props.fallback) {
        return this.props.fallback;
      }

      // 默认错误显示
      return (
        <div style={{ padding: '20px', color: 'red', backgroundColor: '#FEF3F2', border: '1px solid #F04438', borderRadius: '4px' }}>
          <h2 style={{ marginTop: 0, marginBottom: '12px', fontSize: '18px', fontWeight: 600 }}>出现错误</h2>
          <p style={{ marginBottom: '12px', fontSize: '14px', fontWeight: 500 }}>{errorMessage}</p>

          {errorStack && (
            <details style={{ marginTop: '12px' }}>
              <summary style={{
                cursor: 'pointer',
                fontWeight: 600,
                fontSize: '13px',
                color: '#7F1D1D',
                padding: '8px',
                backgroundColor: '#FEE2E2',
                borderRadius: '4px',
                userSelect: 'none'
              }}>
                查看错误详情（点击展开）
              </summary>
              <pre style={{
                marginTop: '8px',
                padding: '12px',
                backgroundColor: '#FEE2E2',
                borderRadius: '4px',
                overflow: 'auto',
                maxHeight: '300px',
                fontSize: '11px',
                lineHeight: '1.5',
                fontFamily: 'monospace',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word'
              }}>
                {errorStack}
              </pre>
            </details>
          )}

          <div style={{ marginTop: '12px', padding: '8px', backgroundColor: '#FEF3F2', borderRadius: '4px', fontSize: '12px', color: '#7F1D1D' }}>
            <strong>提示：</strong>错误详情已记录到浏览器控制台（按 F12 查看）
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
