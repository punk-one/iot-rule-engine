import axios from 'axios';
import {
  RuleFlow,
  PluginInfo,
  RuleSummary,
  PluginMeta,
  RuleValidationResult,
  RuleInstance,
  NodeInstance,
  EdgeInstance,
  TraceEventsResponse,
  RuleSnapshot,
  GrayReleaseConfig,
  LifecycleStatus,
  ConnectorResource,
  ConnectorResourceView,
} from '../types';

const DEFAULT_API_BASE_URL =
  process.env.NODE_ENV === 'production'
    ? '/api/v1'
    : 'http://localhost:11011/api/v1';

const API_BASE_URL = process.env.REACT_APP_API_URL || DEFAULT_API_BASE_URL;
let authToken: string | null = null;

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 15000,
  headers: {
    'Content-Type': 'application/json',
  },
});

export const setAPIToken = (token: string | null) => {
  authToken = token;
  if (token) {
    api.defaults.headers.common.Authorization = `Bearer ${token}`;
    return;
  }
  delete api.defaults.headers.common.Authorization;
};

api.interceptors.request.use((config) => {
  if (authToken) {
    config.headers = config.headers || {};
    config.headers.Authorization = `Bearer ${authToken}`;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  (error) => {
    const status = error?.response?.status;
    const requestUrl = error?.config?.url || '';
    const message =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '请求失败';

    if (
      typeof window !== 'undefined' &&
      status === 401 &&
      authToken &&
      !requestUrl.includes('/auth/login')
    ) {
      window.dispatchEvent(new Event('auth:unauthorized'));
    }

    if (error && typeof error === 'object') {
      error.message = message;
      if (status && error.status == null) {
        error.status = status;
      }
      return Promise.reject(error);
    }

    const wrappedError = Object.assign(new Error(message), {
      status,
      response: error?.response,
      config: error?.config,
      request: error?.request,
      code: error?.code,
    });

    return Promise.reject(wrappedError);
  }
);

export interface AuthUser {
  id: number;
  username: string;
  phone?: string;
  email?: string;
  status: number;
  remark?: string;
  last_login_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface LoginResponse {
  token: string;
  expires_at: string;
  user: AuthUser;
}

export interface CreateUserPayload {
  username: string;
  password: string;
  phone?: string;
  email?: string;
  status: number;
  remark?: string;
}

export const authAPI = {
  login: async (username: string, password: string): Promise<LoginResponse> => {
    const response = await api.post('/auth/login', { username, password });
    return response.data;
  },

  logout: async (): Promise<{ message: string }> => {
    const response = await api.post('/auth/logout');
    return response.data;
  },

  me: async (): Promise<{ user: AuthUser }> => {
    const response = await api.get('/auth/me');
    return response.data;
  },

  changePassword: async (oldPassword: string, newPassword: string): Promise<{ message: string }> => {
    const response = await api.put('/auth/password', {
      old_password: oldPassword,
      new_password: newPassword,
    });
    return response.data;
  },
};

export const usersAPI = {
  list: async (): Promise<AuthUser[]> => {
    const response = await api.get('/users');
    return response.data.users || [];
  },

  create: async (payload: CreateUserPayload): Promise<AuthUser> => {
    const response = await api.post('/users', payload);
    return response.data.user;
  },

  updateStatus: async (id: number, status: number): Promise<{ message: string }> => {
    const response = await api.put(`/users/${id}/status`, { status });
    return response.data;
  },

  resetPassword: async (id: number, newPassword: string): Promise<{ message: string }> => {
    const response = await api.put(`/users/${id}/password`, { new_password: newPassword });
    return response.data;
  },
};

// 规则 API
export const rulesAPI = {
  // 获取所有规则（返回摘要列表）
  list: async (): Promise<RuleSummary[]> => {
    const response = await api.get('/rules');
    return response.data.rules || [];
  },

  // 获取所有规则详情（完整数据）
  listFull: async (): Promise<RuleFlow[]> => {
    const response = await api.get('/rules?full=true');
    return response.data.rules || [];
  },

  // 获取单个规则
  get: async (id: string): Promise<RuleFlow> => {
    const response = await api.get(`/rules/${id}`);
    return response.data;
  },

  // 创建规则
  create: async (rule: RuleFlow): Promise<RuleFlow> => {
    const response = await api.post('/rules', rule);
    return response.data;
  },

  // 更新规则
  update: async (id: string, rule: RuleFlow): Promise<RuleFlow> => {
    const response = await api.put(`/rules/${id}`, rule);
    return response.data;
  },

  // 删除规则
  delete: async (id: string): Promise<void> => {
    await api.delete(`/rules/${id}`);
  },

  // 部署规则
  deploy: async (id: string): Promise<RuleFlow> => {
    const response = await api.post(`/rules/${id}/deploy`);
    return response.data;
  },

  // 取消部署规则
  undeploy: async (id: string): Promise<RuleFlow> => {
    const response = await api.post(`/rules/${id}/disable`);
    return response.data;
  },

  // DryRun 模拟运行
  dryRun: async (rule: RuleFlow, sampleData: Record<string, any>[]): Promise<any> => {
    const response = await api.post('/rules/dry-run', {
      rule_flow: rule,
      sample_data: sampleData,
    });
    return response.data;
  },

  // 启用规则
  enable: async (id: string): Promise<RuleFlow> => {
    const response = await api.post(`/rules/${id}/enable`);
    return response.data;
  },

  // 禁用规则
  disable: async (id: string): Promise<RuleFlow> => {
    const response = await api.post(`/rules/${id}/disable`);
    return response.data;
  },

  // 验证规则
  validate: async (rule: RuleFlow): Promise<RuleValidationResult> => {
    const response = await api.post('/rules/validate', rule);
    return response.data;
  },
};

// 插件 API
export const pluginsAPI = {
  // 获取所有插件（元数据列表）
  list: async (): Promise<PluginMeta[]> => {
    const response = await api.get('/plugins');
    return response.data.plugins || [];
  },

  // 获取所有插件信息（返回 PluginInfo 列表）
  listPluginInfos: async (): Promise<PluginInfo[]> => {
    const response = await api.get('/plugins?descriptors=true');
    return response.data.plugins || [];
  },

  listCompatibleConnectors: async (pluginId: string): Promise<ConnectorResourceView[]> => {
    const response = await api.get(`/plugins/${pluginId}/compatible-connectors`);
    return response.data.connectors || [];
  },

  // 获取单个插件
  get: async (name: string): Promise<PluginMeta> => {
    const response = await api.get(`/plugins/${name}`);
    return response.data;
  },

  // 上传插件
  upload: async (file: File, descriptor?: File): Promise<PluginMeta> => {
    const formData = new FormData();
    formData.append('plugin', file);
    if (descriptor) {
      formData.append('descriptor', descriptor);
    }
    const response = await api.post('/plugins/upload', formData);
    return response.data;
  },

  // 删除插件
  delete: async (name: string): Promise<void> => {
    await api.delete(`/plugins/${name}`);
  },

  // 启用插件
  enable: async (name: string): Promise<PluginMeta> => {
    const response = await api.post(`/plugins/${name}/enable`);
    return response.data;
  },

  // 禁用插件
  disable: async (name: string): Promise<PluginMeta> => {
    const response = await api.post(`/plugins/${name}/disable`);
    return response.data;
  },

  // 升级插件
  upgrade: async (name: string, file: File): Promise<PluginMeta> => {
    const formData = new FormData();
    formData.append('plugin', file);
    const response = await api.post(`/plugins/${name}/upgrade`, formData);
    return response.data;
  },
};

export const connectorsAPI = {
  list: async (): Promise<ConnectorResourceView[]> => {
    const response = await api.get('/connectors');
    return response.data.connectors || [];
  },

  get: async (id: string): Promise<ConnectorResourceView> => {
    const response = await api.get(`/connectors/${id}`);
    return response.data;
  },

  create: async (resource: ConnectorResource): Promise<ConnectorResourceView> => {
    const response = await api.post('/connectors', resource);
    return response.data;
  },

  update: async (id: string, resource: ConnectorResource): Promise<ConnectorResourceView> => {
    const response = await api.put(`/connectors/${id}`, resource);
    return response.data;
  },

  delete: async (id: string): Promise<void> => {
    await api.delete(`/connectors/${id}`);
  },

  listCompatible: async (pluginId: string): Promise<ConnectorResourceView[]> => {
    return pluginsAPI.listCompatibleConnectors(pluginId);
  },
};

// 配置 API
export const configAPI = {
  // 获取系统配置
  get: async (): Promise<{ config: SystemConfig; source: string }> => {
    const response = await api.get('/config');
    return response.data;
  },

  // 更新系统配置
  update: async (config: SystemConfig): Promise<{ message: string; source: string }> => {
    const response = await api.put('/config', { config });
    return response.data;
  },

  // 上传证书文件
  uploadCert: async (file: File): Promise<{ path: string; message: string }> => {
    const formData = new FormData();
    formData.append('file', file);
    const response = await api.post('/config/upload', formData);
    return response.data;
  },
};

// 系统配置类型定义
export interface SystemConfig {
  common: {
    nats: {
      mode: 'internal' | 'external';
      host: string;
      port: number;
      store_dir: string;
      url: string;
      auth_mode: string;
      max_reconnects: number;
      reconnect_wait_ms: number;
      username: string;
      password: string;
      token: string;
      cert_file: string;
      key_file: string;
      ca_file: string;
      creds_file: string;
    };
  };
  security?: {
    plugin: {
      max_emit_qps: number;
      rpc_timeout_ms: number;
      max_payload_kb: number;
    };
    dispatcher: {
      max_queue: number;
      rule_qps: number;
      node_qps: number;
      deduplication_window_ms: number;
    };
    manager: {
      auth_enabled: boolean;
      allow_insecure: boolean;
      allowed_origins: string[];
      allow_runtime_plugin_install: boolean;
      trusted_plugin_manifest: string;
      max_plugin_upload_mb: number;
    };
  };
  manager: {
    port: number;
    db_path: string;
  };
  executor: {
    plugins_dir: string;
    worker_count: number;
    node_error_history_limit: number;
  };
  pipeline: {
    db_path?: string;
  };
}

// 健康检查
export const healthAPI = {
  check: async (): Promise<{ status: string }> => {
    const response = await api.get('/health');
    return response.data;
  },
};

// Runtime API（运行态观测）
export const runtimeAPI = {
  // 列出所有规则运行实例（Rule Overview 页面）
  listInstances: async (): Promise<RuleInstance[]> => {
    const response = await api.get('/runtime/instances');
    const instances = response.data.instances || [];
    return [...instances].sort((a, b) => {
      const left = (a.name || a.rule_id || '').trim();
      const right = (b.name || b.rule_id || '').trim();
      const byName = left.localeCompare(right, 'zh-Hans-CN', { sensitivity: 'base', numeric: true });
      if (byName !== 0) {
        return byName;
      }
      return (a.rule_id || '').localeCompare(b.rule_id || '', 'zh-Hans-CN', { sensitivity: 'base', numeric: true });
    });
  },

  // 获取单个规则运行实例
  getInstance: async (instanceID: string): Promise<RuleInstance> => {
    const response = await api.get(`/runtime/instances/${instanceID}`);
    return response.data;
  },

  // 根据规则ID获取运行实例
  getInstanceByRuleID: async (ruleID: string): Promise<RuleInstance> => {
    const response = await api.get(`/runtime/rules/${ruleID}/instance`);
    return response.data;
  },

  // 列出规则的所有节点实例
  listNodeInstances: async (instanceID: string): Promise<NodeInstance[]> => {
    const response = await api.get(`/runtime/instances/${instanceID}/nodes`);
    return response.data.nodes || [];
  },

  // 获取节点运行实例（Node Inspector 页面）
  getNodeInstance: async (instanceID: string, nodeID: string): Promise<NodeInstance> => {
    const response = await api.get(`/runtime/instances/${instanceID}/nodes/${nodeID}`);
    return response.data;
  },

  // 列出规则的所有边实例
  listEdgeInstances: async (instanceID: string): Promise<EdgeInstance[]> => {
    const response = await api.get(`/runtime/instances/${instanceID}/edges`);
    return response.data.edges || [];
  },

  // 获取边运行实例（Edge Monitor 页面）
  getEdgeInstance: async (instanceID: string, edgeID: string): Promise<EdgeInstance> => {
    const response = await api.get(`/runtime/instances/${instanceID}/edges/${edgeID}`);
    return response.data;
  },

  // 获取 Trace 事件列表
  getTraceEvents: async (instanceID: string, nodeID?: string, limit?: number): Promise<TraceEventsResponse> => {
    const params: any = {};
    if (nodeID) params.node_id = nodeID;
    if (limit) params.limit = limit;
    const response = await api.get(`/runtime/instances/${instanceID}/trace`, { params });
    return {
      debug_enabled: Boolean(response.data.debug_enabled),
      events: response.data.events || [],
      count: response.data.count,
      message: response.data.message,
    };
  },
};

// Snapshot API
export const snapshotsAPI = {
  // 列出规则的所有快照
  list: async (ruleId: string): Promise<RuleSnapshot[]> => {
    const response = await api.get(`/rules/${ruleId}/snapshots`);
    return response.data.snapshots || [];
  },

  // 获取指定版本的快照
  get: async (ruleId: string, version: number): Promise<RuleSnapshot> => {
    const response = await api.get(`/rules/${ruleId}/snapshots/${version}`);
    return response.data;
  },

  // 获取最新快照
  getLatest: async (ruleId: string): Promise<RuleSnapshot> => {
    const response = await api.get(`/rules/${ruleId}/snapshots/latest`);
    return response.data;
  },

  // 创建快照
  create: async (ruleId: string): Promise<RuleSnapshot> => {
    const response = await api.post(`/rules/${ruleId}/snapshots`);
    return response.data;
  },
};

// State Machine API
export const stateMachineAPI = {
  // 执行状态转换
  transition: async (ruleId: string, toStatus: LifecycleStatus): Promise<{ rule_id: string; from_status: string; to_status: string; lifecycle_status: string }> => {
    const response = await api.post(`/rules/${ruleId}/transition`, { to_status: toStatus });
    return response.data;
  },

  // 获取允许的状态转换
  getAllowedTransitions: async (ruleId: string): Promise<{ rule_id: string; current_status: string; allowed_transitions: string[] }> => {
    const response = await api.get(`/rules/${ruleId}/transitions`);
    return response.data;
  },
};

// Gray Release API
export const grayReleaseAPI = {
  // 获取灰度发布配置
  get: async (ruleId: string): Promise<GrayReleaseConfig> => {
    const response = await api.get(`/rules/${ruleId}/gray-release`);
    return response.data;
  },

  // 创建灰度发布配置
  create: async (ruleId: string, config: Omit<GrayReleaseConfig, 'rule_id' | 'created_at' | 'updated_at'>): Promise<GrayReleaseConfig> => {
    const response = await api.post(`/rules/${ruleId}/gray-release`, config);
    return response.data;
  },

  // 更新灰度发布配置
  update: async (ruleId: string, config: Omit<GrayReleaseConfig, 'rule_id' | 'created_at' | 'updated_at'>): Promise<GrayReleaseConfig> => {
    const response = await api.put(`/rules/${ruleId}/gray-release`, config);
    return response.data;
  },

  // 删除灰度发布配置
  delete: async (ruleId: string): Promise<void> => {
    await api.delete(`/rules/${ruleId}/gray-release`);
  },
};

export default api;
