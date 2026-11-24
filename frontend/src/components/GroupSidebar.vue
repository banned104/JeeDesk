<script setup lang="ts">
import { ref } from 'vue';
import type { Group } from '../types/config';
import { RECENT_GROUP_ID } from '../types/config';

withDefaults(
  defineProps<{
    groups: Group[];
    activeGroupId: string;
    collapsed: boolean;
    showRecent?: boolean;
    recentCount?: number;
  }>(),
  { showRecent: false, recentCount: 0 }
);

const emit = defineEmits<{
  (e: 'select', groupId: string): void;
  (e: 'addGroup'): void;
  (e: 'toggleCollapse'): void;
  (e: 'contextmenu', event: MouseEvent, group: Group): void;
  (e: 'dropItem', payload: { groupId: string; fromGroupId: string; itemId: string }): void;
}>();

const dropTargetId = ref('');

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
      <span
        v-if="!collapsed"
        class="px-1.5 text-[11px] font-medium uppercase tracking-wider jd-muted"
      >
        分组
      </span>
      <button
        type="button"
        :title="collapsed ? '展开分组栏' : '收起分组栏'"
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
        title="最近使用"
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
          <span class="mx-auto font-semibold text-[11px]">最</span>
        </template>
        <template v-else>
          <svg class="w-3.5 h-3.5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9">
            <circle cx="12" cy="12" r="9" />
            <path stroke-linecap="round" d="M12 7v5l3 2" />
          </svg>
          <span class="truncate flex-1 text-left">最近使用</span>
          <span class="text-[10px] px-1.5 py-0.5 rounded-md jd-surface">{{ recentCount }}</span>
        </template>
      </button>

      <button
        v-for="group in groups"
        :key="group.id"
        type="button"
        :title="group.name"
        class="group relative w-full h-9 rounded-xl flex items-center gap-2 px-2.5 text-xs transition-all duration-150"
        :class="[
          group.id === activeGroupId
            ? 'jd-surface-strong jd-text font-medium border jd-border-strong shadow-sm'
            : 'jd-muted jd-hover border border-transparent',
          dropTargetId === group.id ? 'ring-2 ring-sky-400/80' : '',
        ]"
        :data-group-id="group.id"
        @click="emit('select', group.id)"
        @contextmenu.prevent="emit('contextmenu', $event, group)"
        @dragover="handleDragOver($event, group.id)"
        @dragleave="handleDragLeave(group.id)"
        @drop="handleDrop($event, group.id)"
      >
        <span
          v-if="group.id === activeGroupId"
          class="absolute left-0.5 top-2 bottom-2 w-0.5 rounded-full jd-accent-bg"
        />

        <template v-if="collapsed">
          <span class="mx-auto font-semibold text-[11px]">
            {{ group.name.slice(0, 1).toUpperCase() || '#' }}
          </span>
        </template>
        <template v-else>
          <span class="truncate flex-1 text-left">{{ group.name }}</span>
          <span
            class="text-[10px] px-1.5 py-0.5 rounded-md transition-colors"
            :class="
              group.id === activeGroupId
                ? 'jd-surface jd-text'
                : 'jd-subtle'
            "
          >
            {{ group.items?.length ?? 0 }}
          </span>
        </template>
      </button>
    </nav>

    <!-- Add Group Button -->
    <div class="p-2 border-t jd-border">
      <button
        type="button"
        title="新建分组"
        class="w-full h-8 rounded-xl flex items-center justify-center gap-1.5 text-xs jd-surface-soft jd-muted jd-hover border jd-border transition-all"
        @click="emit('addGroup')"
      >
        <svg class="w-3.5 h-3.5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
          <path stroke-linecap="round" d="M12 5v14M5 12h14" />
        </svg>
        <span v-if="!collapsed" class="truncate">新建分组</span>
      </button>
    </div>
  </aside>
</template>
