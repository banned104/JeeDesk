<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { registerTranslations, t } from '../i18n';
import type { StartMenuEntry } from '../types/config';

registerTranslations({
  '从开始菜单导入': 'Import from Start Menu',
  '选中要加入「{group}」的快捷方式': 'Choose the shortcuts to add to "{group}"',
  '关闭': 'Close',
  '搜索名称或路径': 'Search name or path',
  '清除搜索': 'Clear search',
  '重新扫描开始菜单': 'Rescan the Start Menu',
  '全部': 'All',
  '当前用户': 'Current user',
  '所有用户': 'All users',
  '{count} 项已选': '{count} selected',
  '全选当前结果': 'Select all results',
  '取消选中当前结果': 'Deselect all results',
  '清空选择': 'Clear selection',
  '正在扫描开始菜单…': 'Scanning the Start Menu…',
  '没有找到可导入的快捷方式': 'No shortcuts to import',
  '没有符合筛选条件的快捷方式': 'No shortcuts match the filter',
  '开始菜单可能是空的，或扫描失败；点击重试重新扫描。':
    'The Start Menu may be empty, or the scan failed. Retry to scan again.',
  '可以尝试其他关键词或切换范围': 'Try another keyword or change the scope',
  '重试': 'Retry',
  '扫描失败：{error}': 'Scan failed: {error}',
  '机器': 'Machine',
  '用户': 'User',
  '共 {count} 个快捷方式 · 显示 {shown} 个': '{count} shortcuts · {shown} shown',
  '取消': 'Cancel',
  '导入选中项': 'Import selected',
  '导入选中项（{count}）': 'Import selected ({count})',
  '导入中…': 'Importing…',
});

const props = defineProps<{
  visible: boolean;
  entries: StartMenuEntry[];
  loading: boolean;
  groupName: string;
  /** True while the parent is still importing the selection; disables the submit button. */
  busy?: boolean;
  /** Scan failure reported by the parent, shown instead of the generic empty-list hint. */
  error?: string;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'refresh'): void;
  (e: 'import', lnkPaths: string[]): void;
}>();

type ScopeFilter = 'all' | 'user' | 'machine';

/** A failed import leaves this dialog open, so the busy flag needs a deadline of its own. */
const IMPORT_BUSY_TIMEOUT_MS = 8000;

const scopeOptions = computed<Array<{ value: ScopeFilter; label: string }>>(() => [
  { value: 'all', label: t('全部') },
  { value: 'user', label: t('当前用户') },
  { value: 'machine', label: t('所有用户') },
]);

const searchQuery = ref('');
const scopeFilter = ref<ScopeFilter>('all');
// Selections are replaced as a whole instead of mutated, so every derived count stays reactive.
const selected = ref<Set<string>>(new Set());
const searchRef = ref<HTMLInputElement | null>(null);
const submitting = ref(false);

// --- dialog focus management ----------------------------------------------

const titleId = 'start-menu-import-modal-title';
const dialogRef = ref<HTMLElement | null>(null);
// Element that had focus before the dialog opened, restored once it closes.
let previousFocus: HTMLElement | null = null;

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusableElements(): HTMLElement[] {
  const root = dialogRef.value;
  if (!root) return [];
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
    (el) => el.getClientRects().length > 0
  );
}

// Simple focus trap: Tab / Shift+Tab cycle inside the dialog instead of reaching the page behind it.
function handleTabKey(event: KeyboardEvent) {
  if (event.key !== 'Tab') return;
  const root = dialogRef.value;
  if (!root) return;
  const elements = focusableElements();
  if (elements.length === 0) {
    event.preventDefault();
    root.focus();
    return;
  }
  const first = elements[0];
  const last = elements[elements.length - 1];
  const active = document.activeElement;
  if (event.shiftKey) {
    if (active === first || !root.contains(active)) {
      event.preventDefault();
      last.focus();
    }
    return;
  }
  if (active === last || !root.contains(active)) {
    event.preventDefault();
    first.focus();
  }
}

