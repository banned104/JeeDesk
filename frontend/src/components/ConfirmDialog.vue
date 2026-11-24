<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { registerTranslations, t } from '../i18n';

registerTranslations({
  确认操作: 'Confirm action',
  确认: 'Confirm',
  取消: 'Cancel',
});

const props = defineProps<{
  visible: boolean;
  title?: string;
  message: string;
  confirmLabel?: string;
  /** Marks a destructive action so the confirm button matches the danger styling. */
  danger?: boolean;
  busy?: boolean;
}>();

const emit = defineEmits<{
  (e: 'confirm'): void;
  (e: 'close'): void;
}>();

const titleId = `confirm-title-${Math.random().toString(36).slice(2, 8)}`;
const dialogRef = ref<HTMLElement | null>(null);
const confirmRef = ref<HTMLButtonElement | null>(null);
let previouslyFocused: HTMLElement | null = null;

function rememberFocus() {
  previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
}

function restoreFocus() {
  previouslyFocused?.focus?.();
  previouslyFocused = null;
}

function focusDialog() {
  void nextTick(() => {
    if (props.busy) {
      dialogRef.value?.focus();
      return;
    }
    // Cancel-first ordering is safer for a destructive action: Enter on the dialog must not delete.
    confirmRef.value?.focus();
  });
}

/** Keeps Tab inside the dialog, the same way the other modals do. */
function handleTabKey(event: KeyboardEvent) {
  if (event.key !== 'Tab') return;
  const root = dialogRef.value;
  if (!root) return;
  const focusable = Array.from(
    root.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')
  ).filter((el) => el.offsetParent !== null);
  if (focusable.length === 0) return;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      rememberFocus();
      focusDialog();
    } else {
      restoreFocus();
    }
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  if (props.visible) {
    restoreFocus();
  }
});
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="visible"
        class="fixed inset-0 z-[70] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4"
        @click.self="emit('close')"
      >
        <div
          ref="dialogRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="glass-modal w-full max-w-sm rounded-2xl border jd-border-strong p-5 flex flex-col gap-4 focus:outline-none"
          @keydown.tab="handleTabKey"
          @keydown.esc.stop.prevent="emit('close')"
        >
          <h2 :id="titleId" class="text-sm font-semibold jd-text">
            {{ title || t('确认操作') }}
          </h2>
          <p class="text-xs leading-5 jd-muted whitespace-pre-line select-text">{{ message }}</p>
          <div class="flex justify-end gap-2">
            <button
              type="button"
              class="h-8 px-3 rounded-lg text-xs jd-muted jd-hover border jd-border transition-colors"
              :disabled="busy"
              @click="emit('close')"
            >
              {{ t('取消') }}
            </button>
            <button
              ref="confirmRef"
              type="button"
              class="h-8 px-3 rounded-lg text-xs font-semibold transition-colors"
              :class="danger
                ? 'bg-rose-500/25 text-rose-200 border border-rose-400/40 hover:bg-rose-500/35'
                : 'jd-accent-bg'"
              :disabled="busy"
              @click="emit('confirm')"
            >
              {{ confirmLabel || t('确认') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
