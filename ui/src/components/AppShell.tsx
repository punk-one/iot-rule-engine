import React from 'react';
import { Breadcrumb, Button, Dropdown, Layout, Menu, Popconfirm, Tooltip, type MenuProps } from 'antd';
import {
  ApartmentOutlined,
  BarChartOutlined,
  ClusterOutlined,
  DashboardOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoonOutlined,
  SettingOutlined,
  SunOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { BrandLogo } from './BrandLogo';
import { useConsoleTheme } from '../theme/ThemeContext';
import '../styles/console-ui.css';

type NavigationLeaf = {
  key: string;
  label: string;
  group?: string;
  groupPath?: string;
  match: (pathname: string) => boolean;
};

const navigationLeaves: NavigationLeaf[] = [
  { key: '/dashboard', label: '工作台', match: (path) => path === '/dashboard' },
  { key: '/rules', label: '规则管理', match: (path) => path.startsWith('/rules') },
  { key: '/runtime', label: '运行监控', match: (path) => path.startsWith('/runtime') },
  { key: '/resources', label: '资源中心', match: (path) => path.startsWith('/connectors') || path.startsWith('/plugins') },
  { key: '/config', label: '系统配置', match: (path) => path.startsWith('/config') },
  { key: '/users', label: '用户管理', match: (path) => path.startsWith('/users') },
];

const navigationItems: MenuProps['items'] = [
  {
    key: '/dashboard',
    label: <Link to="/dashboard">首页</Link>,
    icon: <DashboardOutlined />,
  },
  {
    key: '/rules',
    label: <Link to="/rules">规则</Link>,
    icon: <ApartmentOutlined />,
  },
  {
    key: '/runtime',
    label: <Link to="/runtime">运行</Link>,
    icon: <BarChartOutlined />,
  },
  {
    key: '/resources',
    label: <Link to="/connectors">资源</Link>,
    icon: <ClusterOutlined />,
  },
];

const settingsNavigationItems: MenuProps['items'] = [
  {
    key: '/config',
    label: <Link to="/config">配置</Link>,
    icon: <SettingOutlined />,
  },
];

const getRouteContext = (pathname: string) => {
  const leaf = navigationLeaves.find((item) => item.match(pathname)) || navigationLeaves[0];
  if (pathname === '/rules/new') return { ...leaf, label: '新建规则' };
  if (/^\/rules\/[^/]+\/edit$/.test(pathname)) return { ...leaf, label: '编辑规则' };
  if (/^\/rules\/[^/]+\/lifecycle$/.test(pathname)) return { ...leaf, label: '规则生命周期' };
  if (pathname.includes('/nodes/')) return { ...leaf, label: '节点详情' };
  if (pathname.includes('/edges/')) return { ...leaf, label: '边详情' };
  if (pathname.endsWith('/dag')) return { ...leaf, label: '运行拓扑' };
  return leaf;
};

export const AppShell: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout } = useAuth();
  const { theme, toggleTheme } = useConsoleTheme();
  const [mobileOpen, setMobileOpen] = React.useState(false);
  const [narrow, setNarrow] = React.useState(false);

  React.useEffect(() => {
    if (narrow) setMobileOpen(false);
  }, [location.pathname, narrow]);

  const handleLogout = async () => {
    await logout();
    navigate('/login');
  };

  const routeContext = getRouteContext(location.pathname);
  const username = user?.username || '未知用户';
  const userMenuItems = [
    {
      key: 'current-user',
      disabled: true,
      label: <span className="app-shell__current-user">当前用户：{username}</span>,
    },
    {
      type: 'divider' as const,
      key: 'divider-after-user',
    },
    {
      key: 'users',
      label: '用户管理',
      icon: <UserOutlined />,
      onClick: () => navigate('/users'),
    },
    {
      type: 'divider' as const,
      key: 'divider-before-logout',
    },
    {
      key: 'logout',
      danger: true,
      label: (
        <Popconfirm
          title="确认退出登录？"
          okText="退出"
          cancelText="取消"
          onConfirm={() => void handleLogout()}
        >
          <span>退出登录</span>
        </Popconfirm>
      ),
      icon: <LogoutOutlined />,
    },
  ];

  return (
    <Layout className={`app-shell app-shell--${theme}`}>
      <Layout.Sider
        className="app-shell__sider app-shell__sider--rail"
        data-testid="app-shell-sidebar"
        width={104}
        collapsedWidth={0}
        collapsed={narrow && !mobileOpen}
        breakpoint="lg"
        trigger={null}
        onBreakpoint={(broken) => {
          setNarrow(broken);
          if (broken) setMobileOpen(false);
        }}
      >
        <button
          type="button"
          className="app-shell__brand"
          aria-label="返回工作台"
          onClick={() => navigate('/dashboard')}
        >
          <BrandLogo size={34} showText={false} theme={theme} />
        </button>
        <Menu
          mode="vertical"
          theme={theme}
          selectedKeys={[routeContext.key]}
          items={navigationItems}
          onClick={() => {
            if (narrow) setMobileOpen(false);
          }}
          className="app-shell__menu"
          data-testid="sidebar-nav"
        />
        <Menu
          mode="vertical"
          theme={theme}
          selectedKeys={[routeContext.key]}
          items={settingsNavigationItems}
          className="app-shell__menu app-shell__menu--settings"
          data-testid="sidebar-settings"
        />
      </Layout.Sider>

      <Layout className="app-shell__workspace">
        <Layout.Header data-testid="app-shell-header" className="app-shell__topbar">
          <Tooltip title={mobileOpen ? '关闭侧边栏' : '打开侧边栏'}>
            <Button
              type="text"
              icon={mobileOpen ? <MenuFoldOutlined /> : <MenuUnfoldOutlined />}
              aria-label={mobileOpen ? '关闭侧边栏' : '打开侧边栏'}
              onClick={() => setMobileOpen((value) => !value)}
              className="app-shell__collapse-button"
            />
          </Tooltip>
          <Breadcrumb
            className="app-shell__breadcrumb"
            items={routeContext.group ? [
              { title: <Link to={routeContext.groupPath || routeContext.key}>{routeContext.group}</Link> },
              { title: routeContext.label },
            ] : [{ title: routeContext.label }]}
          />
          <div className="app-shell__account">
            <Tooltip title={theme === 'light' ? '切换到深色模式' : '切换到浅色模式'}>
              <Button
                type="text"
                icon={theme === 'light' ? <MoonOutlined /> : <SunOutlined />}
                aria-label={theme === 'light' ? '切换到深色模式' : '切换到浅色模式'}
                onClick={toggleTheme}
                className="app-shell__theme-button"
              />
            </Tooltip>
            <Dropdown menu={{ items: userMenuItems }} trigger={['click']} placement="bottomRight">
              <Button type="text" icon={<UserOutlined />} aria-label="用户菜单" className="app-shell__account-button">
                <span>{username}</span>
              </Button>
            </Dropdown>
          </div>
        </Layout.Header>

        <Layout.Content className="app-shell__content">
          <Outlet />
        </Layout.Content>
      </Layout>
    </Layout>
  );
};
