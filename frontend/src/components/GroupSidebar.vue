<script setup lang="ts">
import type { Group } from '../types/config';

defineProps<{
  groups: Group[];
  activeGroupId: string;
  collapsed: boolean;
}>();

const emit = defineEmits<{
  (e: 'select', groupId: string): void;
  (e: 'addGroup'): void;
  (e: 'toggleCollapse'): void;
  (e: 'contextmenu', event: MouseEvent, group: Group): void;
}>();
</script>

<template>
  <aside
    :class="collapsed ? 'w-12' : 'w-[152px]'"
    class="h-full border-r border-white/10 bg-white/[0.02] flex flex-col shrink-0 transition-all duration-200"
  >
    <!-- Sidebar Header -->
    <div class="px-2 pt-2.5 pb-1.5 flex items-center justify-between">
      <span
        v-if="!collapsed"
        class="px-1.5 text-[11px] font-medium uppercase tracking-wider text-slate-400"
      >
        分组
      </span>
      <button
        type="button"
        :title="collapsed ? '展开分组栏' : '收起分组栏'"
        class="w-7 h-7 rounded-lg flex items-center justify-center text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
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
      <button
        v-for="group in groups"
        :key="group.id"
        type="button"
        :title="group.name"
        class="group relative w-full h-9 rounded-xl flex items-center gap-2 px-2.5 text-xs transition-all duration-150"
        :class="
          group.id === activeGroupId
            ? 'bg-white/15 text-white font-medium border border-white/15 shadow-sm'
            : 'text-slate-300/90 hover:text-white hover:bg-white/[0.07] border border-transparent'
        "
        @click="emit('select', group.id)"
        @contextmenu.prevent="emit('contextmenu', $event, group)"
      >
        <span
          v-if="group.id === activeGroupId"
          class="absolute left-0.5 top-2 bottom-2 w-0.5 rounded-full bg-sky-400"
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
                ? 'bg-white/15 text-white/90'
                : 'text-slate-400 group-hover:text-slate-300'
            "
          >
            {{ group.items?.length ?? 0 }}
          </span>
        </template>
      </button>
    </nav>

    <!-- Add Group Button -->
    <div class="p-2 border-t border-white/10">
      <button
        type="button"
        title="新建分组"
        class="w-full h-8 rounded-xl flex items-center justify-center gap-1.5 text-xs text-slate-300 hover:text-white bg-white/[0.04] hover:bg-white/10 border border-white/10 transition-all"
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
