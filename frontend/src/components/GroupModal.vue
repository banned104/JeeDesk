<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { registerTranslations, t } from '../i18n';

registerTranslations({
  '新建分组': 'New group',
  '重命名分组': 'Rename group',
  '关闭': 'Close',
  '分组名称': 'Group name',
  '例如：办公软件、开发工具': 'e.g. Office apps, dev tools',
  '取消': 'Cancel',
  '确定': 'OK',
  '提交中…': 'Submitting…',
});

const props = defineProps<{
  visible: boolean;
  mode: 'add' | 'rename';
  initialName?: string;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'submit', name: string): void;
}>();

const name = ref('');
const inputRef = ref<HTMLInputElement | null>(null);

// --- dialog focus management ----------------------------------------------

const titleId = 'group-modal-title';
const dialogRef = ref<HTMLElement | null>(null);
// Element that had focus before the dialog opened, restored once it closes.
let previousFocus: HTMLElement | null = null;

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusableElements(): HTMLElement[] {
  const root = dialogRef.value;
  if (!root) return [];
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (el) => el.getClientRects().length > 0
  );
}

async function focusFirstField() {
  await nextTick();
  const target = inputRef.value ?? focusableElements()[0] ?? dialogRef.value;
  target?.focus();
}

// Simple focus trap: Tab / Shift+Tab cycle inside the dialog instead of reaching the page behind it.
function handleTabKey(event: KeyboardEvent) {
  if (event.key !== 'Tab') return;
  const root = dialogRef.value;
  if (!root) return;
  const elements = focusableElements();
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
  if (target && document.contains(target)) {
    target.focus();
  }
}

// --- submit state ---------------------------------------------------------

// Guards against creating the same group twice when the confirm button is pressed repeatedly.
const submitting = ref(false);
let releaseTimer: number | null = null;

function releaseSubmit() {
  if (releaseTimer !== null) {
    window.clearTimeout(releaseTimer);
    releaseTimer = null;
  }
  submitting.value = false;
}

watch(
  () => [props.visible, props.mode, props.initialName] as const,
  async ([visible]) => {
    if (!visible) {
      releaseSubmit();
      restoreFocus();
      return;
    }
    rememberFocus();
    releaseSubmit();
    name.value = props.mode === 'rename' ? props.initialName || '' : '';
    await focusFirstField();
    inputRef.value?.select();
  }
);

function handleSubmit() {
  const trimmed = name.value.trim();
  if (!trimmed || submitting.value) return;
  submitting.value = true;
  emit('submit', trimmed);
  // The parent owns the request: it closes this dialog on success and reports its own error on
  // failure. The timer is a safety valve so a failed save never leaves the button stuck.
  releaseTimer = window.setTimeout(releaseSubmit, 5000);
}

// Closing or unmounting must clear the pending timer and hand focus back to the page behind.
onBeforeUnmount(() => {
  releaseSubmit();
  restoreFocus();
});
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
          ref="dialogRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="w-full max-w-sm max-h-[90vh] rounded-2xl glass-modal border jd-border-strong shadow-2xl overflow-hidden flex flex-col"
          @keydown="handleTabKey"
        >
          <div class="px-5 py-3.5 border-b jd-border flex items-center justify-between shrink-0">
            <h2 :id="titleId" class="text-sm font-semibold jd-text">
              {{ mode === 'add' ? t('新建分组') : t('重命名分组') }}
            </h2>
            <button
              type="button"
              class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover transition-colors"
              :aria-label="t('关闭')"
              :title="t('关闭')"
              @click="emit('close')"
            >
              <svg
                class="w-4 h-4"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                aria-hidden="true"
              >
                <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <form
            class="flex-1 min-h-0 overflow-y-auto custom-scrollbar p-5 space-y-4 text-xs"
            :aria-busy="submitting"
            @submit.prevent="handleSubmit"
          >
            <label class="block space-y-1.5">
              <span class="jd-muted font-medium">{{ t('分组名称') }}</span>
              <input
                ref="inputRef"
                v-model="name"
                type="text"
                required
                :placeholder="t('例如：办公软件、开发工具')"
                class="w-full h-9 px-3 rounded-xl jd-input border transition-colors select-text"
              />
            </label>

            <div class="flex items-center justify-end gap-2">
              <button
                type="button"
                class="px-4 h-8 rounded-xl jd-surface jd-muted jd-hover border jd-border transition-colors"
                @click="emit('close')"
              >
                {{ t('取消') }}
              </button>
              <button
                type="submit"
                :disabled="!name.trim() || submitting"
                :aria-busy="submitting"
                class="px-4 h-8 rounded-xl bg-sky-500/85 hover:bg-sky-500 disabled:opacity-40 disabled:cursor-not-allowed text-white font-medium shadow-sm transition-colors"
              >
                {{ submitting ? t('提交中…') : t('确定') }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
