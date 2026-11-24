<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import type {
  AppInfo,
  ClickMode,
  HotkeyStatus,
  Language,
  MouseTrigger,
  Settings,
  ThemeMode,
} from '../types/config';

const props = defineProps<{
  visible: boolean;
  settings: Settings;
  hotkeyStatus?: HotkeyStatus;
  appInfo?: AppInfo | null;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
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

const SETTINGS_FIELDS = [
  'hotkey',
  'mouse_trigger',
  'mouse_swallow',
  'hide_on_launch',
  'hide_on_blur',
  'hide_on_esc',
  'click_mode',
  'auto_start',
  'start_hidden',
  'theme',
  'language',
  'show_recent',
  'window_opacity',
  'window_width',
  'window_height',
  'window_x',
  'window_y',
  'font_scale',
] as const;

/** Every field is copied explicitly so a new Settings field cannot be silently dropped. */
function copySettings(source: Settings): Settings {
  return {
    hotkey: source.hotkey ?? '',
    mouse_trigger: source.mouse_trigger ?? 'none',
    mouse_swallow: source.mouse_swallow ?? true,
    hide_on_launch: source.hide_on_launch ?? true,
    hide_on_blur: source.hide_on_blur ?? true,
    hide_on_esc: source.hide_on_esc ?? true,
    click_mode: source.click_mode ?? 'double',
    auto_start: source.auto_start ?? false,
    start_hidden: source.start_hidden ?? false,
    theme: source.theme ?? 'dark',
    language: source.language ?? 'zh',
    show_recent: source.show_recent ?? true,
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
  return SETTINGS_FIELDS.every((field) => a[field] === b[field]);
}

const form = reactive<Settings>(normalizeSettings(props.settings));
// Snapshot of the last applied settings: the form is dirty once it drifts away from it.
const baseline = ref<Settings>(normalizeSettings(props.settings));

const dirty = computed(() => !sameSettings(form, baseline.value));

const recording = ref(false);
const hotkeyHint = ref('');
const saving = ref(false);
const confirmClearRecent = ref(false);

let saveTimer: number | undefined;
let confirmTimer: number | undefined;

const mouseTriggerOptions: Array<{ value: MouseTrigger; label: string }> = [
  { value: 'none', label: '关闭' },
  { value: 'middle', label: '中键' },
  { value: 'xbutton1', label: 'XButton1（后退键）' },
  { value: 'xbutton2', label: 'XButton2（前进键）' },
];

const clickModeOptions: Array<{ value: ClickMode; label: string; desc: string }> = [
  { value: 'single', label: '单击启动', desc: '单击即启动项目' },
  { value: 'double', label: '双击启动', desc: '双击才启动，减少误触' },
];

const themeOptions: Array<{ value: ThemeMode; label: string }> = [
  { value: 'dark', label: '深色' },
  { value: 'light', label: '浅色' },
  { value: 'system', label: '跟随系统' },
];

const languageOptions: Array<{ value: Language; label: string }> = [
  { value: 'zh', label: '中文' },
  { value: 'en', label: 'English' },
];

const triggerToggles = computed<ToggleRow[]>(() => [
  {
    key: 'mouse_swallow',
    label: '吞掉触发键',
    desc: '拦截触发键，不再传给其它程序；关闭后该鼠标键仍会传给其他程序。',
    note: form.mouse_trigger === 'none' ? '当前未启用鼠标触发，此选项暂不生效。' : undefined,
  },
]);

const launchToggles: ToggleRow[] = [
  { key: 'hide_on_launch', label: '隐藏窗口', desc: '点击启动项目后自动隐藏面板。' },
  { key: 'hide_on_blur', label: '失焦隐藏', desc: '切换到其他程序时自动隐藏（拖动标题栏不会触发）。' },
  { key: 'hide_on_esc', label: 'Esc 隐藏', desc: '面板聚焦时按 Esc 快速隐藏到系统托盘。' },
];

const systemToggles = computed<ToggleRow[]>(() => {
  const exePath = props.appInfo?.exe_path?.trim() ?? '';
  const autoStartError = props.appInfo?.auto_start_error?.trim() ?? '';
  return [
    {
      key: 'auto_start',
      label: '开机自启',
      desc: '登录 Windows 后自动启动 JeeDesk。',
      note: exePath ? `将注册：${exePath}` : '尚未获取到程序路径。',
      error: autoStartError,
    },
    {
      key: 'start_hidden',
      label: '启动时隐藏窗口',
      desc: '随系统启动时直接隐藏到托盘，不弹出面板。',
    },
    {
      key: 'show_recent',
      label: '显示「最近使用」分组',
      desc: '在侧边栏顶部展示最近启动过的项目。',
    },
  ];
});

const dataPaths = computed<Array<{ label: string; value: string }>>(() => {
  const info = props.appInfo;
  return [
    { label: '配置路径', value: info?.config_path?.trim() || '未获取' },
    { label: '数据目录', value: info?.base_dir?.trim() || '未获取' },
    { label: '备份路径', value: info?.config_backup?.trim() || '未获取' },
  ];
});

const hotkeyState = computed<HotkeyState>(() => {
  const status = props.hotkeyStatus;
  const current = form.hotkey.trim();
  if (status?.registered && status.hotkey === current) {
    return { tone: 'ok', text: `全局快捷键 ${status.hotkey} 已生效` };
  }
  if (status?.error && status.hotkey === current) {
    return {
      tone: 'error',
      text: `注册失败：${status.error}（可能与其它软件冲突，换一个组合键试试）`,
    };
  }
  if (current !== (props.settings.hotkey ?? '').trim()) {
    return { tone: 'idle', text: '修改尚未保存，保存后生效' };
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

  const mainKey = hotkeyKeyFromEvent(event);
  // Modifier-only presses and unmapped keys keep the recorder open instead of producing a partial spec.
  if (!mainKey) return;

  const modifiers = modifierParts(event);
  // A bare letter would hijack that key system-wide, so Ctrl/Alt/Win is required (Shift alone is not).
  if (!modifiers.some((part) => part !== 'Shift')) {
    hotkeyHint.value = '需要 Ctrl、Alt 或 Win 中的至少一个修饰键';
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
  if (saveTimer !== undefined) {
    window.clearTimeout(saveTimer);
    saveTimer = undefined;
  }
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
  clearTimers();
}

function handleSave() {
  if (saving.value) return;
  const payload = copySettings(form);
  payload.hotkey = payload.hotkey.trim();
  if (!payload.hotkey) {
    hotkeyHint.value = '快捷键不能为空';
    recording.value = false;
    return;
  }
  saving.value = true;
  recording.value = false;
  hotkeyHint.value = '';
  // Keep the form identical to what is being saved, so the parent's echo can refresh the baseline.
  form.hotkey = payload.hotkey;
  emit('save', payload);
  // The parent owns the save round-trip, so the button stays busy briefly to block double submits.
  if (saveTimer !== undefined) window.clearTimeout(saveTimer);
  saveTimer = window.setTimeout(() => {
    saving.value = false;
    saveTimer = undefined;
  }, 900);
}

function handleClose() {
  recording.value = false;
  hotkeyHint.value = '';
  confirmClearRecent.value = false;
  emit('close');
}

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

// Opening always re-reads the latest settings: unsaved edits from a previous session are dropped.
watch(
  () => props.visible,
  (visible) => {
    resetTransientState();
    if (visible) applySettings(props.settings);
  },
  { immediate: true }
);

// While the user has unsaved edits, incoming settings must not overwrite what they typed. The only
// exception is the parent echoing back exactly what was just saved: then the baseline catches up
// and the "unsaved" hint clears after a successful save.
watch(
  () => props.settings,
  (next) => {
    if (!props.visible) return;
    if (dirty.value && !sameSettings(form, normalizeSettings(next))) return;
    applySettings(next);
  },
  { deep: true }
);

onMounted(() => {
  window.addEventListener('keydown', handleRecordingKeydown, true);
});

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleRecordingKeydown, true);
  clearTimers();
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
          class="w-full max-w-[580px] max-h-[calc(100vh-2rem)] rounded-2xl glass-modal border jd-border-strong shadow-2xl flex flex-col overflow-hidden"
          @submit.prevent="handleSave"
        >
          <!-- Header -->
          <div class="px-5 py-3.5 border-b jd-border flex items-center justify-between gap-3 shrink-0">
            <div class="min-w-0">
              <h2 class="text-sm font-semibold jd-text">偏好设置</h2>
              <p class="text-[11px] jd-subtle mt-0.5">修改后点击保存才会写入 config.json</p>
            </div>
            <button
              type="button"
              title="关闭"
              class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover transition-colors shrink-0"
              @click="handleClose"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Scrollable body -->
          <div class="flex-1 min-h-0 overflow-y-auto custom-scrollbar px-5 py-4 space-y-5 text-xs">
            <!-- 呼出 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">呼出</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <div class="space-y-1.5">
                <div class="flex items-center justify-between gap-2">
                  <span class="font-medium jd-text">全局快捷键</span>
                  <span class="text-[11px] jd-subtle">点击后直接按下组合键</span>
                </div>
                <button
                  type="button"
                  class="w-full h-9 px-3 rounded-xl border text-left flex items-center justify-between gap-2 transition-colors"
                  :class="recording ? 'border-sky-400 bg-sky-500/10 ring-1 ring-sky-400/40' : 'jd-input'"
                  @click="toggleRecording"
                >
                  <span class="font-mono text-[12px] truncate" :class="recording ? 'jd-accent' : 'jd-text'">
                    {{ recording ? '请按下组合键…' : form.hotkey || '未设置' }}
                  </span>
                  <span class="text-[11px] shrink-0" :class="recording ? 'jd-accent' : 'jd-subtle'">
                    {{ recording ? 'Esc 取消' : '录制' }}
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
                <span class="font-medium jd-text">鼠标触发</span>
                <select
                  v-model="form.mouse_trigger"
                  class="w-full h-9 px-3 rounded-xl border text-[12px] jd-input transition-colors"
                >
                  <option v-for="opt in mouseTriggerOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
                <span class="block text-[11px] jd-subtle">在任意窗口按下该鼠标键即可呼出面板。</span>
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
                  class="relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors"
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
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">启动</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <div class="space-y-1.5">
                <span class="font-medium jd-text">启动方式</span>
                <div class="grid grid-cols-2 gap-2" role="radiogroup" aria-label="启动方式">
                  <button
                    v-for="opt in clickModeOptions"
                    :key="opt.value"
                    type="button"
                    role="radio"
                    :aria-checked="form.click_mode === opt.value"
                    class="px-3 py-2 rounded-xl border text-left transition-colors"
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
                  class="relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors"
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
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">外观</h3>
                <span class="flex-1 h-px jd-surface-strong" />
              </div>

              <div class="space-y-1.5">
                <span class="font-medium jd-text">主题</span>
                <div class="grid grid-cols-3 gap-2" role="radiogroup" aria-label="主题">
                  <button
                    v-for="opt in themeOptions"
                    :key="opt.value"
                    type="button"
                    role="radio"
                    :aria-checked="form.theme === opt.value"
                    class="h-8 rounded-xl border text-[11px] transition-colors"
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
                  <span class="font-medium jd-text">窗口不透明度</span>
                  <span class="font-mono text-[11px] jd-subtle">{{ form.window_opacity }}%</span>
                </span>
                <input
                  v-model.number="form.window_opacity"
                  type="range"
                  min="50"
                  max="100"
                  step="1"
                  class="w-full accent-sky-400"
                />
                <span class="block text-[11px] jd-subtle">数值越低越通透，50% 为面板最透明的极限。</span>
              </label>

              <label class="block space-y-1.5">
                <span class="flex items-center justify-between gap-2">
                  <span class="font-medium jd-text">界面缩放</span>
                  <span class="font-mono text-[11px] jd-subtle">{{ form.font_scale }}%</span>
                </span>
                <input
                  v-model.number="form.font_scale"
                  type="range"
                  min="80"
                  max="140"
                  step="5"
                  class="w-full accent-sky-400"
                />
                <span class="block text-[11px] jd-subtle">同时影响文字与图标尺寸，保存后生效。</span>
              </label>

              <label class="block space-y-1.5">
                <span class="font-medium jd-text">界面语言</span>
                <select
                  v-model="form.language"
                  class="w-full h-9 px-3 rounded-xl border text-[12px] jd-input transition-colors"
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
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">系统</h3>
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
                  class="relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors"
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
                  <div class="font-medium jd-text">清空最近使用记录</div>
                  <p class="text-[11px] jd-muted leading-relaxed">
                    只删除「最近使用」分组中的记录，不影响项目本身。
                  </p>
                </div>
                <button
                  type="button"
                  class="h-7 px-2.5 rounded-lg border text-[11px] shrink-0 transition-colors"
                  :class="
                    confirmClearRecent
                      ? 'border-rose-400/60 bg-rose-500/15 text-rose-500 font-medium'
                      : 'jd-border jd-surface jd-muted jd-hover'
                  "
                  @click="handleClearRecent"
                >
                  {{ confirmClearRecent ? '确认清空？' : '清空记录' }}
                </button>
              </div>
            </section>

            <!-- 数据 -->
            <section class="space-y-2.5">
              <div class="flex items-center gap-2">
                <h3 class="text-[11px] font-semibold tracking-wide jd-accent">数据</h3>
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
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover"
                  @click="emit('openBaseDir')"
                >
                  打开数据目录
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover"
                  @click="emit('openConfigFile')"
                >
                  打开 config.json
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover"
                  @click="emit('reloadConfig')"
                >
                  重载配置
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover"
                  @click="emit('exportConfig')"
                >
                  导出配置
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover"
                  @click="emit('importConfig')"
                >
                  导入配置
                </button>
                <button
                  type="button"
                  class="h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover"
                  @click="emit('resetWindowState')"
                >
                  重置窗口位置
                </button>
                <button
                  type="button"
                  class="col-span-2 h-8 px-3 rounded-xl border text-[11px] transition-colors jd-border jd-surface-soft jd-muted jd-hover"
                  @click="emit('openStartMenuImport')"
                >
                  从开始菜单导入
                </button>
              </div>

              <p class="text-[11px] jd-subtle leading-relaxed">
                重载配置会从磁盘重新读取设置与分组；导入配置会用备份文件覆盖现有设置与分组。
              </p>
            </section>
          </div>

          <!-- Footer -->
          <div class="px-5 py-3 border-t jd-border flex items-center justify-between gap-3 shrink-0">
            <span class="text-[11px] truncate" :class="dirty ? 'jd-accent' : 'jd-subtle'">
              {{ dirty ? '有未保存的修改' : '设置已同步' }}
            </span>
            <div class="flex items-center gap-2 shrink-0">
              <button
                type="button"
                class="px-3.5 h-8 rounded-xl border text-[11px] transition-colors jd-border jd-surface jd-muted jd-hover"
                @click="handleClose"
              >
                取消
              </button>
              <button
                type="submit"
                :disabled="saving || !form.hotkey.trim()"
                class="px-4 h-8 rounded-xl text-white text-[11px] font-medium transition-all disabled:opacity-50 disabled:cursor-not-allowed"
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
                  保存中…
                </span>
                <span v-else>保存</span>
              </button>
            </div>
          </div>
        </form>
      </div>
    </Transition>
  </Teleport>
</template>
