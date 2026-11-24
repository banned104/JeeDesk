<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { bridge } from './api/bridge';
import type {
  AppInfo,
  Config,
  DisplayItem,
  Group,
  HotkeyStatus,
  Item,
  RecentItem,
  SearchHit,
  Settings,
  StartMenuEntry,
  ToastMessage,
  ToastPayload,
} from './types/config';
import { RECENT_GROUP_ID, defaultSettings } from './types/config';
import TopBar from './components/TopBar.vue';
import GroupSidebar from './components/GroupSidebar.vue';
import ItemGrid from './components/ItemGrid.vue';
import ContextMenu, { type ContextMenuItem } from './components/ContextMenu.vue';
import ItemEditModal, { type ItemSaveRequest } from './components/ItemEditModal.vue';
import SettingsModal from './components/SettingsModal.vue';
import GroupModal from './components/GroupModal.vue';
import StartMenuImportModal from './components/StartMenuImportModal.vue';
import ToastContainer from './components/ToastContainer.vue';

const SEARCH_DEBOUNCE_MS = 120;
const RECENT_LIMIT = 12;

const config = reactive<Config>({
  settings: { ...defaultSettings },
  groups: [],
});

const hotkeyStatus = ref<HotkeyStatus>({ hotkey: 'Alt+G', registered: true, error: '' });
const appInfo = ref<AppInfo | null>(null);

const activeGroupId = ref('');
const sidebarCollapsed = ref(false);
const searchQuery = ref('');
const searchHits = ref<SearchHit[]>([]);
const searching = ref(false);
const recentItems = ref<RecentItem[]>([]);
const selectedIndex = ref(0);
const iconCacheBuster = ref(Date.now());
const configError = ref('');
const undoLabel = ref('');

const topBarRef = ref<InstanceType<typeof TopBar> | null>(null);
const gridRef = ref<InstanceType<typeof ItemGrid> | null>(null);

// Modals
const settingsOpen = ref(false);
const itemModal = reactive<{ visible: boolean; item: Item | null }>({ visible: false, item: null });
const groupModal = reactive<{
  visible: boolean;
  mode: 'add' | 'rename';
  groupId: string;
  initialName: string;
}>({ visible: false, mode: 'add', groupId: '', initialName: '' });
const startMenuModal = reactive<{
  visible: boolean;
  loading: boolean;
  entries: StartMenuEntry[];
}>({ visible: false, loading: false, entries: [] });

// Context Menu
const contextMenu = reactive<{
  visible: boolean;
  x: number;
  y: number;
  targetType: 'item' | 'group' | null;
  entry: DisplayItem | null;
  group: Group | null;
}>({ visible: false, x: 0, y: 0, targetType: null, entry: null, group: null });

// Toasts
const toasts = ref<ToastMessage[]>([]);
let toastSeq = 0;

const activeGroup = computed<Group | undefined>(() => config.groups.find((g) => g.id === activeGroupId.value));
const isRecentView = computed(() => activeGroupId.value === RECENT_GROUP_ID);
const activeGroupName = computed(() => (isRecentView.value ? '最近使用' : activeGroup.value?.name ?? ''));
const hasActiveGroup = computed(() => Boolean(activeGroup.value));
const isSearching = computed(() => searchQuery.value.trim().length > 0);

const displayItems = computed<DisplayItem[]>(() => {
  if (isSearching.value) {
    return searchHits.value.map((hit) => ({
      groupId: hit.group_id,
      groupName: hit.group_name,
      score: hit.score,
      item: {
        id: hit.item_id,
        name: hit.name,
        path: hit.path,
        args: hit.args,
        work_dir: '',
        icon: hit.icon,
        admin: hit.admin,
      },
    }));
  }
  if (isRecentView.value) {
    return recentItems.value.map((recent) => ({
      groupId: recent.group_id,
      groupName: recent.group_name,
      item: {
        id: recent.item_id,
        name: recent.name,
        path: recent.path,
        args: '',
        work_dir: '',
        icon: recent.icon,
        admin: recent.admin,
      },
    }));
  }
  const group = activeGroup.value;
  if (!group) return [];
  return group.items.map((item) => ({ groupId: group.id, groupName: group.name, item }));
});

const gridMode = computed<'group' | 'search' | 'recent'>(() => {
  if (isSearching.value) return 'search';
  if (isRecentView.value) return 'recent';
  return 'group';
});

