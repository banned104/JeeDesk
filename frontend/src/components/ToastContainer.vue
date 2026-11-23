<script setup lang="ts">
import type { ToastMessage } from '../types/config';

defineProps<{
  toasts: ToastMessage[];
}>();

const emit = defineEmits<{
  (e: 'dismiss', id: number): void;
}>();

function badgeClass(type: ToastMessage['type']): string {
  switch (type) {
    case 'success':
      return 'border-emerald-400/30 bg-emerald-950/80 text-emerald-100';
    case 'warning':
      return 'border-amber-400/30 bg-amber-950/80 text-amber-100';
    case 'error':
      return 'border-rose-400/40 bg-rose-950/85 text-rose-100';
    default:
      return 'border-sky-400/30 bg-slate-900/90 text-slate-100';
  }
}
</script>

<template>
  <Teleport to="body">
    <div class="fixed bottom-3 right-3 z-50 flex flex-col gap-2 max-w-xs pointer-events-none">
      <TransitionGroup name="pop">
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="badgeClass(toast.type)"
          class="pointer-events-auto px-3.5 py-2 rounded-xl border backdrop-blur-md shadow-xl text-xs flex items-center gap-2.5"
        >
          <span class="flex-1 leading-snug break-words">{{ toast.message }}</span>
          <button
            type="button"
            class="text-white/60 hover:text-white shrink-0"
            @click="emit('dismiss', toast.id)"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>
