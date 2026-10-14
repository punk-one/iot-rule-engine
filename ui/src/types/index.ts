// TypeScript 类型定义（对应 Go SDK 的 schema）

export interface Position {
  x: number;
  y: number;
}

export interface FlowMetadata {
  canvas_position?: Position;
  zoom?: number;
  [key: string]: any;
}

export interface SourceConfig {
  type: string; // nats, http, mqtt
  topic: string;
  data_format?: string; // json, protobuf, avro
  config?: Record<string, any> | null;
}

export interface NodeMetadata {
  position?: Position;
}

export interface NodeConfig {
  id: string;
  type: string; // filter, router, map, window, script, sink
  name: string;
  plugin?: string;
  config: Record<string, any>;
  metadata?: NodeMetadata;
  wires: string[];
}

export interface RuleFlow {
  id: string;
  name: string;
  version: number;
  enabled: boolean;
  debug_mode: boolean;
  description: string;
  lifecycle_status?: 'draft' | 'reviewed' | 'active' | 'archived';
  metadata?: FlowMetadata;
  source?: SourceConfig;
  nodes: NodeConfig[];
  status?: 'deployed' | 'not_deployed' | 'running' | 'error';
}

export interface TraceMessage {
  rule_id: string;
  node_id: string;
  node_type: string;
  phase?: string;
  status: string;
  timestamp: number | string;
  observed_at?: string;
  event_timestamp?: string;
  trace_id: string;
  message_id: string;
  output_batch_size?: number;
  from_node_id?: string;
  payload?: any[]; // 节点输出的数据（Mock Sink 等会包含）
}

// PluginInfo 和 PluginCapabilities（符合 SDK 规范）
export type PluginType = 'source' | 'processor' | 'sink' | 'utility' | 'connector';

export interface ConnectorDescriptor {
  family: string;
  label?: string;
  capabilities?: string[];
  display_fields?: string[];
}

export interface ConnectorBindingSpec {
  required: boolean;
  accepted_families?: string[];
  required_capabilities?: string[];
}

export interface PluginCapabilities {
  input_ports: number;      // 输入端口数量
  output_ports: number;      // 输出端口数量
  support_batch: boolean;    // 是否支持批量处理
  support_ack: boolean;      // 是否支持确认机制
  config_schema: string;     // JSON Schema（字符串）
}

export interface PluginInfo {
  id: string;
  name: string;
  version: string;
  description: string;
  type: PluginType;
  category: string;
  capabilities: PluginCapabilities;
  connector_descriptor?: ConnectorDescriptor;
  connector_binding?: ConnectorBindingSpec;
  plane?: string;
}

// 旧格式（已废弃，保留用于兼容）
export interface PluginDescriptor {
  name: string;
  type: string; // sink, source, function
  role?: string; // source, processor, sink, control
  version: string;
  description: string;
  inputs: InputDesc[];
}

export interface InputDesc {
  key: string;
  type: string; // text, number, boolean, select, json
  label: string;
  default?: any;
  required: boolean;
  options?: string[];
  hint?: string;
}

// React Flow 节点和边的类型
export interface FlowNode {
  id: string;
  type: string;
  position: Position;
  data: {
    label: string;
    nodeType: string;
    nodeConfig: NodeConfig;
  };
}

export interface FlowEdge {
  id: string;
  source: string;
  target: string;
  type?: string;
}

// 规则摘要（用于列表页）
export interface RuleSummary {
  id: string;
  name: string;
  version: number;
  enabled: boolean;
  debug_mode: boolean;
  description: string;
  updated_at?: string;
  created_at?: string;
  status?: 'deployed' | 'not_deployed' | 'running' | 'error';
  node_count?: number;
}

// 插件元数据（用于插件管理，使用 PluginInfo）
export interface PluginMeta {
  id: string;
  name: string;
  version: string;
  type: string; // sink, source, processor
  description: string;
  enabled: boolean;
  engine_api?: string; // 插件注册的引擎 API 端点
  uploaded_at?: string;
  info?: PluginInfo; // PluginInfo 包含完整的插件能力声明
}

export interface RegistryPluginArtifact {
  plugin: string;
  version: string;
  type: string;
  sdk: string;
  os: string;
  arch: string;
  filename: string;
  sha256: string;
  size: number;
  visibility: 'public' | 'private';
  release: string;
  created_at: string;
}

export interface RegistryCatalogItem {
  plugin: RegistryPluginArtifact;
  download_url: string;
  bundle_url: string;
  sbom_url: string;
}

export type WarmupPolicy = 'lazy' | 'preload' | 'always_on';
export type ResourceStatus =
  | 'unknown'
  | 'healthy'
  | 'degraded'
  | 'congested'
  | 'disconnected'
  | 'readonly'
  | 'blocked'
  | 'recovering';

export interface ResourceStatusEvent {
  resource_id?: string;
  provider_plugin_id?: string;
  provider_instance_id?: string;
  status: ResourceStatus;
  reason_code?: string;
  message?: string;
  details?: Record<string, string>;
  observed_at_unix_nano?: number;
  sequence?: number;
}

export interface ConnectorHealthPolicy {
  enabled?: boolean;
  interval_ms?: number;
  timeout_ms?: number;
  target?: string;
  priority?: string;
}

