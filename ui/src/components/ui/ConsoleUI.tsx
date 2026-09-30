import React from 'react';
import { Alert, Button, Empty, Result, Skeleton, Spin, Tag } from 'antd';
import type { ButtonProps } from 'antd';
import '../../styles/console-ui.css';

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger';

export const PageLayout: React.FC<{
  children: React.ReactNode;
  maxWidth?: number;
  className?: string;
}> = ({ children, maxWidth, className = '' }) => (
  <main className={`console-page ${className}`.trim()}>
    <div className="console-page__inner" style={{ maxWidth: maxWidth ? `${maxWidth}px` : undefined }}>
      {children}
    </div>
  </main>
);

export const PageHeader: React.FC<{
  title: React.ReactNode;
  description?: React.ReactNode;
  eyebrow?: React.ReactNode;
  actions?: React.ReactNode;
}> = ({ title, description, eyebrow, actions }) => (
  <header className="console-page-header">
    <div className="console-page-header__copy">
      {eyebrow ? <div className="console-page-header__eyebrow">{eyebrow}</div> : null}
      <h1 className="console-page-header__title">{title}</h1>
      {description ? <div className="console-page-header__description">{description}</div> : null}
    </div>
    {actions ? <div className="console-page-header__actions">{actions}</div> : null}
  </header>
);

export const FilterBar: React.FC<{
  children: React.ReactNode;
  summary?: React.ReactNode;
  actions?: React.ReactNode;
}> = ({ children, summary, actions }) => (
  <section className="console-filter-bar" aria-label="筛选条件">
    <div className="console-filter-bar__fields">{children}</div>
    {summary || actions ? (
      <div className="console-filter-bar__aside">
        {summary ? <div className="console-filter-bar__summary">{summary}</div> : null}
        {actions}
      </div>
    ) : null}
  </section>
);

export const ActionButton: React.FC<ButtonProps & { pendingText?: React.ReactNode }> = ({
  children,
  loading,
  pendingText,
  disabled,
  ...props
}) => (
  <Button {...props} loading={loading} disabled={disabled || Boolean(loading)} aria-busy={Boolean(loading)}>
    {loading && pendingText ? pendingText : children}
  </Button>
);

export const CompactButton: React.FC<React.ButtonHTMLAttributes<HTMLButtonElement> & {
  tone?: 'default' | 'primary' | 'danger';
  loading?: boolean;
  pendingText?: React.ReactNode;
}> = ({ children, className = '', tone = 'default', loading, pendingText, disabled, ...props }) => (
  <button
    type="button"
    {...props}
    disabled={disabled || loading}
    aria-busy={Boolean(loading)}
    className={`console-compact-button console-compact-button--${tone} ${className}`.trim()}
  >
    {loading && pendingText ? pendingText : children}
  </button>
);

export type ActionMenuItem = {
  key: string;
  label: React.ReactNode;
  danger?: boolean;
  disabled?: boolean;
  onClick: () => void | Promise<void>;
};

export const ActionMenu: React.FC<{ label: string; items: ActionMenuItem[] }> = ({ label, items }) => {
  const [open, setOpen] = React.useState(false);
  const detailsRef = React.useRef<HTMLDetailsElement | null>(null);

  return (
    <details ref={detailsRef} className="console-action-menu" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary aria-label={label}>更多</summary>
      {open ? (
        <div className="console-action-menu__popup" role="menu">
          {items.map((item) => (
            <button
              key={item.key}
              type="button"
              role="menuitem"
              disabled={item.disabled}
              className={item.danger ? 'console-action-menu__danger' : undefined}
              onClick={() => {
                detailsRef.current?.removeAttribute('open');
                void item.onClick();
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      ) : null}
    </details>
  );
};

const toneColors: Record<StatusTone, string> = {
  neutral: 'default',
  info: 'processing',
  success: 'success',
  warning: 'warning',
  danger: 'error',
};

export const StatusTag: React.FC<{
  tone?: StatusTone;
  children: React.ReactNode;
  className?: string;
}> = ({ tone = 'neutral', children, className }) => (
  <Tag color={toneColors[tone]} className={className}>
    {children}
  </Tag>
);

type AsyncStateProps = {
  loading?: boolean;
  error?: React.ReactNode;
  empty?: boolean;
  emptyTitle?: React.ReactNode;
  emptyDescription?: React.ReactNode;
  emptyAction?: React.ReactNode;
  onRetry?: () => void;
  compact?: boolean;
  children?: React.ReactNode;
};

export const AsyncState: React.FC<AsyncStateProps> = ({
  loading,
  error,
  empty,
  emptyTitle = '暂无数据',
  emptyDescription,
  emptyAction,
  onRetry,
  compact,
  children,
}) => {
  if (loading) {
    return (
      <div className={`console-async-state ${compact ? 'console-async-state--compact' : ''}`} role="status" aria-live="polite">
        {compact ? <Spin /> : <Skeleton active paragraph={{ rows: 4 }} />}
        <span className="sr-only">正在加载</span>
      </div>
    );
  }

  if (error) {
    return compact ? (
      <Alert
        type="error"
        showIcon
        message="加载失败"
        description={error}
        action={onRetry ? <Button onClick={onRetry}>重试</Button> : undefined}
      />
    ) : (
      <Result
        status="error"
        title="加载失败"
        subTitle={error}
        extra={onRetry ? <Button type="primary" onClick={onRetry}>重新加载</Button> : undefined}
      />
    );
  }

  if (empty) {
    return (
      <div className={`console-async-state ${compact ? 'console-async-state--compact' : ''}`}>
        <Empty description={emptyDescription || emptyTitle}>{emptyAction}</Empty>
      </div>
    );
  }

  return <>{children}</>;
};
