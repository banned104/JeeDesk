<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';
import type { Group } from '../types/config';
import { RECENT_GROUP_ID } from '../types/config';
import { registerTranslations, t } from '../i18n';

registerTranslations({
  '分组': 'Groups',
  '展开分组栏': 'Expand group sidebar',
  '收起分组栏': 'Collapse group sidebar',
  '最近使用': 'Recent',
  '新建分组': 'New group',
  '折叠分组': 'Fold group',
  '展开分组': 'Unfold group',
  '显示已折叠分组 ({count})': 'Show {count} folded',
  '收起已折叠分组': 'Hide folded groups',
  '搜索命中 {count} 个项目': '{count} matching items',
});

const props = withDefaults(
  defineProps<{
    groups: Group[];
    activeGroupId: string;
    collapsed: boolean;
    showRecent?: boolean;
    recentCount?: number;
    /** Group IDs folded away in settings.collapsed_groups. */
    collapsedGroupIds?: string[];
    /** While searching, folding is ignored so hits inside folded groups stay reachable. */
    searchActive?: boolean;
    /** Per-group search hit counts, shown next to the group name while searching. */
    hitCounts?: Record<string, number>;
  }>(),
  {
    showRecent: false,
    recentCount: 0,
    collapsedGroupIds: () => [],
    searchActive: false,
    hitCounts: () => ({}),
  }
);

const emit = defineEmits<{
  (e: 'select', groupId: string): void;
  (e: 'addGroup'): void;
  (e: 'toggleCollapse'): void;
  (e: 'toggleGroupCollapse', groupId: string): void;
  (e: 'contextmenu', event: MouseEvent, group: Group): void;
  (e: 'reorderGroup', payload: { groupId: string; toIndex: number }): void;
}>();

const navRef = ref<HTMLElement | null>(null);

// Long press turns a group row into a draggable handle: a short press is still a click, and a
// pointer that moves before the timer fires counts as a scroll or a text selection instead.
const LONG_PRESS_MS = 320;
const MOVE_CANCEL_PX = 6;

const dragGroupId = ref('');
const dropSlot = ref(-1);
// The lifted row follows the pointer through the CSS `translate` property: `translate` has no
// transition while dragging so it tracks the pointer 1:1, while `scale`/`box-shadow` transition,
// which is what makes the row read as "picked up" instead of just highlighted.
const dragOffsetY = ref(0);
const settling = ref(false);
let settleTimer: ReturnType<typeof setTimeout> | null = null;

// Geometry is captured when the drag starts: the rows below move while dragging, so measuring them
// live would make the drop slot chase its own animation. Tops are page coordinates at the start,
// corrected by how far the list has scrolled since.
let dragIndex = -1;
let dragSpan = 40;
let dragRowHeight = 36;
let dragTops: number[] = [];
let dragStartScroll = 0;
let pressTimer: ReturnType<typeof setTimeout> | null = null;
let pressOrigin = { x: 0, y: 0 };
let suppressClick = false;
let autoScrollTimer: ReturnType<typeof setInterval> | null = null;
let autoScrollDir = 0;

function rows(): HTMLElement[] {
  return Array.from(navRef.value?.querySelectorAll<HTMLElement>('[data-group-row]') ?? []);
}

/** rowLiftClass is the "picked up" look: raised above the list, scaled, with a soft shadow. */
function rowLiftClass(groupId: string): string[] {
  if (dragGroupId.value !== groupId) return [];
  return [
    'z-10',
    'cursor-grabbing',
    'border',
    'jd-border-strong',
    'scale-[1.04]',
    'shadow-[0_14px_30px_-8px_rgba(0,0,0,0.65)]',
    'ring-1',
    'ring-sky-300/40',
  ];
}

const LIFT_EASING = 'cubic-bezier(0.22, 1, 0.36, 1)';
const DRAG_TRANSITION_MS = 150;

/**
 * rowStyle carries the motion: the lifted row tracks the pointer through `translate` with no
 * transition, while `scale`/`box-shadow` ease in so it reads as picked up. Rows that make room
 * transition their own `translate`, which is the push-apart effect.
 */
