import { create } from 'zustand';
import { TraceMessage } from '../types';

interface TraceStore {
  traces: Map<string, TraceMessage[]>; // rule_id -> traces
  activeTraces: Set<string>; // 当前活跃的 trace_id
  highlightedNodes: Set<string>; // 当前高亮的节点ID
  traceBuffer: TraceMessage[]; // 节流缓冲区

  // Actions
  addTrace: (trace: TraceMessage) => void;
  flushBuffer: () => void; // 批量处理缓冲区
  clearTraces: (ruleId?: string) => void;
  setActiveTrace: (traceId: string | null) => void;
  highlightNode: (nodeId: string) => void;
  clearHighlight: () => void;
  getTracesForRule: (ruleId: string) => TraceMessage[];
  reset: () => void;
}

// 节流定时器引用
let throttleTimer: NodeJS.Timeout | null = null;
const THROTTLE_INTERVAL = 500; // 500ms 批量处理一次

export const useTraceStore = create<TraceStore>((set, get) => ({
  traces: new Map(),
  activeTraces: new Set(),
  highlightedNodes: new Set(),
  traceBuffer: [],

  addTrace: (trace: TraceMessage) => {
    // 将 trace 添加到缓冲区
    set((state) => ({
      traceBuffer: [...state.traceBuffer, trace],
    }));

    // 节流处理：每 500ms 批量处理一次
    if (!throttleTimer) {
      throttleTimer = setTimeout(() => {
        get().flushBuffer();
        throttleTimer = null;
      }, THROTTLE_INTERVAL);
    }
  },

  flushBuffer: () => {
    const buffer = get().traceBuffer;
    if (buffer.length === 0) return;

    set((state) => {
      const traces = new Map(state.traces);
      const activeTraces = new Set(state.activeTraces);
      const highlightedNodes = new Set(state.highlightedNodes);

      // 批量处理缓冲区中的 traces
      buffer.forEach((trace) => {
      const ruleTraces = traces.get(trace.rule_id) || [];
      const updatedTraces = [...ruleTraces, trace].slice(-100);
      traces.set(trace.rule_id, updatedTraces);
        activeTraces.add(trace.trace_id);
        highlightedNodes.add(trace.node_id);

        // 3 秒后清除高亮
        setTimeout(() => {
          set((s) => {
            const highlighted = new Set(s.highlightedNodes);
            highlighted.delete(trace.node_id);
            return { highlightedNodes: highlighted };
          });
        }, 3000);
      });

      return {
        traces,
        activeTraces,
        highlightedNodes,
        traceBuffer: [], // 清空缓冲区
      };
    });
  },

  clearTraces: (ruleId?: string) => {
    set((state) => {
      if (ruleId) {
        const traces = new Map(state.traces);
        traces.delete(ruleId);
        return { traces };
      } else {
        return { traces: new Map() };
      }
    });
  },

  setActiveTrace: (traceId: string | null) => {
    set((state) => {
      if (traceId) {
        return { activeTraces: new Set(Array.from(state.activeTraces).concat([traceId])) };
      } else {
        return { activeTraces: new Set() };
      }
    });
  },

  highlightNode: (nodeId: string) => {
    set((state) => ({
      highlightedNodes: new Set(Array.from(state.highlightedNodes).concat([nodeId])),
    }));
  },

  clearHighlight: () => {
    set({ highlightedNodes: new Set() });
  },

  getTracesForRule: (ruleId: string) => {
    return get().traces.get(ruleId) || [];
  },

  reset: () => {
    set({
      traces: new Map(),
      activeTraces: new Set(),
      highlightedNodes: new Set(),
    });
  },
}));
