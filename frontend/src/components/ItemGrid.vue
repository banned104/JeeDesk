<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import type { ClickMode, DisplayItem } from '../types/config';
import { formatItemTooltip } from '../utils/icon';
import { registerTranslations, t } from '../i18n';
import ItemIcon from './ItemIcon.vue';

registerTranslations({
  '还没有任何项目': 'No items yet',
  '先新建一个分组，再把文件拖进来': 'Create a group first, then drop files in',
  '新建分组': 'New group',
  '当前分组暂无项目': 'This group is empty',
  '拖入 .exe / .lnk / .bat / 文件夹，或用下方的按钮添加':
    'Drop an .exe / .lnk / .bat / folder, or add one with the buttons below',
  '新建项目': 'New item',
  '从开始菜单导入': 'Import from Start Menu',
  '未找到匹配「{query}」的项目': 'No items match "{query}"',
  '可以尝试其他关键词，或清除筛选': 'Try another keyword, or clear the filter',
  '清除搜索': 'Clear search',
  '还没有启动记录': 'No launch history yet',
  '用过的项目会出现在这里，方便再次打开': 'Launched items show up here for quick reuse',
  '松开即可添加到「{group}」': 'Release to add to "{group}"',
  '当前分组': 'current group',
  '目标不存在': 'Target missing',
  '以管理员身份运行': 'Run as administrator',
  '正在加载': 'Loading',
});


const props = withDefaults(
  defineProps<{
    items: DisplayItem[];
    mode: 'group' | 'search' | 'recent';
    activeGroupName: string;
    searchQuery: string;
    cacheBuster: number;
    hasActiveGroup: boolean;
    selectedIndex: number;
    clickMode: ClickMode;
    loading?: boolean;
    /** False until the first getConfig() answer arrives; the grid shows a skeleton instead of "empty". */
    configReady?: boolean;
    /** Whether config has any group at all, so an empty list can offer the right next step. */
    hasGroups?: boolean;
    /** Items whose launch is already in flight, so a fast double click only launches once. */
    launchingIds?: Set<string>;
    /** Item IDs whose target file is gone; such tiles get a warning dot. */
    missingTargets?: Set<string>;
  }>(),
  {
    loading: false,
    configReady: true,
    hasGroups: true,
    launchingIds: () => new Set<string>(),
    missingTargets: () => new Set<string>(),
  }
);

const emit = defineEmits<{
  (e: 'launch', entry: DisplayItem): void;
  (e: 'contextmenu', event: MouseEvent, entry: DisplayItem): void;
  (e: 'clearSearch'): void;
  (e: 'addItem'): void;
  (e: 'addGroup'): void;
  (e: 'select', index: number): void;
  (e: 'openStartMenuImport'): void;
  (
    e: 'reorder',
    payload: { fromGroupId: string; toGroupId: string; itemId: string; toIndex: number }
  ): void;
  /** The group row a dragged tile is currently over, so the sidebar can highlight it. */
  (e: 'dragGroup', groupId: string | null): void;
}>();

const dragDepth = ref(0);
const isDragOver = ref(false);
const tileRefs = ref<HTMLButtonElement[]>([]);

// --- pointer drag -----------------------------------------------------------
// WebView2 runs with AllowExternalDrop=false (main.go DisableWebViewDrop), which also takes down
// in-page HTML5 drag and drop: dragover/drop never fire for a tile dragged inside the page. Tile
// reordering therefore uses pointer events, the same way the sidebar's long-press drag does. The
// HTML5 handlers further down are only for files dragged in from Explorer.

const DRAG_THRESHOLD_PX = 6;
/** Height of the strip at the top/bottom of the scroll area that auto-scrolls while dragging. */
const EDGE_SCROLL_PX = 48;
/** Pixels per frame at full tilt; the ramp starts at a third of it. */
const EDGE_SCROLL_MAX_STEP = 14;
/** How long the tiles below take to open/close the slot, matching the sidebar's make-room effect. */
const SHIFT_TRANSITION_MS = 150;
const LIFT_EASING = 'cubic-bezier(0.22, 1, 0.36, 1)';

