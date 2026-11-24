import type {
  AppInfo,
  Config,
  Group,
  HotkeyStatus,
  Item,
  RecentItem,
  SearchHit,
  Settings,
  StartMenuEntry,
  ToastPayload,
} from '../types/config';
import { defaultSettings } from '../types/config';

interface WailsGoApp {
  GetConfig(): Promise<Config>;
  GetHotkeyStatus(): Promise<HotkeyStatus>;
  LaunchItem(groupID: string, itemID: string): Promise<void>;
  AddItem(groupID: string, item: Item): Promise<void>;
  UpdateItem(groupID: string, item: Item): Promise<void>;
  DeleteItem(groupID: string, itemID: string): Promise<void>;
  OpenItemLocation(path: string): Promise<void>;
  AddGroup(name: string): Promise<string>;
  RenameGroup(groupID: string, name: string): Promise<void>;
  DeleteGroup(groupID: string): Promise<void>;
  SaveSettings(settings: Settings): Promise<void>;
  ReloadConfig(): Promise<void>;
  OpenConfigFile(): Promise<void>;
  PickFile(title: string, filterName: string, pattern: string): Promise<string>;
  PickDirectory(title: string): Promise<string>;
  HideWindow(): Promise<void>;
  OnWindowBlur(): Promise<void>;
  SetActiveGroup(groupID: string): Promise<void>;

  AddDroppedFiles(groupID: string, paths: string[]): Promise<void>;
  SearchItems(query: string): Promise<SearchHit[]>;
  GetRecentItems(limit: number): Promise<RecentItem[]>;
  ClearRecentItems(): Promise<void>;
  MoveItem(fromGroupID: string, itemID: string, toGroupID: string, toIndex: number): Promise<void>;
  MoveGroup(groupID: string, toIndex: number): Promise<void>;
  UndoDeleteItem(): Promise<string>;
  UndoDeleteGroup(): Promise<string>;
  GetUndoLabel(): Promise<string>;
  ScanStartMenu(): Promise<StartMenuEntry[]>;
  ImportStartMenuItems(groupID: string, lnkPaths: string[]): Promise<number>;
  GetAppInfo(): Promise<AppInfo>;
  SetAutoStart(enabled: boolean): Promise<void>;
  GetConfigError(): Promise<string>;
  ExportConfig(): Promise<string>;
  ImportConfig(): Promise<number>;
  OpenBaseDir(): Promise<void>;
  ApplyWindowState(): Promise<void>;
  SaveWindowState(): Promise<void>;
  ResetWindowState(): Promise<void>;
  ShowWindow(): Promise<void>;
}

interface WailsRuntime {
  EventsOn(eventName: string, callback: (...data: unknown[]) => void): (() => void) | void;
  EventsOff(eventName: string, ...additionalEventNames: string[]): void;
  OnFileDrop(callback: (x: number, y: number, paths: string[]) => void, useDropTarget?: boolean): void;
}

function getApp(): WailsGoApp | undefined {
  return (window as unknown as { go?: { main?: { App?: WailsGoApp } } }).go?.main?.App;
}

function getRuntime(): WailsRuntime | undefined {
  return (window as unknown as { runtime?: WailsRuntime }).runtime;
}

// Local mock state for standalone Vite browser development outside Wails.
const mockEventListeners = new Map<string, Set<(...data: unknown[]) => void>>();

type MockUsage = Record<string, { count: number; lastUsed: number }>;
const mockUsage: MockUsage = {};

let mockConfig: Config = {
  settings: { ...defaultSettings },
  groups: [
    {
      id: 'default',
      name: '常用工具',
      items: [
        {
          id: 'item-1',
          name: '记事本',
          path: 'C:\\Windows\\System32\\notepad.exe',
          args: '',
          work_dir: '',
          icon: '',
          admin: false,
        },
        {
          id: 'item-2',
          name: '终端 Terminal',
          path: 'C:\\Windows\\System32\\wt.exe',
          args: '',
          work_dir: '',
          icon: '',
          admin: false,
        },
        {
          id: 'item-3',
          name: '项目目录',
          path: 'F:\\Codes\\JeeDesk',
          args: '',
          work_dir: '',
          icon: '',
          admin: false,
        },
      ],
    },
    {
      id: 'dev',
      name: '开发效率',
      items: [
        {
          id: 'item-4',
          name: 'VS Code',
          path: 'C:\\Program Files\\Microsoft VS Code\\Code.exe',
          args: '--reuse-window',
          work_dir: '',
          icon: '',
          admin: false,
        },
      ],
    },
  ],
};