const contextMenuItems = computed<ContextMenuItem[]>(() => {
  if (contextMenu.targetType === 'item' && contextMenu.entry) {
    const entry = contextMenu.entry;
    const items: ContextMenuItem[] = [
      { id: 'launch', label: '启动', icon: 'edit' },
      { id: 'edit-item', label: '编辑', icon: 'edit' },
      { id: 'toggle-admin', label: entry.item.admin ? '取消管理员身份运行' : '以管理员身份运行' },
      { id: 'open-location', label: '打开所在文件夹', icon: 'folder' },
      { id: 'copy-path', label: '复制目标路径' },
    ];
    if (entry.groupId === activeGroupId.value) {
      items.push({ id: 'move-up', label: '上移' });
      items.push({ id: 'move-down', label: '下移' });
    }
    items.push({ id: 'delete-item', label: '删除', icon: 'trash', danger: true, dividerBefore: true });
    return items;
  }
  if (contextMenu.targetType === 'group' && contextMenu.group) {
    return [
      { id: 'rename-group', label: '重命名分组', icon: 'rename' },
      { id: 'group-up', label: '分组上移' },
      { id: 'group-down', label: '分组下移' },
      { id: 'delete-group', label: '删除分组', icon: 'trash', danger: true, dividerBefore: true },
    ];
  }
  return [];
});

function showToast(payload: ToastPayload) {
  const id = ++toastSeq;
  const type = payload.type ?? 'info';
  toasts.value.push({ id, type, message: payload.message });
  window.setTimeout(() => dismissToast(id), 3200);
}

