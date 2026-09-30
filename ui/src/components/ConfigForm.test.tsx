import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { ConfigForm } from './ConfigForm';
import type { SystemConfig } from '../services/api';

jest.mock('../services/api', () => ({
  configAPI: { uploadCert: jest.fn() },
}));

const baseConfig = {
  common: {
    nats: {
      mode: 'internal',
      host: '127.0.0.1',
      port: 0,
      store_dir: './data/nats',
      url: '',
      auth_mode: 'none',
      max_reconnects: -1,
      reconnect_wait_ms: 2000,
      username: '',
      password: '',
      token: '',
      cert_file: '',
      key_file: '',
      ca_file: '',
      creds_file: '',
    },
  },
  manager: { port: 11011, db_path: './data' },
  executor: { plugins_dir: './plugins', worker_count: 1, node_error_history_limit: 100 },
  pipeline: {},
  database: { host: 'database.internal', name: 'rules' },
} as unknown as SystemConfig;

test('defaults to embedded NATS with a random port and preserves unknown config sections', () => {
  const onChange = jest.fn();
  render(<ConfigForm config={baseConfig} onChange={onChange} />);

  expect(screen.getByDisplayValue('internal')).toBeInTheDocument();
  expect(screen.getByDisplayValue('0')).toBeInTheDocument();

  fireEvent.change(screen.getByDisplayValue('internal'), { target: { value: 'external' } });

  const updated = onChange.mock.calls[0][0] as SystemConfig & { database: { host: string } };
  expect(updated.common.nats.mode).toBe('external');
  expect(updated.database.host).toBe('database.internal');
});

test('shows the Manager API port as a read-only bootstrap setting', () => {
  render(<ConfigForm config={baseConfig} onChange={jest.fn()} />);

  fireEvent.click(screen.getByText(/服务组件/));

  expect(screen.getByDisplayValue('11011')).toBeDisabled();
  expect(screen.getByText(/不能在当前连接中在线修改/)).toBeInTheDocument();
});
