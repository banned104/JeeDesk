<script setup lang="ts">
import { ref } from 'vue';
import type { Item } from '../types/config';
import { formatItemTooltip } from '../utils/icon';
import ItemIcon from './ItemIcon.vue';

defineProps<{
  items: Item[];
  activeGroupName: string;
  searchQuery: string;
  cacheBuster: number;
  hasActiveGroup: boolean;
}>();

const emit = defineEmits<{
  (e: 'launch', item: Item): void;
  (e: 'contextmenu', event: MouseEvent, item: Item): void;
  (e: 'clearSearch'): void;
  (e: 'addItem'): void;
}>();

const dragDepth = ref(0);
const isDragOver = ref(false);

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
    <!-- Item Grid -->
    <div
      v-if="items.length > 0"
      class="grid grid-cols-[repeat(auto-fill,minmax(92px,1fr))] gap-2.5 content-start"
    >
      <button
        v-for="item in items"
        :key="item.id"
        type="button"
        :title="formatItemTooltip(item)"
        class="group flex flex-col items-center justify-start gap-2 p-2.5 rounded-xl border border-transparent hover:bg-white/10 hover:border-white/15 active:scale-[0.97] transition-all duration-150 focus:outline-none focus:ring-1 focus:ring-sky-400/50"
        @click="emit('launch', item)"
        @contextmenu.prevent="emit('contextmenu', $event, item)"
      >
        <ItemIcon
          :icon="item.icon"
          :path="item.path"
          :name="item.name"
          :cache-buster="cacheBuster"
          size-class="w-12 h-12 group-hover:scale-105"
        />
        <span class="w-full text-center text-xs text-slate-200/95 group-hover:text-white truncate">
          {{ item.name }}
        </span>
      </button>
    </div>

    <!-- Empty Search Result -->
    <div
      v-else-if="searchQuery"
      class="h-full min-h-[240px] flex flex-col items-center justify-center text-center gap-3 text-slate-400"
    >
      <svg class="w-10 h-10 opacity-50" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="11" cy="11" r="7" />
        <path stroke-linecap="round" d="M20 20l-3.5-3.5" />
      </svg>
      <div class="space-y-1">
        <p class="text-sm text-slate-200">未找到匹配「{{ searchQuery }}」的项目</p>
        <p class="text-xs text-slate-400">可以尝试其他关键词或清除筛选</p>
      </div>
      <button
        type="button"
        class="px-3 py-1.5 rounded-lg text-xs bg-white/10 hover:bg-white/15 text-slate-200 border border-white/10 transition-colors"
        @click="emit('clearSearch')"
      >
        清除搜索
      </button>
    </div>

    <!-- Empty Group Drop Hint -->
    <div
      v-else
      class="h-full min-h-[240px] flex flex-col items-center justify-center text-center"
    >
      <div
        class="max-w-xs w-full px-6 py-8 rounded-2xl border border-dashed border-white/15 bg-white/[0.02] flex flex-col items-center gap-3"
      >
        <div
          class="w-12 h-12 rounded-2xl bg-sky-400/10 border border-sky-300/20 flex items-center justify-center text-sky-300"
        >
          <svg class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 16V4m0 0l-4 4m4-4l4 4" />
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2"
            />
          </svg>
        </div>
        <div class="space-y-1">
          <p class="text-sm font-medium text-slate-200">当前分组暂无项目</p>
          <p class="text-xs text-slate-400 leading-relaxed">
            拖入 .exe / .lnk / .bat / 文件夹添加到当前分组
          </p>
        </div>
        <button
          v-if="hasActiveGroup"
          type="button"
          class="mt-1 px-3 py-1.5 rounded-lg text-xs bg-white/10 hover:bg-white/15 text-slate-100 border border-white/10 transition-colors"
          @click="emit('addItem')"
        >
          + 手动添加项目
        </button>
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
          松开即可添加到「{{ activeGroupName || '当前分组' }}」
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