function dismissToast(id: number) {
  toasts.value = toasts.value.filter((t) => t.id !== id);
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// --- theme, scale, opacity -------------------------------------------------

const prefersLight = ref(false);
let themeMedia: MediaQueryList | null = null;

const resolvedTheme = computed<'dark' | 'light'>(() => {
  const theme = config.settings.theme;
  if (theme === 'light') return 'light';
  if (theme === 'system') return prefersLight.value ? 'light' : 'dark';
  return 'dark';
});

watch(resolvedTheme, (theme) => {
  document.documentElement.dataset.theme = theme;
}, { immediate: true });

const shellStyle = computed<Record<string, string>>(() => {
  const style: Record<string, string> = {};
  const opacity = Math.min(100, Math.max(50, config.settings.window_opacity));
  if (opacity < 100) {
    style.opacity = `${opacity / 100}`;
  }
  const scale = Math.min(140, Math.max(80, config.settings.font_scale));
  if (scale !== 100) {
    // zoom keeps proportions of the whole layout, unlike font-size which only scales text.
    style.zoom = `${scale / 100}`;
  }
  return style;
});

function handleSystemThemeChange(event: MediaQueryListEvent) {
  prefersLight.value = event.matches;
}

// --- config ---------------------------------------------------------------

function applyConfig(next: Config) {
  const settings = { ...defaultSettings, ...next.settings };
  config.settings = settings;
  config.groups = Array.isArray(next.groups) ? next.groups : [];
  iconCacheBuster.value = Date.now();
  configError.value = '';

  const stillExists =
    activeGroupId.value === RECENT_GROUP_ID || config.groups.some((g) => g.id === activeGroupId.value);
  if (!stillExists) {
    selectGroup(config.groups[0]?.id ?? '', { silent: true });
  } else if (activeGroupId.value && activeGroupId.value !== RECENT_GROUP_ID) {
    void bridge.setActiveGroup(activeGroupId.value);
  }

  void refreshRecent();
  void refreshUndoLabel();
}

function selectGroup(groupId: string, options: { silent?: boolean } = {}) {
  activeGroupId.value = groupId;
  selectedIndex.value = 0;
  if (!options.silent && groupId && groupId !== RECENT_GROUP_ID) {
    void bridge.setActiveGroup(groupId);
  }
  if (groupId === RECENT_GROUP_ID) {
    void refreshRecent();
  }
}

async function refreshRecent() {
  if (!config.settings.show_recent) {
    recentItems.value = [];
    return;
  }
  try {
    recentItems.value = await bridge.getRecentItems(RECENT_LIMIT);
  } catch {
    recentItems.value = [];
  }
}

async function refreshUndoLabel() {
  try {
    undoLabel.value = await bridge.getUndoLabel();
  } catch {
    undoLabel.value = '';
  }
}

let searchTimer = 0;

function handleSearchInput(value: string) {
  searchQuery.value = value;
  selectedIndex.value = 0;
  window.clearTimeout(searchTimer);
  if (!value.trim()) {
    searchHits.value = [];
    searching.value = false;
    return;
  }
  searching.value = true;
  searchTimer = window.setTimeout(async () => {
    try {
      searchHits.value = await bridge.searchItems(value);
    } catch (err) {
      searchHits.value = [];
      showToast({ type: 'error', message: `搜索失败: ${errorText(err)}` });
    } finally {
      searching.value = false;
    }
  }, SEARCH_DEBOUNCE_MS);
}

// --- launching ------------------------------------------------------------

async function launchEntry(entry: DisplayItem) {
  try {
    await bridge.launchItem(entry.groupId, entry.item.id);
    void refreshRecent();
  } catch (err) {
    showToast({ type: 'error', message: `启动失败: ${errorText(err)}` });
  }
}

async function launchSelected() {
  const entry = displayItems.value[selectedIndex.value];
  if (entry) {
    await launchEntry(entry);
  }
}

// --- selection ------------------------------------------------------------

function moveSelection(delta: number) {
  const total = displayItems.value.length;
  if (total === 0) return;
  const next = (selectedIndex.value + delta + total) % total;
  selectedIndex.value = next;
  void gridRef.value?.ensureSelectionVisible();
}

function selectEntry(event: MouseEvent, entry: DisplayItem) {
  contextMenu.targetType = 'item';
  contextMenu.entry = entry;
  contextMenu.group = null;
  contextMenu.x = event.clientX;
  contextMenu.y = event.clientY;
  contextMenu.visible = true;
}

function openGroupContextMenu(event: MouseEvent, group: Group) {
  contextMenu.targetType = 'group';
  contextMenu.group = group;
  contextMenu.entry = null;
  contextMenu.x = event.clientX;
  contextMenu.y = event.clientY;
  contextMenu.visible = true;
}

function closeContextMenu() {
  contextMenu.visible = false;
}

async function handleContextMenuSelect(actionId: string) {
  const entry = contextMenu.entry;
  const group = contextMenu.group;
  closeContextMenu();

  if (!entry && !group) return;

  try {
    if (actionId === 'launch' && entry) {
      await launchEntry(entry);
      return;
    }

    if (actionId === 'edit-item' && entry) {
      itemModal.item = { ...entry.item };
      itemModal.visible = true;
      return;
    }

    if (actionId === 'toggle-admin' && entry) {
      await bridge.updateItem(entry.groupId, { ...entry.item, admin: !entry.item.admin });
      showToast({
        type: 'success',
        message: entry.item.admin ? '已取消管理员身份运行' : '已设为以管理员身份运行',
      });
      return;
    }

    if (actionId === 'open-location' && entry) {
      await bridge.openItemLocation(entry.item.path);
      return;
    }

    if (actionId === 'copy-path' && entry) {
      await navigator.clipboard.writeText(entry.item.path);
      showToast({ type: 'success', message: '已复制目标路径' });
      return;
    }

    if ((actionId === 'move-up' || actionId === 'move-down') && entry) {
      const items = activeGroup.value?.items ?? [];
      const current = items.findIndex((i) => i.id === entry.item.id);
      const target = actionId === 'move-up' ? current - 1 : current + 1;
      if (current >= 0 && target >= 0 && target < items.length) {
        await bridge.moveItem(entry.groupId, entry.item.id, entry.groupId, target);
      }
      return;
    }

    if (actionId === 'delete-item' && entry) {
      await bridge.deleteItem(entry.groupId, entry.item.id);
      selectedIndex.value = Math.max(0, Math.min(selectedIndex.value, displayItems.value.length - 2));
      await refreshUndoLabel();
      if (undoLabel.value) {
        showToast({ type: 'info', message: `${undoLabel.value}（Ctrl+Z）` });
      }
      return;
    }

    if (actionId === 'rename-group' && group) {
      groupModal.mode = 'rename';
      groupModal.groupId = group.id;
      groupModal.initialName = group.name;
      groupModal.visible = true;
      return;
    }

    if ((actionId === 'group-up' || actionId === 'group-down') && group) {
      const index = config.groups.findIndex((g) => g.id === group.id);
      const target = actionId === 'group-up' ? index - 1 : index + 1;
      if (index >= 0 && target >= 0 && target < config.groups.length) {
        await bridge.moveGroup(group.id, target);
      }
      return;
    }

    if (actionId === 'delete-group' && group) {
      await bridge.deleteGroup(group.id);
      await refreshUndoLabel();
      if (undoLabel.value) {
        showToast({ type: 'info', message: `${undoLabel.value}（Ctrl+Z）` });
      }
    }
  } catch (err) {
    showToast({ type: 'error', message: `操作失败: ${errorText(err)}` });
  }
}

// --- items ----------------------------------------------------------------

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
    showToast({ type: 'error', message: `保存项目失败: ${errorText(err)}` });
  } finally {
    // Always report back: on failure the modal stays open and must accept another attempt.
    request.done();
  }
}

