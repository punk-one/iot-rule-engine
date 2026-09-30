/**
 * Config Store - 配置状态管理
 * 当前实现：集中管理系统配置
 */
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { configAPI, SystemConfig } from '../services/api';

// 重新导出 SystemConfig
export type { SystemConfig };

// Config Store 接口
interface ConfigStore {
    // 状态
    config: SystemConfig | null;
    configSource: string;
    loading: boolean;
    error: string | null;
    saving: boolean;
    saveError: string | null;

    // 本地配置差异（用于检测未保存的修改）
    localChanges: Partial<SystemConfig> | null;
    hasUnsavedChanges: boolean;

    // Actions
    fetchConfig: () => Promise<void>;
    updateConfig: (config: SystemConfig) => Promise<void>;
    uploadCertificate: (file: File) => Promise<string>;

    // 本地修改
    setLocalChanges: (changes: Partial<SystemConfig>) => void;
    clearLocalChanges: () => void;

    // 重置
    reset: () => void;
}

export const useConfigStore = create<ConfigStore>()(
    persist(
        (set, get) => ({
            config: null,
            configSource: '',
            loading: false,
            error: null,
            saving: false,
            saveError: null,
            localChanges: null,
            hasUnsavedChanges: false,

            fetchConfig: async () => {
                set({ loading: true, error: null });
                try {
                    const result = await configAPI.get();
                    set({
                        config: result.config,
                        configSource: result.source,
                        loading: false
                    });
                } catch (error: any) {
                    set({
                        error: error.message || 'Failed to fetch config',
                        loading: false,
                    });
                }
            },

            updateConfig: async (configUpdates: SystemConfig) => {
                set({ saving: true, saveError: null });
                try {
                    await configAPI.update(configUpdates);
                    set({
                        config: configUpdates,
                        saving: false,
                        localChanges: null,
                        hasUnsavedChanges: false,
                    });
                } catch (error: any) {
                    set({ saveError: error.message || 'Failed to save config', saving: false });
                    throw error;
                }
            },

            uploadCertificate: async (file: File) => {
                try {
                    const result = await configAPI.uploadCert(file);
                    return result.path;
                } catch (error: any) {
                    throw new Error(error.message || 'Failed to upload certificate');
                }
            },

            setLocalChanges: (changes: Partial<SystemConfig>) => {
                set({ localChanges: changes, hasUnsavedChanges: true });
            },

            clearLocalChanges: () => {
                set({ localChanges: null, hasUnsavedChanges: false });
            },

            reset: () => {
                set({
                    config: null,
                    configSource: '',
                    loading: false,
                    error: null,
                    saving: false,
                    saveError: null,
                    localChanges: null,
                    hasUnsavedChanges: false,
                });
            },
        }),
        {
            name: 'iot-rule-config-store',
            partialize: () => ({
                // 不持久化任何配置到 localStorage（避免敏感信息泄露）
            }),
        }
    )
);

// 选择器 hooks
export const useConfig = () => useConfigStore((state) => state.config);
export const useConfigLoading = () => useConfigStore((state) => state.loading);
export const useConfigError = () => useConfigStore((state) => state.error);
export const useConfigSaving = () => useConfigStore((state) => state.saving);
export const useHasUnsavedChanges = () => useConfigStore((state) => state.hasUnsavedChanges);