function rowStyle(groupId: string, index: number): Record<string, string> | undefined {
  if (dragGroupId.value === groupId) {
    return {
      translate: `0 ${dragOffsetY.value}px`,
      transitionProperty: settling.value ? 'translate, scale, box-shadow' : 'scale, box-shadow',
      transitionDuration: `${DRAG_TRANSITION_MS}ms`,
      transitionTimingFunction: LIFT_EASING,
    };
  }
  const shift = rowShift(index);
  if (!shift && !dragGroupId.value && !settling.value) return undefined;
  // Rows keep the transition while a drag is active, so both directions animate: making room and
  // sliding back when the pointer leaves the slot.
  return {
    translate: `0 ${shift}px`,
    transitionProperty: 'translate',
    transitionDuration: `${DRAG_TRANSITION_MS}ms`,
    transitionTimingFunction: LIFT_EASING,
  };
}

function clearSettleTimer() {
  if (settleTimer !== null) {
    clearTimeout(settleTimer);
    settleTimer = null;
  }
}

function clearPressTimer() {
  if (pressTimer !== null) {
    clearTimeout(pressTimer);
    pressTimer = null;
  }
}

function stopAutoScroll() {
  if (autoScrollTimer !== null) {
    clearInterval(autoScrollTimer);
    autoScrollTimer = null;
  }
  autoScrollDir = 0;
}

function releasePointerListeners() {
  window.removeEventListener('pointermove', trackPointer);
  window.removeEventListener('pointerup', finishPress);
  window.removeEventListener('pointercancel', finishPress);
  window.removeEventListener('blur', finishPress);
}

function beginPress(event: PointerEvent, groupId: string) {
  if (event.button !== 0) return;
  const target = event.target instanceof HTMLElement ? event.target : null;
  // The fold toggle sits inside the row; pressing it must not start a drag.
  if (target?.closest('[data-no-drag]')) return;

  pressOrigin = { x: event.clientX, y: event.clientY };
  clearPressTimer();
  pressTimer = setTimeout(() => {
    pressTimer = null;
    dragGroupId.value = groupId;
    dragOffsetY.value = 0;
    settling.value = false;

    const list = rows();
    dragIndex = list.findIndex((row) => row.dataset.groupRow === groupId);
    dragTops = list.map((row) => row.getBoundingClientRect().top);
    dragRowHeight = list[0]?.getBoundingClientRect().height || 36;
    dragSpan = dragTops.length > 1 ? dragTops[1] - dragTops[0] : dragRowHeight + 4;
    dragStartScroll = navRef.value?.scrollTop ?? 0;
    dropSlot.value = dragIndex >= 0 ? dragIndex : 0;

    suppressClick = true;
    document.body.classList.add('select-none');
  }, LONG_PRESS_MS);

  window.addEventListener('pointermove', trackPointer);
  window.addEventListener('pointerup', finishPress);
  window.addEventListener('pointercancel', finishPress);
  // Losing the window mid-press would otherwise leave the row stuck in its drag state.
  window.addEventListener('blur', finishPress);
}

// Scrolling near the edges keeps a long list reachable while the pointer stays still.
function updateAutoScroll(clientY: number) {
  const nav = navRef.value;
  if (!nav) return;
  const rect = nav.getBoundingClientRect();
  const margin = 26;
  let dir = 0;
  if (clientY < rect.top + margin) dir = -1;
  else if (clientY > rect.bottom - margin) dir = 1;
  if (dir === autoScrollDir) return;
  stopAutoScroll();
  autoScrollDir = dir;
  if (dir !== 0) {
    autoScrollTimer = setInterval(() => {
      nav.scrollTop += dir * 8;
    }, 30);
  }
}

