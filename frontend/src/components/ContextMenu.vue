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

// --- keyboard support -----------------------------------------------------

// Element that had focus when the menu opened; Escape hands focus back to it.
let previousFocus: HTMLElement | null = null;

function menuItems(): HTMLElement[] {
  const root = menuRef.value;
  if (!root) return [];
  return Array.from(root.querySelectorAll<HTMLElement>('[role="menuitem"]'));
}

function focusedItemIndex(items: HTMLElement[]): number {
  const active = document.activeElement;
  if (!(active instanceof HTMLElement)) return -1;
  return items.indexOf(active);
}

/** Focuses an item by index, wrapping around at both ends. */
function focusItem(index: number) {
  const items = menuItems();
  if (items.length === 0) return;
  const count = items.length;
  items[((index % count) + count) % count]?.focus();
}

function restoreFocus() {
  const target = previousFocus;
  previousFocus = null;
  if (target && document.contains(target)) {
    target.focus();
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (!props.visible) return;
  const items = menuItems();
  const current = focusedItemIndex(items);
  switch (event.key) {
    case 'Escape':
      event.preventDefault();
      // Restore first: closing flips `visible`, which clears the remembered element.
      restoreFocus();
      emit('close');
      return;
    case 'ArrowDown':
      event.preventDefault();
      focusItem(current < 0 ? 0 : current + 1);
      return;
    case 'ArrowUp':
      event.preventDefault();
      focusItem(current < 0 ? items.length - 1 : current - 1);
      return;
    case 'Home':
      event.preventDefault();
      focusItem(0);
      return;
    case 'End':
      event.preventDefault();
      focusItem(items.length - 1);
      return;
    case 'Enter':
    case ' ':
      // A focused menuitem is a <button>, so the browser already turns Enter/Space into a click;
      // only the case where the menu itself holds focus needs an explicit activation.
      if (current >= 0) return;
      event.preventDefault();
      items[0]?.click();
      return;
    default:
      return;
  }
}

watch(
  () => [props.visible, props.x, props.y, props.items.length],
  async ([visible]) => {
    if (!visible) {
      previousFocus = null;
      return;
    }
    const active = document.activeElement;
    if (active instanceof HTMLElement && !menuRef.value?.contains(active)) {
      previousFocus = active;
    }
    await measureMenu();
    focusItem(0);
  }
);

function handlePointerDown(event: MouseEvent) {
  if (!props.visible) return;
  if (menuRef.value && !menuRef.value.contains(event.target as Node)) {
    emit('close');
  }
}

// The menu is anchored to viewport coordinates, so any scroll or resize would leave it pointing at
// the wrong place. Closing is cheaper and less surprising than re-positioning it.
function handleViewportChange() {
  if (props.visible) {
    emit('close');
  }
}

onMounted(() => {
  window.addEventListener('mousedown', handlePointerDown);
  // Capture phase also sees scrolls of inner scroll containers, which do not bubble.
  window.addEventListener('scroll', handleViewportChange, true);
  window.addEventListener('resize', handleViewportChange);
});

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', handlePointerDown);
  window.removeEventListener('scroll', handleViewportChange, true);
  window.removeEventListener('resize', handleViewportChange);
  previousFocus = null;
});
</script>

<template>
  <Teleport to="body">
    <Transition name="pop">
      <div
        v-if="visible"
        ref="menuRef"
        role="menu"
        tabindex="-1"
        :style="clampedPosition"
        class="fixed z-50 min-w-[164px] py-1.5 rounded-xl glass-modal border jd-border-strong shadow-2xl text-xs jd-text select-none outline-none"
        @contextmenu.prevent
        @keydown="handleKeydown"
      >
        <template v-for="item in items" :key="item.id">
          <div v-if="item.dividerBefore" class="my-1 border-t jd-border" />
          <button
            type="button"
            role="menuitem"
            tabindex="-1"
            class="w-full px-3 py-1.5 flex items-center gap-2.5 text-left transition-colors outline-none focus-visible:bg-white/10"
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
              aria-hidden="true"
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
              aria-hidden="true"
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
              aria-hidden="true"
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
