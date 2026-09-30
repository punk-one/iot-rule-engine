import React from 'react';
import { Link, useLocation } from 'react-router-dom';

const tabs = [
  { key: 'system', label: '系统配置', to: '/config' },
  { key: 'users', label: '用户管理', to: '/config/users' },
];

export const ConfigTabs: React.FC = () => {
  const location = useLocation();

  return (
    <div
      style={{
        display: 'flex',
        gap: '10px',
        alignItems: 'center',
        padding: '8px',
        borderRadius: '14px',
        backgroundColor: '#f8fafc',
        border: '1px solid #e2e8f0',
      }}
    >
      {tabs.map((tab) => {
        const active =
          tab.to === '/config'
            ? location.pathname === '/config'
            : location.pathname.startsWith(tab.to);

        return (
          <Link
            key={tab.key}
            to={tab.to}
            style={{
              padding: '10px 16px',
              borderRadius: '10px',
              textDecoration: 'none',
              color: active ? 'var(--color-text)' : 'var(--color-text-secondary)',
              backgroundColor: active ? 'var(--color-surface-raised)' : 'transparent',
              border: active ? '1px solid #dbeafe' : '1px solid transparent',
              boxShadow: active ? '0 8px 18px rgba(15, 23, 42, 0.08)' : 'none',
              fontSize: '14px',
              fontWeight: 600,
            }}
          >
            {tab.label}
          </Link>
        );
      })}
    </div>
  );
};