async function handleDroppedPaths(paths: string[]) {
  const groupId = activeGroup.value?.id ?? config.groups[0]?.id;
  if (!groupId) {
    showToast({ type: 'warning', message: '请先创建一个分组再拖入文件' });
    return;
  }
  try {
    await bridge.addDroppedFiles(groupId, paths);
  } catch (err) {
    showToast({ type: 'error', message: `添加拖入文件失败: ${errorText(err)}` });
  }
}

// --- groups & settings ----------------------------------------------------

function openAddGroupModal() {
  groupModal.mode = 'add';
  groupModal.groupId = '';
  groupModal.initialName = '';
  groupModal.visible = true;
}

async function handleGroupModalSubmit(name: string) {
  try {
    if (groupModal.mode === 'add') {
      // The backend has already made the new group active; select it here too, so the config
      // update arriving next keeps that choice instead of restoring the previous group.
      selectGroup(await bridge.addGroup(name));
    } else if (groupModal.groupId) {
      await bridge.renameGroup(groupModal.groupId, name);
    }
    groupModal.visible = false;
  } catch (err) {
    showToast({ type: 'error', message: `分组操作失败: ${errorText(err)}` });
  }
}

async function handleSaveSettings(nextSettings: Settings) {
  try {
    await bridge.saveSettings(nextSettings);
    settingsOpen.value = false;
  } catch (err) {
    showToast({ type: 'error', message: `保存设置失败: ${errorText(err)}` });
  }
}

async function handleExportConfig() {
  try {
    const target = await bridge.exportConfig();
    if (target) {
      showToast({ type: 'success', message: `配置已导出到 ${target}` });
    }
  } catch (err) {
    showToast({ type: 'error', message: `导出失败: ${errorText(err)}` });
  }
}

async function handleImportConfig() {
  try {
    const count = await bridge.importConfig();
    if (count > 0) {
      showToast({ type: 'success', message: `已导入 ${count} 个分组` });
    }
  } catch (err) {
    showToast({ type: 'error', message: `导入失败: ${errorText(err)}` });
  }
}

async function handleReloadConfig() {
  try {
    await bridge.reloadConfig();
  } catch (err) {
    showToast({ type: 'error', message: `重载配置失败: ${errorText(err)}` });
  }
}

async function handleOpenConfigFile() {
  try {
    await bridge.openConfigFile();
  } catch (err) {
    showToast({ type: 'error', message: `打开配置文件失败: ${errorText(err)}` });
  }
}

async function handleOpenBaseDir() {
  try {
    await bridge.openBaseDir();
  } catch (err) {
    showToast({ type: 'error', message: `打开数据目录失败: ${errorText(err)}` });
  }
}

async function handleResetWindowState() {
  try {
    await bridge.resetWindowState();
    showToast({ type: 'success', message: '已重置窗口位置和大小' });
  } catch (err) {
    showToast({ type: 'error', message: `重置失败: ${errorText(err)}` });
  }
}

async function handleMoveItem(payload: { groupId: string; fromGroupId: string; itemId: string }) {
  try {
    await bridge.moveItem(payload.fromGroupId, payload.itemId, payload.groupId, 0);
    const target = config.groups.find((g) => g.id === payload.groupId);
    showToast({ type: 'success', message: `已移动到「${target?.name ?? '分组'}」` });
  } catch (err) {
    showToast({ type: 'error', message: `移动失败: ${errorText(err)}` });
  }
}

