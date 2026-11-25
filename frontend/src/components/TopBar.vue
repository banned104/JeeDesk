<script setup lang="ts">
import { computed, ref } from 'vue';
import { bridge } from '../api/bridge';
import { registerTranslations, t } from '../i18n';

registerTranslations({
  '搜索项目…（/）': 'Search items… (/)',
  '搜索项目': 'Search items',
  '清除搜索': 'Clear search',
  '手动添加项目': 'Add an item manually',
  '添加': 'Add',
  '设置': 'Settings',
  '隐藏窗口': 'Hide window',
  '搜索结果': 'Search results',
  '浏览器 mock': 'Browser mock',
  'Alt+←/→ 切换分组（Ctrl+Tab 也可）': 'Alt+←/→ switch groups (Ctrl+Tab also works)',
});

const props = defineProps<{
  searchQuery: string;
  activeGroupName?: string;
  /** Items currently on screen: the hits while searching, otherwise the active group's items. */
  itemCount?: number;
  /** Items in the whole config, used as the denominator while searching. */
  totalCount?: number;
}>();

const emit = defineEmits<{
  (e: 'update:searchQuery', value: string): void;
  (e: 'addItem'): void;
  (e: 'openSettings'): void;
  (e: 'hide'): void;
}>();

const searchInputRef = ref<HTMLInputElement | null>(null);

// Outside the Wails runtime the bridge answers from an in-memory mock. Saying so on screen stops a
// mock-only defect (or a mock-only success) from being taken for real behaviour.
const isMock = !bridge.isWails();

const searchActive = computed(() => props.searchQuery.trim().length > 0);

// E7: while searching the badge states "hits / total", otherwise just the count.
const countText = computed(() => {
  if (searchActive.value) {
    return `${props.itemCount ?? 0} / ${props.totalCount ?? 0}`;
  }
  return `${props.itemCount ?? 0}`;
});

// App.vue drives "type anywhere" and the "/" shortcut through these two methods.
function focusSearch() {
  const input = searchInputRef.value;
  if (!input) return;
  input.focus();
  input.select();
}

function blurSearch() {
  searchInputRef.value?.blur();
}

defineExpose({ focusSearch, blurSearch });
</script>

<template>
  <header
    class="wails-drag h-11 px-3.5 flex items-center justify-between gap-3 border-b jd-border jd-surface-soft shrink-0"
    style="--wails-draggable: drag"
  >
    <!-- Brand -->
    <div class="flex items-center gap-2.5 min-w-0">
      <div class="w-6 h-6 rounded-lg jd-surface border jd-border-strong flex items-center justify-center shrink-0 p-[1px]">
        <img src="/brand-m9.png" alt="" class="w-full h-full object-contain scale-110" aria-hidden="true" />
      </div>
      <span class="text-sm font-semibold tracking-wide jd-text">JeeDesk</span>
      <span
        v-if="isMock"
        class="px-1.5 py-0.5 rounded text-[10px] font-semibold tracking-wider uppercase bg-amber-500/20 text-amber-300 border border-amber-400/30"
        :title="t('浏览器 mock')"
      >
        mock
      </span>
      <span
        v-if="activeGroupName || searchActive"
        :title="t('Alt+←/→ 切换分组（Ctrl+Tab 也可）')"
        class="hidden sm:inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] jd-surface-soft jd-muted border jd-border truncate max-w-[180px]"
      >
        <span class="truncate">{{ searchActive ? t('搜索结果') : activeGroupName }}</span>
        <span v-if="typeof itemCount === 'number'" class="jd-subtle whitespace-nowrap">· {{ countText }}</span>
      </span>
    </div>

    <!-- Controls (no-drag) -->
    <div class="wails-no-drag flex items-center gap-1.5" style="--wails-draggable: no-drag">
      <!-- Quick Filter -->
      <div class="relative flex items-center">
        <svg
          class="w-3.5 h-3.5 jd-subtle absolute left-2.5 pointer-events-none"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          aria-hidden="true"
        >
          <circle cx="11" cy="11" r="7" />
          <path stroke-linecap="round" d="M20 20l-3.5-3.5" />
        </svg>
        <input
          ref="searchInputRef"
          :value="searchQuery"
          type="text"
          data-jd-search
          :placeholder="t('搜索项目…（/）')"
          :aria-label="t('搜索项目')"
          class="w-36 sm:w-44 h-7 pl-7 pr-6 rounded-lg jd-input border text-xs transition-all"
          @input="emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
        />
        <button
          v-if="searchQuery"
          type="button"
          :aria-label="t('清除搜索')"
          :title="t('清除搜索')"
          class="absolute right-1.5 p-0.5 rounded jd-subtle jd-hover transition-colors"
          @click="emit('update:searchQuery', '')"
        >
          <svg
            class="w-3 h-3"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
            aria-hidden="true"
          >
            <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
          </svg>
        </button>
      </div>

      <!-- Add Item -->
      <button
        type="button"
        :aria-label="t('手动添加项目')"
        :title="t('手动添加项目')"
        class="h-7 px-2 rounded-lg flex items-center gap-1 text-xs jd-muted jd-hover border border-transparent transition-all"
        @click="emit('addItem')"
      >
        <svg
          class="w-3.5 h-3.5"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.2"
          aria-hidden="true"
        >
          <path stroke-linecap="round" d="M12 5v14M5 12h14" />
        </svg>
        <span class="hidden sm:inline">{{ t('添加') }}</span>
      </button>

      <!-- Settings -->
      <button
        type="button"
        :aria-label="t('设置')"
        :title="t('设置')"
        class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover border border-transparent transition-all"
        @click="emit('openSettings')"
      >
        <svg
          class="w-4 h-4"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.9"
          aria-hidden="true"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
          />
          <circle cx="12" cy="12" r="3" />
        </svg>
      </button>

      <!-- Hide / Close -->
      <button
        type="button"
        :aria-label="t('隐藏窗口')"
        :title="t('隐藏窗口') + ' (Esc)'"
        class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted hover:text-rose-200 hover:bg-rose-500/20 border border-transparent hover:border-rose-400/20 transition-all"
        @click="emit('hide')"
      >
        <svg
          class="w-4 h-4"
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
  </header>
</template>