function rememberFocus() {
  const active = document.activeElement;
  if (active instanceof HTMLElement && !dialogRef.value?.contains(active)) {
    previousFocus = active;
  }
}

function restoreFocus() {
  const target = previousFocus;
  previousFocus = null;
  if (target && document.contains(target)) target.focus();
}

/** The shell handles Esc globally, so it calls this instead of flipping `visible` itself. */
function handleClose() {
  emit('close');
}

defineExpose({ handleClose });

let busyTimer: number | undefined;

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

const busy = computed(() => submitting.value || Boolean(props.busy));

const scanError = computed(() => (props.error ?? '').trim());

/** Empty state hint: a scan failure is spelled out, an empty Start Menu gets the retry advice. */
const emptyHint = computed(() => {
  if (props.entries.length > 0) return t('可以尝试其他关键词或切换范围');
  if (scanError.value) return t('扫描失败：{error}', { error: scanError.value });
  return t('开始菜单可能是空的，或扫描失败；点击重试重新扫描。');
});

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

/** The toolbar button flips between select-all and deselect-all for what is on screen. */
function toggleAllFiltered() {
  if (!allFilteredSelected.value) {
    selectAllFiltered();
    return;
  }
  const next = new Set(selected.value);
  for (const entry of filteredEntries.value) {
    next.delete(entry.lnk_path);
  }
  selected.value = next;
}

function clearSelection() {
  selected.value = new Set();
}

function clearBusyTimer() {
  if (busyTimer !== undefined) {
    window.clearTimeout(busyTimer);
    busyTimer = undefined;
  }
}

function releaseBusy() {
  submitting.value = false;
  clearBusyTimer();
}

function submitImport() {
  // A second click while the parent is still importing would duplicate every selected item.
  if (selectedCount.value === 0 || busy.value) return;
  submitting.value = true;
  clearBusyTimer();
  busyTimer = window.setTimeout(releaseBusy, IMPORT_BUSY_TIMEOUT_MS);
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
    releaseBusy();
    if (!visible) {
      restoreFocus();
      return;
    }
    rememberFocus();
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
    // A rescan replaces the entry list, which also means the previous import finished or failed.
    releaseBusy();
    if (selected.value.size === 0) return;
    // Anything that disappeared from the list cannot be imported anymore.
    const available = new Set(entries.map((entry) => entry.lnk_path));
    const kept = Array.from(selected.value).filter((lnkPath) => available.has(lnkPath));
    if (kept.length !== selected.value.size) {
      selected.value = new Set(kept);
    }
  }
);