export interface ConnectorQuotaPolicy {
  max_inflight_requests?: number;
  max_inflight_weight?: number;
  default_usage_weight?: number;
}

export interface ConnectorProviderPolicy {
  max_resources_per_provider?: number;
  max_inflight_per_provider?: number;
  preferred_provider_count?: number;
}

export interface ConnectorResource {
  id: string;
  name?: string;
  plugin_id: string;
  connection?: Record<string, any>;
  session_policy?: string;
  warmup_policy?: WarmupPolicy;
  health_policy?: ConnectorHealthPolicy;
  quota_policy?: ConnectorQuotaPolicy;
  provider_policy?: ConnectorProviderPolicy;
  enabled: boolean;
  metadata?: Record<string, string>;
}

export interface ConnectorCompatibility {
  compatible: boolean;
  reasons?: string[];
}

export interface ConnectorResourceView extends ConnectorResource {
  plugin_info?: PluginInfo;
  status?: ResourceStatusEvent;
  connection_summary?: string;
  connection_preview?: Record<string, string>;
  compatibility?: ConnectorCompatibility;
  ref_count?: number;
  pending?: number;
  inflight_weight?: number;
}

// 节点配置 Schema 定义
export interface NodeConfigSchema {
  type: string;
  displayName: string;
  icon?: string;
  color?: string;
  description?: string;
  configSchema: Record<string, ConfigFieldSchema>;
  validation?: {
    required?: string[];
    custom?: string; // CEL 表达式
  };
}

// 配置字段 Schema
export interface ConfigFieldSchema {
  type: 'string' | 'number' | 'boolean' | 'enum' | 'json' | 'cel' | 'text';
  label: string;
  required?: boolean;
  default?: any;
  options?: string[]; // 用于 enum 类型
  hint?: string;
  editor?: 'text' | 'code' | 'json'; // 编辑器类型
  min?: number; // 用于 number 类型
  max?: number; // 用于 number 类型
}

// 规则验证结果
export interface RuleValidationResult {
  valid: boolean;
  errors: ValidationError[];
  warnings: ValidationWarning[];
}

export interface ValidationError {
  node_id?: string;
  field?: string;
  message: string;
  code: string;
}

export interface ValidationWarning {
  node_id?: string;
  field?: string;
  message: string;
}

// Runtime Model Types
export type RuleStatus = 'created' | 'running' | 'paused' | 'failed' | 'stopped';
export type NodeStatus = 'init' | 'running' | 'idle' | 'error' | 'stopped';

export interface RuleMetrics {
  events_in: number;
  events_out: number;
  events_failed: number;
  last_event_at?: string;
}

export interface NodeMetrics {
  events_in: number;
  events_out: number;
  events_failed: number;
  avg_latency_ms: number;
  last_event_at?: string;
}

export interface EdgeMetrics {
  events_published: number;
  events_consumed: number;
  events_dropped: number;
  last_event_at?: string;
}

export interface NodeError {
  time: string;
  message: string;
  event_id?: string;
}

export interface NodeInstance {
  id: string; // runtime_node_id
  node_id: string; // canvas node id
  plugin_id: string;
  version: string;
  config?: Record<string, any>;
  status: NodeStatus;
  started_at: string;
  input_edges: string[];
  output_edges: string[];
  metrics: NodeMetrics;
  last_error?: NodeError;
  error_history?: NodeError[];
}

export interface EdgeInstance {
  id: string; // runtime_edge_id
  edge_id: string; // canvas edge id
  from_node: string;
  to_node: string;
  topic: string; // NATS Topic
  metrics: EdgeMetrics;
}

export interface RuleInstance {
  id: string; // runtime_rule_id
  rule_id: string; // 配置态 rule_id
  name?: string; // 规则名称（从 RuleDefinition 获取）
  version: string;
  debug_mode: boolean;
  status: RuleStatus;
  created_at: string;
  started_at: string;
  stopped_at?: string;
  nodes: Record<string, NodeInstance>;
  edges: Record<string, EdgeInstance>;
  metrics: RuleMetrics;
}

export interface TraceEvent {
  instance_id?: string;
  trace_id: string;
  rule_id: string;
  node_id: string; // canvas node id
  event_id: string;
  phase?: string;
  status: string; // success, failed, processing
  message?: string;
  payload?: Record<string, any>;
  timestamp: string;
  observed_at?: string;
  event_timestamp?: string;
}

export interface TraceEventsResponse {
  debug_enabled: boolean;
  events: TraceEvent[];
  count?: number;
  message?: string;
}

// Snapshot, State Machine, Gray Release types
export type LifecycleStatus = 'draft' | 'reviewed' | 'active' | 'archived';

export interface RuleSnapshot {
  id: string;
  rule_id: string;
  version: number;
  rule_flow: RuleFlow;
  created_at: string;
  created_by?: string;
  description?: string;
}

export interface GrayReleaseConfig {
  rule_id: string;
  version: number;
  enabled: boolean;
  strategy: 'percentage' | 'condition';
  percentage?: number; // 0-100, only when strategy=percentage
  condition?: Record<string, string>; // only when strategy=condition
  target_nodes: string[];
  created_at: string;
  updated_at: string;
}
