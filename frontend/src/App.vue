<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { bridge } from './api/bridge';
import type { Config, Group, HotkeyStatus, Item, Settings, ToastMessage, ToastPayload } from './types/config';
import TopBar from './components/TopBar.vue';
import GroupSidebar from './components/GroupSidebar.vue';
import ItemGrid from './components/ItemGrid.vue';
import ContextMenu, { type ContextMenuItem } from './components/ContextMenu.vue';
import ItemEditModal, { type ItemSaveRequest } from './components/ItemEditModal.vue';
import SettingsModal from './components/SettingsModal.vue';
import GroupModal from './components/GroupModal.vue';
import ToastContainer from './components/ToastContainer.vue';

const config = reactive<Config>({
  settings: {
    hotkey: 'Alt+G',
    mouse_trigger: 'none',
    hide_on_launch: true,
    hide_on_blur: true,
    hide_on_esc: true,
  },
  groups: [],
});

const hotkeyStatus = ref<HotkeyStatus>({
  hotkey: 'Alt+G',
  registered: true,
  error: '',
});

const activeGroupId = ref('');
const sidebarCollapsed = ref(false);
const searchQuery = ref('');
const iconCacheBuster = ref(Date.now());
const configError = ref('');

// Modals
const settingsOpen = ref(false);
const itemModal = reactive<{ visible: boolean; item: Item | null }>({
  visible: false,
  item: null,
});
const groupModal = reactive<{
  visible: boolean;
  mode: 'add' | 'rename';
  groupId: string;
  initialName: string;
}>({
  visible: false,
  mode: 'add',
  groupId: '',
  initialName: '',
});

// Context Menu
const contextMenu = reactive<{
  visible: boolean;
  x: number;
  y: number;
  targetType: 'item' | 'group' | null;
  item: Item | null;
  group: Group | null;
}>({
  visible: false,
  x: 0,
  y: 0,
  targetType: null,
  item: null,
  group: null,
});

// Toasts
const toasts = ref<ToastMessage[]>([]);
let toastSeq = 0;

const activeGroup = computed<Group | undefined>(() => {
  return config.groups.find((g) => g.id === activeGroupId.value) ?? config.groups[0];
});

const filteredItems = computed<Item[]>(() => {
  const items = activeGroup.value?.items ?? [];
  const q = searchQuery.value.trim().toLowerCase();
  if (!q) return items;
  return items.filter(
    (item) =>
      item.name.toLowerCase().includes(q) ||
      item.path.toLowerCase().includes(q) ||
      item.args.toLowerCase().includes(q)
  );
});

const contextMenuItems = computed<ContextMenuItem[]>(() => {
  if (contextMenu.targetType === 'item') {
    return [
      { id: 'edit-item', label: '编辑', icon: 'edit' },
      { id: 'open-location', label: '打开所在文件夹', icon: 'folder' },
      { id: 'delete-item', label: '删除', icon: 'trash', danger: true, dividerBefore: true },
    ];
  }
  if (contextMenu.targetType === 'group') {
    return [
      { id: 'rename-group', label: '重命名分组', icon: 'rename' },
      { id: 'delete-group', label: '删除分组', icon: 'trash', danger: true, dividerBefore: true },
    ];
  }
  return [];
});

function showToast(payload: ToastPayload) {
  const id = ++toastSeq;
  const type = payload.type ?? 'info';
  toasts.value.push({ id, type, message: payload.message });
  window.setTimeout(() => {
    dismissToast(id);
  }, 3200);
}

function dismissToast(id: number) {
  toasts.value = toasts.value.filter((t) => t.id !== id);
}

function applyConfig(next: Config) {
  config.settings = {
    ...next.settings,
    hide_on_esc: next.settings.hide_on_esc ?? true,
  };
  config.groups = Array.isArray(next.groups) ? next.groups : [];
  iconCacheBuster.value = Date.now();
  configError.value = '';

  const stillExists = config.groups.some((g) => g.id === activeGroupId.value);
  if (!stillExists) {
    const fallbackId = config.groups[0]?.id ?? '';
    activeGroupId.value = fallbackId;
    if (fallbackId) {
      void bridge.setActiveGroup(fallbackId);
    }
  } else if (activeGroupId.value) {
    void bridge.setActiveGroup(activeGroupId.value);
  }
}

function selectGroup(groupId: string) {
  activeGroupId.value = groupId;
  void bridge.setActiveGroup(groupId);
}

async function handleLaunchItem(item: Item) {
  const gid = activeGroup.value?.id;
  if (!gid) return;
  try {
    await bridge.launchItem(gid, item.id);
  } catch (err) {
    showToast({ type: 'error', message: `启动失败: ${String(err)}` });
  }
}

