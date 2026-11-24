<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';

export interface ContextMenuItem {
  id: string;
  label: string;
  danger?: boolean;
  dividerBefore?: boolean;
  icon?: 'edit' | 'folder' | 'trash' | 'rename';
}

const props = defineProps<{
  visible: boolean;
  x: number;
  y: number;
  items: ContextMenuItem[];
}>();

const emit = defineEmits<{
  (e: 'select', id: string): void;
  (e: 'close'): void;
}>();

const menuRef = ref<HTMLElement | null>(null);
const menuSize = ref({ width: 176, height: 140 });

const clampedPosition = computed(() => {
  const padding = 8;
  const maxX = Math.max(padding, window.innerWidth - menuSize.value.width - padding);
  const maxY = Math.max(padding, window.innerHeight - menuSize.value.height - padding);
  return {
    left: `${Math.min(Math.max(padding, props.x), maxX)}px`,
    top: `${Math.min(Math.max(padding, props.y), maxY)}px`,
  };
});

async function measureMenu() {
  await nextTick();
  if (menuRef.value) {
    const rect = menuRef.value.getBoundingClientRect();
    if (rect.width > 0 && rect.height > 0) {
      menuSize.value = { width: rect.width, height: rect.height };
    }
  }
}

watch(
  () => [props.visible, props.x, props.y, props.items.length],
  ([visible]) => {
    if (visible) {
      void measureMenu();
    }
  }
);

function handlePointerDown(event: MouseEvent) {
  if (!props.visible) return;
  if (menuRef.value && !menuRef.value.contains(event.target as Node)) {
    emit('close');
  }
}

onMounted(() => {
  window.addEventListener('mousedown', handlePointerDown);
});

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', handlePointerDown);
});
</script>

<template>
  <Teleport to="body">
    <Transition name="pop">
      <div
        v-if="visible"
        ref="menuRef"
        :style="clampedPosition"
        class="fixed z-50 min-w-[164px] py-1.5 rounded-xl glass-modal border jd-border-strong shadow-2xl text-xs jd-text select-none"
        @contextmenu.prevent
      >
        <template v-for="item in items" :key="item.id">
          <div v-if="item.dividerBefore" class="my-1 border-t jd-border" />
          <button
            type="button"
            class="w-full px-3 py-1.5 flex items-center gap-2.5 text-left transition-colors"
            :class="
              item.danger
                ? 'text-rose-300 hover:bg-rose-500/20 hover:text-rose-200'
                : 'jd-text jd-hover'
            "
            @click="emit('select', item.id)"
          >
            <!-- Edit / Rename -->
            <svg
              v-if="item.icon === 'edit' || item.icon === 'rename'"
              class="w-3.5 h-3.5 opacity-80 shrink-0"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"
              />
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M18.5 2.5a2.121 2.121 0 113 3L12 15l-4 1 1-4 9.5-9.5z"
              />
            </svg>

            <!-- Folder -->
            <svg
              v-else-if="item.icon === 'folder'"
              class="w-3.5 h-3.5 opacity-80 shrink-0"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M3 7a2 2 0 012-2h4.172a2 2 0 011.414.586l1.828 1.828A2 2 0 0013.828 8H19a2 2 0 012 2v7a2 2 0 01-2 2H5a2 2 0 01-2-2V7z"
              />
            </svg>

            <!-- Trash -->
            <svg
              v-else-if="item.icon === 'trash'"
              class="w-3.5 h-3.5 opacity-80 shrink-0"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
              />
            </svg>

            <span class="truncate">{{ item.label }}</span>
          </button>
        </template>
      </div>
    </Transition>
  </Teleport>
</template>
