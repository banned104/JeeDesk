export type MouseTrigger = 'none' | 'middle' | 'xbutton1' | 'xbutton2';
export type ClickMode = 'single' | 'double';
export type ThemeMode = 'dark' | 'light' | 'system';
export type Language = 'zh' | 'en';

export interface Settings {
  hotkey: string;
  mouse_trigger: MouseTrigger;
  /** Hide the trigger mouse button from other applications. */
  mouse_swallow: boolean;
  hide_on_launch: boolean;
  hide_on_blur: boolean;
  hide_on_esc: boolean;
  /** Whether a single or a double click launches an item. */
  click_mode: ClickMode;
  auto_start: boolean;
  start_hidden: boolean;
  theme: ThemeMode;
  language: Language;
  show_recent: boolean;
  window_opacity: number;
  window_width: number;
  window_height: number;
  window_x: number;
  window_y: number;
  font_scale: number;
}

export interface HotkeyStatus {
  hotkey: string;
  registered: boolean;
  error: string;
}

export interface Item {
  id: string;
  name: string;
  path: string;
  args: string;
  work_dir: string;
  icon: string;
  /** Launch through UAC. */
  admin: boolean;
}

export interface Group {
  id: string;
  name: string;
  items: Item[];
}

export interface Config {
  settings: Settings;
  groups: Group[];
}

export interface SearchHit {
  group_id: string;
  group_name: string;
  item_id: string;
  name: string;
  path: string;
  args: string;
  icon: string;
  admin: boolean;
  score: number;
}

export interface RecentItem {
  group_id: string;
  group_name: string;
  item_id: string;
  name: string;
  path: string;
  icon: string;
  admin: boolean;
  count: number;
  last_used: number;
}

export interface AppInfo {
  version: string;
  base_dir: string;
  config_path: string;
  config_backup: string;
  icons_dir: string;
  exe_path: string;
  auto_start: boolean;
  auto_start_error: string;
}

/** One shortcut offered by the Start Menu import dialog. */
export interface StartMenuEntry {
  name: string;
  path: string;
  args: string;
  work_dir: string;
  lnk_path: string;
  icon_src: string;
  scope: 'user' | 'machine';
}

/** What the grid renders: an item together with the group it belongs to. */
export interface DisplayItem {
  groupId: string;
  groupName: string;
  item: Item;
  /** Present for search results, used to show why a distant item matched. */
  score?: number;
}

export const RECENT_GROUP_ID = '__recent__';

export const defaultSettings: Settings = {
  hotkey: 'Alt+G',
  mouse_trigger: 'none',
  mouse_swallow: true,
  hide_on_launch: true,
  hide_on_blur: true,
  hide_on_esc: true,
  click_mode: 'double',
  auto_start: false,
  start_hidden: false,
  theme: 'dark',
  language: 'zh',
  show_recent: true,
  window_opacity: 100,
  window_width: 0,
  window_height: 0,
  window_x: 0,
  window_y: 0,
  font_scale: 100,
};

export interface ToastMessage {
  id: number;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
}

export interface ToastPayload {
  type?: 'info' | 'success' | 'warning' | 'error';
  message: string;
}
