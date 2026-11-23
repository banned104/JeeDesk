<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';

const props = defineProps<{
  visible: boolean;
  mode: 'add' | 'rename';
  initialName?: string;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'submit', name: string): void;
}>();

const name = ref('');
const inputRef = ref<HTMLInputElement | null>(null);

watch(
  () => [props.visible, props.mode, props.initialName] as const,
  async ([visible]) => {
    if (!visible) return;
    name.value = props.mode === 'rename' ? props.initialName || '' : '';
    await nextTick();
    inputRef.value?.focus();
    inputRef.value?.select();
  }
);

function handleSubmit() {
  const trimmed = name.value.trim();
  if (!trimmed) return;
  emit('submit', trimmed);
}
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
          class="w-full max-w-sm rounded-2xl glass-modal border border-white/15 shadow-2xl overflow-hidden"
        >
          <div class="px-5 py-3.5 border-b border-white/10 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-white">
              {{ mode === 'add' ? '新建分组' : '重命名分组' }}
            </h2>
            <button
              type="button"
              class="w-7 h-7 rounded-lg flex items-center justify-center text-slate-400 hover:text-white hover:bg-white/10 transition-colors"
              @click="emit('close')"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <form class="p-5 space-y-4 text-xs" @submit.prevent="handleSubmit">
            <label class="block space-y-1.5">
              <span class="text-slate-300 font-medium">分组名称</span>
              <input
                ref="inputRef"
                v-model="name"
                type="text"
                required
                placeholder="例如：办公软件、开发工具"
                class="w-full h-9 px-3 rounded-xl bg-white/[0.06] border border-white/10 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-400/60 focus:bg-white/[0.09] transition-colors"
              />
            </label>

            <div class="flex items-center justify-end gap-2">
              <button
                type="button"
                class="px-4 h-8 rounded-xl bg-white/[0.06] hover:bg-white/10 text-slate-300 border border-white/10 transition-colors"
                @click="emit('close')"
              >
                取消
              </button>
              <button
                type="submit"
                :disabled="!name.trim()"
                class="px-4 h-8 rounded-xl bg-sky-500/85 hover:bg-sky-500 disabled:opacity-40 text-white font-medium shadow-sm transition-colors"
              >
                确定
              </button>
            </div>
          </form>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
