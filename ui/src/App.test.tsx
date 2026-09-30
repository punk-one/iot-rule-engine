import React from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { LoginPage } from './pages/LoginPage';

const mockNavigate = jest.fn();

jest.mock('./auth/AuthContext', () => ({
  useAuth: () => ({
    login: jest.fn(),
    loading: false,
    isAuthenticated: false,
  }),
}));

jest.mock('react-router-dom', () => ({
  Navigate: () => null,
  useLocation: () => ({ state: null }),
  useNavigate: () => mockNavigate,
}));

test('renders IoT Rule Engine login branding by default', () => {
  render(<LoginPage />);

  expect(screen.getByText('IoT Rule Engine')).toBeInTheDocument();
  expect(screen.getByText('INDUSTRIAL IoT RULE ENGINE')).toBeInTheDocument();
  expect(screen.getByText(/让工业数据沿着可靠的规则流持续运转/)).toBeInTheDocument();
  expect(screen.getByAltText('IoT Rule Engine')).toHaveAttribute('src', 'iot-rule-engine-mark.svg');
  expect(screen.getByText('欢迎登录')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '登录' })).toBeInTheDocument();
});

test('supports password visibility toggle and removes extra form description copy', async () => {
  render(<LoginPage />);

  expect(screen.queryByText('请输入账号信息进入规则控制台。')).not.toBeInTheDocument();

  const passwordInput = screen.getByPlaceholderText('请输入密码') as HTMLInputElement;
  const toggleButton = screen.getByRole('button', { name: '显示密码' });

  expect(passwordInput.type).toBe('password');

  await userEvent.click(toggleButton);
  expect(passwordInput.type).toBe('text');

  await userEvent.click(screen.getByRole('button', { name: '隐藏密码' }));
  expect(passwordInput.type).toBe('password');
});