interface TileDrag {
  groupId: string;
  itemId: string;
  originIndex: number;
  /** Where the gesture started; the ghost is placed by the pointer's delta from this. */
  startX: number;
  startY: number;
  curX: number;
  curY: number;
  /** Size of the tile when it was picked up, so the ghost keeps the tile's footprint. */
  width: number;
  height: number;
  /** Pointer offset inside the tile at pick-up, so the ghost does not jump to the cursor. */
  offsetX: number;
  offsetY: number;
}

/**
 * Grid geometry, captured once when the drag starts. Measuring live would make the insert slot
 * chase the tiles it is shifting out of the way.
 */
interface GridRow {
  index: number;
  top: number;
  bottom: number;
  left: number;
  width: number;
}

interface GridMetrics {
  /** Distance between two neighbouring slots, gap included; the unit of the make-room shift. */
  slotWidth: number;
  slotHeight: number;
  rows: GridRow[];
  /** Scroll offset of the scroller at pick-up; row tops are page coordinates at that moment. */
  scrollTop: number;
}

const tileDrag = ref<TileDrag | null>(null);
/** Insert slot: "insert before items[insertAt]", in 0..items.length. Null while over the sidebar. */
const insertAt = ref<number | null>(null);
/** Sidebar group highlighted as a cross-group drop target, or null. */
const dropGroupId = ref<string | null>(null);
const scrollerRef = ref<HTMLElement | null>(null);
const gridRef = ref<HTMLElement | null>(null);
let metrics: GridMetrics | null = null;
let pressIndex = -1;
let pressStart: { x: number; y: number } | null = null;
let lastPointer = { x: 0, y: 0 };
let suppressClick = false;
let scrollFrame = 0;
let scrollDir = 0;
/** Style left on the tiles by the drag state, so it can be taken off again on reset. */
let shiftTouched = false;

function handleTilePointerDown(event: PointerEvent, index: number) {
  if (event.button !== 0 || props.mode !== 'group') return;
  pressIndex = index;
  pressStart = { x: event.clientX, y: event.clientY };
  window.addEventListener('pointermove', handleTilePointerMove);
  window.addEventListener('pointerup', handleTilePointerUp);
  window.addEventListener('pointercancel', handleTilePointerCancel);
  // Losing the window mid-drag never delivers its pointerup, so it has to be ended from here.
  window.addEventListener('blur', handleTilePointerCancel);
}

function releaseTileListeners() {
  window.removeEventListener('pointermove', handleTilePointerMove);
  window.removeEventListener('pointerup', handleTilePointerUp);
  window.removeEventListener('pointercancel', handleTilePointerCancel);
  window.removeEventListener('blur', handleTilePointerCancel);
}

/**
 * setDragGroup publishes the sidebar highlight target, but only when it changes: the pointer moves
 * every frame and a repeated emit would re-render the sidebar for nothing.
 */
function setDragGroup(groupId: string | null) {
  if (dropGroupId.value === groupId) return;
  dropGroupId.value = groupId;
  emit('dragGroup', groupId);
}

/** clearShift drops the make-room offsets so the grid snaps back to its resting layout. */
function clearShift() {
  if (!shiftTouched) return;
  shiftTouched = false;
  const grid = gridRef.value;
  if (!grid) return;
  grid.querySelectorAll<HTMLElement>('[data-tile-shift]').forEach((tile) => {
    tile.style.removeProperty('translate');
    tile.removeAttribute('data-tile-shift');
  });
}

function resetTileDrag() {
  releaseTileListeners();
  stopEdgeScroll();
  clearShift();
  tileDrag.value = null;
  insertAt.value = null;
  metrics = null;
  pressStart = null;
  setDragGroup(null);
  // The missing class is the real cleanup: a drag cut short must never leave the page unselectable.
  document.body.classList.remove('select-none');
}

/**
 * interruptTileDrag ends a gesture that will never get its own pointerup: the window losing focus
 * (Alt+G hiding the panel, another app taking over) or a right click mid-drag. Without it the ghost
 * and the make-room offsets would stay on screen until the next left-button release.
 */
function interruptTileDrag() {
  if (!tileDrag.value && !pressStart) return;
  resetTileDrag();
}

function slotWidthFrom(element: HTMLElement): number {
  const style = getComputedStyle(element);
  const gap = parseFloat(style.columnGap || style.gap || '0') || 0;
  const first = element.querySelector<HTMLElement>('[data-tile-index="0"]');
  if (first) {
    const rect = first.getBoundingClientRect();
    if (rect.width > 0) return rect.width + gap;
  }
  return 92 + gap;
}