function openItemContextMenu(event: MouseEvent, item: Item) {
  contextMenu.targetType = 'item';
  contextMenu.item = item;
  contextMenu.group = null;
  contextMenu.x = event.clientX;
  contextMenu.y = event.clientY;
  contextMenu.visible = true;
}

function openGroupContextMenu(event: MouseEvent, group: Group) {
  contextMenu.targetType = 'group';
  contextMenu.group = group;
  contextMenu.item = null;
  contextMenu.x = event.clientX;
  contextMenu.y = event.clientY;
  contextMenu.visible = true;
}

function closeContextMenu() {
  contextMenu.visible = false;
}

async function handleContextMenuSelect(actionId: string) {
  const targetItem = contextMenu.item;
  const targetGroup = contextMenu.group;
  closeContextMenu();

  if (actionId === 'edit-item' && targetItem) {
    itemModal.item = { ...targetItem };
    itemModal.visible = true;
    return;
  }

  if (actionId === 'open-location' && targetItem) {
    try {
      await bridge.openItemLocation(targetItem.path);
    } catch (err) {
      showToast({ type: 'error', message: `打开目录失败: ${String(err)}` });
    }
    return;
  }

  if (actionId === 'delete-item' && targetItem && activeGroup.value) {
    try {
      await bridge.deleteItem(activeGroup.value.id, targetItem.id);
    } catch (err) {
      showToast({ type: 'error', message: `删除失败: ${String(err)}` });
    }
    return;
  }

  if (actionId === 'rename-group' && targetGroup) {
    groupModal.mode = 'rename';
    groupModal.groupId = targetGroup.id;
    groupModal.initialName = targetGroup.name;
    groupModal.visible = true;
    return;
  }

  if (actionId === 'delete-group' && targetGroup) {
    try {
      await bridge.deleteGroup(targetGroup.id);
    } catch (err) {
      showToast({ type: 'error', message: `删除分组失败: ${String(err)}` });
    }
  }
}

function openCreateItemModal() {
  if (!activeGroup.value) {
    showToast({ type: 'warning', message: '请先创建一个分组' });
    return;
  }
  itemModal.item = null;
  itemModal.visible = true;
}

async function handleSaveItem(request: ItemSaveRequest) {
  try {
    if (request.isNew) {
      await bridge.addItem(request.groupId, request.item);
    } else {
      await bridge.updateItem(request.groupId, request.item);
    }
    itemModal.visible = false;
  } catch (err) {
    showToast({ type: 'error', message: `保存项目失败: ${String(err)}` });
  } finally {
    // Always report back: on failure the modal stays open and must accept another attempt.
    request.done();
  }
}

function openAddGroupModal() {
  groupModal.mode = 'add';
  groupModal.groupId = '';
  groupModal.initialName = '';
  groupModal.visible = true;
}

async function handleGroupModalSubmit(name: string) {
  try {
    if (groupModal.mode === 'add') {
      await bridge.addGroup(name);
    } else if (groupModal.groupId) {
      await bridge.renameGroup(groupModal.groupId, name);
    }
    groupModal.visible = false;
  } catch (err) {
    showToast({ type: 'error', message: `分组操作失败: ${String(err)}` });
  }
}

async function handleSaveSettings(nextSettings: Settings) {
  try {
    await bridge.saveSettings(nextSettings);
    settingsOpen.value = false;
  } catch (err) {
    showToast({ type: 'error', message: `保存设置失败: ${String(err)}` });
  }
}

async function handleReloadConfig() {
  try {
    await bridge.reloadConfig();
  } catch (err) {
    showToast({ type: 'error', message: `重载配置失败: ${String(err)}` });
  }
}

async function handleOpenConfigFile() {
  try {
    await bridge.openConfigFile();
  } catch (err) {
    showToast({ type: 'error', message: `打开配置文件失败: ${String(err)}` });
  }
}

function handleWindowBlur() {
  closeContextMenu();
  void bridge.onWindowBlur();
}

function handleKeyDown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return;
  if (contextMenu.visible) {
    closeContextMenu();
    return;
  }
  if (itemModal.visible) {
    itemModal.visible = false;
    return;
  }
  if (groupModal.visible) {
    groupModal.visible = false;
    return;
  }
  if (settingsOpen.value) {
    settingsOpen.value = false;
    return;
  }
  if (config.settings.hide_on_esc !== false) {
    void bridge.hideWindow();
  }
}

const unsubscribers: Array<() => void> = [];

