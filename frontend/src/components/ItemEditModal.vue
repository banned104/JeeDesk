<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue';
import type { Item } from '../types/config';
import { bridge } from '../api/bridge';
import { inferNameFromPath } from '../utils/icon';
import { registerTranslations, t } from '../i18n';
import ItemIcon from './ItemIcon.vue';

registerTranslations({
  '选择启动目标': 'Choose launch target',
  '可执行文件与快捷方式 (*.exe;*.lnk;*.bat;*.cmd)': 'Executables and shortcuts (*.exe;*.lnk;*.bat;*.cmd)',
  '选择工作目录': 'Choose working directory',
  '选择自定义图标': 'Choose custom icon',
  '图标或图片 (*.ico;*.png;*.jpg;*.jpeg;*.exe)': 'Icon or image (*.ico;*.png;*.jpg;*.jpeg;*.exe)',
  '打开文件选择框失败：{error}': 'Could not open the file picker: {error}',
  '打开文件夹选择框失败：{error}': 'Could not open the folder picker: {error}',
  '添加项目': 'Add item',
  '编辑项目': 'Edit item',
  '关闭': 'Close',
  '显示名称': 'Display name',
  '例如：VS Code（留空则自动提取文件名）': 'e.g. VS Code (leave empty to use the file name)',
  '目标路径': 'Target path',
  'C:\\Path\\To\\App.exe 或文件夹路径': 'C:\\Path\\To\\App.exe or a folder path',
  '浏览...': 'Browse…',
  '启动参数': 'Arguments',
  '可选，例如：--reuse-window': 'Optional, e.g. --reuse-window',
  '工作目录': 'Working directory',
  '可选，默认使用目标所在目录': 'Optional, defaults to the target folder',
  '自定义图标': 'Custom icon',
  '留空使用默认/自动提取图标': 'Leave empty to use the default or the extracted icon',
  '清除': 'Clear',
  '启动时申请提权，可能弹出系统确认提示':
    'Requests elevation on launch; Windows may ask you to confirm',
  '目标路径已改动，保存后会重新提取图标':
    'Target path changed — the icon will be extracted again when you save',
  '取消': 'Cancel',
  '添加': 'Add',
  '保存': 'Save',
  '未命名项目': 'Untitled item',
});

export interface ItemSaveRequest {
  isNew: boolean;
  groupId: string;
  item: Item;
  // Called once the parent knows how the save went, so a failed save can be retried in place.
  done: () => void;
}

const props = defineProps<{
  visible: boolean;
  item: Item | null;
  groupId: string;
  cacheBuster: number;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'save', request: ItemSaveRequest): void;
  (e: 'error', message: string): void;
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
  admin: false,
});

// --- dialog focus management ----------------------------------------------

const titleId = 'item-edit-modal-title';
const dialogRef = ref<HTMLElement | null>(null);
const nameInputRef = ref<HTMLInputElement | null>(null);
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

async function focusFirstField() {
  await nextTick();
  const target = nameInputRef.value ?? focusableElements()[0] ?? dialogRef.value;
  target?.focus();
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
  if (target && document.contains(target)) {
    target.focus();
  }
}

onBeforeUnmount(restoreFocus);

// --- browse buttons -------------------------------------------------------

type BrowseTarget = 'path' | 'workDir' | 'icon';

// Which native picker is open right now; keeps the buttons disabled so a second dialog cannot
// be requested on top of the first one.
const browsing = ref<BrowseTarget | ''>('');
const browseError = ref('');

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function reportBrowseError(message: string) {
  browseError.value = message;
  emit('error', message);
}

/** Runs a native picker and returns its result, or null when it failed or was cancelled. */
async function pickValue(
  target: BrowseTarget,
  picker: () => Promise<string>,
  failSource: string
): Promise<string | null> {
  if (browsing.value) return null;
  browsing.value = target;
  browseError.value = '';
  try {
    return (await picker()) || '';
  } catch (err) {
    reportBrowseError(t(failSource, { error: errorText(err) }));
    return null;
  } finally {
    browsing.value = '';
  }
}

async function browseTargetFile() {
  const picked = await pickValue(
    'path',
    () =>
      bridge.pickFile(
        t('选择启动目标'),
        t('可执行文件与快捷方式 (*.exe;*.lnk;*.bat;*.cmd)'),
        '*.exe;*.lnk;*.bat;*.cmd;*'
      ),
    '打开文件选择框失败：{error}'
  );
  if (!picked) return;
  form.path = picked;
  if (!form.name.trim()) {
    form.name = inferNameFromPath(picked);
  }
}

async function browseWorkDir() {
  const picked = await pickValue(
    'workDir',
    () => bridge.pickDirectory(t('选择工作目录')),
    '打开文件夹选择框失败：{error}'
  );
  if (!picked) return;
  form.work_dir = picked;
}