/**
 * measureGrid records one rect per row. The grid is `auto-fill`, so the column count comes from
 * counting how many tiles share the first row rather than from any CSS the code can read back.
 */
function measureGrid(source: HTMLElement): GridMetrics | null {
  const grid = gridRef.value;
  const scroller = scrollerRef.value;
  if (!grid || !scroller) return null;

  const tiles = Array.from(grid.querySelectorAll<HTMLElement>('[data-tile-index]'));
  if (tiles.length === 0) return null;

  const firstRect = tiles[0].getBoundingClientRect();
  // A zero-height tile means the grid is not laid out (still hidden); the drag has nothing to aim at.
  if (firstRect.height <= 0) return null;
  const gridStyle = getComputedStyle(grid);
  const gapV = parseFloat(gridStyle.rowGap || '0') || 0;
  const slotWidth = slotWidthFrom(source);

  const rows: GridRow[] = [];
  for (const tile of tiles) {
    const index = Number(tile.dataset.tileIndex);
    const rect = tile.getBoundingClientRect();
    const row = rows.find((candidate) => Math.abs(candidate.top - rect.top) < 1);
    if (row) {
      row.bottom = Math.max(row.bottom, rect.bottom);
      row.left = Math.min(row.left, rect.left);
      row.width = Math.max(row.width, rect.right - row.left);
      continue;
    }
    rows.push({ index, top: rect.top, bottom: rect.bottom, left: rect.left, width: rect.width });
  }
  return {
    slotWidth,
    slotHeight: firstRect.height + gapV,
    rows,
    scrollTop: scroller.scrollTop,
  };
}

/**
 * insertAtFromPointer turns the cursor into an insert slot following docs/item-drag.md 5.2: the
 * left/right half of the hovered tile decides the slot, and anything below the last row appends.
 * Working from the captured row geometry instead of `elementFromPoint` is what makes the gaps
 * between tiles (and any blank space to the right of the last row) droppable.
 */
function insertAtFromPointer(x: number, y: number): number | null {
  const scroller = scrollerRef.value;
  if (!scroller || !metrics) return null;
  const { rows } = metrics;
  if (rows.length === 0) return null;

  // Row tops were captured in page coordinates; scrolling has moved them by this much since.
  const scrolled = scroller.scrollTop - metrics.scrollTop;
  const target = rows.find((row) => y <= row.bottom - scrolled);
  // Below the final row is the append slot, which is what makes the trailing blank space droppable.
  if (!target) return props.items.length;

  const slotWidth = metrics.slotWidth > 0 ? metrics.slotWidth : 1;
  const column = Math.max(
    0,
    Math.min(Math.floor((x - target.left) / slotWidth), Math.ceil(target.width / slotWidth) - 1)
  );
  const into = target.index + column;
  const inRightHalf = x >= target.left + column * slotWidth + slotWidth / 2;
  return Math.max(0, Math.min(props.items.length, inRightHalf ? into + 1 : into));
}
/**
 * shiftFor is the make-room offset in slots: everything between the origin and the insert slot
 * moves out of the way. The dragged tile keeps offset 0 — it is the gap being moved around.
 */
function shiftFor(index: number): number {
  const drag = tileDrag.value;
  const slot = insertAt.value;
  if (!drag || slot === null || index === drag.originIndex) return 0;
  if (slot < drag.originIndex && index >= slot && index < drag.originIndex) return 1;
  if (slot > drag.originIndex && index > drag.originIndex && index < slot) return -1;
  return 0;
}

/**
 * applyShift writes the offsets straight onto the tiles instead of going through reactive state:
 * the pointer moves every frame and a re-render per frame would be wasted work. `clearShift`
 * takes the marker back off, so every reset path — drop, cancel, unmount — cleans up.
 */
function applyShift() {
  const grid = gridRef.value;
  if (!grid || !metrics) return;
  shiftTouched = true;
  const slotWidth = metrics.slotWidth;
  grid.querySelectorAll<HTMLElement>('[data-tile-index]').forEach((tile) => {
    const units = shiftFor(Number(tile.dataset.tileIndex));
    tile.dataset.tileShift = units === 0 ? '0' : String(units);
    if (units === 0) {
      tile.style.removeProperty('translate');
      return;
    }
    tile.style.translate = `${units * slotWidth}px 0px`;
  });
}

