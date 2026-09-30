/**
 * Runtime Store - 运行时状态管理
 * 当前实现：集中管理运行时状态
 */
import { create } from 'zustand';
import { runtimeAPI } from '../services/api';
import {
    RuleInstance,
    NodeInstance,
    EdgeInstance,
    TraceEvent,
} from '../types';

// Runtime Store 接口
interface RuntimeStore {
    // 状态
    instances: RuleInstance[];
    currentInstance: RuleInstance | null;
    nodes: NodeInstance[];
    edges: EdgeInstance[];
    traceEvents: TraceEvent[];
    loading: boolean;
    error: string | null;

    // 轮询状态
    pollingInterval: number | null;

    // Actions
    fetchInstances: () => Promise<void>;
    fetchInstance: (instanceId: string) => Promise<void>;
    fetchInstanceByRuleId: (ruleId: string) => Promise<void>;
    fetchNodes: (instanceId: string) => Promise<void>;
    fetchEdges: (instanceId: string) => Promise<void>;
    fetchTraceEvents: (instanceId: string, limit?: number) => Promise<void>;

    // 完整的实例数据获取
    fetchFullInstanceData: (instanceId: string) => Promise<void>;

    // 轮询控制
    startPolling: (instanceId: string, intervalMs?: number) => void;
    stopPolling: () => void;

    // 清理
    clearCurrentInstance: () => void;
    reset: () => void;
}

export const useRuntimeStore = create<RuntimeStore>((set, get) => ({
    instances: [],
    currentInstance: null,
    nodes: [],
    edges: [],
    traceEvents: [],
    loading: false,
    error: null,
    pollingInterval: null,

    fetchInstances: async () => {
        set({ loading: true, error: null });
        try {
            const instances = await runtimeAPI.listInstances();
            set({ instances, loading: false });
        } catch (error: any) {
            set({ error: error.message || 'Failed to fetch instances', loading: false });
        }
    },

    fetchInstance: async (instanceId: string) => {
        set({ loading: true, error: null });
        try {
            const instance = await runtimeAPI.getInstance(instanceId);
            set({ currentInstance: instance, loading: false });
        } catch (error: any) {
            set({ error: error.message || 'Failed to fetch instance', loading: false });
        }
    },

    fetchInstanceByRuleId: async (ruleId: string) => {
        set({ loading: true, error: null });
        try {
            const instance = await runtimeAPI.getInstanceByRuleID(ruleId);
            set({ currentInstance: instance, loading: false });
        } catch (error: any) {
            set({ error: error.message || 'Failed to fetch instance', loading: false });
        }
    },

    fetchNodes: async (instanceId: string) => {
        try {
            const nodes = await runtimeAPI.listNodeInstances(instanceId);
            set({ nodes });
        } catch (error: any) {
            console.error('Failed to fetch nodes:', error);
        }
    },

    fetchEdges: async (instanceId: string) => {
        try {
            const edges = await runtimeAPI.listEdgeInstances(instanceId);
            set({ edges });
        } catch (error: any) {
            console.error('Failed to fetch edges:', error);
        }
    },

    fetchTraceEvents: async (instanceId: string, limit = 100) => {
        try {
            const response = await runtimeAPI.getTraceEvents(instanceId, undefined, limit);
            set({ traceEvents: response.events });
        } catch (error: any) {
            console.error('Failed to fetch trace events:', error);
        }
    },

    fetchFullInstanceData: async (instanceId: string) => {
        set({ loading: true, error: null });
        try {
            // 并行获取所有数据
            const [instance, nodes, edges, traceResponse] = await Promise.all([
                runtimeAPI.getInstance(instanceId),
                runtimeAPI.listNodeInstances(instanceId),
                runtimeAPI.listEdgeInstances(instanceId),
                runtimeAPI.getTraceEvents(instanceId, undefined, 100),
            ]);

            set({
                currentInstance: instance,
                nodes,
                edges,
                traceEvents: traceResponse.events,
                loading: false,
            });
        } catch (error: any) {
            set({ error: error.message || 'Failed to fetch instance data', loading: false });
        }
    },

    startPolling: (instanceId: string, intervalMs = 5000) => {
        const { stopPolling, fetchFullInstanceData } = get();

        // 先停止现有轮询
        stopPolling();

        // 立即获取一次数据
        fetchFullInstanceData(instanceId);

        // 设置定时轮询
        const interval = window.setInterval(() => {
            fetchFullInstanceData(instanceId);
        }, intervalMs);

        set({ pollingInterval: interval });
    },

    stopPolling: () => {
        const { pollingInterval } = get();
        if (pollingInterval !== null) {
            window.clearInterval(pollingInterval);
            set({ pollingInterval: null });
        }
    },

    clearCurrentInstance: () => {
        set({
            currentInstance: null,
            nodes: [],
            edges: [],
            traceEvents: [],
        });
    },

    reset: () => {
        const { stopPolling } = get();
        stopPolling();
        set({
            instances: [],
            currentInstance: null,
            nodes: [],
            edges: [],
            traceEvents: [],
            loading: false,
            error: null,
        });
    },
}));

// 选择器 hooks（优化性能）
export const useRuntimeInstances = () => useRuntimeStore((state) => state.instances);
export const useCurrentInstance = () => useRuntimeStore((state) => state.currentInstance);
export const useRuntimeNodes = () => useRuntimeStore((state) => state.nodes);
export const useRuntimeEdges = () => useRuntimeStore((state) => state.edges);
export const useRuntimeLoading = () => useRuntimeStore((state) => state.loading);
export const useRuntimeError = () => useRuntimeStore((state) => state.error);