async function handleClearRecent() {
  try {
    await bridge.clearRecentItems();
    recentItems.value = [];
    showToast({ type: 'success', message: '最近记录已清空' });
  } catch (err) {
    showToast({ type: 'error', message: `清空失败: ${errorText(err)}` });
  }
}

// --- start menu import ----------------------------------------------------

async function openStartMenuImport() {
  startMenuModal.visible = true;
  await loadStartMenuEntries();
}

async function loadStartMenuEntries() {
  startMenuModal.loading = true;
  try {
    startMenuModal.entries = await bridge.scanStartMenu();
  } catch (err) {
    startMenuModal.entries = [];
    showToast({ type: 'error', message: `扫描开始菜单失败: ${errorText(err)}` });
  } finally {
    startMenuModal.loading = false;
  }
}

async function handleStartMenuImport(lnkPaths: string[]) {
  const groupId = activeGroup.value?.id;
  if (!groupId) {
    showToast({ type: 'warning', message: '请先选择一个分组' });
    return;
  }
  try {
    await bridge.importStartMenuItems(groupId, lnkPaths);
    startMenuModal.visible = false;
  } catch (err) {
    showToast({ type: 'error', message: `导入失败: ${errorText(err)}` });
  }
}

// --- undo -----------------------------------------------------------------

async function handleUndo() {
  try {
    const name = (await bridge.getUndoLabel()).startsWith('group:')
      ? await bridge.undoDeleteGroup()
      : await bridge.undoDeleteItem();
    if (name) {
      showToast({ type: 'success', message: `已恢复「${name}」` });
    } else {
      showToast({ type: 'info', message: '没有可撤销的删除' });
    }
    await refreshUndoLabel();
  } catch (err) {
    showToast({ type: 'error', message: `撤销失败: ${errorText(err)}` });
  }
}

// --- keyboard -------------------------------------------------------------

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable;
}

function isOverlayOpen(): boolean {
  return settingsOpen.value || itemModal.visible || groupModal.visible || startMenuModal.visible || contextMenu.visible;
}

function cycleGroup(delta: number) {
  const ids = config.groups.map((g) => g.id);
  if (config.settings.show_recent) ids.unshift(RECENT_GROUP_ID);
  if (ids.length === 0) return;
  const current = ids.indexOf(activeGroupId.value);
  const next = (current + delta + ids.length) % ids.length;
  selectGroup(ids[next]);
}

function handleWindowKeyDown(event: KeyboardEvent) {
  // Modals and dialogs keep their own key handling.
  if (event.key === 'Escape') {
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
    if (startMenuModal.visible) {
      startMenuModal.visible = false;
      return;
    }
    if (settingsOpen.value) {
      settingsOpen.value = false;
      return;
    }
    if (isSearching.value) {
      handleSearchInput('');
      return;
    }
    if (config.settings.hide_on_esc !== false) {
      void bridge.hideWindow();
    }
    return;
  }

  if (isOverlayOpen()) return;

  const typing = isTypingTarget(event.target);

  if (event.ctrlKey && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'z') {
    event.preventDefault();
    void handleUndo();
    return;
  }

  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f') {
    event.preventDefault();
    topBarRef.value?.focusSearch();
    return;
  }

  if (event.ctrlKey && !event.altKey && (event.key === ',' || event.key === '，')) {
    event.preventDefault();
    settingsOpen.value = true;
    return;
  }

  if (event.ctrlKey && !event.altKey && event.key.toLowerCase() === 'n') {
    event.preventDefault();
    if (event.shiftKey) {
      openAddGroupModal();
    } else {
      openCreateItemModal();
    }
    return;
  }

  if (event.ctrlKey && !event.altKey && !event.shiftKey && /^[1-9]$/.test(event.key)) {
    const index = Number(event.key) - 1;
    const ids = config.groups.map((g) => g.id);
    if (config.settings.show_recent) ids.unshift(RECENT_GROUP_ID);
    if (ids[index]) {
      event.preventDefault();
      selectGroup(ids[index]);
    }
    return;
  }

  if (event.altKey && !event.ctrlKey && !event.shiftKey && /^[1-9]$/.test(event.key)) {
    const index = Number(event.key) - 1;
    const entry = displayItems.value[index];
    if (entry) {
      event.preventDefault();
      void launchEntry(entry);
    }
    return;
  }

  if (event.key === 'Tab') {
    event.preventDefault();
    cycleGroup(event.shiftKey ? -1 : 1);
    return;
  }

  if (event.key === 'F5') {
    event.preventDefault();
    void refreshRecent();
    return;
  }

  if (typing) {
    // The search box owns the arrow keys while it has focus; only Enter launches.
    if (event.key === 'Enter') {
      event.preventDefault();
      void launchSelected();
    }
    return;
  }

  switch (event.key) {
    case 'ArrowDown':
    case 'ArrowRight':
      event.preventDefault();
      moveSelection(1);
      return;
    case 'ArrowUp':
    case 'ArrowLeft':
      event.preventDefault();
      moveSelection(-1);
      return;
    case 'Home':
      event.preventDefault();
      selectedIndex.value = 0;
      void gridRef.value?.ensureSelectionVisible();
      return;
    case 'End':
      event.preventDefault();
      selectedIndex.value = Math.max(0, displayItems.value.length - 1);
      void gridRef.value?.ensureSelectionVisible();
      return;
    case 'Enter':
      event.preventDefault();
      void launchSelected();
      return;
    case 'Delete':
      event.preventDefault();
      void deleteSelected();
      return;
    case '/':
      event.preventDefault();
      topBarRef.value?.focusSearch();
      return;
    default:
      break;
  }

  // Any printable character starts a search, the way a launcher is expected to behave.
  if (!event.ctrlKey && !event.altKey && event.key.length === 1 && event.key !== ' ') {
    topBarRef.value?.focusSearch();
  }
}