function emitMockEvent(eventName: string, ...args: unknown[]): void {
  const listeners = mockEventListeners.get(eventName);
  if (!listeners) return;
  for (const cb of listeners) {
    cb(...args);
  }
}

function cloneConfig(cfg: Config): Config {
  return JSON.parse(JSON.stringify(cfg)) as Config;
}

function createId(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
}

function mockItemKey(groupId: string, itemId: string): string {
  return `${groupId}\u0000${itemId}`;
}

function mockSearch(query: string): SearchHit[] {
  const tokens = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return [];

  const hits: SearchHit[] = [];
  for (const group of mockConfig.groups) {
    for (const item of group.items) {
      const name = item.name.toLowerCase();
      const path = item.path.toLowerCase();
      let score = 0;
      let matchedAll = true;
      for (const token of tokens) {
        if (name.startsWith(token)) score += 100;
        else if (name.includes(token)) score += 40;
        else if (path.includes(token)) score += 10;
        else {
          matchedAll = false;
          break;
        }
      }
      if (!matchedAll) continue;
      hits.push({
        group_id: group.id,
        group_name: group.name,
        item_id: item.id,
        name: item.name,
        path: item.path,
        args: item.args,
        icon: item.icon,
        admin: item.admin,
        score,
      });
    }
  }
  return hits.sort((a, b) => b.score - a.score).slice(0, 60);
}

function mockRecent(limit: number): RecentItem[] {
  const entries = Object.entries(mockUsage)
    .map(([key, usage]) => {
      const [groupId, itemId] = key.split('\u0000');
      const group = mockConfig.groups.find((g) => g.id === groupId);
      const item = group?.items.find((i) => i.id === itemId);
      if (!group || !item) return null;
      return {
        group_id: group.id,
        group_name: group.name,
        item_id: item.id,
        name: item.name,
        path: item.path,
        icon: item.icon,
        admin: item.admin,
        count: usage.count,
        last_used: usage.lastUsed,
      } satisfies RecentItem;
    })
    .filter((entry): entry is RecentItem => entry !== null);

  return entries.sort((a, b) => b.last_used - a.last_used).slice(0, limit);
}

