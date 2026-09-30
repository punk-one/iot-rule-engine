import React from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { BrandLogo } from './BrandLogo';

export const Navigation: React.FC = () => {
  const location = useLocation();
  const { user, logout } = useAuth();

  const isActive = (path: string) => {
    return location.pathname.startsWith(path);
  };

  const navLinkStyle = (active: boolean): React.CSSProperties => ({
    color: active ? 'var(--color-text)' : 'var(--color-text-secondary)',
    textDecoration: 'none',
    padding: '10px 16px',
    borderRadius: '12px',
    backgroundColor: active ? 'var(--color-surface-raised)' : 'var(--color-surface-subtle)',
    border: `1px solid ${active ? '#dbeafe' : 'rgba(255,255,255,0.08)'}`,
    fontWeight: active ? 700 : 500,
    boxShadow: active ? '0 10px 24px rgba(15, 23, 42, 0.14)' : 'none',
    whiteSpace: 'nowrap',
  });

  return (
    <div
      style={{
        padding: '14px 20px',
        background: 'linear-gradient(135deg, #0f172a 0%, #1d4ed8 48%, #2563eb 100%)',
        color: 'white',
        display: 'flex',
        gap: '14px',
        alignItems: 'center',
        boxShadow: '0 14px 36px rgba(15, 23, 42, 0.14)',
        flexWrap: 'wrap',
      }}
    >
      <div style={{ marginRight: '8px', whiteSpace: 'nowrap' }}>
        <BrandLogo size={42} theme="dark" />
      </div>
      <Link
        to="/rules"
        style={navLinkStyle(isActive('/rules'))}
      >
        📋 规则管理
      </Link>
      <Link
        to="/connectors"
        style={navLinkStyle(isActive('/connectors'))}
      >
        🔗 Connector 资源
      </Link>
      <Link
        to="/plugins"
        style={navLinkStyle(isActive('/plugins'))}
      >
        🔌 插件管理
      </Link>
      <Link
        to="/runtime"
        style={navLinkStyle(isActive('/runtime'))}
      >
        📊 运行态监控
      </Link>
      <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
        <div
          style={{
            fontSize: '13px',
            opacity: 0.95,
            padding: '8px 14px',
            borderRadius: '999px',
            backgroundColor: 'rgba(255,255,255,0.1)',
            border: '1px solid rgba(255,255,255,0.14)',
          }}
        >
          当前用户：<strong>{user?.username || '未知用户'}</strong>
        </div>
        <button
          onClick={() => void logout()}
          style={{
            padding: '8px 14px',
            borderRadius: '10px',
            border: '1px solid rgba(255,255,255,0.28)',
            backgroundColor: 'rgba(255,255,255,0.14)',
            color: '#fff',
            cursor: 'pointer',
            fontWeight: 600,
          }}
        >
          退出登录
        </button>
      </div>
    </div>
  );
};
