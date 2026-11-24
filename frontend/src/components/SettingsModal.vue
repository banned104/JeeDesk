<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { bridge } from '../api/bridge';
import { registerTranslations, setLanguage, t } from '../i18n';
import type {
  AppInfo,
  ClickMode,
  HotkeyStatus,
  Language,
  MouseTrigger,
  Settings,
  ThemeMode,
  UpdateInfo,
} from '../types/config';

registerTranslations({
  '偏好设置': 'Preferences',
  '修改后点击保存才会写入 config.json': 'Changes are written to config.json only when you click Save',
  '关闭': 'Close',
  '不启用': 'Off',
  '呼出': 'Summon',
  '全局快捷键': 'Global hotkey',
  '点击后直接按下组合键': 'Click, then press a key combination',
  '请按下组合键…': 'Press a key combination…',
  '未设置': 'Not set',
  'Esc 取消': 'Esc to cancel',
  '录制': 'Record',
  '快捷键不能为空': 'The hotkey cannot be empty',
  '需要至少一个修饰键（Ctrl / Alt / Shift / Win），仅 F13–F24 可以单独使用':
    'At least one modifier (Ctrl / Alt / Shift / Win) is required; only F13–F24 work on their own',
  '暂不支持数字键盘的「+」键，请改用主键盘的「=」（同一物理键）':
    'The numpad "+" key is not supported yet; use "=" on the main keyboard (same physical key)',
  '全局快捷键 {hotkey} 已生效': 'Global hotkey {hotkey} is active',
  '注册失败：{error}（可能与其它软件冲突，换一个组合键试试）':
    'Registration failed: {error} (another app may hold the key, try another combination)',
  '修改尚未保存，保存后生效': 'Not saved yet; it takes effect after saving',
  '鼠标触发': 'Mouse trigger',
  '中键': 'Middle button',
  'XButton1（后退键）': 'XButton1 (back)',
  'XButton2（前进键）': 'XButton2 (forward)',
  '在任意窗口按下该鼠标键即可呼出面板。': 'Press that mouse button in any window to summon the panel.',
  '吞掉触发键': 'Swallow the trigger button',
  '拦截触发键，不再传给其它程序，避免点开面板时同时触发其它程序里的动作。':
    'Blocks the trigger button so it no longer reaches other programs, so summoning the panel does not also trigger their actions.',
  '副作用：该鼠标键在其它程序里也会失灵（例如浏览器后退）。':
    'Side effect: that mouse button also stops working in other programs (browser Back, for example).',
  '当前未启用鼠标触发，此选项暂不生效。': 'Mouse trigger is off, so this option has no effect right now.',
  '启动': 'Launch',
  '启动方式': 'Launch gesture',
  '单击启动': 'Single click',
  '双击启动': 'Double click',
  '单击即启动项目': 'One click launches the item',
  '双击才启动，减少误触': 'Two clicks launch the item, so stray clicks do less harm',
  '隐藏窗口': 'Hide window',
  '点击启动项目后自动隐藏面板。': 'Hides the panel after launching an item.',
  '失焦隐藏': 'Hide on blur',
  '切换到其他程序时自动隐藏（拖动标题栏不会触发）。':
    'Hides when you switch to another program (dragging the title bar does not count).',
  '严格失焦隐藏': 'Strict hide on blur',
  '窗口失去焦点就隐藏，光标停在面板上也会隐藏。':
    'Hides as soon as the window loses focus, even while the cursor rests on the panel.',
  '关闭时，只要光标还停在面板内，失焦就不会隐藏面板。':
    'When off, the panel stays visible while the cursor rests on it.',
  'Esc 隐藏': 'Hide with Esc',
  '面板聚焦时按 Esc 快速隐藏到系统托盘。': 'Press Esc while the panel is focused to hide it to the tray.',
  '外观': 'Appearance',
  '主题': 'Theme',
  '深色': 'Dark',
  '浅色': 'Light',
  '跟随系统': 'Follow system',
  '窗口不透明度': 'Window opacity',
  '数值越低越通透，50% 为面板最透明的极限。':
    'Lower is more transparent; 50% is the most transparent the panel gets.',
  '界面缩放': 'UI scale',
  '同时影响文字与图标尺寸，保存后生效。': 'Scales text and icons; takes effect after saving.',
  '界面语言': 'Language',
  '中文': 'Chinese',
  '系统': 'System',
  '开机自启': 'Start with Windows',
  '登录 Windows 后自动启动 JeeDesk。': 'Launches JeeDesk after you sign in to Windows.',
  '将注册：{path}': 'Will register: {path}',
  '尚未获取到程序路径。': 'The executable path is not available yet.',
  '启动时隐藏窗口': 'Start hidden',
  '随系统启动时直接隐藏到托盘，不弹出面板。':
    'When started with Windows, go straight to the tray without showing the panel.',
  '显示「最近使用」分组': 'Show the Recent group',
  '在侧边栏顶部展示最近启动过的项目。': 'Lists recently launched items at the top of the sidebar.',
  '清空最近使用记录': 'Clear recent items',
  '只删除「最近使用」分组中的记录，不影响项目本身。':
    'Removes only the Recent entries; the items themselves stay.',
  '清空记录': 'Clear',
  '确认清空？': 'Confirm clear?',
  '数据': 'Data',
  '配置路径': 'Config path',
  '数据目录': 'Data folder',
  '备份路径': 'Backup file',
  '未获取': 'Not available',
  '打开数据目录': 'Open data folder',
  '打开 config.json': 'Open config.json',
  '重载配置': 'Reload config',
  '导出配置': 'Export config',
  '导入配置': 'Import config',
  '重置窗口位置': 'Reset window position',
  '从开始菜单导入': 'Import from Start Menu',
  '重载配置会从磁盘重新读取设置与分组；导入配置会用备份文件覆盖现有设置与分组。':
    'Reloading re-reads settings and groups from disk; importing overwrites them with a backup file.',
  '关于': 'About',
  '版本': 'Version',
  '配置文件': 'Config file',
  '日志文件': 'Log file',
  '检查更新': 'Check for updates',
  '检查中…': 'Checking…',
  '打开下载页': 'Open download page',
  '打开日志': 'Open log',
  '清理图标缓存': 'Clean icon cache',
  '清理中…': 'Cleaning…',
  '已是最新版本（{current}）': 'You are up to date ({current})',
  '发现新版本 {latest}（当前 {current}）': 'New version {latest} is available (current {current})',
  '检查更新失败：{error}': 'Update check failed: {error}',
  '已清理 {count} 个图标文件': 'Removed {count} icon files',
  '没有需要清理的图标': 'No icon files needed cleaning',
  '清理图标缓存失败：{error}': 'Cleaning the icon cache failed: {error}',
  '打开日志失败：{error}': 'Opening the log failed: {error}',
  '打开下载页失败：{error}': 'Opening the download page failed: {error}',
  '未知': 'Unknown',
  '有未保存的修改': 'Unsaved changes',
  '设置已同步': 'Settings in sync',
  '取消': 'Cancel',
  '保存': 'Save',
  '保存中…': 'Saving…',
  '保存失败：{error}': 'Saving failed: {error}',
  '配置已在外部更新，点击重载': 'The config changed outside this dialog; reload to pick it up',
  '重载': 'Reload',
});