function stopEdgeScroll() {
  scrollDir = 0;
  if (scrollFrame !== 0) {
    cancelAnimationFrame(scrollFrame);
    scrollFrame = 0;
  }
}

/** edgeScrollTick keeps scrolling while the pointer sits in the edge strip, re-aiming each frame. */
function edgeScrollTick() {
  scrollFrame = 0;
  const scroller = scrollerRef.value;
  if (!scroller || scrollDir === 0 || !tileDrag.value) return;
  const rect = scroller.getBoundingClientRect();
  const distance = scrollDir < 0 ? lastPointer.y - rect.top : rect.bottom - lastPointer.y;
  // Ramp from a third of full speed at the edge of the strip up to full speed at the border.
  const intensity = Math.min(1, Math.max(0, distance / EDGE_SCROLL_PX));
  const step = EDGE_SCROLL_MAX_STEP * (0.35 + 0.65 * intensity);
  const before = scroller.scrollTop;
  scroller.scrollTop += scrollDir * step;
  if (scroller.scrollTop === before) {
    scrollDir = 0;
    return;
  }
  // Scrolling moved the rows under a still pointer, so the slot has to be recomputed.
  insertAt.value = insertAtFromPointer(lastPointer.x, lastPointer.y);
  applyShift();
  scrollFrame = requestAnimationFrame(edgeScrollTick);
}

/** updateEdgeScroll arms the auto-scroll loop whenever the pointer sits in an edge strip. */
function updateEdgeScroll(y: number) {
  const scroller = scrollerRef.value;
  if (!scroller) return;
  const rect = scroller.getBoundingClientRect();
  let dir = 0;
  if (y < rect.top + EDGE_SCROLL_PX) dir = -1;
  else if (y > rect.bottom - EDGE_SCROLL_PX) dir = 1;
  // Only scroll when there is somewhere left to go.
  if (dir < 0 && scroller.scrollTop <= 0) dir = 0;
  if (dir > 0 && scroller.scrollTop >= scroller.scrollHeight - scroller.clientHeight) dir = 0;
  if (dir === scrollDir) {
    if (dir !== 0 && scrollFrame === 0) scrollFrame = requestAnimationFrame(edgeScrollTick);
    return;
  }
  stopEdgeScroll();
  scrollDir = dir;
  if (dir !== 0) scrollFrame = requestAnimationFrame(edgeScrollTick);
}

/** dropTargetAt reports the sidebar group row under the pointer, if any. */
function dropTargetAt(x: number, y: number): string | null {
  const hovered = document.elementFromPoint(x, y);
  if (!(hovered instanceof HTMLElement)) return null;
  return hovered.closest<HTMLElement>('[data-group-row]')?.dataset.groupRow ?? null;
}

function handleTilePointerMove(event: PointerEvent) {
  if (!pressStart) return;
  // The button may have been released while the window was not listening; the gesture is over then.
  if ((event.buttons & 1) === 0) {
    resetTileDrag();
    return;
  }
  if (!tileDrag.value) {
    if (Math.hypot(event.clientX - pressStart.x, event.clientY - pressStart.y) < DRAG_THRESHOLD_PX) return;
    const origin = props.items[pressIndex];
    const tile = tileRefs.value[pressIndex];
    if (!origin || !tile) {
      resetTileDrag();
      return;
    }
    const rect = tile.getBoundingClientRect();
    tileDrag.value = {
      groupId: origin.groupId,
      itemId: origin.item.id,
      originIndex: pressIndex,
      startX: pressStart.x,
      startY: pressStart.y,
      curX: event.clientX,
      curY: event.clientY,
      width: rect.width,
      height: rect.height,
      offsetX: pressStart.x - rect.left,
      offsetY: pressStart.y - rect.top,
    };
    metrics = measureGrid(tile);
    document.body.classList.add('select-none');
    lastPointer = { x: event.clientX, y: event.clientY };
    insertAt.value = insertAtFromPointer(event.clientX, event.clientY);
    applyShift();
    return;
  }

  tileDrag.value = { ...tileDrag.value, curX: event.clientX, curY: event.clientY };
  lastPointer = { x: event.clientX, y: event.clientY };

  // The dragged tile is invisible and pointer-events-none, so only the sidebar or the background
  // can be under the cursor; the insert slot comes from captured geometry, never from hit testing.
  const groupRow = dropTargetAt(event.clientX, event.clientY);
  setDragGroup(groupRow);
  if (groupRow) {
    insertAt.value = null;
    applyShift();
    stopEdgeScroll();
    return;
  }

  insertAt.value = insertAtFromPointer(event.clientX, event.clientY);
  applyShift();
  updateEdgeScroll(event.clientY);
}

