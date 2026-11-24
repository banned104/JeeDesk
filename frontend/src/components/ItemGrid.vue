<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
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
}>();

const dragDepth = ref(0);
const isDragOver = ref(false);
const tileRefs = ref<HTMLButtonElement[]>([]);

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
  const base = formatItemTooltip(entry.item);
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

function handleDragStart(event: DragEvent, entry: DisplayItem) {
  if (!event.dataTransfer) return;
  event.dataTransfer.effectAllowed = 'move';
  event.dataTransfer.setData(
    'application/x-jeedesk-item',
    JSON.stringify({ groupId: entry.groupId, itemId: entry.item.id })
  );
}

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
      class="grid grid-cols-[repeat(auto-fill,minmax(92px,1fr))] gap-2.5 content-start"
      style="content-visibility: auto; contain-intrinsic-size: auto 240px"
    >
      <button
        v-for="(entry, index) in items"
        :key="entryKey(entry)"
        :ref="(el) => setTileRef(el as Element | null, index)"
        type="button"
        draggable="true"
        :title="tileTitle(entry)"
        :class="[
          'group relative flex flex-col items-center justify-start gap-2 p-2.5 rounded-xl border transition-all duration-150 focus:outline-none',
          index === selectedIndex
            ? 'jd-surface-strong jd-border-strong ring-1 ring-sky-400/70'
            : 'border-transparent jd-hover',
          isLaunching(entry) ? 'opacity-50 saturate-50 cursor-progress' : '',
        ]"
        :aria-disabled="isLaunching(entry)"
        :data-launching="isLaunching(entry) ? 'true' : undefined"
        style="content-visibility: auto; contain-intrinsic-size: 92px 96px"
        @click="handleClick(entry, index)"
        @dblclick="handleDoubleClick(entry, index)"
        @contextmenu.prevent="handleContextMenu($event, entry, index)"
        @dragstart="handleDragStart($event, entry)"
        @mouseenter="emit('select', index)"
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
  </main>
</template>

<style scoped>
/* Highlight when Wails runtime sets wails-drop-target-active on WebView2 drop */
.wails-drop-zone.wails-drop-target-active {
  box-shadow: inset 0 0 0 2px rgba(56, 189, 248, 0.55);
  background-color: rgba(56, 189, 248, 0.06);
}
</style>