const props = defineProps<{
  visible: boolean;
  settings: Settings;
  hotkeyStatus?: HotkeyStatus;
  appInfo?: AppInfo | null;
  /** Inline error supplied by the parent when it performs the save itself instead of this dialog. */
  saveError?: string;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  /** Kept declared for parents that still wire @save; the dialog saves itself so a failure stays inline. */
  (e: 'save', settings: Settings): void;
  (e: 'openConfigFile'): void;
  (e: 'reloadConfig'): void;
  (e: 'openBaseDir'): void;
  (e: 'exportConfig'): void;
  (e: 'importConfig'): void;
  (e: 'resetWindowState'): void;
  (e: 'openStartMenuImport'): void;
  (e: 'clearRecent'): void;
}>();

type BooleanSettingKey =
  | 'mouse_swallow'
  | 'hide_on_launch'
  | 'hide_on_blur'
  | 'hide_on_blur_strict'
  | 'hide_on_esc'
  | 'auto_start'
  | 'start_hidden'
  | 'show_recent';

interface ToggleRow {
  key: BooleanSettingKey;
  label: string;
  desc: string;
  note?: string;
  error?: string;
}

interface HotkeyState {
  tone: 'ok' | 'error' | 'idle';
  text: string;
}

/** Scalar fields only: collapsed_groups is an array and is compared separately. */
const SETTINGS_FIELDS = [
  'hotkey',
  'mouse_trigger',
  'mouse_swallow',
  'hide_on_launch',
  'hide_on_blur',
  'hide_on_blur_strict',
  'hide_on_esc',
  'click_mode',
  'auto_start',
  'start_hidden',
  'theme',
  'language',
  'show_recent',
  'window_positioned',
  'window_opacity',
  'window_width',
  'window_height',
  'window_x',
  'window_y',
  'font_scale',
] as const;

/** Function keys below F13 do not exist on most keyboards, so only F13-F24 may be used bare. */
const BARE_FUNCTION_KEYS = new Set([
  'F13',
  'F14',
  'F15',
  'F16',
  'F17',
  'F18',
  'F19',
  'F20',
  'F21',
  'F22',
  'F23',
  'F24',
]);

const MODIFIER_TOKENS = ['ctrl', 'control', 'alt', 'shift', 'win', 'super', 'meta'];

function copyStringList(source: string[] | undefined): string[] {
  return Array.isArray(source) ? [...source] : [];
}

function sameStringList(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((value, index) => value === b[index]);
}

/** Every field is copied explicitly so a new Settings field cannot be silently dropped. */
function copySettings(source: Settings): Settings {
  return {
    hotkey: source.hotkey ?? '',
    mouse_trigger: source.mouse_trigger ?? 'none',
    mouse_swallow: source.mouse_swallow ?? true,
    hide_on_launch: source.hide_on_launch ?? true,
    hide_on_blur: source.hide_on_blur ?? true,
    hide_on_blur_strict: source.hide_on_blur_strict ?? false,
    hide_on_esc: source.hide_on_esc ?? true,
    click_mode: source.click_mode ?? 'double',
    auto_start: source.auto_start ?? false,
    start_hidden: source.start_hidden ?? false,
    theme: source.theme ?? 'dark',
    language: source.language ?? 'zh',
    show_recent: source.show_recent ?? true,
    // Copied, never aliased: the draft must round-trip the collapsed sidebar groups untouched.
    collapsed_groups: copyStringList(source.collapsed_groups),
    // Window geometry belongs to the shell, so it is only carried through, never edited here.
    window_positioned: source.window_positioned ?? false,
    window_opacity: source.window_opacity ?? 100,
    window_width: source.window_width ?? 0,
    window_height: source.window_height ?? 0,
    window_x: source.window_x ?? 0,
    window_y: source.window_y ?? 0,
    font_scale: source.font_scale ?? 100,
  };
}

function clampNumber(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, Math.round(value)));
}

/** The sliders cannot represent out-of-range values, so normalise once for both form and baseline. */
function normalizeSettings(source: Settings): Settings {
  const copy = copySettings(source);
  copy.hotkey = copy.hotkey.trim();
  copy.window_opacity = clampNumber(copy.window_opacity, 50, 100, 100);
  copy.font_scale = clampNumber(copy.font_scale, 80, 140, 100);
  return copy;
}

function sameSettings(a: Settings, b: Settings): boolean {
  return (
    SETTINGS_FIELDS.every((field) => a[field] === b[field]) &&
    sameStringList(a.collapsed_groups ?? [], b.collapsed_groups ?? [])
  );
}

const form = reactive<Settings>(normalizeSettings(props.settings));
// Snapshot of the last applied settings: the form is dirty once it drifts away from it.
const baseline = ref<Settings>(normalizeSettings(props.settings));

const dirty = computed(() => !sameSettings(form, baseline.value));

