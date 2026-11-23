<script setup lang="ts">
defineProps<{
  searchQuery: string;
  activeGroupName?: string;
  itemCount?: number;
}>();

const emit = defineEmits<{
  (e: 'update:searchQuery', value: string): void;
  (e: 'addItem'): void;
  (e: 'openSettings'): void;
  (e: 'hide'): void;
}>();
</script>

<template>
  <header
    class="wails-drag h-11 px-3.5 flex items-center justify-between gap-3 border-b border-white/10 bg-white/[0.03] shrink-0"
    style="--wails-draggable: drag"
  >
    <!-- Brand -->
    <div class="flex items-center gap-2.5 min-w-0">
      <div
        class="w-6 h-6 rounded-lg bg-gradient-to-br from-sky-400/90 to-indigo-500/90 flex items-center justify-center shadow-sm border border-white/20 shrink-0"
      >
        <svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
          <rect x="3" y="3" width="7" height="7" rx="1.5" />
          <rect x="14" y="3" width="7" height="7" rx="1.5" />
          <rect x="3" y="14" width="7" height="7" rx="1.5" />
          <rect x="14" y="14" width="7" height="7" rx="1.5" />
        </svg>
      </div>
      <span class="text-sm font-semibold tracking-wide text-white/95">JeeDesk</span>
      <span
        v-if="activeGroupName"
        class="hidden sm:inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] bg-white/[0.07] text-slate-300 border border-white/10 truncate max-w-[140px]"
      >
        <span class="truncate">{{ activeGroupName }}</span>
        <span v-if="typeof itemCount === 'number'" class="text-slate-400">· {{ itemCount }}</span>
      </span>
    </div>

    <!-- Controls (no-drag) -->
    <div class="wails-no-drag flex items-center gap-1.5" style="--wails-draggable: no-drag">
      <!-- Quick Filter -->
      <div class="relative flex items-center">
        <svg
          class="w-3.5 h-3.5 text-slate-400 absolute left-2.5 pointer-events-none"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <circle cx="11" cy="11" r="7" />
          <path stroke-linecap="round" d="M20 20l-3.5-3.5" />
        </svg>
        <input
          :value="searchQuery"
          type="text"
          placeholder="搜索项目..."
          class="w-36 sm:w-44 h-7 pl-7 pr-6 rounded-lg bg-white/[0.06] border border-white/10 text-xs text-slate-100 placeholder:text-slate-400 focus:outline-none focus:bg-white/[0.1] focus:border-sky-400/50 transition-all"
          @input="emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
        />
        <button
          v-if="searchQuery"
          type="button"
          title="清除搜索"
          class="absolute right-1.5 text-slate-400 hover:text-white p-0.5 rounded transition-colors"
          @click="emit('update:searchQuery', '')"
        >
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
            <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
          </svg>
        </button>
      </div>

      <!-- Add Item -->
      <button
        type="button"
        title="手动添加项目"
        class="h-7 px-2 rounded-lg flex items-center gap-1 text-xs text-slate-300 hover:text-white hover:bg-white/10 border border-transparent hover:border-white/10 transition-all"
        @click="emit('addItem')"
      >
        <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
          <path stroke-linecap="round" d="M12 5v14M5 12h14" />
        </svg>
        <span class="hidden sm:inline">添加</span>
      </button>

      <!-- Settings -->
      <button
        type="button"
        title="设置"
        class="w-7 h-7 rounded-lg flex items-center justify-center text-slate-300 hover:text-white hover:bg-white/10 border border-transparent hover:border-white/10 transition-all"
        @click="emit('openSettings')"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9">
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
        title="隐藏窗口 (Esc)"
        class="w-7 h-7 rounded-lg flex items-center justify-center text-slate-300 hover:text-rose-200 hover:bg-rose-500/20 border border-transparent hover:border-rose-400/20 transition-all"
        @click="emit('hide')"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
        </svg>
      </button>
    </div>
  </header>
</template>
