<script setup lang="ts">
import { computed, ref } from 'vue';
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
  (e: 'dropItem', payload: { groupId: string; fromGroupId: string; itemId: string }): void;
}>();

const dropTargetId = ref('');
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

function dragPayload(event: DragEvent): { groupId: string; itemId: string } | null {
  const raw = event.dataTransfer?.getData('application/x-jeedesk-item');
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as { groupId?: string; itemId?: string };
    if (!parsed.groupId || !parsed.itemId) return null;
    return { groupId: parsed.groupId, itemId: parsed.itemId };
  } catch {
    return null;
  }
}

function handleDragOver(event: DragEvent, groupId: string) {
  if (!event.dataTransfer) return;
  // Only react to items dragged out of the grid, not to files from Explorer.
  if (!event.dataTransfer.types.includes('application/x-jeedesk-item')) return;
  event.preventDefault();
  event.dataTransfer.dropEffect = 'move';
  dropTargetId.value = groupId;
}

function handleDragLeave(groupId: string) {
  if (dropTargetId.value === groupId) {
    dropTargetId.value = '';
  }
}

function handleDrop(event: DragEvent, groupId: string) {
  const payload = dragPayload(event);
  dropTargetId.value = '';
  if (!payload || payload.groupId === groupId) return;
  event.preventDefault();
  emit('dropItem', { groupId, fromGroupId: payload.groupId, itemId: payload.itemId });
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
    <nav class="flex-1 overflow-y-auto custom-scrollbar px-1.5 space-y-1">
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
        v-for="group in visibleGroups"
        :key="group.id"
        class="group relative w-full h-9 rounded-xl transition-all duration-150"
        :class="[
          group.id === activeGroupId
            ? 'jd-surface-strong jd-text font-medium border jd-border-strong shadow-sm'
            : 'jd-muted jd-hover border border-transparent',
          dropTargetId === group.id ? 'ring-2 ring-sky-400/80' : '',
        ]"
        :data-group-id="group.id"
        @contextmenu.prevent="emit('contextmenu', $event, group)"
        @dragover="handleDragOver($event, group.id)"
        @dragleave="handleDragLeave(group.id)"
        @drop="handleDrop($event, group.id)"
      >
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