function handleTilePointerUp() {
  const drag = tileDrag.value;
  const slot = insertAt.value;
  const targetGroup = dropGroupId.value;
  resetTileDrag();
  if (!drag) return;
  // The click that follows the release must not launch the tile the drag ended on.
  suppressClick = true;
  window.setTimeout(() => {
    suppressClick = false;
  }, 0);

  if (targetGroup && targetGroup !== drag.groupId) {
    // MoveItem clamps toIndex to the end of the target list, so an out-of-range value appends.
    emit('reorder', {
      fromGroupId: drag.groupId,
      toGroupId: targetGroup,
      itemId: drag.itemId,
      toIndex: 1 << 30,
    });
    return;
  }
  if (slot === null) return;

  // MoveItem takes the post-removal index: anything after the origin shifted down by one.
  const toIndex = slot > drag.originIndex ? slot - 1 : slot;
  if (toIndex === drag.originIndex) return;
  const anchor = props.items[Math.min(slot, props.items.length - 1)];
  const toGroupId = anchor?.groupId ?? drag.groupId;
  emit('reorder', {
    fromGroupId: drag.groupId,
    toGroupId,
    itemId: drag.itemId,
    toIndex,
  });
}

function handleTileClickCapture(event: MouseEvent) {
  if (!suppressClick) return;
  suppressClick = false;
  event.stopPropagation();
  event.preventDefault();
}

/** Right click or a stolen gesture: no pointerup is coming, so end it here. */
function handleTilePointerCancel() {
  interruptTileDrag();
}

// A context menu opening mid-drag would otherwise freeze the ghost until the next release.
function handleTileContextMenu(event: MouseEvent, entry: DisplayItem, index: number) {
  interruptTileDrag();
  handleContextMenu(event, entry, index);
}

onBeforeUnmount(() => {
  // A gesture interrupted by an unmount must not leave window listeners or a select-none body.
  resetTileDrag();
});

const showSkeleton = computed(() => !props.configReady || props.loading);

function entryKey(entry: DisplayItem): string {
  return `${entry.groupId}:${entry.item.id}`;
}

function isLaunching(entry: DisplayItem): boolean {
  return props.launchingIds.has(entryKey(entry));
}

function isMissing(entry: DisplayItem): boolean {
  return props.missingTargets.has(entry.item.id);
}

function tileTitle(entry: DisplayItem): string {
  const base = formatItemTooltip(entry.item, `${t('参数:')}:`);
  return isMissing(entry) ? `${t('目标不存在')}\n${base}` : base;
}

/**
 * isFileDrag tells a real file drop apart from an item dragged inside the grid. Without it the
 * "release to add" overlay flashed up while rearranging items between groups.
 */
function isFileDrag(event: DragEvent): boolean {
  const types = event.dataTransfer?.types;
  if (!types) return false;
  return Array.from(types).includes('Files');
}

function handleDragEnter(event: DragEvent) {
  if (!isFileDrag(event)) return;
  event.preventDefault();
  dragDepth.value += 1;
  isDragOver.value = true;
}

function handleDragOver(event: DragEvent) {
  if (!isFileDrag(event)) return;
  event.preventDefault();
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'copy';
  }
  isDragOver.value = true;
}

function handleDragLeave() {
  if (!isDragOver.value) return;
  dragDepth.value = Math.max(0, dragDepth.value - 1);
  if (dragDepth.value === 0) {
    isDragOver.value = false;
  }
}

function handleDrop() {
  dragDepth.value = 0;
  isDragOver.value = false;
}

