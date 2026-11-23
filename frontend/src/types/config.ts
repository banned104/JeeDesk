export type MouseTrigger = 'none' | 'middle' | 'xbutton1' | 'xbutton2';

export interface Settings {
  hotkey: string;
  mouse_trigger: MouseTrigger;
  hide_on_launch: boolean;
  hide_on_blur: boolean;
  hide_on_esc: boolean;
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

export interface ToastMessage {
  id: number;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
}

export interface ToastPayload {
  type?: 'info' | 'success' | 'warning' | 'error';
  message: string;
}