// --- dialog focus management ----------------------------------------------

const titleId = 'settings-modal-title';
const dialogRef = ref<HTMLElement | null>(null);
const bodyRef = ref<HTMLElement | null>(null);
// Element that had focus before the dialog opened, restored once it closes.
let previousFocus: HTMLElement | null = null;

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusableElements(root: HTMLElement | null): HTMLElement[] {
  if (!root) return [];
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (el) => el.getClientRects().length > 0
  );
}

/** Focus the first control of the body, so Tab walks the form instead of the backdrop. */
async function focusFirstField() {
  await nextTick();
  const target = focusableElements(bodyRef.value)[0] ?? dialogRef.value;
  target?.focus();
}

// Simple focus trap: Tab / Shift+Tab cycle inside the dialog instead of reaching the page behind it.
function handleTabKey(event: KeyboardEvent) {
  if (event.key !== 'Tab') return;
  const root = dialogRef.value;
  if (!root) return;
  const elements = focusableElements(root);
  if (elements.length === 0) {
    event.preventDefault();
    root.focus();
    return;
  }
  const first = elements[0];
  const last = elements[elements.length - 1];
  const active = document.activeElement;
  if (event.shiftKey) {
    if (active === first || !root.contains(active)) {
      event.preventDefault();
      last.focus();
    }
    return;
  }
  if (active === last || !root.contains(active)) {
    event.preventDefault();
    first.focus();
  }
}

function rememberFocus() {
  const active = document.activeElement;
  if (active instanceof HTMLElement && !dialogRef.value?.contains(active)) {
    previousFocus = active;
  }
}

function restoreFocus() {
  const target = previousFocus;
  previousFocus = null;
  if (target && document.contains(target)) target.focus();
}

const recording = ref(false);
const hotkeyHint = ref('');
const saving = ref(false);
const saveErrorText = ref('');
const confirmClearRecent = ref(false);
/** Set when config.json changed elsewhere while the form held unsaved edits. */
const externalUpdate = ref(false);

/** App info the dialog fetches itself when the parent has not loaded it yet. */
const fetchedAppInfo = ref<AppInfo | null>(null);
const aboutInfo = computed<AppInfo | null>(() => props.appInfo ?? fetchedAppInfo.value);

const updateChecking = ref(false);
const updateResult = ref<UpdateInfo | null>(null);
const updateError = ref('');

const cleanupRunning = ref(false);
const cleanupMessage = ref('');
const cleanupError = ref('');
const logError = ref('');

let confirmTimer: number | undefined;

const mouseTriggerOptions = computed<Array<{ value: MouseTrigger; label: string }>>(() => [
  { value: 'none', label: t('不启用') },
  { value: 'middle', label: t('中键') },
  { value: 'xbutton1', label: t('XButton1（后退键）') },
  { value: 'xbutton2', label: t('XButton2（前进键）') },
]);

const clickModeOptions = computed<Array<{ value: ClickMode; label: string; desc: string }>>(() => [
  { value: 'single', label: t('单击启动'), desc: t('单击即启动项目') },
  { value: 'double', label: t('双击启动'), desc: t('双击才启动，减少误触') },
]);

const themeOptions = computed<Array<{ value: ThemeMode; label: string }>>(() => [
  { value: 'dark', label: t('深色') },
  { value: 'light', label: t('浅色') },
  { value: 'system', label: t('跟随系统') },
]);

const languageOptions = computed<Array<{ value: Language; label: string }>>(() => [
  { value: 'zh', label: t('中文') },
  { value: 'en', label: 'English' },
]);

const triggerToggles = computed<ToggleRow[]>(() => [
  {
    key: 'mouse_swallow',
    label: t('吞掉触发键'),
    desc: t('拦截触发键，不再传给其它程序，避免点开面板时同时触发其它程序里的动作。'),
    // The hook is global, so swallowing cannot be limited to this window.
    note:
      form.mouse_trigger === 'none'
        ? `${t('当前未启用鼠标触发，此选项暂不生效。')} ${t('副作用：该鼠标键在其它程序里也会失灵（例如浏览器后退）。')}`
        : t('副作用：该鼠标键在其它程序里也会失灵（例如浏览器后退）。'),
  },
]);

const launchToggles = computed<ToggleRow[]>(() => [
  { key: 'hide_on_launch', label: t('隐藏窗口'), desc: t('点击启动项目后自动隐藏面板。') },
  {
    key: 'hide_on_blur',
    label: t('失焦隐藏'),
    desc: t('切换到其他程序时自动隐藏（拖动标题栏不会触发）。'),
  },
  {
    key: 'hide_on_blur_strict',
    label: t('严格失焦隐藏'),
    desc: t('窗口失去焦点就隐藏，光标停在面板上也会隐藏。'),
    note: t('关闭时，只要光标还停在面板内，失焦就不会隐藏面板。'),
  },
  { key: 'hide_on_esc', label: t('Esc 隐藏'), desc: t('面板聚焦时按 Esc 快速隐藏到系统托盘。') },
]);

const systemToggles = computed<ToggleRow[]>(() => {
  const info = aboutInfo.value;
  const exePath = info?.exe_path?.trim() ?? '';
  const autoStartError = info?.auto_start_error?.trim() ?? '';
  return [
    {
      key: 'auto_start',
      label: t('开机自启'),
      desc: t('登录 Windows 后自动启动 JeeDesk。'),
      note: exePath ? t('将注册：{path}', { path: exePath }) : t('尚未获取到程序路径。'),
      error: autoStartError,
    },
    {
      key: 'start_hidden',
      label: t('启动时隐藏窗口'),
      desc: t('随系统启动时直接隐藏到托盘，不弹出面板。'),
    },
    {
      key: 'show_recent',
      label: t('显示「最近使用」分组'),
      desc: t('在侧边栏顶部展示最近启动过的项目。'),
    },
  ];
});

