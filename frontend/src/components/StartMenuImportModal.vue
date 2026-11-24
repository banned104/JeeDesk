<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import type { StartMenuEntry } from '../types/config';

const props = defineProps<{
  visible: boolean;
  entries: StartMenuEntry[];
  loading: boolean;
  groupName: string;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'refresh'): void;
  (e: 'import', lnkPaths: string[]): void;
}>();

type ScopeFilter = 'all' | 'user' | 'machine';

const scopeOptions: Array<{ value: ScopeFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'user', label: '当前用户' },
  { value: 'machine', label: '所有用户' },
];

const searchQuery = ref('');
const scopeFilter = ref<ScopeFilter>('all');
// Selections are replaced as a whole instead of mutated, so every derived count stays reactive.
const selected = ref<Set<string>>(new Set());
const searchRef = ref<HTMLInputElement | null>(null);

const filteredEntries = computed(() => {
  const query = searchQuery.value.trim().toLowerCase();
  return props.entries.filter((entry) => {
    if (scopeFilter.value !== 'all' && entry.scope !== scopeFilter.value) return false;
    if (!query) return true;
    return (
      entry.name.toLowerCase().includes(query) ||
      entry.path.toLowerCase().includes(query) ||
      entry.lnk_path.toLowerCase().includes(query)
    );
  });
});

const selectedCount = computed(() => selected.value.size);

const allFilteredSelected = computed(
  () => filteredEntries.value.length > 0 && filteredEntries.value.every((e) => selected.value.has(e.lnk_path))
);

function isSelected(entry: StartMenuEntry): boolean {
  return selected.value.has(entry.lnk_path);
}

function toggleEntry(entry: StartMenuEntry) {
  const next = new Set(selected.value);
  if (next.has(entry.lnk_path)) {
    next.delete(entry.lnk_path);
  } else {
    next.add(entry.lnk_path);
  }
  selected.value = next;
}

function selectAllFiltered() {
  const next = new Set(selected.value);
  for (const entry of filteredEntries.value) {
    next.add(entry.lnk_path);
  }
  selected.value = next;
}

function clearSelection() {
  selected.value = new Set();
}

function submitImport() {
  if (selectedCount.value === 0) return;
  // The parent decides when to close, so the selection stays intact if the import fails.
  emit('import', Array.from(selected.value));
}

function initialOf(name: string): string {
  const trimmed = name.trim();
  return trimmed ? trimmed.slice(0, 1).toUpperCase() : '·';
}

watch(
  () => props.visible,
  async (visible) => {
    if (!visible) return;
    // A reopened dialog always starts clean; the parent keeps this component mounted between scans.
    selected.value = new Set();
    searchQuery.value = '';
    scopeFilter.value = 'all';
    await nextTick();
    searchRef.value?.focus();
  },
  { immediate: true }
);

