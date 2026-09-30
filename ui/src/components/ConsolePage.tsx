import React from 'react';
import { PageHeader, PageLayout } from './ui/ConsoleUI';

export const ConsolePage: React.FC<{
  children: React.ReactNode;
  maxWidth?: number;
}> = ({ children, maxWidth }) => {
  return <PageLayout maxWidth={maxWidth}>{children}</PageLayout>;
};

export const ConsoleHeader: React.FC<{
  title: React.ReactNode;
  description?: React.ReactNode;
  eyebrow?: React.ReactNode;
  actions?: React.ReactNode;
}> = ({ title, description, eyebrow, actions }) => {
  return <PageHeader title={title} description={description} eyebrow={eyebrow} actions={actions} />;
};

export const StatCard: React.FC<{
  label: string;
  value: React.ReactNode;
  hint?: React.ReactNode;
  accent?: string;
}> = ({ label, value, hint, accent = 'var(--color-text)' }) => {
  return (
    <div className="card console-stat-card">
      <div className="console-stat-card__label">{label}</div>
      <div className="console-stat-card__value" style={{ color: accent }}>{value}</div>
      {hint ? <div className="console-stat-card__hint">{hint}</div> : null}
    </div>
  );
};

export const SegmentTabs: React.FC<{
  activeKey: string;
  items: Array<{ key: string; label: React.ReactNode }>;
  onChange: (key: string) => void;
}> = ({ activeKey, items, onChange }) => {
  return (
    <div
      aria-label="页面分区"
      style={{
        display: 'flex',
        gap: '8px',
        flexWrap: 'wrap',
        padding: '8px',
        borderRadius: '16px',
        backgroundColor: '#f8fafc',
        border: '1px solid #e2e8f0',
        marginBottom: '20px',
      }}
    >
      {items.map((item) => {
        const active = item.key === activeKey;
        return (
          <button
            key={item.key}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(item.key)}
            style={{
              padding: '10px 16px',
              borderRadius: '10px',
              border: active ? '1px solid #bfdbfe' : '1px solid transparent',
              backgroundColor: active ? 'var(--color-surface-raised)' : 'transparent',
              color: active ? 'var(--color-primary)' : 'var(--color-text-secondary)',
              fontSize: '14px',
              fontWeight: 700,
              cursor: 'pointer',
              boxShadow: active ? '0 8px 18px rgba(15, 23, 42, 0.08)' : 'none',
            }}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
};