const dataPaths = computed<Array<{ label: string; value: string }>>(() => {
  const info = aboutInfo.value;
  return [
    { label: t('配置路径'), value: info?.config_path?.trim() || t('未获取') },
    { label: t('数据目录'), value: info?.base_dir?.trim() || t('未获取') },
    { label: t('备份路径'), value: info?.config_backup?.trim() || t('未获取') },
  ];
});

const aboutRows = computed<Array<{ label: string; value: string }>>(() => {
  const info = aboutInfo.value;
  return [
    { label: t('版本'), value: info?.version?.trim() || t('未获取') },
    { label: t('数据目录'), value: info?.base_dir?.trim() || t('未获取') },
    { label: t('配置文件'), value: info?.config_path?.trim() || t('未获取') },
    { label: t('日志文件'), value: info?.log_path?.trim() || t('未获取') },
  ];
});

const updateMessage = computed(() => {
  const info = updateResult.value;
  if (!info) return '';
  if (info.error) return t('检查更新失败：{error}', { error: info.error });
  const current = info.current || t('未知');
  if (info.available) {
    return t('发现新版本 {latest}（当前 {current}）', { latest: info.latest, current });
  }
  return t('已是最新版本（{current}）', { current });
});

const updateUrl = computed(() => {
  const info = updateResult.value;
  if (!info || info.error || !info.available) return '';
  return info.url?.trim() ?? '';
});

const updateToneClass = computed(() => {
  if (updateError.value) return 'text-rose-400';
  const info = updateResult.value;
  if (!info) return 'jd-subtle';
  if (info.error) return 'text-rose-400';
  if (info.available) return 'text-sky-400';
  return 'text-emerald-400';
});

const hotkeyState = computed<HotkeyState>(() => {
  const status = props.hotkeyStatus;
  const current = form.hotkey.trim();
  if (status?.registered && status.hotkey === current) {
    return { tone: 'ok', text: t('全局快捷键 {hotkey} 已生效', { hotkey: status.hotkey }) };
  }
  if (status?.error && status.hotkey === current) {
    return {
      tone: 'error',
      text: t('注册失败：{error}（可能与其它软件冲突，换一个组合键试试）', { error: status.error }),
    };
  }
  if (current !== (props.settings.hotkey ?? '').trim()) {
    return { tone: 'idle', text: t('修改尚未保存，保存后生效') };
  }
  return { tone: 'idle', text: '' };
});

const hotkeyStateClass = computed(() => {
  if (hotkeyState.value.tone === 'ok') return 'text-emerald-400';
  if (hotkeyState.value.tone === 'error') return 'text-rose-400';
  return 'jd-subtle';
});

const hotkeyDotClass = computed(() => {
  if (hotkeyState.value.tone === 'ok') return 'bg-emerald-400';
  if (hotkeyState.value.tone === 'error') return 'bg-rose-400';
  return 'jd-accent-bg';
});

/** The parent may report the failure itself; either source is shown in the same banner. */
const visibleError = computed(() => (props.saveError ?? '').trim() || saveErrorText.value);

// `event.code` keeps Shift+1 as "1" instead of "!", so the string stays parseable by the Go hook.
const codeKeyNames: Record<string, string> = {
  Space: 'Space',
  Tab: 'Tab',
  Enter: 'Enter',
  NumpadEnter: 'Enter',
  Backspace: 'Backspace',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  Home: 'Home',
  End: 'End',
  Insert: 'Insert',
  Delete: 'Delete',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
  Backquote: '`',
  Minus: '-',
  Equal: '=',
  BracketLeft: '[',
  BracketRight: ']',
  Backslash: '\\',
  Semicolon: ';',
  Quote: "'",
  Comma: ',',
  Period: '.',
  Slash: '/',
};

function hotkeyKeyFromEvent(event: KeyboardEvent): string | null {
  const code = event.code;
  if (/^Key[A-Z]$/.test(code)) return code.slice(3);
  if (/^Digit[0-9]$/.test(code)) return code.slice(5);
  if (/^Numpad[0-9]$/.test(code)) return code.slice(6);
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(code)) return code;
  return codeKeyNames[code] ?? null;
}

function modifierParts(event: KeyboardEvent): string[] {
  const parts: string[] = [];
  if (event.ctrlKey) parts.push('Ctrl');
  if (event.altKey) parts.push('Alt');
  if (event.shiftKey) parts.push('Shift');
  if (event.metaKey) parts.push('Win');
  return parts;
}

/** Mirrors hook.ParseHotkey: one modifier is mandatory and only F13-F24 may stand alone. */
function bareKeyError(spec: string): string {
  const parts = spec
    .split('+')
    .map((part) => part.trim())
    .filter(Boolean);
  if (parts.length < 2) {
    return t('需要至少一个修饰键（Ctrl / Alt / Shift / Win），仅 F13–F24 可以单独使用');
  }
  const mainKey = parts[parts.length - 1].toUpperCase();
  const hasModifier = parts
    .slice(0, -1)
    .some((part) => MODIFIER_TOKENS.includes(part.toLowerCase()));
  if (hasModifier || BARE_FUNCTION_KEYS.has(mainKey)) return '';
  return t('需要至少一个修饰键（Ctrl / Alt / Shift / Win），仅 F13–F24 可以单独使用');
}

function toggleRecording() {
  if (recording.value) {
    recording.value = false;
    hotkeyHint.value = '';
    return;
  }
  recording.value = true;
  hotkeyHint.value = '';
}

