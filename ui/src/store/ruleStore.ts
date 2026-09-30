import { create } from 'zustand';
import { temporal } from 'zundo';
import { RuleFlow, RuleSummary } from '../types';
import { rulesAPI } from '../services/api';
import { hydrateRule, dehydrateRule } from '../utils/ruleUtils';

interface RuleStore {
  rules: RuleSummary[]; // 列表使用摘要
  currentRule: RuleFlow | null; // 当前编辑的完整规则
  loading: boolean;
  error: string | null;

  // Actions
  fetchRules: () => Promise<void>;
  fetchRule: (id: string) => Promise<void>;
  createRule: (rule: RuleFlow) => Promise<void>;
  updateRule: (id: string, rule: RuleFlow) => Promise<void>;
  deleteRule: (id: string) => Promise<void>;
  deployRule: (id: string) => Promise<void>;
  undeployRule: (id: string) => Promise<void>;
  setCurrentRule: (rule: RuleFlow | null) => void;
  updateCurrentRule: (updates: Partial<RuleFlow>) => void;
  reset: () => void;
}

let fetchRuleSequence = 0;

export const useRuleStore = create<RuleStore>()(
  temporal(
    (set, get) => ({
  rules: [],
  currentRule: null,
  loading: false,
  error: null,

  fetchRules: async () => {
    set({ loading: true, error: null });
    try {
      const rules = await rulesAPI.list();
      set({ rules, loading: false });
    } catch (error: any) {
      set({ error: error.message || 'Failed to fetch rules', loading: false });
    }
  },

  fetchRule: async (id: string) => {
    const sequence = ++fetchRuleSequence;
    set({ loading: true, error: null, currentRule: null });
    try {
      const rule = await rulesAPI.get(id);
      // Hydrate the rule to convert source config to source node
      const hydratedRule = hydrateRule(rule);
      if (sequence === fetchRuleSequence) {
        set({ currentRule: hydratedRule, loading: false });
      }
    } catch (error: any) {
      if (sequence === fetchRuleSequence) {
        set({ currentRule: null, error: error.message || 'Failed to fetch rule', loading: false });
      }
    }
  },

  createRule: async (rule: RuleFlow) => {
    set({ loading: true, error: null });
    try {
      // 保存当前规则中的节点位置信息（metadata.position）
      const currentState = get();
      const currentRuleWithPositions = currentState.currentRule;

      // 确保保存的规则包含所有节点位置信息
      const ruleToSave: RuleFlow = {
        ...rule,
        nodes: rule.nodes.map((node) => {
          // 如果当前规则中有这个节点，保留其位置信息
          const existingNode = currentRuleWithPositions?.nodes.find((n) => n.id === node.id);
          if (existingNode?.metadata?.position) {
            return {
              ...node,
              metadata: {
                ...node.metadata,
                position: existingNode.metadata.position,
              },
            };
          }
          // 如果节点没有位置信息，保留原有的 metadata 结构
          return {
            ...node,
            metadata: node.metadata || {},
          };
        }),
        // 确保 metadata 也被保存（包括 source_wires 和 source_position）
        metadata: {
          ...(rule.metadata || {}),
          ...(currentRuleWithPositions?.metadata || {}),
        },
      };

      // Dehydrate the rule to remove the visual source node before sending to backend
      const dehydratedRule = dehydrateRule(ruleToSave);
      const newRule = await rulesAPI.create(dehydratedRule);

      // 合并后端返回的规则和前端的位置信息
      // 因为后端可能没有返回完整的位置信息，我们需要保留前端的
      const mergedRule: RuleFlow = {
        ...newRule,
        nodes: newRule.nodes.map((node) => {
          const existingNode = currentRuleWithPositions?.nodes.find((n) => n.id === node.id);
          if (existingNode?.metadata?.position) {
            return {
              ...node,
              metadata: {
                ...node.metadata,
                position: existingNode.metadata.position,
              },
            };
          }
          return node;
        }),
        // 合并 metadata，确保 source_wires 和 source_position 被保留
        metadata: {
          ...newRule.metadata,
          ...(ruleToSave.metadata || {}),
        },
      };

      // 创建后重新加载列表
      const rules = await rulesAPI.list();

      // Hydrate the merged rule to convert source config back to source node
      const hydratedMergedRule = hydrateRule(mergedRule);

      set({
        rules,
        currentRule: hydratedMergedRule,
        loading: false,
      });
    } catch (error: any) {
      set({ error: error.message || 'Failed to create rule', loading: false });
      throw error;
    }
  },

  updateRule: async (id: string, rule: RuleFlow) => {
    set({ loading: true, error: null });
    try {
      // 保存当前规则中的节点位置信息（metadata.position）
      const currentState = get();
      const currentRuleWithPositions = currentState.currentRule;

      // 确保保存的规则包含所有节点位置信息
      const ruleToSave: RuleFlow = {
        ...rule,
        nodes: rule.nodes.map((node) => {
          // 如果当前规则中有这个节点，保留其位置信息
          const existingNode = currentRuleWithPositions?.nodes.find((n) => n.id === node.id);
          if (existingNode?.metadata?.position) {
            return {
              ...node,
              metadata: {
                ...node.metadata,
                position: existingNode.metadata.position,
              },
            };
          }
          // 如果节点没有位置信息，保留原有的 metadata 结构
          return {
            ...node,
            metadata: node.metadata || {},
          };
        }),
        // 确保 metadata 也被保存（包括 source_wires 和 source_position）
        metadata: {
          ...(rule.metadata || {}),
          ...(currentRuleWithPositions?.metadata || {}),
        },
      };

      // Dehydrate the rule to remove the visual source node before sending to backend
      const dehydratedRule = dehydrateRule(ruleToSave);
      const updatedRule = await rulesAPI.update(id, dehydratedRule);

      // 合并后端返回的规则和前端的位置信息
      // 因为后端可能没有返回完整的位置信息，我们需要保留前端的
      const mergedRule: RuleFlow = {
        ...updatedRule,
        nodes: updatedRule.nodes.map((node) => {
          const existingNode = currentRuleWithPositions?.nodes.find((n) => n.id === node.id);
          if (existingNode?.metadata?.position) {
            return {
              ...node,
              metadata: {
                ...node.metadata,
                position: existingNode.metadata.position,
              },
            };
          }
          return node;
        }),
        // 合并 metadata，确保 source_wires 和 source_position 被保留
        metadata: {
          ...updatedRule.metadata,
          ...(ruleToSave.metadata || {}),
        },
      };

      // 更新后重新加载列表
      const rules = await rulesAPI.list();

      // Hydrate the merged rule to convert source config back to source node
      const hydratedMergedRule = hydrateRule(mergedRule);

      set({
        rules,
        currentRule: currentState.currentRule?.id === id ? hydratedMergedRule : currentState.currentRule,
        loading: false,
      });
    } catch (error: any) {
      set({ error: error.message || 'Failed to update rule', loading: false });
      throw error;
    }
  },

  deleteRule: async (id: string) => {
    set({ loading: true, error: null });
    try {
      await rulesAPI.delete(id);
      set((state) => ({
        rules: state.rules.filter((r) => r.id !== id),
        currentRule: state.currentRule?.id === id ? null : state.currentRule,
        loading: false,
      }));
    } catch (error: any) {
      set({ error: error.message || 'Failed to delete rule', loading: false });
      throw error;
    }
  },

  deployRule: async (id: string) => {
    set({ loading: true, error: null });
    try {
      const deployedRule = await rulesAPI.deploy(id);
      // 部署后重新加载列表
      const rules = await rulesAPI.list();
      const currentState = get();

      // Hydrate the deployed rule to convert source config to source node
      const hydratedDeployedRule = hydrateRule(deployedRule);

      set({
        rules,
        currentRule: currentState.currentRule?.id === id ? hydratedDeployedRule : currentState.currentRule,
        loading: false,
      });
    } catch (error: any) {
      set({ error: error.message || 'Failed to deploy rule', loading: false });
      throw error;
    }
  },

  undeployRule: async (id: string) => {
    set({ loading: true, error: null });
    try {
      const undeployedRule = await rulesAPI.undeploy(id);
      // 取消部署后重新加载列表
      const rules = await rulesAPI.list();
      const currentState = get();

      // Hydrate the undeployed rule to convert source config to source node
      const hydratedUndeployedRule = hydrateRule(undeployedRule);

      set({
        rules,
        currentRule: currentState.currentRule?.id === id ? hydratedUndeployedRule : currentState.currentRule,
        loading: false,
      });
    } catch (error: any) {
      set({ error: error.message || 'Failed to undeploy rule', loading: false });
      throw error;
    }
  },

  setCurrentRule: (rule: RuleFlow | null) => {
    set({ currentRule: rule });
  },

  updateCurrentRule: (updates: Partial<RuleFlow>) => {
    set((state) => ({
      currentRule: state.currentRule
        ? { ...state.currentRule, ...updates }
        : null,
    }));
  },

  reset: () => {
    fetchRuleSequence += 1;
    set({
      rules: [],
      currentRule: null,
      loading: false,
      error: null,
    });
  },
    }),
    {
      limit: 50, // 最多保存 50 个历史状态
      partialize: (state) => ({
        currentRule: state.currentRule,
      }), // 只保存 currentRule 的历史
    }
  )
);