function handleClick(entry: DisplayItem, index: number) {
  emit('select', index);
  if (props.clickMode === 'single' && !isLaunching(entry)) {
    emit('launch', entry);
  }
}

function handleDoubleClick(entry: DisplayItem, index: number) {
  emit('select', index);
  if (props.clickMode === 'double' && !isLaunching(entry)) {
    emit('launch', entry);
  }
}

function handleContextMenu(event: MouseEvent, entry: DisplayItem, index: number) {
  emit('select', index);
  emit('contextmenu', event, entry);
}

/** draggedTile returns the drag state when this tile is the one being dragged. */
function draggedTile(index: number): TileDrag | null {
  const drag = tileDrag.value;
  return drag && drag.originIndex === index ? drag : null;
}

/**
 * tileDragClass keeps the source tile in place as the slot placeholder and stops it intercepting
 * hits. It stays in the layout (opacity only) so the grid does not reflow when the ghost lifts out.
 */
function tileDragClass(index: number): string {
  return draggedTile(index) ? 'pointer-events-none opacity-0' : '';
}

/**
 * tileShiftStyle is the make-room animation: every tile between the origin and the insert slot
 * slides one slot. The dragged tile is excluded; it is the gap that is being moved around.
 */
function tileShiftStyle(index: number): Record<string, string> | undefined {
  if (!tileDrag.value || draggedTile(index)) return undefined;
  return {
    transitionProperty: 'translate',
    transitionDuration: `${SHIFT_TRANSITION_MS}ms`,
    transitionTimingFunction: LIFT_EASING,
  };
}

/** ghostStyle places the fixed-position copy of the dragged tile, cursor offset preserved. */
function ghostStyle(drag: TileDrag): Record<string, string> {
  return {
    left: '0',
    top: '0',
    width: `${drag.width}px`,
    height: `${drag.height}px`,
    transform: `translate3d(${drag.curX - drag.offsetX}px, ${drag.curY - drag.offsetY}px, 0) scale(1.04)`,
  };
}

const draggedEntry = computed<DisplayItem | null>(() => {
  const drag = tileDrag.value;
  if (!drag) return null;
  return props.items.find((entry) => entry.groupId === drag.groupId && entry.item.id === drag.itemId) ?? null;
});

async function ensureSelectionVisible() {
  await nextTick();
  const tile = tileRefs.value[props.selectedIndex];
  if (tile) {
    tile.scrollIntoView({ block: 'nearest' });
  }
}

function setTileRef(el: Element | null, index: number) {
  if (el instanceof HTMLButtonElement) {
    tileRefs.value[index] = el;
  }
}

watch(
  () => props.selectedIndex,
  () => {
    void ensureSelectionVisible();
  }
);

defineExpose({ ensureSelectionVisible });
</script>

