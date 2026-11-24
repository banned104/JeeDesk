import type { Config, Group, HotkeyStatus, Item, Settings, ToastPayload } from '../types/config';

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
}

interface WailsRuntime {
  EventsOn(eventName: string, callback: (...data: unknown[]) => void): (() => void) | void;
  EventsOff(eventName: string, ...additionalEventNames: string[]): void;
}

function getApp(): WailsGoApp | undefined {
  return (window as unknown as { go?: { main?: { App?: WailsGoApp } } }).go?.main?.App;
}

function getRuntime(): WailsRuntime | undefined {
  return (window as unknown as { runtime?: WailsRuntime }).runtime;
}

// Local mock state for standalone Vite browser development outside Wails.
const mockEventListeners = new Map<string, Set<(...data: unknown[]) => void>>();

let mockConfig: Config = {
  settings: {
    hotkey: 'Alt+G',
    mouse_trigger: 'none',
    hide_on_launch: true,
    hide_on_blur: true,
    hide_on_esc: true,
  },
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
        },
        {
          id: 'item-2',
          name: '终端 Terminal',
          path: 'C:\\Windows\\System32\\wt.exe',
          args: '',
          work_dir: '',
          icon: '',
        },
        {
          id: 'item-3',
          name: '项目目录',
          path: 'F:\\Codes\\JeeDesk',
          args: '',
          work_dir: '',
          icon: '',
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