export const bridge = {
  isWails(): boolean {
    return Boolean(getApp());
  },

  async getConfig(): Promise<Config> {
    const app = getApp();
    if (app) {
      return await app.GetConfig();
    }
    return cloneConfig(mockConfig);
  },

  async getHotkeyStatus(): Promise<HotkeyStatus> {
    const app = getApp();
    if (app) {
      return await app.GetHotkeyStatus();
    }
    return {
      hotkey: mockConfig.settings.hotkey,
      registered: true,
      error: '',
    };
  },

  async launchItem(groupID: string, itemID: string): Promise<void> {
    const app = getApp();
    if (app) {
      await app.LaunchItem(groupID, itemID);
      return;
    }
    const group = mockConfig.groups.find((g) => g.id === groupID);
    const item = group?.items.find((i) => i.id === itemID);
    if (group && item) {
      const key = mockItemKey(groupID, itemID);
      const usage = mockUsage[key] ?? { count: 0, lastUsed: 0 };
      mockUsage[key] = { count: usage.count + 1, lastUsed: Date.now() };
      emitMockEvent('usage:updated');
    }
    emitMockEvent('ui:toast', {
      type: 'info',
      message: `[Dev] 启动: ${item?.name ?? itemID}`,
    } satisfies ToastPayload);
  },

  async addItem(groupID: string, item: Item): Promise<void> {
    const app = getApp();
    if (app) {
      await app.AddItem(groupID, item);
      return;
    }
    const group = mockConfig.groups.find((g) => g.id === groupID);
    if (group) {
      group.items.push({ ...item, id: item.id || createId('item') });
      emitMockEvent('config:updated', cloneConfig(mockConfig));
      emitMockEvent('ui:toast', { type: 'success', message: `已添加 ${item.name}` } satisfies ToastPayload);
    }
  },

  async updateItem(groupID: string, item: Item): Promise<void> {
    const app = getApp();
    if (app) {
      await app.UpdateItem(groupID, item);
      return;
    }
    const group = mockConfig.groups.find((g) => g.id === groupID);
    if (group) {
      const idx = group.items.findIndex((i) => i.id === item.id);
      if (idx >= 0) {
        group.items[idx] = { ...item };
        emitMockEvent('config:updated', cloneConfig(mockConfig));
        emitMockEvent('ui:toast', { type: 'success', message: `已保存 ${item.name}` } satisfies ToastPayload);
      }
    }
  },

  async deleteItem(groupID: string, itemID: string): Promise<void> {
    const app = getApp();
    if (app) {
      await app.DeleteItem(groupID, itemID);
      return;
    }
    const group = mockConfig.groups.find((g) => g.id === groupID);
    if (group) {
      group.items = group.items.filter((i) => i.id !== itemID);
      delete mockUsage[mockItemKey(groupID, itemID)];
      emitMockEvent('config:updated', cloneConfig(mockConfig));
    }
  },

  async openItemLocation(path: string): Promise<void> {
    const app = getApp();
    if (app) {
      await app.OpenItemLocation(path);
      return;
    }
    emitMockEvent('ui:toast', {
      type: 'info',
      message: `[Dev] 打开位置: ${path}`,
    } satisfies ToastPayload);
  },

  async addGroup(name: string): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.AddGroup(name);
    }
    const newGroup: Group = { id: createId('grp'), name, items: [] };
    mockConfig.groups.push(newGroup);
    emitMockEvent('config:updated', cloneConfig(mockConfig));
    return newGroup.id;
  },

  async renameGroup(groupID: string, name: string): Promise<void> {
    const app = getApp();
    if (app) {
      await app.RenameGroup(groupID, name);
      return;
    }
    const group = mockConfig.groups.find((g) => g.id === groupID);
    if (group) {
      group.name = name;
      emitMockEvent('config:updated', cloneConfig(mockConfig));
    }
  },

  async deleteGroup(groupID: string): Promise<void> {
    const app = getApp();
    if (app) {
      await app.DeleteGroup(groupID);
      return;
    }
    mockConfig.groups = mockConfig.groups.filter((g) => g.id !== groupID);
    emitMockEvent('config:updated', cloneConfig(mockConfig));
  },

  async saveSettings(settings: Settings): Promise<void> {
    const app = getApp();
    if (app) {
      await app.SaveSettings(settings);
      return;
    }
    mockConfig.settings = { ...settings };
    emitMockEvent('config:updated', cloneConfig(mockConfig));
    emitMockEvent('hotkey:status', {
      hotkey: mockConfig.settings.hotkey,
      registered: true,
      error: '',
    } satisfies HotkeyStatus);
    emitMockEvent('ui:toast', { type: 'success', message: '设置已保存' } satisfies ToastPayload);
  },

  async reloadConfig(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.ReloadConfig();
      return;
    }
    emitMockEvent('config:updated', cloneConfig(mockConfig));
    emitMockEvent('ui:toast', { type: 'info', message: '[Dev] 配置已重新加载' } satisfies ToastPayload);
  },

  async openConfigFile(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.OpenConfigFile();
      return;
    }
    emitMockEvent('ui:toast', { type: 'info', message: '[Dev] 打开 config.json' } satisfies ToastPayload);
  },

  async pickFile(title: string, filterName: string, pattern: string): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.PickFile(title, filterName, pattern);
    }
    return '';
  },

  async pickDirectory(title: string): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.PickDirectory(title);
    }
    return '';
  },

  async hideWindow(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.HideWindow();
      return;
    }
  },

  async showWindow(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.ShowWindow();
    }
  },

  async onWindowBlur(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.OnWindowBlur();
    }
  },

  async setActiveGroup(groupID: string): Promise<void> {
    const app = getApp();
    if (app) {
      await app.SetActiveGroup(groupID);
    }
  },

  async addDroppedFiles(groupID: string, paths: string[]): Promise<void> {
    const app = getApp();
    if (app) {
      await app.AddDroppedFiles(groupID, paths);
      return;
    }
    const group = mockConfig.groups.find((g) => g.id === groupID);
    if (!group) return;
    for (const path of paths) {
      const name = path.split(/[\\/]/).filter(Boolean).pop() ?? path;
      group.items.push({
        id: createId('item'),
        name,
        path,
        args: '',
        work_dir: '',
        icon: '',
        admin: false,
      });
    }
    emitMockEvent('config:updated', cloneConfig(mockConfig));
    emitMockEvent('ui:toast', {
      type: 'success',
      message: `已添加 ${paths.length} 个项目`,
    } satisfies ToastPayload);
  },

  async searchItems(query: string): Promise<SearchHit[]> {
    const app = getApp();
    if (app) {
      return await app.SearchItems(query);
    }
    return mockSearch(query);
  },

  async getRecentItems(limit = 12): Promise<RecentItem[]> {
    const app = getApp();
    if (app) {
      return await app.GetRecentItems(limit);
    }
    return mockRecent(limit);
  },

  async clearRecentItems(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.ClearRecentItems();
      return;
    }
    for (const key of Object.keys(mockUsage)) {
      delete mockUsage[key];
    }
    emitMockEvent('usage:updated');
  },

  async moveItem(fromGroupID: string, itemID: string, toGroupID: string, toIndex: number): Promise<void> {
    const app = getApp();
    if (app) {
      await app.MoveItem(fromGroupID, itemID, toGroupID, toIndex);
      return;
    }
    const from = mockConfig.groups.find((g) => g.id === fromGroupID);
    const to = mockConfig.groups.find((g) => g.id === toGroupID);
    if (!from || !to) return;
    const idx = from.items.findIndex((i) => i.id === itemID);
    if (idx < 0) return;
    const [item] = from.items.splice(idx, 1);
    to.items.splice(Math.max(0, Math.min(toIndex, to.items.length)), 0, item);
    emitMockEvent('config:updated', cloneConfig(mockConfig));
  },

  async moveGroup(groupID: string, toIndex: number): Promise<void> {
    const app = getApp();
    if (app) {
      await app.MoveGroup(groupID, toIndex);
      return;
    }
    const idx = mockConfig.groups.findIndex((g) => g.id === groupID);
    if (idx < 0) return;
    const [group] = mockConfig.groups.splice(idx, 1);
    mockConfig.groups.splice(Math.max(0, Math.min(toIndex, mockConfig.groups.length)), 0, group);
    emitMockEvent('config:updated', cloneConfig(mockConfig));
  },

  async undoDeleteItem(): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.UndoDeleteItem();
    }
    return '';
  },

  async undoDeleteGroup(): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.UndoDeleteGroup();
    }
    return '';
  },

  async getUndoLabel(): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.GetUndoLabel();
    }
    return '';
  },

  async scanStartMenu(): Promise<StartMenuEntry[]> {
    const app = getApp();
    if (app) {
      return await app.ScanStartMenu();
    }
    return [];
  },

  async importStartMenuItems(groupID: string, lnkPaths: string[]): Promise<number> {
    const app = getApp();
    if (app) {
      return await app.ImportStartMenuItems(groupID, lnkPaths);
    }
    return 0;
  },

  async getAppInfo(): Promise<AppInfo> {
    const app = getApp();
    if (app) {
      return await app.GetAppInfo();
    }
    return {
      version: 'dev',
      base_dir: '',
      config_path: '',
      config_backup: '',
      icons_dir: '',
      exe_path: '',
      auto_start: false,
      auto_start_error: '',
    };
  },

  async setAutoStart(enabled: boolean): Promise<void> {
    const app = getApp();
    if (app) {
      await app.SetAutoStart(enabled);
      return;
    }
    mockConfig.settings.auto_start = enabled;
    emitMockEvent('config:updated', cloneConfig(mockConfig));
  },

  async getConfigError(): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.GetConfigError();
    }
    return '';
  },

  async exportConfig(): Promise<string> {
    const app = getApp();
    if (app) {
      return await app.ExportConfig();
    }
    return '';
  },

  async importConfig(): Promise<number> {
    const app = getApp();
    if (app) {
      return await app.ImportConfig();
    }
    return 0;
  },

  async openBaseDir(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.OpenBaseDir();
      return;
    }
    emitMockEvent('ui:toast', { type: 'info', message: '[Dev] 打开数据目录' } satisfies ToastPayload);
  },

  async applyWindowState(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.ApplyWindowState();
    }
  },

  async saveWindowState(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.SaveWindowState();
    }
  },

  async resetWindowState(): Promise<void> {
    const app = getApp();
    if (app) {
      await app.ResetWindowState();
    }
  },

  /** Receives files dropped onto the window. Returns a no-op when the runtime is absent. */
  onFileDrop(callback: (paths: string[]) => void): () => void {
    const rt = getRuntime();
    if (!rt?.OnFileDrop) {
      return () => undefined;
    }
    rt.OnFileDrop((_x, _y, paths) => {
      if (Array.isArray(paths) && paths.length > 0) {
        callback(paths);
      }
    }, false);
    return () => undefined;
  },

  eventsOn<T = unknown>(eventName: string, callback: (payload: T) => void): () => void {
    const rt = getRuntime();
    if (rt) {
      const wrapped = (data: unknown) => callback(data as T);
      const cancel = rt.EventsOn(eventName, wrapped);
      return typeof cancel === 'function'
        ? cancel
        : () => {
            rt.EventsOff(eventName);
          };
    }

    let set = mockEventListeners.get(eventName);
    if (!set) {
      set = new Set();
      mockEventListeners.set(eventName, set);
    }
    const wrapped = (data: unknown) => callback(data as T);
    set.add(wrapped);
    return () => {
      mockEventListeners.get(eventName)?.delete(wrapped);
    };
  },

  eventsOff(eventName: string): void {
    const rt = getRuntime();
    if (rt) {
      rt.EventsOff(eventName);
      return;
    }
    mockEventListeners.delete(eventName);
  },
};