async function browseIconFile() {
  const picked = await pickValue(
    'icon',
    () =>
      bridge.pickFile(
        t('选择自定义图标'),
        t('图标或图片 (*.ico;*.png;*.jpg;*.jpeg;*.exe)'),
        '*.ico;*.png;*.jpg;*.jpeg;*.exe'
      ),
    '打开文件选择框失败：{error}'
  );
  if (!picked) return;
  form.icon = picked;
}

// --- form state -----------------------------------------------------------

// True while the dialog is copying props.item into the form, so the path watcher below does not
// mistake that assignment for a user edit.
let syncingForm = false;
// Set when a cached icons/ path was dropped because the target changed; shown as a hint under the
// icon field and mirrors what the backend does on save (it extracts a fresh icon).
const iconWillBeExtracted = ref(false);

function isCachedIconPath(icon: string): boolean {
  const normalized = (icon || '').trim().replace(/\\/g, '/');
  return normalized.startsWith('icons/') || normalized.startsWith('/icons/');
}

watch(
  () => form.path,
  (path) => {
    if (syncingForm) return;
    if (!isCachedIconPath(form.icon)) return;
    if (path.trim() === (props.item?.path || '').trim()) return;
    // The stored icon belongs to the old target, so it is cleared here; the backend extracts a new
    // one from the new path when the item is saved.
    form.icon = '';
    iconWillBeExtracted.value = true;
  },
  { flush: 'sync' }
);

watch(
  () => form.icon,
  (icon) => {
    if (icon.trim()) {
      iconWillBeExtracted.value = false;
    }
  },
  { flush: 'sync' }
);

function loadForm(item: Item | null) {
  syncingForm = true;
  if (item) {
    form.id = item.id;
    form.name = item.name;
    form.path = item.path;
    form.args = item.args;
    form.work_dir = item.work_dir;
    form.icon = item.icon;
    form.admin = item.admin === true;
  } else {
    form.id = '';
    form.name = '';
    form.path = '';
    form.args = '';
    form.work_dir = '';
    form.icon = '';
    form.admin = false;
  }
  syncingForm = false;
  iconWillBeExtracted.value = false;
  browseError.value = '';
  browsing.value = '';
  saving.value = false;
}

watch(
  () => [props.visible, props.item] as const,
  async ([visible, item]) => {
    if (!visible) {
      restoreFocus();
      return;
    }
    rememberFocus();
    loadForm(item);
    await focusFirstField();
  },
  { immediate: true }
);