function handleRecordingKeydown(event: KeyboardEvent) {
  if (!recording.value) return;
  event.preventDefault();
  event.stopPropagation();

  if (event.key === 'Escape') {
    recording.value = false;
    hotkeyHint.value = '';
    return;
  }

  // hook.ParseHotkey splits on "+" and has no "plus" alias yet, so recording "+" would produce
  // the unparsable string "Alt++". "=" is the very same physical key (VK_OEM_PLUS).
  if (event.code === 'NumpadAdd') {
    hotkeyHint.value = t('暂不支持数字键盘的「+」键，请改用主键盘的「=」（同一物理键）');
    return;
  }

  const mainKey = hotkeyKeyFromEvent(event);
  // Modifier-only presses and unmapped keys keep the recorder open instead of producing a partial spec.
  if (!mainKey) return;

  const modifiers = modifierParts(event);
  // A bare letter would hijack that key system-wide, so one modifier is required (F13-F24 excepted).
  if (modifiers.length === 0 && !BARE_FUNCTION_KEYS.has(mainKey)) {
    hotkeyHint.value = t('需要至少一个修饰键（Ctrl / Alt / Shift / Win），仅 F13–F24 可以单独使用');
    return;
  }

  form.hotkey = [...modifiers, mainKey].join('+');
  hotkeyHint.value = '';
  recording.value = false;
  // The recorder button keeps focus after capture; drop it so the next Enter submits the form
  // instead of re-arming the recorder.
  const active = document.activeElement;
  if (active instanceof HTMLElement) active.blur();
}

function toggleSetting(key: BooleanSettingKey) {
  form[key] = !form[key];
}

function applySettings(source: Settings) {
  const normalized = normalizeSettings(source);
  Object.assign(form, normalized);
  baseline.value = copySettings(normalized);
}

function clearTimers() {
  if (confirmTimer !== undefined) {
    window.clearTimeout(confirmTimer);
    confirmTimer = undefined;
  }
}

function resetTransientState() {
  recording.value = false;
  hotkeyHint.value = '';
  confirmClearRecent.value = false;
  saving.value = false;
  saveErrorText.value = '';
  externalUpdate.value = false;
  updateChecking.value = false;
  updateResult.value = null;
  updateError.value = '';
  cleanupRunning.value = false;
  cleanupMessage.value = '';
  cleanupError.value = '';
  logError.value = '';
  clearTimers();
}

async function ensureAppInfo() {
  if (props.appInfo) return;
  try {
    fetchedAppInfo.value = await bridge.getAppInfo();
  } catch {
    // The rows fall back to "not available"; a missing info blob must not break the dialog.
  }
}

/** Explicit user action: drop the draft and take the settings that arrived from outside. */
function handleReloadDraft() {
  applySettings(props.settings);
  externalUpdate.value = false;
  hotkeyHint.value = '';
  saveErrorText.value = '';
}