function trackPointer(event: PointerEvent) {
  if (!dragGroupId.value) {
    if (Math.hypot(event.clientX - pressOrigin.x, event.clientY - pressOrigin.y) > MOVE_CANCEL_PX) {
      clearPressTimer();
      releasePointerListeners();
    }
    return;
  }
  event.preventDefault();
  dragOffsetY.value = event.clientY - pressOrigin.y;

  const scrolled = (navRef.value?.scrollTop ?? 0) - dragStartScroll;
  let slot = dragTops.length;
  for (let i = 0; i < dragTops.length; i += 1) {
    if (event.clientY < dragTops[i] - scrolled + dragRowHeight / 2) {
      slot = i;
      break;
    }
  }
  dropSlot.value = slot;
  updateAutoScroll(event.clientY);
}

/** targetIndex maps a drop slot in the visible rows to MoveGroup's post-removal index. */
/**
 * rowShift is the "make room" offset: the rows between the dragged one and the drop slot slide by
 * one row, which is what opens a gap where the row will land.
 */
function rowShift(index: number): number {
  if (!dragGroupId.value || dropSlot.value < 0 || index === dragIndex) return 0;
  if (dragIndex < dropSlot.value && index > dragIndex && index < dropSlot.value) return -dragSpan;
  if (dragIndex > dropSlot.value && index >= dropSlot.value && index < dragIndex) return dragSpan;
  return 0;
}

function targetIndex(groupId: string, slot: number): number | null {
  const ids = visibleGroups.value.map((group) => group.id);
  const from = props.groups.findIndex((group) => group.id === groupId);
  if (from < 0) return null;

  const anchorId = slot < ids.length ? ids[slot] : '';
  if (anchorId === groupId) return null;

  let toIndex: number;
  if (!anchorId) {
    toIndex = props.groups.length - 1;
  } else {
    const anchor = props.groups.findIndex((group) => group.id === anchorId);
    if (anchor < 0) return null;
    toIndex = anchor > from ? anchor - 1 : anchor;
  }
  return toIndex === from ? null : toIndex;
}

function finishPress() {
  clearPressTimer();
  releasePointerListeners();
  stopAutoScroll();
  document.body.classList.remove('select-none');

  const groupId = dragGroupId.value;
  const slot = dropSlot.value;
  if (!groupId) {
    resetDragState();
    return;
  }

  const toIndex = targetIndex(groupId, slot);
  // Settle first, so the row visibly lands instead of snapping back to its slot. The reorder is
  // emitted right away; the list has already moved by the time the animation ends.
  settling.value = true;
  dragOffsetY.value = 0;
  dropSlot.value = -1;
  if (toIndex !== null) {
    emit('reorderGroup', { groupId, toIndex });
  }
  clearSettleTimer();
  settleTimer = setTimeout(() => {
    settleTimer = null;
    resetDragState();
  }, 160);

  // The click that follows a drop is dispatched right after pointerup, in the same task. Clearing
  // the flag on the next tick keeps the drop from also selecting a group while making sure a drag
  // that ended over a gap (where no row click fires) cannot swallow the next real click.
  window.setTimeout(() => {
    suppressClick = false;
  }, 0);
}

function resetDragState() {
  clearSettleTimer();
  dragGroupId.value = '';
  dragOffsetY.value = 0;
  settling.value = false;
  dropSlot.value = -1;
  dragIndex = -1;
  dragTops = [];
}

/** Swallows the click that would otherwise select a group right after dropping it. */
function handleRowClickCapture(event: MouseEvent) {
  if (!suppressClick) return;
  suppressClick = false;
  event.stopPropagation();
  event.preventDefault();
}

onBeforeUnmount(() => {
  clearPressTimer();
  clearSettleTimer();
  releasePointerListeners();
  stopAutoScroll();
  document.body.classList.remove('select-none');
});
// Folded groups can be revealed again, otherwise renaming or deleting one would need a search.
const showFolded = ref(false);

const foldedIds = computed(() => new Set(props.collapsedGroupIds));

function isFolded(groupId: string): boolean {
  return foldedIds.value.has(groupId);
}

function isGroupVisible(group: Group): boolean {
  if (!isFolded(group.id)) return true;
  // Search ignores folding, and the group in view keeps its row so the selection stays visible.
  if (props.searchActive) return true;
  if (group.id === props.activeGroupId) return true;
  return showFolded.value;
}