function handleSubmit() {
  if (!form.path.trim() || !props.groupId) return;
  saving.value = true;
  const finalName = form.name.trim() || inferNameFromPath(form.path) || t('未命名项目');
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
      admin: form.admin,
    },
    done: () => {
      saving.value = false;
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
          ref="dialogRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="w-full max-w-md max-h-[90vh] rounded-2xl glass-modal border jd-border-strong shadow-2xl overflow-hidden flex flex-col"
          @keydown="handleTabKey"
        >
          <!-- Modal Header -->
          <div class="px-5 py-3.5 border-b jd-border flex items-center justify-between shrink-0">
            <div class="flex items-center gap-3">
              <ItemIcon
                :icon="form.icon"
                :path="form.path"
                :name="form.name"
                :cache-buster="cacheBuster"
                size-class="w-8 h-8"
              />
              <h2 :id="titleId" class="text-sm font-semibold jd-text">
                {{ isNew ? t('添加项目') : t('编辑项目') }}
              </h2>
            </div>
            <button
              type="button"
              class="w-7 h-7 rounded-lg flex items-center justify-center jd-muted jd-hover transition-colors"
              :aria-label="t('关闭')"
              :title="t('关闭')"
              @click="emit('close')"
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

          <!-- Form: scrolls on its own, so the header and footer stay reachable -->
          <form
            id="item-edit-form"
            class="flex-1 min-h-0 overflow-y-auto custom-scrollbar p-5 space-y-3.5 text-xs"
            @submit.prevent="handleSubmit"
          >
            <!-- Name -->
            <label class="block space-y-1.5">
              <span class="jd-muted font-medium">{{ t('显示名称') }}</span>
              <input
                ref="nameInputRef"
                v-model="form.name"
                type="text"
                :placeholder="t('例如：VS Code（留空则自动提取文件名）')"
                class="w-full h-9 px-3 rounded-xl jd-input border transition-colors select-text"
              />
            </label>

            <!-- Path -->
            <label class="block space-y-1.5">
              <span class="jd-muted font-medium">
                {{ t('目标路径') }} <span class="text-rose-400">*</span>
              </span>
              <div class="flex gap-2">
                <input
                  v-model="form.path"
                  type="text"
                  required
                  :placeholder="t('C:\\Path\\To\\App.exe 或文件夹路径')"
                  class="flex-1 min-w-0 h-9 px-3 rounded-xl jd-input border transition-colors select-text"
                />
                <button
                  type="button"
                  class="px-3 h-9 rounded-xl jd-surface-strong jd-border border jd-text jd-hover shrink-0 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                  :disabled="browsing !== ''"
                  :aria-busy="browsing === 'path'"
                  @click="browseTargetFile"
                >
                  {{ t('浏览...') }}
                </button>
              </div>
            </label>

            <!-- Args -->
            <label class="block space-y-1.5">
              <span class="jd-muted font-medium">{{ t('启动参数') }}</span>
              <input
                v-model="form.args"
                type="text"
                :placeholder="t('可选，例如：--reuse-window')"
                class="w-full h-9 px-3 rounded-xl jd-input border transition-colors select-text"
              />
            </label>

            <!-- Work Dir -->
            <label class="block space-y-1.5">
              <span class="jd-muted font-medium">{{ t('工作目录') }}</span>
              <div class="flex gap-2">
                <input
                  v-model="form.work_dir"
                  type="text"
                  :placeholder="t('可选，默认使用目标所在目录')"
                  class="flex-1 min-w-0 h-9 px-3 rounded-xl jd-input border transition-colors select-text"
                />
                <button
                  type="button"
                  class="px-3 h-9 rounded-xl jd-surface-strong jd-border border jd-text jd-hover shrink-0 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                  :disabled="browsing !== ''"
                  :aria-busy="browsing === 'workDir'"
                  @click="browseWorkDir"
                >
                  {{ t('浏览...') }}
                </button>
              </div>
            </label>

            <!-- Custom Icon -->
            <label class="block space-y-1.5">
              <span class="jd-muted font-medium">{{ t('自定义图标') }}</span>
              <div class="flex gap-2">
                <input
                  v-model="form.icon"
                  type="text"
                  :placeholder="t('留空使用默认/自动提取图标')"
                  class="flex-1 min-w-0 h-9 px-3 rounded-xl jd-input border transition-colors select-text"
                />
                <button
                  type="button"
                  class="px-3 h-9 rounded-xl jd-surface-strong jd-border border jd-text jd-hover shrink-0 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                  :disabled="browsing !== ''"
                  :aria-busy="browsing === 'icon'"
                  @click="browseIconFile"
                >
                  {{ t('浏览...') }}
                </button>
                <button
                  v-if="form.icon"
                  type="button"
                  class="px-2.5 h-9 rounded-xl jd-surface border jd-border jd-muted hover:bg-rose-500/20 hover:border-rose-400/30 hover:text-rose-200 shrink-0 transition-colors"
                  @click="form.icon = ''"
                >
                  {{ t('清除') }}
                </button>
              </div>
              <span v-if="iconWillBeExtracted" class="block text-[11px] jd-subtle select-text">
                {{ t('目标路径已改动，保存后会重新提取图标') }}
              </span>
            </label>

            <!-- Run as administrator -->
            <div class="flex items-center justify-between gap-3 pt-0.5">
              <div class="min-w-0">
                <p class="jd-text font-medium">{{ t('以管理员身份运行') }}</p>
                <p class="mt-0.5 text-[11px] jd-subtle select-text">
                  {{ t('启动时申请提权，可能弹出系统确认提示') }}
                </p>
              </div>
              <label class="relative inline-flex items-center shrink-0 cursor-pointer">
                <input v-model="form.admin" type="checkbox" class="peer sr-only" />
                <span
                  class="block w-9 h-5 rounded-full border transition-colors peer-focus-visible:ring-2 peer-focus-visible:ring-sky-400/60"
                  :class="form.admin ? 'jd-accent-bg border-transparent' : 'jd-surface-strong jd-border'"
                />
                <span
                  class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform"
                  :class="form.admin ? 'translate-x-4' : ''"
                />
              </label>
            </div>
          </form>

          <!-- Picker failure, kept out of the scrolling form so it stays readable -->
          <p
            v-if="browseError"
            role="alert"
            class="shrink-0 px-5 py-2 text-[11px] text-rose-300 bg-rose-500/10 border-t jd-border select-text"
          >
            {{ browseError }}
          </p>

          <!-- Footer: always visible, even at 150% zoom or in a small window -->
          <div
            class="shrink-0 px-5 py-3 border-t jd-border flex items-center justify-end gap-2 jd-surface-soft"
          >
            <button
              type="button"
              class="px-4 h-9 rounded-xl jd-surface jd-border border jd-muted jd-hover transition-colors"
              @click="emit('close')"
            >
              {{ t('取消') }}
            </button>
            <button
              type="submit"
              form="item-edit-form"
              :disabled="!form.path.trim() || saving"
              class="px-4 h-9 rounded-xl bg-sky-500/85 hover:bg-sky-500 disabled:opacity-40 text-white font-medium shadow-sm transition-colors"
            >
              {{ isNew ? t('添加') : t('保存') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