onMounted(async () => {
  window.addEventListener('blur', handleWindowBlur);
  window.addEventListener('keydown', handleKeyDown);

  unsubscribers.push(
    bridge.eventsOn<Config>('config:updated', (next) => {
      if (next) {
        applyConfig(next);
      }
    }),
    bridge.eventsOn<string | { message?: string; error?: string }>('config:error', (payload) => {
      if (typeof payload === 'string') {
        configError.value = payload;
      } else {
        configError.value = payload?.message || payload?.error || 'config.json 解析错误';
      }
    }),
    bridge.eventsOn('ui:open-settings', () => {
      settingsOpen.value = true;
    }),
    bridge.eventsOn<ToastPayload>('ui:toast', (payload) => {
      if (payload?.message) {
        showToast(payload);
      }
    }),
    bridge.eventsOn<HotkeyStatus>('hotkey:status', (status) => {
      if (status) {
        hotkeyStatus.value = status;
      }
    })
  );

  try {
    const initial = await bridge.getConfig();
    if (initial) {
      applyConfig(initial);
    }
  } catch (err) {
    configError.value = `加载配置失败: ${String(err)}`;
  }

  try {
    const status = await bridge.getHotkeyStatus();
    if (status) {
      hotkeyStatus.value = status;
    }
  } catch {
    // Ignore if hotkey status is not yet available; events will keep it updated.
  }
});

onBeforeUnmount(() => {
  window.removeEventListener('blur', handleWindowBlur);
  window.removeEventListener('keydown', handleKeyDown);
  for (const unsub of unsubscribers) {
    unsub();
  }
});
</script>

<template>
  <div class="w-screen h-screen p-0.5 select-none">
    <div
      class="w-full h-full rounded-2xl glass-panel border border-white/10 shadow-glass flex flex-col overflow-hidden"
    >
      <!-- Top Bar -->
      <TopBar
        v-model:search-query="searchQuery"
        :active-group-name="activeGroup?.name"
        :item-count="activeGroup?.items?.length"
        @add-item="openCreateItemModal"
        @open-settings="settingsOpen = true"
        @hide="bridge.hideWindow()"
      />

      <!-- Non-crashing Config Error Banner -->
      <Transition name="fade">
        <div
          v-if="configError"
          class="px-4 py-2 bg-rose-500/20 border-b border-rose-400/30 text-rose-100 text-xs flex items-center justify-between gap-3 shrink-0"
        >
          <div class="flex items-center gap-2 min-w-0">
            <svg class="w-4 h-4 text-rose-300 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="9" />
              <path stroke-linecap="round" d="M12 8v5m0 3h.01" />
            </svg>
            <span class="truncate" :title="configError">配置文件异常: {{ configError }}</span>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <button
              type="button"
              class="px-2.5 py-1 rounded-lg bg-rose-400/25 hover:bg-rose-400/35 border border-rose-300/30 text-rose-50 transition-colors"
              @click="handleOpenConfigFile"
            >
              打开 config.json
            </button>
            <button
              type="button"
              title="关闭提示"
              class="p-1 rounded-lg text-rose-200/80 hover:text-white hover:bg-white/10 transition-colors"
              @click="configError = ''"
            >
              <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      </Transition>

      <!-- Workspace Body -->
      <div class="flex-1 flex min-h-0">
        <GroupSidebar
          :groups="config.groups"
          :active-group-id="activeGroup?.id ?? ''"
          :collapsed="sidebarCollapsed"
          @select="selectGroup"
          @add-group="openAddGroupModal"
          @toggle-collapse="sidebarCollapsed = !sidebarCollapsed"
          @contextmenu="openGroupContextMenu"
        />

        <ItemGrid
          :items="filteredItems"
          :active-group-name="activeGroup?.name ?? ''"
          :search-query="searchQuery"
          :cache-buster="iconCacheBuster"
          :has-active-group="Boolean(activeGroup)"
          @launch="handleLaunchItem"
          @contextmenu="openItemContextMenu"
          @clear-search="searchQuery = ''"
          @add-item="openCreateItemModal"
        />
      </div>
    </div>

    <!-- Context Menu -->
    <ContextMenu
      :visible="contextMenu.visible"
      :x="contextMenu.x"
      :y="contextMenu.y"
      :items="contextMenuItems"
      @select="handleContextMenuSelect"
      @close="closeContextMenu"
    />

    <!-- Modals -->
    <ItemEditModal
      :visible="itemModal.visible"
      :item="itemModal.item"
      :group-id="activeGroup?.id ?? ''"
      :cache-buster="iconCacheBuster"
      @close="itemModal.visible = false"
      @save="handleSaveItem"
    />

    <SettingsModal
      :visible="settingsOpen"
      :settings="config.settings"
      :hotkey-status="hotkeyStatus"
      @close="settingsOpen = false"
      @save="handleSaveSettings"
      @open-config-file="handleOpenConfigFile"
      @reload-config="handleReloadConfig"
    />

    <GroupModal
      :visible="groupModal.visible"
      :mode="groupModal.mode"
      :initial-name="groupModal.initialName"
      @close="groupModal.visible = false"
      @submit="handleGroupModalSubmit"
    />

    <!-- Transient Toast Container -->
    <ToastContainer :toasts="toasts" @dismiss="dismissToast" />
  </div>
</template>