const visibleGroups = computed(() => props.groups.filter(isGroupVisible));
const foldedCount = computed(
  () => props.groups.filter((group) => isFolded(group.id) && group.id !== props.activeGroupId).length
);

function hitCount(groupId: string): number {
  return props.hitCounts[groupId] ?? 0;
}
</script>

<template>
  <aside
    :class="collapsed ? 'w-12' : 'w-[152px]'"
    class="h-full border-r jd-border jd-surface-soft flex flex-col shrink-0 transition-all duration-200"
  >
    <!-- Sidebar Header -->
    <div class="px-2 pt-2.5 pb-1.5 flex items-center justify-between">
      <span v-if="!collapsed" class="px-1.5 text-[11px] font-medium uppercase tracking-wider jd-muted">
        {{ t('分组') }}
      </span>
      <button
        type="button"
        :aria-label="collapsed ? t('展开分组栏') : t('收起分组栏')"
        :title="collapsed ? t('展开分组栏') : t('收起分组栏')"
        class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover transition-colors"
        :class="collapsed ? 'mx-auto' : ''"
        @click="emit('toggleCollapse')"
      >
        <svg
          class="w-3.5 h-3.5 transition-transform duration-200"
          :class="collapsed ? 'rotate-180' : ''"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          aria-hidden="true"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
        </svg>
      </button>
    </div>

    <!-- Group List -->
    <nav ref="navRef" class="flex-1 overflow-y-auto custom-scrollbar px-1.5 space-y-1">
      <!-- Virtual recent group -->
      <button
        v-if="showRecent"
        type="button"
        :title="t('最近使用')"
        class="group relative w-full h-9 rounded-xl flex items-center gap-2 px-2.5 text-xs transition-all duration-150"
        :class="
          activeGroupId === RECENT_GROUP_ID
            ? 'jd-surface-strong jd-text font-medium border jd-border-strong shadow-sm'
            : 'jd-muted jd-hover border border-transparent'
        "
        @click="emit('select', RECENT_GROUP_ID)"
      >
        <span
          v-if="activeGroupId === RECENT_GROUP_ID"
          class="absolute left-0.5 top-2 bottom-2 w-0.5 rounded-full jd-accent-bg"
        />
        <template v-if="collapsed">
          <span class="mx-auto font-semibold text-[11px]">{{ t('最近使用').slice(0, 1) }}</span>
        </template>
        <template v-else>
          <svg
            class="w-3.5 h-3.5 shrink-0"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.9"
            aria-hidden="true"
          >
            <circle cx="12" cy="12" r="9" />
            <path stroke-linecap="round" d="M12 7v5l3 2" />
          </svg>
          <span class="truncate flex-1 text-left">{{ t('最近使用') }}</span>
          <span class="text-[10px] px-1.5 py-0.5 rounded-md jd-surface">{{ recentCount }}</span>
        </template>
      </button>

      <div
        v-for="(group, index) in visibleGroups"
        :key="group.id"
        class="group relative w-full h-9 rounded-xl transition-colors duration-150 touch-none"
        :class="[
          group.id === activeGroupId
            ? 'jd-surface-strong jd-text font-medium border jd-border-strong shadow-sm'
            : 'jd-muted jd-hover border border-transparent',
          rowLiftClass(group.id),
          dragGroupId && dragGroupId !== group.id ? 'cursor-grabbing' : '',
        ]"
        :style="rowStyle(group.id, index)"
        :data-group-id="group.id"
        :data-group-row="group.id"
        @pointerdown="beginPress($event, group.id)"
        @click.capture="handleRowClickCapture"
        @contextmenu.prevent="emit('contextmenu', $event, group)"
      >
        <span
          v-if="dragGroupId && dropSlot === index"
          class="pointer-events-none absolute -top-0.5 left-1 right-1 h-0.5 rounded-full jd-accent-bg"
          aria-hidden="true"
        />
        <span
          v-if="group.id === activeGroupId"
          class="absolute left-0.5 top-2 bottom-2 w-0.5 rounded-full jd-accent-bg"
          aria-hidden="true"
        />

        <button
          type="button"
          class="absolute inset-0 w-full h-full rounded-xl flex items-center gap-2 pl-2.5 pr-2 text-xs text-left"
          :class="collapsed ? 'justify-center px-1' : ''"
          :title="group.name"
          @click="emit('select', group.id)"
        >
          <template v-if="collapsed">
            <span class="font-semibold text-[11px]">
              {{ group.name.slice(0, 1).toUpperCase() || '#' }}
            </span>
          </template>
          <template v-else>
            <span class="truncate flex-1 text-left">{{ group.name }}</span>
            <!-- Search mode shows where the hits are; folding is ignored there. -->
            <span
              v-if="searchActive && hitCount(group.id) > 0"
              class="text-[10px] px-1.5 py-0.5 rounded-md jd-accent bg-sky-400/15"
              :title="t('搜索命中 {count} 个项目', { count: hitCount(group.id) })"
            >
              {{ hitCount(group.id) }}
            </span>
            <span
              v-else
              class="text-[10px] px-1.5 py-0.5 rounded-md transition-colors"
              :class="group.id === activeGroupId ? 'jd-surface jd-text' : 'jd-subtle'"
            >
              {{ group.items?.length ?? 0 }}
            </span>
            <span class="w-5 shrink-0" aria-hidden="true" />
          </template>
        </button>

        <!-- Fold toggle: kept outside the row button, nested buttons are not valid HTML -->
        <button
          v-if="!collapsed"
          type="button"
          data-no-drag
          class="absolute right-0.5 top-1/2 -translate-y-1/2 w-5 h-5 rounded-md flex items-center justify-center opacity-60 group-hover:opacity-100 transition-opacity"
          :aria-label="isFolded(group.id) ? t('展开分组') : t('折叠分组')"
          :title="isFolded(group.id) ? t('展开分组') : t('折叠分组')"
          :aria-expanded="!isFolded(group.id)"
          @click.stop="emit('toggleGroupCollapse', group.id)"
        >
          <svg
            class="w-3 h-3 transition-transform duration-200"
            :class="isFolded(group.id) ? '-rotate-90' : ''"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
            aria-hidden="true"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 9l6 6 6-6" />
          </svg>
        </button>
      </div>
      <span
        v-if="dragGroupId && dropSlot === visibleGroups.length"
        class="pointer-events-none block h-0.5 mx-1 rounded-full jd-accent-bg"
        aria-hidden="true"
      />
    </nav>

    <!-- Folded groups can be revealed, so a folded group stays editable -->
    <div v-if="!collapsed && !searchActive && foldedCount > 0" class="px-1.5 pb-1">
      <button
        type="button"
        class="w-full h-6 rounded-lg flex items-center justify-center gap-1 text-[10px] jd-subtle jd-hover border border-transparent transition-colors"
        @click="showFolded = !showFolded"
      >
        <svg
          class="w-3 h-3 transition-transform duration-200"
          :class="showFolded ? 'rotate-180' : ''"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.2"
          aria-hidden="true"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 9l6 6 6-6" />
        </svg>
        {{ showFolded ? t('收起已折叠分组') : t('显示已折叠分组 ({count})', { count: foldedCount }) }}
      </button>
    </div>

    <!-- Add Group Button -->
    <div class="p-2 border-t jd-border">
      <button
        type="button"
        :aria-label="t('新建分组')"
        :title="t('新建分组')"
        class="w-full h-8 rounded-xl flex items-center justify-center gap-1.5 text-xs jd-surface-soft jd-muted jd-hover border jd-border transition-all"
        @click="emit('addGroup')"
      >
        <svg
          class="w-3.5 h-3.5 shrink-0"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.2"
          aria-hidden="true"
        >
          <path stroke-linecap="round" d="M12 5v14M5 12h14" />
        </svg>
        <span v-if="!collapsed" class="truncate">{{ t('新建分组') }}</span>
      </button>
    </div>
  </aside>
</template>