<template>
  <main
    ref="scrollerRef"
    class="wails-drop-zone relative flex-1 h-full overflow-y-auto custom-scrollbar p-4"
    style="--wails-drop-target: drop"
    :aria-busy="showSkeleton"
    @dragenter="handleDragEnter"
    @dragover="handleDragOver"
    @dragleave="handleDragLeave"
    @drop="handleDrop"
  >
    <!-- First-frame skeleton: config has not answered yet, so the empty state would be a lie -->
    <div v-if="showSkeleton" class="grid grid-cols-[repeat(auto-fill,minmax(92px,1fr))] gap-2.5 content-start">
      <div v-for="n in 8" :key="n" class="h-[86px] rounded-xl jd-surface-soft animate-pulse" />
      <span class="sr-only">{{ t('正在加载') }}</span>
    </div>

    <!-- Item Grid -->
    <div
      v-else-if="items.length > 0"
      ref="gridRef"
      class="grid grid-cols-[repeat(auto-fill,minmax(92px,1fr))] gap-2.5 content-start"
      style="contain: layout style"
      @click.capture="handleTileClickCapture"
    >
      <button
        v-for="(entry, index) in items"
        :key="entryKey(entry)"
        :ref="(el) => setTileRef(el as Element | null, index)"
        type="button"
        :title="tileTitle(entry)"
        :data-tile-index="index"
        :class="[
          'group relative flex flex-col items-center justify-start gap-2 p-2.5 rounded-xl border transition-all duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400',
          index === selectedIndex
            ? 'jd-surface-strong jd-border-strong ring-1 ring-sky-400/70'
            : 'border-transparent jd-hover',
          isLaunching(entry) ? 'opacity-50 saturate-50 cursor-progress' : '',
          tileDragClass(index),
        ]"
        :aria-disabled="isLaunching(entry)"
        :data-launching="isLaunching(entry) ? 'true' : undefined"
        :style="tileShiftStyle(index)"
        @pointerdown="handleTilePointerDown($event, index)"
        @click="handleClick(entry, index)"
        @dblclick="handleDoubleClick(entry, index)"
        @contextmenu.prevent="handleTileContextMenu($event, entry, index)"
        @mouseenter="!tileDrag && emit('select', index)"
      >
        <ItemIcon
          :icon="entry.item.icon"
          :path="entry.item.path"
          :name="entry.item.name"
          :cache-buster="cacheBuster"
          size-class="w-12 h-12 group-hover:scale-105"
        />
        <span class="w-full text-center text-xs jd-text truncate">{{ entry.item.name }}</span>

        <!-- Group badge in cross-group views, so a result is identifiable -->
        <span v-if="mode !== 'group'" class="max-w-full truncate text-[10px] jd-subtle">
          {{ entry.groupName }}
        </span>

        <!-- Broken target marker: the card stays clickable, editing still fixes the path -->
        <span
          v-if="isMissing(entry)"
          class="absolute top-1.5 left-1.5 w-2 h-2 rounded-full bg-rose-400 shadow-[0_0_0_2px_rgba(244,63,94,0.25)]"
          :title="t('目标不存在')"
          aria-hidden="true"
        />

        <!-- Administrator marker -->
        <span
          v-if="entry.item.admin"
          class="absolute top-1.5 right-1.5 px-1 rounded text-[9px] font-semibold bg-amber-400/20 text-amber-200 border border-amber-300/30"
          :title="t('以管理员身份运行')"
        >
          A
        </span>
      </button>
    </div>

    <!-- Empty Search Result -->
    <div
      v-else-if="mode === 'search'"
      class="absolute inset-4 flex flex-col items-center justify-center text-center gap-3 jd-muted"
    >
      <svg class="w-10 h-10 opacity-50" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="11" cy="11" r="7" />
        <path stroke-linecap="round" d="M20 20l-3.5-3.5" />
      </svg>
      <div class="space-y-1">
        <p class="text-sm jd-text">{{ t('未找到匹配「{query}」的项目', { query: searchQuery }) }}</p>
        <p class="text-xs jd-subtle">{{ t('可以尝试其他关键词，或清除筛选') }}</p>
      </div>
      <button
        type="button"
        class="px-3 py-1.5 rounded-lg text-xs jd-surface jd-border border jd-text jd-hover transition-colors"
        @click="emit('clearSearch')"
      >
        {{ t('清除搜索') }}
      </button>
    </div>

    <!-- Empty Recent List -->
    <div
      v-else-if="mode === 'recent'"
      class="absolute inset-4 flex flex-col items-center justify-center text-center gap-2 jd-muted"
    >
      <svg class="w-10 h-10 opacity-50" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="12" cy="12" r="9" />
        <path stroke-linecap="round" d="M12 7v5l3 2" />
      </svg>
      <p class="text-sm jd-text">{{ t('还没有启动记录') }}</p>
      <p class="text-xs jd-subtle">{{ t('用过的项目会出现在这里，方便再次打开') }}</p>
    </div>

    <!-- No group at all: the only useful next step is creating one -->
    <div
      v-else-if="!hasGroups"
      class="absolute inset-4 flex flex-col items-center justify-center text-center"
    >
      <div
        class="max-w-xs w-full px-6 py-8 rounded-2xl border border-dashed jd-border jd-surface-soft flex flex-col items-center gap-3"
      >
        <div class="w-12 h-12 rounded-2xl bg-sky-400/10 border border-sky-300/20 flex items-center justify-center text-sky-300">
          <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3 7a2 2 0 012-2h4.172a2 2 0 011.414.586l1.828 1.828A2 2 0 0013.828 8H19a2 2 0 012 2v7a2 2 0 01-2 2H5a2 2 0 01-2-2V7z" />
          </svg>
        </div>
        <div class="space-y-1">
          <p class="text-sm font-medium jd-text">{{ t('还没有任何项目') }}</p>
          <p class="text-xs jd-muted leading-relaxed max-w-[260px]">
            {{ t('先新建一个分组，再把文件拖进来') }}
          </p>
        </div>
        <button
          type="button"
          class="mt-1 px-4 h-8 rounded-lg text-xs bg-sky-500/85 hover:bg-sky-500 text-white font-medium shadow-sm transition-colors"
          @click="emit('addGroup')"
        >
          + {{ t('新建分组') }}
        </button>
      </div>
    </div>

    <!-- Empty Group Drop Hint -->
    <div v-else class="absolute inset-4 flex flex-col items-center justify-center text-center">
      <div
        class="max-w-xs w-full px-6 py-8 rounded-2xl border border-dashed jd-border jd-surface-soft flex flex-col items-center gap-3"
      >
        <div
          class="w-12 h-12 rounded-2xl bg-sky-400/10 border border-sky-300/20 flex items-center justify-center text-sky-300"
        >
          <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 16V4m0 0l-4 4m4-4l4 4" />
            <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2" />
          </svg>
        </div>
        <div class="space-y-1">
          <p class="text-sm font-medium jd-text">{{ t('当前分组暂无项目') }}</p>
          <p class="text-xs jd-muted leading-relaxed max-w-[260px]">
            {{ t('拖入 .exe / .lnk / .bat / 文件夹，或用下方的按钮添加') }}
          </p>
        </div>
        <div class="mt-1 flex items-center gap-2">
          <button
            v-if="hasActiveGroup"
            type="button"
            class="px-3 py-1.5 rounded-lg text-xs jd-surface-strong jd-text jd-hover border jd-border transition-colors"
            @click="emit('addItem')"
          >
            + {{ t('新建项目') }}
          </button>
          <button
            v-if="hasActiveGroup"
            type="button"
            class="px-3 py-1.5 rounded-lg text-xs jd-surface jd-text jd-hover border jd-border transition-colors"
            @click="emit('openStartMenuImport')"
          >
            {{ t('从开始菜单导入') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Drag & Drop Visual Overlay (HTML5 + Wails active class) -->
    <Transition name="fade">
      <div
        v-if="isDragOver"
        class="pointer-events-none absolute inset-3 rounded-2xl border-2 border-dashed border-sky-400/70 bg-sky-500/15 backdrop-blur-sm flex flex-col items-center justify-center gap-2 z-20"
      >
        <svg class="w-8 h-8 text-sky-300" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 16V4m0 0l-4 4m4-4l4 4" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2" />
        </svg>
        <p class="text-sm font-medium text-sky-100">
          {{ t('松开即可添加到「{group}」', { group: activeGroupName || t('当前分组') }) }}
        </p>
      </div>
    </Transition>

    <!--
      Tile ghost. Teleported to <body> and position: fixed, so the scrolling container cannot clip
      it and the grid's `contain` cannot skip it. The source tile stays in the grid as the
      placeholder that the make-room animation shifts around.
    -->
    <Teleport to="body">
      <div
        v-if="tileDrag && draggedEntry"
        class="pointer-events-none fixed z-[999] flex flex-col items-center justify-start gap-2 p-2.5 rounded-xl border jd-border-strong jd-surface-strong shadow-[0_18px_38px_-10px_rgba(0,0,0,0.7)] ring-1 ring-sky-400/60 will-change-transform"
        :style="ghostStyle(tileDrag)"
        aria-hidden="true"
      >
        <ItemIcon
          :icon="draggedEntry.item.icon"
          :path="draggedEntry.item.path"
          :name="draggedEntry.item.name"
          :cache-buster="cacheBuster"
          size-class="w-12 h-12"
        />
        <span class="w-full text-center text-xs jd-text truncate">{{ draggedEntry.item.name }}</span>
      </div>
    </Teleport>
  </main>
</template>

<style scoped>
/* Highlight when Wails runtime sets wails-drop-target-active on WebView2 drop */
.wails-drop-zone.wails-drop-target-active {
  box-shadow: inset 0 0 0 2px rgba(56, 189, 248, 0.55);
  background-color: rgba(56, 189, 248, 0.06);
}
</style>
