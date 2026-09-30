import { create } from 'zustand';
import { PluginMeta, PluginInfo } from '../types';
import { pluginsAPI } from '../services/api';

const normalizePluginMeta = (plugin: PluginMeta): PluginMeta => {
  const pluginInfo = plugin.info;
  const normalizedId = plugin.id || pluginInfo?.id || plugin.name;
  const normalizedName = plugin.name || pluginInfo?.name || normalizedId;

  return {
    ...plugin,
    id: normalizedId,
    name: normalizedName,
  };
};

const getPluginIdentity = (plugin: PluginMeta): string => plugin.id || plugin.info?.id || plugin.name;

interface PluginStore {
  plugins: PluginMeta[];
  pluginInfos: PluginInfo[]; // 使用 PluginInfo
  loading: boolean;
  error: string | null;

  // Actions
  loadPlugins: () => Promise<void>;
  loadPluginInfos: () => Promise<void>; // 加载 PluginInfo 列表
  enablePlugin: (id: string) => Promise<void>;
  disablePlugin: (id: string) => Promise<void>;
  uploadPlugin: (file: File, descriptor?: File) => Promise<void>;
  deletePlugin: (id: string) => Promise<void>;
  upgradePlugin: (id: string, file: File) => Promise<void>;
  reset: () => void;
}

export const usePluginStore = create<PluginStore>((set, get) => ({
  plugins: [],
  pluginInfos: [], // PluginInfo 列表
  loading: false,
  error: null,

  loadPlugins: async () => {
    set({ loading: true, error: null });
    try {
      const plugins = (await pluginsAPI.list()).map(normalizePluginMeta);
      set({ plugins, loading: false });
    } catch (error: any) {
      set({ error: error.message || 'Failed to load plugins', loading: false });
    }
  },

  loadPluginInfos: async () => {
    set({ loading: true, error: null });
    try {
      const pluginInfos = await pluginsAPI.listPluginInfos();
      console.log('[PluginStore] Loaded plugin infos:', pluginInfos.length, pluginInfos);
      set({ pluginInfos, loading: false });
    } catch (error: any) {
      console.error('[PluginStore] Failed to load plugin infos:', error);
      set({ error: error.message || 'Failed to load plugin infos', loading: false, pluginInfos: [] });
    }
  },

  enablePlugin: async (id: string) => {
    set({ loading: true, error: null });
    try {
      const updated = normalizePluginMeta(await pluginsAPI.enable(id));
      set((state) => ({
        plugins: state.plugins.map((p) => (getPluginIdentity(p) === id ? updated : p)),
        loading: false,
      }));
    } catch (error: any) {
      set({ error: error.message || 'Failed to enable plugin', loading: false });
      throw error;
    }
  },

  disablePlugin: async (id: string) => {
    set({ loading: true, error: null });
    try {
      const updated = normalizePluginMeta(await pluginsAPI.disable(id));
      set((state) => ({
        plugins: state.plugins.map((p) => (getPluginIdentity(p) === id ? updated : p)),
        loading: false,
      }));
    } catch (error: any) {
      set({ error: error.message || 'Failed to disable plugin', loading: false });
      throw error;
    }
  },

  uploadPlugin: async (file: File, descriptor?: File) => {
    set({ loading: true, error: null });
    try {
      const newPlugin = normalizePluginMeta(await pluginsAPI.upload(file, descriptor));
      set((state) => ({
        plugins: [...state.plugins, newPlugin],
        loading: false,
      }));
    } catch (error: any) {
      set({ error: error.message || 'Failed to upload plugin', loading: false });
      throw error;
    }
  },

  deletePlugin: async (id: string) => {
    set({ loading: true, error: null });
    try {
      await pluginsAPI.delete(id);
      set((state) => ({
        plugins: state.plugins.filter((p) => getPluginIdentity(p) !== id),
        loading: false,
      }));
    } catch (error: any) {
      set({ error: error.message || 'Failed to delete plugin', loading: false });
      throw error;
    }
  },

  upgradePlugin: async (id: string, file: File) => {
    set({ loading: true, error: null });
    try {
      const updated = normalizePluginMeta(await pluginsAPI.upgrade(id, file));
      set((state) => ({
        plugins: state.plugins.map((p) => (getPluginIdentity(p) === id ? updated : p)),
        loading: false,
      }));
    } catch (error: any) {
      set({ error: error.message || 'Failed to upgrade plugin', loading: false });
      throw error;
    }
  },

  reset: () => {
    set({
      plugins: [],
      pluginInfos: [],
      loading: false,
      error: null,
    });
  },
}));
