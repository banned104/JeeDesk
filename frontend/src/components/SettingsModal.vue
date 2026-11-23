<script setup lang="ts">
import { reactive, ref, watch } from 'vue';
import type { HotkeyStatus, MouseTrigger, Settings } from '../types/config';

const props = defineProps<{
  visible: boolean;
  settings: Settings;
  hotkeyStatus?: HotkeyStatus;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'save', settings: Settings): void;
  (e: 'openConfigFile'): void;
  (e: 'reloadConfig'): void;
}>();

const form = reactive<Settings>({
  hotkey: 'Alt+G',
  mouse_trigger: 'none',
  hide_on_launch: true,
  hide_on_blur: true,
  hide_on_esc: true,
});

const recordingHotkey = ref(false);

const mouseTriggerOptions: Array<{ value: MouseTrigger; label: string }> = [
  { value: 'none', label: '关闭' },
  { value: 'middle', label: '鼠标中键' },
  { value: 'xbutton1', label: '侧键后退 / XButton1' },
  { value: 'xbutton2', label: '侧键前进 / XButton2' },
];

watch(
  () => [props.visible, props.settings] as const,
  ([visible, settings]) => {
    if (!visible) {
      recordingHotkey.value = false;
      return;
    }
    form.hotkey = settings.hotkey;
    form.mouse_trigger = settings.mouse_trigger;
    form.hide_on_launch = settings.hide_on_launch;
    form.hide_on_blur = settings.hide_on_blur;
    form.hide_on_esc = settings.hide_on_esc ?? true;
  },
  { immediate: true, deep: true }
);

function normalizeKeyName(event: KeyboardEvent): string | null {
  const key = event.key;
  if (['Control', 'Alt', 'Shift', 'Meta'].includes(key)) {
    return null;
  }
  if (key === ' ') return 'Space';
  if (/^F([1-9]|1[0-2])$/i.test(key)) return key.toUpperCase();
  if (key.length === 1) return key.toUpperCase();
  const keyMap: Record<string, string> = {
    ArrowUp: 'Up',
    ArrowDown: 'Down',
    ArrowLeft: 'Left',
    ArrowRight: 'Right',
    Escape: 'Esc',
    Enter: 'Enter',
    Tab: 'Tab',
    Backspace: 'Backspace',
    Delete: 'Delete',
    Insert: 'Insert',
    Home: 'Home',
    End: 'End',
    PageUp: 'PageUp',
    PageDown: 'PageDown',
  };
  return keyMap[key] || key;
}

function handleHotkeyKeydown(event: KeyboardEvent) {
  const hasModifier = event.ctrlKey || event.altKey || event.metaKey;
  const isFunctionKey = /^F([1-9]|1[0-2])$/i.test(event.key);

  if (recordingHotkey.value && event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    recordingHotkey.value = false;
    return;
  }

  // Capture combo automatically if modifiers are held, F1-F12 is pressed, or recorder mode is active
  if (recordingHotkey.value || hasModifier || isFunctionKey) {
    event.preventDefault();
    event.stopPropagation();

    const mainKey = normalizeKeyName(event);
    if (!mainKey) {
      return;
    }

    const parts: string[] = [];
    if (event.ctrlKey) parts.push('Ctrl');
    if (event.altKey) parts.push('Alt');
    if (event.shiftKey) parts.push('Shift');
    if (event.metaKey) parts.push('Win');
    parts.push(mainKey);

    form.hotkey = parts.join('+');
    recordingHotkey.value = false;
  }
}