async function handleSave() {
  if (saving.value) return;
  const payload = copySettings(form);
  payload.hotkey = payload.hotkey.trim();
  if (!payload.hotkey) {
    hotkeyHint.value = t('快捷键不能为空');
    recording.value = false;
    return;
  }
  const bareKey = bareKeyError(payload.hotkey);
  if (bareKey) {
    hotkeyHint.value = bareKey;
    recording.value = false;
    return;
  }

  saving.value = true;
  saveErrorText.value = '';
  recording.value = false;
  hotkeyHint.value = '';
  try {
    // The dialog owns the round trip: the backend rejects the save when the hotkey cannot be
    // registered, and that reason belongs next to the form instead of a toast that disappears.
    await bridge.saveSettings(payload);
    applySettings(payload);
    externalUpdate.value = false;
    saving.value = false;
    // Only a confirmed save closes the dialog; the parent already learns the new values from the
    // backend's config:updated event, so @save is not emitted here (that would save twice).
    emit('close');
  } catch (err) {
    saving.value = false;
    saveErrorText.value = t('保存失败：{error}', { error: errorText(err) });
  }
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function handleClose() {
  // Esc while recording only cancels the capture: the dialog itself stays open.
  if (recording.value) {
    recording.value = false;
    hotkeyHint.value = '';
    return;
  }
  // Busy: a save is in flight, so nothing closes and nothing is rolled back.
  if (saving.value) return;
  // A language switch previews immediately; cancelling must put the saved language back.
  if (dirty.value) setLanguage(props.settings.language);
  resetTransientState();
  emit('close');
}

// The shell owns the global Esc shortcut, so it calls this instead of flipping `visible` itself;
// that keeps the busy guard and the language rollback in one place.
defineExpose({ handleClose });

function handleClearRecent() {
  if (!confirmClearRecent.value) {
    confirmClearRecent.value = true;
    if (confirmTimer !== undefined) window.clearTimeout(confirmTimer);
    confirmTimer = window.setTimeout(() => {
      confirmClearRecent.value = false;
      confirmTimer = undefined;
    }, 4000);
    return;
  }
  if (confirmTimer !== undefined) {
    window.clearTimeout(confirmTimer);
    confirmTimer = undefined;
  }
  confirmClearRecent.value = false;
  emit('clearRecent');
}

async function handleCheckUpdates() {
  if (updateChecking.value) return;
  updateChecking.value = true;
  updateError.value = '';
  try {
    updateResult.value = await bridge.checkUpdates();
  } catch (err) {
    // A transport failure is reported next to the button, never swallowed.
    updateResult.value = null;
    updateError.value = t('检查更新失败：{error}', { error: errorText(err) });
  } finally {
    updateChecking.value = false;
  }
}

async function openUpdatePage() {
  const url = updateUrl.value;
  if (!url) return;
  try {
    // The system browser, not a second WebView window: window.open would open a frameless panel.
    await bridge.openUrl(url);
  } catch (err) {
    updateError.value = t('打开下载页失败：{error}', { error: errorText(err) });
  }
}

async function handleOpenLogFile() {
  logError.value = '';
  try {
    await bridge.openLogFile();
  } catch (err) {
    logError.value = t('打开日志失败：{error}', { error: errorText(err) });
  }
}

async function handleCleanupIcons() {
  if (cleanupRunning.value) return;
  cleanupRunning.value = true;
  cleanupMessage.value = '';
  cleanupError.value = '';
  try {
    const removed = await bridge.cleanupIcons();
    cleanupMessage.value =
      removed > 0 ? t('已清理 {count} 个图标文件', { count: removed }) : t('没有需要清理的图标');
  } catch (err) {
    cleanupError.value = t('清理图标缓存失败：{error}', { error: errorText(err) });
  } finally {
    cleanupRunning.value = false;
  }
}

// The language switch previews instantly; App.vue re-syncs it from the saved config afterwards.
watch(
  () => form.language,
  (lang) => {
    setLanguage(lang);
  }
);

// Opening always re-reads the latest settings: unsaved edits from a previous session are dropped.
watch(
  () => props.visible,
  (visible) => {
    resetTransientState();
    if (!visible) {
      restoreFocus();
      return;
    }
    rememberFocus();
    applySettings(props.settings);
    void ensureAppInfo();
    void focusFirstField();
  },
  { immediate: true }
);

// While the user has unsaved edits, incoming settings must not overwrite what they typed. The only
// exception is the parent echoing back exactly what is in the form: then the baseline catches up.
// Everything else is announced with a reload hint instead of silently replacing the draft.
watch(
  () => props.settings,
  (next) => {
    if (!props.visible) {
      externalUpdate.value = false;
      return;
    }
    const normalized = normalizeSettings(next);
    if (sameSettings(form, normalized) || !dirty.value) {
      applySettings(next);
      externalUpdate.value = false;
      return;
    }
    if (!sameSettings(baseline.value, normalized)) externalUpdate.value = true;
  },
  { deep: true }
);

onMounted(() => {
  window.addEventListener('keydown', handleRecordingKeydown, true);
});

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleRecordingKeydown, true);
  clearTimers();
  restoreFocus();
});
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="visible"
        class="fixed inset-0 z-40 bg-black/50 backdrop-blur-[2px] flex items-center justify-center p-4"
        @mousedown.self="handleClose"
      >
        <form
          ref="dialogRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="w-full max-w-[580px] max-h-[90vh] rounded-2xl glass-modal border jd-border-strong shadow-2xl flex flex-col overflow-hidden focus:outline-none"
          @keydown="handleTabKey"
          @submit.prevent="handleSave"
        >
          <!-- Header -->
          <div class="px-5 py-3.5 border-b jd-border flex items-center justify-between gap-3 shrink-0">
            <div class="min-w-0">
              <h2 :id="titleId" class="text-sm font-semibold jd-text">{{ t('偏好设置') }}</h2>
              <p class="text-[11px] jd-subtle mt-0.5">{{ t('修改后点击保存才会写入 config.json') }}</p>
            </div>
            <button
              type="button"
              :aria-label="t('关闭')"
              :title="t('关闭')"
              class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover transition-colors shrink-0 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
              @click="handleClose"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Scrollable body -->
          <div ref="bodyRef" class="flex-1 min-h-0 overflow-y-auto custom-scrollbar px-5 py-4 space-y-5 text-xs">
            <!-- 呼出 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">{{ t('呼出') }}</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <div class="space-y-1.5">
                <div class="flex items-center justify-between gap-2">
                  <span class="font-medium jd-text">{{ t('全局快捷键') }}</span>
                  <span class="text-[11px] jd-subtle">{{ t('点击后直接按下组合键') }}</span>
                </div>
                <button
                  type="button"
                  class="w-full h-9 px-3 rounded-xl border text-left flex items-center justify-between gap-2 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  :class="recording ? 'border-sky-400 bg-sky-500/10 ring-1 ring-sky-400/40' : 'jd-input'"
                  @click="toggleRecording"
                >
                  <span class="font-mono text-[12px] truncate" :class="recording ? 'jd-accent' : 'jd-text'">
                    {{ recording ? t('请按下组合键…') : form.hotkey || t('未设置') }}
                  </span>
                  <span class="text-[11px] shrink-0" :class="recording ? 'jd-accent' : 'jd-subtle'">
                    {{ recording ? t('Esc 取消') : t('录制') }}
                  </span>
                </button>
                <p v-if="hotkeyHint" class="text-[11px] text-rose-400">{{ hotkeyHint }}</p>
                <div
                  v-if="hotkeyState.text"
                  class="flex items-start gap-1.5 text-[11px] pt-0.5"
                  :class="hotkeyStateClass"
                >
                  <span class="w-1.5 h-1.5 rounded-full shrink-0 mt-1" :class="hotkeyDotClass" />
                  <span class="min-w-0 break-all">{{ hotkeyState.text }}</span>
                </div>
              </div>

              <label class="block space-y-1.5">
                <span class="font-medium jd-text">{{ t('鼠标触发') }}</span>
                <select
                  v-model="form.mouse_trigger"
                  class="w-full h-9 px-3 rounded-xl border text-[12px] jd-input transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/60"
                >
                  <option v-for="opt in mouseTriggerOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
                <span class="block text-[11px] jd-subtle">{{ t('在任意窗口按下该鼠标键即可呼出面板。') }}</span>
              </label>

              <div
                v-for="row in triggerToggles"
                :key="row.key"
                class="flex items-start justify-between gap-3 rounded-xl border jd-border jd-surface-soft px-3 py-2.5"
              >
                <div class="min-w-0 space-y-0.5">
                  <div class="font-medium jd-text">{{ row.label }}</div>
                  <p class="text-[11px] jd-muted leading-relaxed">{{ row.desc }}</p>
                  <p v-if="row.note" class="text-[11px] jd-subtle leading-relaxed break-all">{{ row.note }}</p>
                </div>
                <button
                  type="button"
                  role="switch"
                  :aria-checked="form[row.key]"
                  :title="row.label"
                  class="relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70 focus-visible:ring-offset-0"
                  :class="form[row.key] ? 'jd-accent-bg' : 'jd-surface-strong'"
                  @click="toggleSetting(row.key)"
                >
                  <span
                    class="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-transform"
                    :class="form[row.key] ? 'translate-x-4' : 'translate-x-0'"
                  />
                </button>
              </div>
            </section>

            <!-- 启动 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">{{ t('启动') }}</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <div class="space-y-1.5">
                <span class="font-medium jd-text">{{ t('启动方式') }}</span>
                <div class="grid grid-cols-2 gap-2" role="radiogroup" :aria-label="t('启动方式')">
                  <button
                    v-for="opt in clickModeOptions"
                    :key="opt.value"
                    type="button"
                    role="radio"
                    :aria-checked="form.click_mode === opt.value"
                    class="px-3 py-2 rounded-xl border text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                    :class="
                      form.click_mode === opt.value
                        ? 'border-sky-400/70 bg-sky-500/10'
                        : 'jd-border jd-surface-soft jd-muted jd-hover'
                    "
                    @click="form.click_mode = opt.value"
                  >
                    <span class="block font-medium" :class="form.click_mode === opt.value ? 'jd-accent' : ''">
                      {{ opt.label }}
                    </span>
                    <span class="block text-[11px] mt-0.5" :class="form.click_mode === opt.value ? 'jd-muted' : 'jd-subtle'">
                      {{ opt.desc }}
                    </span>
                  </button>
                </div>
              </div>

              <div
                v-for="row in launchToggles"
                :key="row.key"
                class="flex items-start justify-between gap-3 rounded-xl border jd-border jd-surface-soft px-3 py-2.5"
              >
                <div class="min-w-0 space-y-0.5">
                  <div class="font-medium jd-text">{{ row.label }}</div>
                  <p class="text-[11px] jd-muted leading-relaxed">{{ row.desc }}</p>
                  <p v-if="row.note" class="text-[11px] jd-subtle leading-relaxed break-all">{{ row.note }}</p>
                </div>
                <button
                  type="button"
                  role="switch"
                  :aria-checked="form[row.key]"
                  :title="row.label"
                  class="relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70 focus-visible:ring-offset-0"
                  :class="form[row.key] ? 'jd-accent-bg' : 'jd-surface-strong'"
                  @click="toggleSetting(row.key)"
                >
                  <span
                    class="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-transform"
                    :class="form[row.key] ? 'translate-x-4' : 'translate-x-0'"
                  />
                </button>
              </div>
            </section>

            <!-- 外观 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">{{ t('外观') }}</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <div class="space-y-1.5">
                <span class="font-medium jd-text">{{ t('主题') }}</span>
                <div class="grid grid-cols-3 gap-2" role="radiogroup" :aria-label="t('主题')">
                  <button
                    v-for="opt in themeOptions"
                    :key="opt.value"
                    type="button"
                    role="radio"
                    :aria-checked="form.theme === opt.value"
                    class="h-8 rounded-xl border text-[11px] transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                    :class="
                      form.theme === opt.value
                        ? 'border-sky-400/70 bg-sky-500/10 jd-accent font-medium'
                        : 'jd-border jd-surface-soft jd-muted jd-hover'
                    "
                    @click="form.theme = opt.value"
                  >
                    {{ opt.label }}
                  </button>
                </div>
              </div>

              <label class="block space-y-1.5">
                <span class="flex items-center justify-between gap-2">
                  <span class="font-medium jd-text">{{ t('窗口不透明度') }}</span>
                  <span class="font-mono text-[11px] jd-subtle">{{ form.window_opacity }}%</span>
                </span>
                <input
                  v-model.number="form.window_opacity"
                  type="range"
                  min="50"
                  max="100"
                  step="1"
                  class="w-full accent-sky-400 rounded-md focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/60"
                />
                <span class="block text-[11px] jd-subtle">{{ t('数值越低越通透，50% 为面板最透明的极限。') }}</span>
              </label>

              <label class="block space-y-1.5">
                <span class="flex items-center justify-between gap-2">
                  <span class="font-medium jd-text">{{ t('界面缩放') }}</span>
                  <span class="font-mono text-[11px] jd-subtle">{{ form.font_scale }}%</span>
                </span>
                <input
                  v-model.number="form.font_scale"
                  type="range"
                  min="80"
                  max="140"
                  step="5"
                  class="w-full accent-sky-400 rounded-md focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/60"
                />
                <span class="block text-[11px] jd-subtle">{{ t('同时影响文字与图标尺寸，保存后生效。') }}</span>
              </label>

              <label class="block space-y-1.5">
                <span class="font-medium jd-text">{{ t('界面语言') }}</span>
                <select
                  v-model="form.language"
                  class="w-full h-9 px-3 rounded-xl border text-[12px] jd-input transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/60"
                >
                  <option v-for="opt in languageOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </label>
            </section>

            <!-- 系统 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">{{ t('系统') }}</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <div
                v-for="row in systemToggles"
                :key="row.key"
                class="flex items-start justify-between gap-3 rounded-xl border jd-border jd-surface-soft px-3 py-2.5"
              >
                <div class="min-w-0 space-y-0.5">
                  <div class="font-medium jd-text">{{ row.label }}</div>
                  <p class="text-[11px] jd-muted leading-relaxed">{{ row.desc }}</p>
                  <p v-if="row.note" class="text-[11px] jd-subtle leading-relaxed break-all">{{ row.note }}</p>
                  <p v-if="row.error" class="text-[11px] text-rose-400 leading-relaxed break-all">
                    {{ row.error }}
                  </p>
                </div>
                <button
                  type="button"
                  role="switch"
                  :aria-checked="form[row.key]"
                  :title="row.label"
                  class="relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70 focus-visible:ring-offset-0"
                  :class="form[row.key] ? 'jd-accent-bg' : 'jd-surface-strong'"
                  @click="toggleSetting(row.key)"
                >
                  <span
                    class="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-transform"
                    :class="form[row.key] ? 'translate-x-4' : 'translate-x-0'"
                  />
                </button>
              </div>

              <div class="flex items-start justify-between gap-3 rounded-xl border jd-border jd-surface-soft px-3 py-2.5">
                <div class="min-w-0 space-y-0.5">
                  <div class="font-medium jd-text">{{ t('清空最近使用记录') }}</div>
                  <p class="text-[11px] jd-muted leading-relaxed">
                    {{ t('只删除「最近使用」分组中的记录，不影响项目本身。') }}
                  </p>
                </div>
                <button
                  type="button"
                  class="h-7 px-2.5 rounded-lg border text-[11px] shrink-0 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  :class="
                    confirmClearRecent
                      ? 'border-rose-400/60 bg-rose-500/15 text-rose-500 font-medium'
                      : 'jd-border jd-surface jd-muted jd-hover'
                  "
                  @click="handleClearRecent"
                >
                  {{ confirmClearRecent ? t('确认清空？') : t('清空记录') }}
                </button>
              </div>
            </section>

            <!-- 数据 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">{{ t('数据') }}</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <dl class="space-y-1.5">
                <div v-for="item in dataPaths" :key="item.label" class="flex items-baseline gap-2">
                  <dt class="w-14 shrink-0 text-[11px] jd-subtle">{{ item.label }}</dt>
                  <dd
                    class="min-w-0 flex-1 font-mono text-[11px] jd-muted select-text truncate"
                    :title="item.value"
                  >
                    {{ item.value }}
                  </dd>
                </div>
              </dl>

              <div class="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="emit('openBaseDir')"
                >
                  {{ t('打开数据目录') }}
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="emit('openConfigFile')"
                >
                  {{ t('打开 config.json') }}
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="emit('reloadConfig')"
                >
                  {{ t('重载配置') }}
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="emit('exportConfig')"
                >
                  {{ t('导出配置') }}
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="emit('importConfig')"
                >
                  {{ t('导入配置') }}
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="emit('resetWindowState')"
                >
                  {{ t('重置窗口位置') }}
                </button>
                <button
                  type="button"
                  class="col-span-2 h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="emit('openStartMenuImport')"
                >
                  {{ t('从开始菜单导入') }}
                </button>
              </div>

              <p class="text-[11px] jd-subtle leading-relaxed">
                {{ t('重载配置会从磁盘重新读取设置与分组；导入配置会用备份文件覆盖现有设置与分组。') }}
              </p>
            </section>

            <!-- 关于 / 更新 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">{{ t('关于') }}</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <dl class="space-y-1.5">
                <div v-for="item in aboutRows" :key="item.label" class="flex items-baseline gap-2">
                  <dt class="w-14 shrink-0 text-[11px] jd-subtle">{{ item.label }}</dt>
                  <dd
                    class="min-w-0 flex-1 font-mono text-[11px] jd-muted select-text truncate"
                    :title="item.value"
                  >
                    {{ item.value }}
                  </dd>
                </div>
              </dl>

              <div class="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70 disabled:opacity-50 disabled:pointer-events-none"
                  :disabled="updateChecking"
                  @click="handleCheckUpdates"
                >
                  {{ updateChecking ? t('检查中…') : t('检查更新') }}
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="handleOpenLogFile"
                >
                  {{ t('打开日志') }}
                </button>
                <button
                  type="button"
                  class="col-span-2 h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70 disabled:opacity-50 disabled:pointer-events-none"
                  :disabled="cleanupRunning"
                  @click="handleCleanupIcons"
                >
                  {{ cleanupRunning ? t('清理中…') : t('清理图标缓存') }}
                </button>
              </div>

              <div v-if="updateMessage || updateError" class="space-y-1.5">
                <p class="text-[11px] leading-relaxed break-all" :class="updateToneClass">
                  {{ updateError || updateMessage }}
                </p>
                <button
                  v-if="updateUrl"
                  type="button"
                  class="h-7 px-2.5 rounded-lg border border-sky-400/60 bg-sky-500/15 text-[11px] jd-accent font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="openUpdatePage"
                >
                  {{ t('打开下载页') }}
                </button>
              </div>

              <p v-if="cleanupMessage" class="text-[11px] text-emerald-400 leading-relaxed">
                {{ cleanupMessage }}
              </p>
              <p v-if="cleanupError" class="text-[11px] text-rose-400 leading-relaxed break-all">
                {{ cleanupError }}
              </p>
              <p v-if="logError" class="text-[11px] text-rose-400 leading-relaxed break-all">{{ logError }}</p>
            </section>
          </div>

          <!-- Outside changes and save failures stay visible above the fixed footer -->
          <div
            v-if="externalUpdate"
            class="mx-5 mb-2 flex items-center justify-between gap-2 rounded-xl border border-amber-400/50 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-500 shrink-0"
          >
            <span class="min-w-0 leading-relaxed">{{ t('配置已在外部更新，点击重载') }}</span>
            <button
              type="button"
              class="h-6 px-2 rounded-md border border-amber-400/60 text-[11px] font-medium shrink-0 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-400/70"
              @click="handleReloadDraft"
            >
              {{ t('重载') }}
            </button>
          </div>

          <div
            v-if="visibleError"
            class="mx-5 mb-2 rounded-xl border border-rose-400/50 bg-rose-500/10 px-3 py-2 text-[11px] text-rose-400 leading-relaxed break-all shrink-0"
          >
            {{ visibleError }}
          </div>

          <!-- Footer -->
          <div class="px-5 py-3 border-t jd-border flex items-center justify-between gap-3 shrink-0">
            <span class="text-[11px] truncate" :class="dirty ? 'jd-accent' : 'jd-subtle'">
              {{ dirty ? t('有未保存的修改') : t('设置已同步') }}
            </span>
            <div class="flex items-center gap-2 shrink-0">
              <button
                type="button"
                class="px-3.5 h-8 rounded-xl border text-[11px] transition-colors jd-border jd-surface jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                @click="handleClose"
              >
                {{ t('取消') }}
              </button>
              <button
                type="submit"
                :disabled="saving || !form.hotkey.trim()"
                class="px-4 h-8 rounded-xl text-white text-[11px] font-medium transition-all disabled:opacity-50 disabled:cursor-not-allowed focus:outline-none focus-visible:ring-2 focus-visible:ring-white/70"
                :class="
                  dirty
                    ? 'bg-sky-500 hover:bg-sky-400 shadow-md shadow-sky-500/25 ring-2 ring-sky-400/30'
                    : 'bg-sky-500/70 hover:bg-sky-500'
                "
              >
                <span v-if="saving" class="inline-flex items-center gap-1.5">
                  <svg class="w-3 h-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                    <path stroke-linecap="round" d="M12 3a9 9 0 1 0 9 9" />
                  </svg>
                  {{ t('保存中…') }}
                </span>
                <span v-else>{{ t('保存') }}</span>
              </button>
            </div>
          </div>
        </form>
      </div>
    </Transition>
  </Teleport>
</template>
