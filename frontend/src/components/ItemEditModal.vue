<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { Item } from '../types/config';
import { bridge } from '../api/bridge';
import { inferNameFromPath } from '../utils/icon';
import ItemIcon from './ItemIcon.vue';

const props = defineProps<{
  visible: boolean;
  item: Item | null;
  groupId: string;
  cacheBuster: number;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'save', payload: { isNew: boolean; groupId: string; item: Item }): void;
}>();

const isNew = computed(() => !props.item?.id);
const saving = ref(false);

const form = reactive<Item>({
  id: '',
  name: '',
  path: '',
  args: '',
  work_dir: '',
  icon: '',
});

watch(
  () => [props.visible, props.item] as const,
  ([visible, item]) => {
    if (!visible) return;
    if (item) {
      form.id = item.id;
      form.name = item.name;
      form.path = item.path;
      form.args = item.args;
      form.work_dir = item.work_dir;
      form.icon = item.icon;
    } else {
      form.id = '';
      form.name = '';
      form.path = '';
      form.args = '';
      form.work_dir = '';
      form.icon = '';
    }
    saving.value = false;
  },
  { immediate: true }
);

async function browseTargetFile() {
  const picked = await bridge.pickFile(
    '选择启动目标',
    '可执行文件与快捷方式 (*.exe;*.lnk;*.bat;*.cmd)',
    '*.exe;*.lnk;*.bat;*.cmd;*'
  );
  if (picked) {
    form.path = picked;
    if (!form.name.trim()) {
      form.name = inferNameFromPath(picked);
    }
  }
}

async function browseWorkDir() {
  const picked = await bridge.pickDirectory('选择工作目录');
  if (picked) {
    form.work_dir = picked;
  }
}

async function browseIconFile() {
  const picked = await bridge.pickFile(
    '选择自定义图标',
    '图标或图片 (*.ico;*.png;*.jpg;*.jpeg;*.exe)',
    '*.ico;*.png;*.jpg;*.jpeg;*.exe'
  );
  if (picked) {
    form.icon = picked;
  }
}

function handleSubmit() {
  if (!form.path.trim() || !props.groupId) return;
  saving.value = true;
  const finalName = form.name.trim() || inferNameFromPath(form.path) || '未命名项目';
  emit('save', {
    isNew: isNew.value,
    groupId: props.groupId,
    item: {
      id: form.id,
      name: finalName,
      path: form.path.trim(),
      args: form.args.trim(),
      work_dir: form.work_dir.trim(),
      icon: form.icon.trim(),
    },
  });
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
          class="w-full max-w-md rounded-2xl glass-modal border border-white/15 shadow-2xl overflow-hidden"
        >
          <!-- Modal Header -->
          <div class="px-5 py-3.5 border-b border-white/10 flex items-center justify-between">
            <div class="flex items-center gap-3">
              <ItemIcon
                :icon="form.icon"
                :path="form.path"
                :name="form.name"
                :cache-buster="cacheBuster"
                size-class="w-8 h-8"
              />
              <h2 class="text-sm font-semibold text-white">
                {{ isNew ? '添加项目' : '编辑项目' }}
              </h2>
            </div>
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

          <!-- Form -->
          <form class="p-5 space-y-3.5 text-xs" @submit.prevent="handleSubmit">
            <!-- Name -->
            <label class="block space-y-1.5">
              <span class="text-slate-300 font-medium">显示名称</span>
              <input
                v-model="form.name"
                type="text"
                placeholder="例如：VS Code（留空则自动提取文件名）"
                class="w-full h-9 px-3 rounded-xl bg-white/[0.06] border border-white/10 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-400/60 focus:bg-white/[0.09] transition-colors"
              />
            </label>

            <!-- Path -->
            <label class="block space-y-1.5">
              <span class="text-slate-300 font-medium">
                目标路径 <span class="text-rose-400">*</span>
              </span>
              <div class="flex gap-2">
                <input
                  v-model="form.path"
                  type="text"
                  required
                  placeholder="C:\Path\To\App.exe 或文件夹路径"
                  class="flex-1 h-9 px-3 rounded-xl bg-white/[0.06] border border-white/10 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-400/60 focus:bg-white/[0.09] transition-colors"
                />
                <button
                  type="button"
                  class="px-3 h-9 rounded-xl bg-white/10 hover:bg-white/15 border border-white/10 text-slate-200 shrink-0 transition-colors"
                  @click="browseTargetFile"
                >
                  浏览...
                </button>
              </div>
            </label>

            <!-- Args -->
            <label class="block space-y-1.5">
              <span class="text-slate-300 font-medium">启动参数</span>
              <input
                v-model="form.args"
                type="text"
                placeholder="可选，例如：--reuse-window"
                class="w-full h-9 px-3 rounded-xl bg-white/[0.06] border border-white/10 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-400/60 focus:bg-white/[0.09] transition-colors"
              />
            </label>

            <!-- Work Dir -->
            <label class="block space-y-1.5">
              <span class="text-slate-300 font-medium">工作目录</span>
              <div class="flex gap-2">
                <input
                  v-model="form.work_dir"
                  type="text"
                  placeholder="可选，默认使用目标所在目录"
                  class="flex-1 h-9 px-3 rounded-xl bg-white/[0.06] border border-white/10 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-400/60 focus:bg-white/[0.09] transition-colors"
                />
                <button
                  type="button"
                  class="px-3 h-9 rounded-xl bg-white/10 hover:bg-white/15 border border-white/10 text-slate-200 shrink-0 transition-colors"
                  @click="browseWorkDir"
                >
                  浏览...
                </button>
              </div>
            </label>

            <!-- Custom Icon -->
            <label class="block space-y-1.5">
              <span class="text-slate-300 font-medium">自定义图标</span>
              <div class="flex gap-2">
                <input
                  v-model="form.icon"
                  type="text"
                  placeholder="留空使用默认/自动提取图标"
                  class="flex-1 h-9 px-3 rounded-xl bg-white/[0.06] border border-white/10 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-400/60 focus:bg-white/[0.09] transition-colors"
                />
                <button
                  type="button"
                  class="px-3 h-9 rounded-xl bg-white/10 hover:bg-white/15 border border-white/10 text-slate-200 shrink-0 transition-colors"
                  @click="browseIconFile"
                >
                  浏览...
                </button>
                <button
                  v-if="form.icon"
                  type="button"
                  class="px-2.5 h-9 rounded-xl bg-white/[0.06] hover:bg-rose-500/20 border border-white/10 hover:border-rose-400/30 text-slate-300 hover:text-rose-200 shrink-0 transition-colors"
                  @click="form.icon = ''"
                >
                  清除
                </button>
              </div>
            </label>

            <!-- Footer Buttons -->
            <div class="pt-2 flex items-center justify-end gap-2">
              <button
                type="button"
                class="px-4 h-9 rounded-xl bg-white/[0.06] hover:bg-white/10 text-slate-300 border border-white/10 transition-colors"
                @click="emit('close')"
              >
                取消
              </button>
              <button
                type="submit"
                :disabled="!form.path.trim() || saving"
                class="px-4 h-9 rounded-xl bg-sky-500/85 hover:bg-sky-500 disabled:opacity-40 text-white font-medium shadow-sm transition-colors"
              >
                {{ isNew ? '添加' : '保存' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
