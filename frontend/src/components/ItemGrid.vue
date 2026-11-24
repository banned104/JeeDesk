<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';
import type { ClickMode, DisplayItem } from '../types/config';
import { formatItemTooltip } from '../utils/icon';
import ItemIcon from './ItemIcon.vue';

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
  }>(),
  { loading: false }
);

const emit = defineEmits<{
  (e: 'launch', entry: DisplayItem): void;
  (e: 'contextmenu', event: MouseEvent, entry: DisplayItem): void;
  (e: 'clearSearch'): void;
  (e: 'addItem'): void;
  (e: 'select', index: number): void;
  (e: 'openStartMenuImport'): void;
}>();

const dragDepth = ref(0);
const isDragOver = ref(false);
const tileRefs = ref<HTMLButtonElement[]>([]);

function handleDragEnter(event: DragEvent) {
  event.preventDefault();
  dragDepth.value += 1;
  isDragOver.value = true;
}

function handleDragOver(event: DragEvent) {
  event.preventDefault();
  isDragOver.value = true;
}

function handleDragLeave() {
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
  if (props.clickMode === 'single') {
    emit('launch', entry);
  }
}

function handleDoubleClick(entry: DisplayItem, index: number) {
  emit('select', index);
  if (props.clickMode === 'double') {
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
    @dragenter="handleDragEnter"
    @dragover="handleDragOver"
    @dragleave="handleDragLeave"
    @drop="handleDrop"
  >
    <!-- Loading skeleton while a cross-group search is running -->
    <div v-if="loading" class="grid grid-cols-[repeat(auto-fill,minmax(92px,1fr))] gap-2.5 content-start">
      <div v-for="n in 8" :key="n" class="h-[86px] rounded-xl jd-surface-soft animate-pulse" />
    </div>

    <!-- Item Grid -->
    <div
      v-else-if="items.length > 0"
      class="grid grid-cols-[repeat(auto-fill,minmax(92px,1fr))] gap-2.5 content-start"
    >
      <button
        v-for="(entry, index) in items"
        :key="`${entry.groupId}:${entry.item.id}`"
        :ref="(el) => setTileRef(el as Element | null, index)"
        type="button"
        draggable="true"
        :title="formatItemTooltip(entry.item)"
        :class="[
          'group relative flex flex-col items-center justify-start gap-2 p-2.5 rounded-xl border transition-all duration-150 focus:outline-none',
          index === selectedIndex
            ? 'jd-surface-strong jd-border-strong ring-1 ring-sky-400/70'
            : 'border-transparent jd-hover',
        ]"
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
        <span
          v-if="mode !== 'group'"
          class="max-w-full truncate text-[10px] jd-subtle"
        >
          {{ entry.groupName }}
        </span>

        <!-- Administrator marker -->
        <span
          v-if="entry.item.admin"
          class="absolute top-1.5 right-1.5 px-1 rounded text-[9px] font-semibold bg-amber-400/20 text-amber-200 border border-amber-300/30"
          title="以管理员身份运行"
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
        <p class="text-sm jd-text">未找到匹配「{{ searchQuery }}」的项目</p>
        <p class="text-xs jd-subtle">可以尝试其他关键词，或清除筛选</p>
      </div>
      <button
        type="button"
        class="px-3 py-1.5 rounded-lg text-xs jd-surface jd-border border jd-text jd-hover transition-colors"
        @click="emit('clearSearch')"
      >
        清除搜索
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
      <p class="text-sm jd-text">还没有启动记录</p>
      <p class="text-xs jd-subtle">用过的项目会出现在这里，方便再次打开</p>
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
          <p class="text-sm font-medium jd-text">当前分组暂无项目</p>
          <p class="text-xs jd-muted leading-relaxed max-w-[260px]">
            拖入 .exe / .lnk / .bat / 文件夹，或用下方的按钮添加
          </p>
        </div>
        <div class="mt-1 flex items-center gap-2">
          <button
            v-if="hasActiveGroup"
            type="button"
            class="px-3 py-1.5 rounded-lg text-xs jd-surface-strong jd-text jd-hover border jd-border transition-colors"
            @click="emit('addItem')"
          >
            + 手动添加
          </button>
          <button
            v-if="hasActiveGroup"
            type="button"
            class="px-3 py-1.5 rounded-lg text-xs jd-surface jd-text jd-hover border jd-border transition-colors"
            @click="emit('openStartMenuImport')"
          >
            从开始菜单导入
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
        <p class="text-sm font-medium text-sky-100">松开即可添加到「{{ activeGroupName || '当前分组' }}」</p>
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
