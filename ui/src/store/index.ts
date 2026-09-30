/**
 * Store Index - 统一导出所有 Store
 * 当前实现：集中状态管理
 */

// Rule Store
export { useRuleStore } from './ruleStore';

// Plugin Store
export { usePluginStore } from './pluginStore';

// Trace Store
export { useTraceStore } from './traceStore';

// Runtime Store
export {
    useRuntimeStore,
    useRuntimeInstances,
    useCurrentInstance,
    useRuntimeNodes,
    useRuntimeEdges,
    useRuntimeLoading,
    useRuntimeError,
} from './runtimeStore';

// Config Store
export {
    useConfigStore,
    useConfig,
    useConfigLoading,
    useConfigError,
    useConfigSaving,
    useHasUnsavedChanges,
} from './configStore';
export type { SystemConfig } from './configStore';

// Types from types/index.ts (re-export for convenience)
export type {
    RuleInstance,
    NodeInstance,
    EdgeInstance,
    TraceEvent,
    NodeMetrics,
    EdgeMetrics,
} from '../types';