async function deleteSelected() {
  const entry = displayItems.value[selectedIndex.value];
  if (!entry) return;
  try {
    await bridge.deleteItem(entry.groupId, entry.item.id);
    await refreshUndoLabel();
    if (undoLabel.value) {
      showToast({ type: 'info', message: `${undoLabel.value}（Ctrl+Z）` });
    }
  } catch (err) {
    showToast({ type: 'error', message: `删除失败: ${errorText(err)}` });
  }
}

// --- window & lifecycle ---------------------------------------------------

function handleWindowBlur() {
  closeContextMenu();
  void bridge.onWindowBlur();
}

function handleWindowResize() {
  void bridge.saveWindowState();
}

function handleMouseUp() {
  // Drag ends on the title bar; the backend debounces the actual write.
  void bridge.saveWindowState();
}

watch(displayItems, () => {
  if (selectedIndex.value >= displayItems.value.length) {
    selectedIndex.value = Math.max(0, displayItems.value.length - 1);
  }
});

const unsubscribers: Array<() => void> = [];

onMounted(async () => {
  themeMedia = window.matchMedia('(prefers-color-scheme: light)');
  prefersLight.value = themeMedia.matches;
  themeMedia.addEventListener('change', handleSystemThemeChange);

  window.addEventListener('blur', handleWindowBlur);
  window.addEventListener('keydown', handleWindowKeyDown);
  window.addEventListener('resize', handleWindowResize);
  window.addEventListener('mouseup', handleMouseUp);

  unsubscribers.push(
    bridge.eventsOn<Config>('config:updated', (next) => {
      if (next) applyConfig(next);
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
      if (payload?.message) showToast(payload);
    }),
    bridge.eventsOn<HotkeyStatus>('hotkey:status', (status) => {
      if (status) hotkeyStatus.value = status;
    }),
    bridge.eventsOn('window:shown', () => {
      selectedIndex.value = 0;
      void topBarRef.value?.focusSearch();
      void refreshRecent();
    }),
    bridge.eventsOn('usage:updated', () => {
      void refreshRecent();
    }),
    bridge.onFileDrop((paths) => {
      void handleDroppedPaths(paths);
    })
  );

  try {
    const initial = await bridge.getConfig();
    if (initial) applyConfig(initial);
  } catch (err) {
    configError.value = `加载配置失败: ${errorText(err)}`;
  }

  try {
    const status = await bridge.getHotkeyStatus();
    if (status) hotkeyStatus.value = status;
  } catch {
    // Hotkey status events keep this up to date when the call is not available yet.
  }

  try {
    appInfo.value = await bridge.getAppInfo();
  } catch {
    appInfo.value = null;
  }

  try {
    const backendError = await bridge.getConfigError();
    if (backendError) configError.value = backendError;
  } catch {
    // The banner stays empty when the query fails.
  }

  await nextTick();
  void bridge.applyWindowState();

  if (!isSearching.value && !config.settings.hide_on_blur) {
    // Keep focus predictable: the launcher is meant to be typed into right away.
    topBarRef.value?.focusSearch();
  }
});