watch(
  () => props.entries,
  (entries) => {
    if (selected.value.size === 0) return;
    // A rescan replaces the entry list; anything that disappeared from it cannot be imported anymore.
    const available = new Set(entries.map((entry) => entry.lnk_path));
    const kept = Array.from(selected.value).filter((lnkPath) => available.has(lnkPath));
    if (kept.length !== selected.value.size) {
      selected.value = new Set(kept);
    }
  }
);
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="visible"
        class="fixed inset-0 z-40 bg-black/50 backdrop-blur-[2px] flex items-center justify-center p-4"
        @mousedown.self="emit('close')"
      >
        <div
          class="w-[560px] max-w-[calc(100vw-2rem)] h-[520px] max-h-[calc(100vh-2rem)] rounded-2xl glass-modal border jd-border-strong shadow-2xl overflow-hidden flex flex-col"
        >
          <!-- Header -->
          <div class="px-5 py-3.5 border-b jd-border flex items-start justify-between gap-3 shrink-0">
            <div class="min-w-0">
              <h2 class="text-sm font-semibold jd-text">从开始菜单导入</h2>
              <p class="mt-0.5 text-[11px] jd-subtle truncate" :title="groupName">
                选中要加入「{{ groupName }}」的快捷方式
              </p>
            </div>
            <button
              type="button"
              title="关闭"
              class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover transition-colors shrink-0"
              @click="emit('close')"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Toolbar -->
          <div class="px-4 py-2.5 border-b jd-border space-y-2 shrink-0">
            <div class="flex items-center gap-2">
              <div class="relative flex-1 min-w-0 flex items-center">
                <svg
                  class="w-3.5 h-3.5 jd-subtle absolute left-2.5 pointer-events-none"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <circle cx="11" cy="11" r="7" />
                  <path stroke-linecap="round" d="M20 20l-3.5-3.5" />
                </svg>
                <input
                  ref="searchRef"
                  v-model="searchQuery"
                  type="text"
                  placeholder="搜索名称或路径"
                  class="w-full h-8 pl-7 pr-6 rounded-lg jd-input border text-xs transition-colors"
                />
                <button
                  v-if="searchQuery"
                  type="button"
                  title="清除搜索"
                  class="absolute right-1.5 p-0.5 rounded jd-subtle jd-hover transition-colors"
                  @click="searchQuery = ''"
                >
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
                    <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
                  </svg>
                </button>
              </div>

              <button
                type="button"
                title="重新扫描开始菜单"
                :disabled="loading"
                class="w-8 h-8 rounded-lg flex items-center justify-center jd-surface border jd-border jd-muted jd-hover disabled:opacity-40 disabled:pointer-events-none transition-colors shrink-0"
                @click="emit('refresh')"
              >
                <svg
                  class="w-3.5 h-3.5"
                  :class="loading ? 'animate-spin' : ''"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M20 11a8 8 0 10-2.5 6.4M20 4v7h-7" />
                </svg>
              </button>
            </div>

            <div class="flex items-center justify-between gap-2">
              <div class="flex items-center gap-0.5 p-0.5 rounded-lg jd-surface-soft border jd-border">
                <button
                  v-for="option in scopeOptions"
                  :key="option.value"
                  type="button"
                  class="px-2 h-6 rounded-md text-[11px] transition-colors"
                  :class="
                    scopeFilter === option.value ? 'jd-surface-strong jd-text' : 'jd-muted jd-hover'
                  "
                  @click="scopeFilter = option.value"
                >
                  {{ option.label }}
                </button>
              </div>

              <div class="flex items-center gap-1.5 shrink-0">
                <span class="text-[11px] jd-subtle tabular-nums">{{ selectedCount }} 项已选</span>
                <button
                  type="button"
                  :disabled="allFilteredSelected || filteredEntries.length === 0"
                  class="px-2 h-6 rounded-md text-[11px] jd-surface border jd-border jd-muted jd-hover disabled:opacity-40 disabled:pointer-events-none transition-colors"
                  @click="selectAllFiltered"
                >
                  全选当前结果
                </button>
                <button
                  type="button"
                  :disabled="selectedCount === 0"
                  class="px-2 h-6 rounded-md text-[11px] jd-surface border jd-border jd-muted jd-hover disabled:opacity-40 disabled:pointer-events-none transition-colors"
                  @click="clearSelection"
                >
                  清空选择
                </button>
              </div>
            </div>
          </div>

          <!-- Entry list -->
          <div class="flex-1 min-h-0 overflow-y-auto custom-scrollbar px-2 py-2">
            <div v-if="loading" class="px-1 py-1 space-y-2">
              <p class="px-2 text-[11px] jd-subtle">正在扫描开始菜单…</p>
              <div v-for="n in 5" :key="n" class="h-11 rounded-lg jd-surface-soft animate-pulse" />
            </div>

            <div
              v-else-if="filteredEntries.length === 0"
              class="flex flex-col items-center justify-center gap-2 py-12 text-center"
            >
              <svg
                class="w-8 h-8 jd-subtle"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.6"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M3 7a2 2 0 012-2h4.172a2 2 0 011.414.586l1.828 1.828A2 2 0 0013.828 8H19a2 2 0 012 2v7a2 2 0 01-2 2H5a2 2 0 01-2-2V7z"
                />
              </svg>
              <p class="text-xs jd-text">
                {{ entries.length === 0 ? '没有找到可导入的快捷方式' : '没有符合筛选条件的快捷方式' }}
              </p>
              <p class="text-[11px] jd-subtle">
                {{ entries.length === 0 ? '可以稍后点击刷新重新扫描' : '可以尝试其他关键词或切换范围' }}
              </p>
            </div>

            <template v-else>
              <label
                v-for="entry in filteredEntries"
                :key="entry.lnk_path"
                class="flex items-center gap-2.5 px-2 py-1.5 rounded-lg border cursor-pointer select-none transition-colors"
                :class="
                  isSelected(entry) ? 'jd-surface-strong jd-border-strong' : 'border-transparent jd-hover'
                "
              >
                <input
                  type="checkbox"
                  class="peer sr-only"
                  :checked="isSelected(entry)"
                  @change="toggleEntry(entry)"
                />
                <span
                  class="w-4 h-4 rounded-[5px] border flex items-center justify-center shrink-0 transition-colors peer-focus-visible:ring-2 peer-focus-visible:ring-sky-400/60"
                  :class="isSelected(entry) ? 'jd-accent-bg border-transparent' : 'jd-border jd-surface-soft'"
                >
                  <svg
                    v-if="isSelected(entry)"
                    class="w-3 h-3 text-white"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="3"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                </span>

                <span
                  class="w-7 h-7 rounded-lg border jd-border jd-surface flex items-center justify-center shrink-0 text-[11px] font-semibold jd-text"
                >
                  {{ initialOf(entry.name) }}
                </span>

                <span class="min-w-0 flex-1">
                  <span class="block text-xs jd-text truncate" :title="entry.name">{{ entry.name }}</span>
                  <span class="block text-[10px] jd-subtle truncate" :title="entry.path">{{ entry.path }}</span>
                </span>

                <span
                  class="px-1.5 py-0.5 rounded-md text-[10px] border jd-border jd-surface-soft jd-muted shrink-0"
                >
                  {{ entry.scope === 'machine' ? '机器' : '用户' }}
                </span>
              </label>
            </template>
          </div>

          <!-- Footer -->
          <div class="px-5 py-3 border-t jd-border flex items-center justify-between gap-3 shrink-0">
            <span class="text-[11px] jd-subtle truncate">
              共 {{ entries.length }} 个快捷方式 · 显示 {{ filteredEntries.length }} 个
            </span>
            <div class="flex items-center gap-2 shrink-0">
              <button
                type="button"
                class="px-4 h-8 rounded-xl text-xs jd-surface jd-border border jd-muted jd-hover transition-colors"
                @click="emit('close')"
              >
                取消
              </button>
              <button
                type="button"
                :disabled="selectedCount === 0"
                class="px-4 h-8 rounded-xl text-xs bg-sky-500/85 hover:bg-sky-500 disabled:opacity-40 disabled:pointer-events-none text-white font-medium shadow-sm transition-colors"
                @click="submitImport"
              >
                导入选中项<template v-if="selectedCount > 0">（{{ selectedCount }}）</template>
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
