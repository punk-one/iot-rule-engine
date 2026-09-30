import React from 'react';
import { Tabs } from 'antd';
import { useLocation, useNavigate } from 'react-router-dom';

const resourceTabs = [
  { key: '/connectors', label: 'Connector 资源' },
  { key: '/connectors/overview', label: '资源概览' },
  { key: '/plugins', label: '插件管理' },
];

export const ResourceTabs: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const activeKey = location.pathname.startsWith('/plugins')
    ? '/plugins'
    : location.pathname.startsWith('/connectors/overview')
      ? '/connectors/overview'
      : '/connectors';

  return (
    <Tabs
      className="resource-tabs"
      activeKey={activeKey}
      items={resourceTabs}
      onChange={navigate}
      aria-label="资源中心页面"
    />
  );
};