onBeforeUnmount(() => {
  themeMedia?.removeEventListener('change', handleSystemThemeChange);
  window.removeEventListener('blur', handleWindowBlur);
  window.removeEventListener('keydown', handleWindowKeyDown);
  window.removeEventListener('resize', handleWindowResize);
  window.removeEventListener('mouseup', handleMouseUp);
  window.clearTimeout(searchTimer);
  for (const unsub of unsubscribers) unsub();
});
</script>

<template>
  <div class="w-screen h-screen select-none wails-drop-zone">
    <div
      class="w-full h-full rounded-lg glass-panel border jd-border grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      :style="shellStyle"
    >
      <!-- Top Bar -->
      <TopBar
        ref="topBarRef"
        :search-query="searchQuery"
        :active-group-name="activeGroupName || undefined"
        :item-count="displayItems.length"
        @update:search-query="handleSearchInput"
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
      <div class="flex min-h-0 overflow-hidden">
        <GroupSidebar
          :groups="config.groups"
          :active-group-id="activeGroupId"
          :collapsed="sidebarCollapsed"
          :show-recent="config.settings.show_recent"
          :recent-count="recentItems.length"
          @select="selectGroup"
          @add-group="openAddGroupModal"
          @toggle-collapse="sidebarCollapsed = !sidebarCollapsed"
          @contextmenu="openGroupContextMenu"
          @drop-item="handleMoveItem"
        />

        <ItemGrid
          ref="gridRef"
          :items="displayItems"
          :mode="gridMode"
          :active-group-name="activeGroupName"
          :search-query="searchQuery"
          :cache-buster="iconCacheBuster"
          :has-active-group="hasActiveGroup"
          :selected-index="selectedIndex"
          :click-mode="config.settings.click_mode"
          :loading="searching"
          @launch="launchEntry"
          @contextmenu="selectEntry"
          @select="selectedIndex = $event"
          @clear-search="handleSearchInput('')"
          @add-item="openCreateItemModal"
          @open-start-menu-import="openStartMenuImport"
        />
      </div>

      <!-- Status Bar -->
      <footer class="h-7 px-3 flex items-center justify-between gap-3 border-t jd-border jd-surface-soft text-[11px] jd-subtle shrink-0">
        <div class="flex items-center gap-3 min-w-0">
          <span class="truncate">
            {{ config.settings.hotkey }}
            <span v-if="!hotkeyStatus.registered" class="text-rose-300">(未注册)</span>
          </span>
          <span v-if="undoLabel" class="truncate jd-muted">{{ undoLabel }} · Ctrl+Z</span>
        </div>
        <div class="flex items-center gap-3 shrink-0">
          <span>{{ config.groups.length }} 个分组</span>
          <span v-if="isSearching">{{ displayItems.length }} 个结果</span>
          <button
            type="button"
            class="px-1.5 py-0.5 rounded jd-hover transition-colors"
            title="导入开始菜单"
            @click="openStartMenuImport"
          >
            导入
          </button>
        </div>
      </footer>
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
      :app-info="appInfo"
      @close="settingsOpen = false"
      @save="handleSaveSettings"
      @open-config-file="handleOpenConfigFile"
      @reload-config="handleReloadConfig"
      @open-base-dir="handleOpenBaseDir"
      @export-config="handleExportConfig"
      @import-config="handleImportConfig"
      @reset-window-state="handleResetWindowState"
      @open-start-menu-import="openStartMenuImport"
      @clear-recent="handleClearRecent"
    />

    <GroupModal
      :visible="groupModal.visible"
      :mode="groupModal.mode"
      :initial-name="groupModal.initialName"
      @close="groupModal.visible = false"
      @submit="handleGroupModalSubmit"
    />

    <StartMenuImportModal
      :visible="startMenuModal.visible"
      :entries="startMenuModal.entries"
      :loading="startMenuModal.loading"
      :group-name="activeGroupName"
      @close="startMenuModal.visible = false"
      @refresh="loadStartMenuEntries"
      @import="handleStartMenuImport"
    />

    <!-- Transient Toast Container -->
    <ToastContainer :toasts="toasts" @dismiss="dismissToast" />
  </div>
</template>
