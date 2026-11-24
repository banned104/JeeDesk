<script setup lang="ts">
import type { ToastMessage } from '../types/config';
import { registerTranslations, t } from '../i18n';

registerTranslations({
  '关闭提示': 'Dismiss notification',
});

defineProps<{
  toasts: ToastMessage[];
}>();

const emit = defineEmits<{
  (e: 'dismiss', id: number): void;
}>();

// Toasts keep a state-coloured tint + border, but the text follows the theme so light mode stays readable.
function badgeClass(type: ToastMessage['type']): string {
  switch (type) {
    case 'success':
      return 'border-emerald-400/40 bg-emerald-500/15 jd-text';
    case 'warning':
      return 'border-amber-400/40 bg-amber-500/15 jd-text';
    case 'error':
      return 'border-rose-400/50 bg-rose-500/20 jd-text';
    default:
      return 'border-sky-400/40 bg-sky-500/15 jd-text';
  }
}
</script>

<template>
  <Teleport to="body">
    <!-- aria-live makes screen readers announce new toasts; errors additionally get role="alert". -->
    <div
      class="fixed bottom-3 right-3 z-50 flex flex-col gap-2 max-w-xs pointer-events-none"
      aria-live="polite"
      aria-relevant="additions text"
    >
      <TransitionGroup name="pop">
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="badgeClass(toast.type)"
          :role="toast.type === 'error' ? 'alert' : undefined"
          class="pointer-events-auto px-3.5 py-2 rounded-xl border backdrop-blur-md shadow-xl text-xs flex items-center gap-2.5"
        >
          <span class="flex-1 leading-snug break-words select-text">{{ toast.message }}</span>
          <button
            type="button"
            class="shrink-0 rounded p-0.5 jd-subtle jd-hover transition-colors"
            :aria-label="t('关闭提示')"
            :title="t('关闭提示')"
            @click="emit('dismiss', toast.id)"
          >
            <svg
              class="w-3.5 h-3.5"
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
      </TransitionGroup>
    </div>
  </Teleport>
</template>