function handleSave() {
  emit('save', {
    hotkey: form.hotkey.trim(),
    mouse_trigger: form.mouse_trigger,
    hide_on_launch: form.hide_on_launch,
    hide_on_blur: form.hide_on_blur,
    hide_on_esc: form.hide_on_esc,
  });
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="visible"
        class="fixed inset-0 z-40 bg-black/50 backdrop-blur-[2px] flex items-center justify-center p-4"
        @mousedown.self="emit('close')"
      >
        <div
          class="w-full max-w-md max-h-[90vh] rounded-2xl glass-modal border border-white/15 shadow-2xl flex flex-col overflow-hidden"
        >
          <!-- Header -->
          <div class="px-5 py-3.5 border-b border-white/10 flex items-center justify-between shrink-0">
            <h2 class="text-sm font-semibold text-white">偏好设置</h2>
            <button
              type="button"
              class="w-7 h-7 rounded-lg flex items-center justify-center text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
              @click="emit('close')"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Content -->
          <div class="p-5 space-y-4 text-xs overflow-y-auto">
            <!-- Hotkey Recorder -->
            <div class="space-y-1.5">
              <div class="flex items-center justify-between">
                <span class="text-slate-300 font-medium">全局唤起快捷键</span>
                <span class="text-[11px] text-slate-400">聚焦后直接按下组合键，或手动输入</span>
              </div>
              <div class="flex gap-2">
                <input
                  v-model="form.hotkey"
                  type="text"
                  :placeholder="recordingHotkey ? '请按下快捷键组合 (如 Alt+G)...' : '例如 Alt+G / Ctrl+Shift+Space'"
                  class="flex-1 h-9 px-3 rounded-xl bg-white/[0.06] border text-slate-100 placeholder:text-slate-500 focus:outline-none transition-colors"
                  :class="
                    recordingHotkey
                      ? 'border-sky-400 bg-sky-500/10 ring-1 ring-sky-400/40'
                      : 'border-white/10 focus:border-sky-400/60'
                  "
                  @keydown="handleHotkeyKeydown"
                  @blur="recordingHotkey = false"
                />
                <button
                  type="button"
                  class="px-3 h-9 rounded-xl border transition-colors shrink-0"
                  :class="
                    recordingHotkey
                      ? 'bg-sky-500/25 border-sky-400/60 text-sky-200'
                      : 'bg-white/10 hover:bg-white/15 border-white/10 text-slate-200'
                  "
                  @click="recordingHotkey = !recordingHotkey"
                >
                  {{ recordingHotkey ? '按键盘...' : '录制' }}
                </button>
              </div>
              <div
                v-if="hotkeyStatus?.registered"
                class="flex items-center gap-1.5 text-[11px] text-emerald-300/90 pt-0.5"
              >
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 shrink-0" />
                <span>全局快捷键 {{ hotkeyStatus.hotkey }} 已生效</span>
              </div>
              <div
                v-else-if="hotkeyStatus?.error"
                class="flex items-start gap-1.5 text-[11px] text-rose-300/95 pt-0.5"
              >
                <span class="w-1.5 h-1.5 rounded-full bg-rose-400 shrink-0 mt-1" />
                <span>注册失败: {{ hotkeyStatus.error }}（可能与其他软件冲突，请更换快捷键）</span>
              </div>
            </div>

            <!-- Mouse Trigger -->
            <div class="space-y-2">
              <span class="block text-slate-300 font-medium">鼠标唤起触发</span>
              <div class="grid grid-cols-2 gap-2">
                <button
                  v-for="opt in mouseTriggerOptions"
                  :key="opt.value"
                  type="button"
                  class="h-9 px-3 rounded-xl border text-left transition-all flex items-center justify-between"
                  :class="
                    form.mouse_trigger === opt.value
                      ? 'bg-sky-500/20 border-sky-400/60 text-white font-medium'
                      : 'bg-white/[0.05] hover:bg-white/10 border-white/10 text-slate-300'
                  "
                  @click="form.mouse_trigger = opt.value"
                >
                  <span class="truncate">{{ opt.label }}</span>
                  <span
                    v-if="form.mouse_trigger === opt.value"
                    class="w-2 h-2 rounded-full bg-sky-400 shrink-0"
                  />
                </button>
              </div>
            </div>

            <!-- Hide / Close Behavior -->
            <div class="space-y-2 pt-1">
              <span class="block text-slate-300 font-medium">窗口隐藏 / 关闭方式</span>
              <div class="space-y-2">
                <label
                  class="flex items-center justify-between gap-3 px-3.5 py-2.5 rounded-xl bg-white/[0.04] border border-white/10 cursor-pointer hover:bg-white/[0.07] transition-colors"
                >
                  <div>
                    <div class="text-slate-200 font-medium">失去焦点自动隐藏</div>
                    <div class="text-[11px] text-slate-400">光标移出窗口且切换到其他应用时隐藏（拖动顶部栏不会触发）</div>
                  </div>
                  <input v-model="form.hide_on_blur" type="checkbox" class="sr-only peer" />
                  <div
                    class="w-9 h-5 rounded-full bg-white/15 peer-checked:bg-sky-500 relative transition-colors shrink-0 after:content-[''] after:absolute after:top-0.5 after:left-0.5 after:w-4 after:h-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4"
                  />
                </label>

                <label
                  class="flex items-center justify-between gap-3 px-3.5 py-2.5 rounded-xl bg-white/[0.04] border border-white/10 cursor-pointer hover:bg-white/[0.07] transition-colors"
                >
                  <div>
                    <div class="text-slate-200 font-medium">按 Esc 键隐藏</div>
                    <div class="text-[11px] text-slate-400">主面板聚焦时按 Esc 键快速隐藏至托盘</div>
                  </div>
                  <input v-model="form.hide_on_esc" type="checkbox" class="sr-only peer" />
                  <div
                    class="w-9 h-5 rounded-full bg-white/15 peer-checked:bg-sky-500 relative transition-colors shrink-0 after:content-[''] after:absolute after:top-0.5 after:left-0.5 after:w-4 after:h-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4"
                  />
                </label>

                <label
                  class="flex items-center justify-between gap-3 px-3.5 py-2.5 rounded-xl bg-white/[0.04] border border-white/10 cursor-pointer hover:bg-white/[0.07] transition-colors"
                >
                  <div>
                    <div class="text-slate-200 font-medium">启动程序后自动隐藏</div>
                    <div class="text-[11px] text-slate-400">点击启动快捷项后自动隐藏面板</div>
                  </div>
                  <input v-model="form.hide_on_launch" type="checkbox" class="sr-only peer" />
                  <div
                    class="w-9 h-5 rounded-full bg-white/15 peer-checked:bg-sky-500 relative transition-colors shrink-0 after:content-[''] after:absolute after:top-0.5 after:left-0.5 after:w-4 after:h-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4"
                  />
                </label>
              </div>
              <p class="text-[11px] text-slate-400/90 px-0.5">
                提示：点击右上角 × 按钮可随时隐藏至系统托盘。
              </p>
            </div>

            <!-- Footer Actions -->
            <div class="pt-3 border-t border-white/10 flex flex-wrap items-center justify-between gap-2">
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  class="px-3 h-8 rounded-xl bg-white/[0.06] hover:bg-white/10 border border-white/10 text-slate-200 transition-colors"
                  @click="emit('openConfigFile')"
                >
                  打开 config.json
                </button>
                <button
                  type="button"
                  class="px-3 h-8 rounded-xl bg-white/[0.06] hover:bg-white/10 border border-white/10 text-slate-200 transition-colors"
                  @click="emit('reloadConfig')"
                >
                  重新加载配置
                </button>
              </div>

              <div class="flex items-center gap-2">
                <button
                  type="button"
                  class="px-3.5 h-8 rounded-xl bg-white/[0.06] hover:bg-white/10 text-slate-300 border border-white/10 transition-colors"
                  @click="emit('close')"
                >
                  取消
                </button>
                <button
                  type="button"
                  class="px-4 h-8 rounded-xl bg-sky-500/85 hover:bg-sky-500 text-white font-medium shadow-sm transition-colors"
                  @click="handleSave"
                >
                  保存
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