onBeforeUnmount(() => {
  clearBusyTimer();
  restoreFocus();
});
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
          ref="dialogRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="w-[560px] max-w-[calc(100vw-2rem)] h-[520px] max-h-[90vh] rounded-2xl glass-modal border jd-border-strong shadow-2xl overflow-hidden flex flex-col focus:outline-none"
          @keydown="handleTabKey"
        >
          <!-- Header -->
          <div class="px-5 py-3.5 border-b jd-border flex items-start justify-between gap-3 shrink-0">
            <div class="min-w-0">
              <h2 :id="titleId" class="text-sm font-semibold jd-text">{{ t('从开始菜单导入') }}</h2>
              <p class="mt-0.5 text-[11px] jd-subtle truncate" :title="groupName">
                {{ t('选中要加入「{group}」的快捷方式', { group: groupName }) }}
              </p>
            </div>
            <button
              type="button"
              :aria-label="t('关闭')"
              :title="t('关闭')"
              class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover transition-colors shrink-0 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
              @click="handleClose"
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
                  :placeholder="t('搜索名称或路径')"
                  class="w-full h-8 pl-7 pr-6 rounded-lg jd-input border text-xs transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/60"
                />
                <button
                  v-if="searchQuery"
                  type="button"
                  :title="t('清除搜索')"
                  class="absolute right-1.5 p-0.5 rounded jd-subtle jd-hover transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="searchQuery = ''"
                >
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
                    <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
                  </svg>
                </button>
              </div>

              <button
                type="button"
                :title="t('重新扫描开始菜单')"
                :disabled="loading || busy"
                class="w-8 h-8 rounded-lg flex items-center justify-center jd-surface border jd-border jd-muted jd-hover disabled:opacity-40 disabled:pointer-events-none transition-colors shrink-0 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
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
                  class="px-2 h-6 rounded-md text-[11px] transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  :class="scopeFilter === option.value ? 'jd-surface-strong jd-text' : 'jd-muted jd-hover'"
                  @click="scopeFilter = option.value"
                >
                  {{ option.label }}
                </button>
              </div>

              <div class="flex items-center gap-1.5 shrink-0">
                <span class="text-[11px] jd-subtle tabular-nums">
                  {{ t('{count} 项已选', { count: selectedCount }) }}
                </span>
                <button
                  type="button"
                  :disabled="filteredEntries.length === 0 || busy"
                  class="px-2 h-6 rounded-md text-[11px] jd-surface border jd-border jd-muted jd-hover disabled:opacity-40 disabled:pointer-events-none transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="toggleAllFiltered"
                >
                  {{ allFilteredSelected ? t('取消选中当前结果') : t('全选当前结果') }}
                </button>
                <button
                  type="button"
                  :disabled="selectedCount === 0 || busy"
                  class="px-2 h-6 rounded-md text-[11px] jd-surface border jd-border jd-muted jd-hover disabled:opacity-40 disabled:pointer-events-none transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                  @click="clearSelection"
                >
                  {{ t('清空选择') }}
                </button>
              </div>
            </div>
          </div>

          <!-- Entry list -->
          <div class="flex-1 min-h-0 overflow-y-auto custom-scrollbar px-2 py-2">
            <div v-if="loading" class="px-1 py-1 space-y-2">
              <p class="px-2 text-[11px] jd-subtle">{{ t('正在扫描开始菜单…') }}</p>
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
                {{ entries.length === 0 ? t('没有找到可导入的快捷方式') : t('没有符合筛选条件的快捷方式') }}
              </p>
              <p class="text-[11px] jd-subtle max-w-[320px] leading-relaxed break-all">
                {{ emptyHint }}
              </p>
              <button
                v-if="entries.length === 0"
                type="button"
                class="mt-1 h-7 px-3 rounded-lg border text-[11px] transition-colors jd-border jd-surface jd-muted jd-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                @click="emit('refresh')"
              >
                {{ t('重试') }}
              </button>
            </div>

            <template v-else>
              <label
                v-for="entry in filteredEntries"
                :key="entry.lnk_path"
                class="flex items-center gap-2.5 px-2 py-1.5 rounded-lg border cursor-pointer select-none transition-colors"
                :class="isSelected(entry) ? 'jd-surface-strong jd-border-strong' : 'border-transparent jd-hover'"
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

                <span class="px-1.5 py-0.5 rounded-md text-[10px] border jd-border jd-surface-soft jd-muted shrink-0">
                  {{ entry.scope === 'machine' ? t('机器') : t('用户') }}
                </span>
              </label>
            </template>
          </div>

          <!-- Footer -->
          <div class="px-5 py-3 border-t jd-border flex items-center justify-between gap-3 shrink-0">
            <span class="text-[11px] jd-subtle truncate">
              {{ t('共 {count} 个快捷方式 · 显示 {shown} 个', { count: entries.length, shown: filteredEntries.length }) }}
            </span>
            <div class="flex items-center gap-2 shrink-0">
              <button
                type="button"
                class="px-4 h-8 rounded-xl text-xs jd-surface jd-border border jd-muted jd-hover transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/70"
                @click="emit('close')"
              >
                {{ t('取消') }}
              </button>
              <button
                type="button"
                :disabled="selectedCount === 0 || busy"
                class="px-4 h-8 rounded-xl text-xs bg-sky-500/85 hover:bg-sky-500 disabled:opacity-40 disabled:pointer-events-none text-white font-medium shadow-sm transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-white/70"
                @click="submitImport"
              >
                {{ busy ? t('导入中…') : selectedCount > 0 ? t('导入选中项（{count}）', { count: selectedCount }) : t('导入选中项') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
