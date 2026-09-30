import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { AppShell } from './AppShell';

const mockNavigate = jest.fn();
const mockLogout = jest.fn();
const mockToggleTheme = jest.fn();

jest.mock('antd', () => ({
  Button: ({ children, ...props }: any) => <button {...props}>{children}</button>,
  Breadcrumb: ({ items, ...props }: any) => (
    <nav {...props}>{(items || []).map((item: any, index: number) => <span key={index}>{item.title}</span>)}</nav>
  ),
  Dropdown: ({ children, menu }: any) => (
    <div>
      {children}
      <div data-testid="user-dropdown-menu">
        {(menu?.items || []).map((item: any) => (
          item.type === 'divider'
            ? <hr key={item.key} data-testid="user-menu-divider" />
            : item.disabled
              ? <div key={item.key}>{item.label}</div>
              : <button key={item.key} onClick={item.onClick}>{item.label}</button>
        ))}
      </div>
    </div>
  ),
  Popconfirm: ({ children }: any) => <div>{children}</div>,
  Tooltip: ({ children }: any) => <>{children}</>,
  Layout: Object.assign(
    ({ children, ...props }: any) => <div {...props}>{children}</div>,
    {
      Header: ({ children, ...props }: any) => <header {...props}>{children}</header>,
      Content: ({ children, ...props }: any) => <main {...props}>{children}</main>,
      Sider: ({ children, collapsed: _collapsed, collapsedWidth: _collapsedWidth, breakpoint: _breakpoint, trigger: _trigger, onBreakpoint: _onBreakpoint, ...props }: any) => <aside {...props}>{children}</aside>,
    }
  ),
  Menu: ({ items, selectedKeys, ...props }: any) => (
    <nav data-testid={props['data-testid']} data-selected-keys={JSON.stringify(selectedKeys || [])}>
      {(items || []).map((item: any) => (
        <span key={item.key}>
          {item.label}
          {(item.children || []).map((child: any) => <span key={child.key}>{child.label}</span>)}
        </span>
      ))}
    </nav>
  ),
  theme: {
    useToken: () => ({
      token: {
        colorBgLayout: '#f4faf7',
      },
    }),
  },
}));

jest.mock('react-router-dom', () => ({
  Outlet: () => <div>Outlet Content</div>,
  useLocation: () => ({ pathname: '/dashboard' }),
  useNavigate: () => mockNavigate,
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

jest.mock('../auth/AuthContext', () => ({
  useAuth: () => ({
    user: { username: 'admin' },
    logout: mockLogout,
  }),
}));

jest.mock('../theme/ThemeContext', () => ({
  useConsoleTheme: () => ({
    theme: 'light',
    toggleTheme: mockToggleTheme,
  }),
}));

test('renders the IoT Rule Engine shell with workbench navigation', () => {
  render(<AppShell />);

  expect(screen.getByTestId('sidebar-nav')).toHaveTextContent('首页规则运行资源');
  expect(screen.getByTestId('sidebar-settings')).toHaveTextContent('配置');
  expect(screen.getByAltText('IoT Rule Engine')).toBeInTheDocument();
  const sidebar = screen.getByTestId('app-shell-sidebar');
  expect(sidebar).toHaveClass('app-shell__sider--rail');
  expect(screen.getByTestId('app-shell-header')).toBeInTheDocument();
  expect(screen.getAllByText('工作台').length).toBeGreaterThan(0);
  expect(screen.getByText('规则')).toBeInTheDocument();
  expect(screen.getByText('运行')).toBeInTheDocument();
  expect(screen.getByTestId('sidebar-nav')).toHaveAttribute('data-selected-keys', '["/dashboard"]');
  expect(screen.getByTestId('sidebar-settings')).toHaveAttribute('data-selected-keys', '["/dashboard"]');
  expect(screen.getByRole('button', { name: '打开侧边栏' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '切换到深色模式' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /当前用户：admin/ })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: '用户菜单' })).toBeInTheDocument();
  expect(screen.getByTestId('user-dropdown-menu')).toBeInTheDocument();
  expect(screen.getByText(/当前用户：admin/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '用户管理' })).toBeInTheDocument();
  expect(screen.getAllByTestId('user-menu-divider')).toHaveLength(2);
  expect(screen.getByRole('button', { name: '退出登录' })).toBeInTheDocument();
  expect(screen.getByText('Outlet Content')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: '切换到深色模式' }));
  expect(mockToggleTheme).toHaveBeenCalledTimes(1);

});
