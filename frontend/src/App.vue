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
  StartMenuEntry,
  ToastMessage,
  ToastPayload,
} from './types/config';
import { RECENT_GROUP_ID, defaultSettings } from './types/config';
import { registerTranslations, setLanguage, t } from './i18n';
import TopBar from './components/TopBar.vue';
import GroupSidebar from './components/GroupSidebar.vue';
import ItemGrid from './components/ItemGrid.vue';
import ContextMenu, { type ContextMenuItem } from './components/ContextMenu.vue';
import ItemEditModal, { type ItemSaveRequest } from './components/ItemEditModal.vue';
import SettingsModal from './components/SettingsModal.vue';
import GroupModal from './components/GroupModal.vue';
import StartMenuImportModal from './components/StartMenuImportModal.vue';
import ConfirmDialog from './components/ConfirmDialog.vue';
import ToastContainer from './components/ToastContainer.vue';

registerTranslations({
  最近使用: 'Recent',
  启动: 'Launch',
  编辑: 'Edit',
  取消管理员身份运行: 'Stop running as administrator',
  以管理员身份运行: 'Run as administrator',
  打开所在文件夹: 'Open containing folder',
  复制目标路径: 'Copy target path',
  上移: 'Move up',
  下移: 'Move down',
  删除: 'Delete',
  重命名分组: 'Rename group',
  分组上移: 'Move group up',
  分组下移: 'Move group down',
  删除分组: 'Delete group',
  '更新路径…': 'Update target…',
  '分组「{name}」包含 {count} 个项目，删除后可以按 Ctrl+Z 撤销。':
    'Group "{name}" has {count} items. Press Ctrl+Z to undo the deletion.',
  '确定删除分组「{name}」吗？可以按 Ctrl+Z 撤销。':
    'Delete group "{name}"? Press Ctrl+Z to undo it.',
  未命名分组: 'Untitled group',
  '搜索失败: {error}': 'Search failed: {error}',
  '启动失败: {error}': 'Launch failed: {error}',
  已取消管理员身份运行: 'Administrator mode turned off',
  已设为以管理员身份运行: 'Administrator mode turned on',
  已复制目标路径: 'Target path copied',
  '{label}（Ctrl+Z）': '{label} (Ctrl+Z)',
  '操作失败: {error}': 'Action failed: {error}',
  请先创建一个分组: 'Create a group first',
  '保存项目失败: {error}': 'Saving the item failed: {error}',
  请先创建一个分组再拖入文件: 'Create a group before dropping files',
  '添加拖入文件失败: {error}': 'Adding the dropped files failed: {error}',
  '分组操作失败: {error}': 'Group action failed: {error}',
  '配置已导出到 {target}': 'Configuration exported to {target}',
  '导出失败: {error}': 'Export failed: {error}',
  '已导入 {count} 个分组': 'Imported {count} groups',
  '导入失败: {error}': 'Import failed: {error}',
  '重载配置失败: {error}': 'Reloading the configuration failed: {error}',
  '打开配置文件失败: {error}': 'Opening config.json failed: {error}',
  '打开数据目录失败: {error}': 'Opening the data folder failed: {error}',
  已重置窗口位置和大小: 'Window position and size reset',
  '重置失败: {error}': 'Reset failed: {error}',
  '已移动到「{name}」': 'Moved to "{name}"',
  '移动失败: {error}': 'Moving failed: {error}',
  最近记录已清空: 'Recent items cleared',
  '清空失败: {error}': 'Clearing failed: {error}',
  '扫描开始菜单失败: {error}': 'Scanning the Start Menu failed: {error}',
  请先选择一个分组: 'Select a group first',
  '已恢复「{name}」': 'Restored "{name}"',
  没有可撤销的删除: 'Nothing to undo',
  '撤销失败: {error}': 'Undo failed: {error}',
  '删除失败: {error}': 'Delete failed: {error}',
  '保存分组折叠状态失败: {error}': 'Saving the folded groups failed: {error}',
  '检查失效目标失败: {error}': 'Checking for missing targets failed: {error}',
  'Alt+←/→ 切换分组（Ctrl+Tab 也可）': 'Alt+←/→ switch groups (Ctrl+Tab also works)',
  '配置文件异常: {error}': 'Configuration problem: {error}',
  '打开 config.json': 'Open config.json',
  关闭提示: 'Dismiss notification',
  '加载配置失败: {error}': 'Loading the configuration failed: {error}',
  'config.json 解析错误': 'config.json could not be parsed',
  'config.json 已被删除，当前仍在用内存中的配置':
    'config.json was deleted; the in-memory configuration is still in use',
  重建配置文件: 'Rebuild config file',
  打开数据目录: 'Open data folder',
  '已重建 config.json': 'config.json rebuilt',
  '重建配置文件失败: {error}': 'Rebuilding config.json failed: {error}',
  关闭横幅: 'Dismiss banner',
  '(未注册)': '(not registered)',
  '{count} 个分组': '{count} groups',
  '{count} 个结果': '{count} results',
  导入: 'Import',
  导入开始菜单: 'Import from Start Menu',
});

const SEARCH_DEBOUNCE_MS = 120;
// D13: how long an item stays "launching" after its launch call settles, so the second click of a
// double click cannot send a second launch request.
const LAUNCH_GUARD_MS = 400;
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

// D11: the first frame must not claim the list is empty before getConfig() has answered.
const configReady = ref(false);
// D13: entries whose launch is in flight, so a fast double click only launches once.
const launchingIds = ref<Set<string>>(new Set());
// C7: item IDs whose target file is gone, so the grid can mark them.
const missingTargets = ref<Set<string>>(new Set());
// E10: config.json was deleted while the app is running; a banner offers the recovery action.
const configRemoved = ref(false);

const topBarRef = ref<InstanceType<typeof TopBar> | null>(null);
const gridRef = ref<InstanceType<typeof ItemGrid> | null>(null);
const settingsRef = ref<InstanceType<typeof SettingsModal> | null>(null);

// Modals
const settingsOpen = ref(false);
// groupId is stored next to the item: in the recent view there is no active group, and the dialog
// would otherwise save against an empty group id.
const itemModal = reactive<{ visible: boolean; item: Item | null; groupId: string }>({
  visible: false,
  item: null,
  groupId: '',
});
const groupModal = reactive<{
  visible: boolean;
  mode: 'add' | 'rename';
  groupId: string;
  initialName: string;
}>({ visible: false, mode: 'add', groupId: '', initialName: '' });
const startMenuModal = reactive<{
  visible: boolean;
  loading: boolean;
  busy: boolean;
  error: string;
  entries: StartMenuEntry[];
}>({ visible: false, loading: false, busy: false, error: '', entries: [] });

// Destructive actions go through one confirm dialog instead of a browser confirm(), which
// WebView2 renders as a separate frameless window.
const confirmDialog = reactive<{
  visible: boolean;
  title: string;
  message: string;
  confirmLabel: string;
  danger: boolean;
  busy: boolean;
}>({ visible: false, title: '', message: '', confirmLabel: '', danger: false, busy: false });
let confirmAction: (() => Promise<void>) | null = null;

function askConfirm(options: { title: string; message: string; confirmLabel?: string; danger?: boolean }, action: () => Promise<void>) {
  confirmDialog.title = options.title;
  confirmDialog.message = options.message;
  confirmDialog.confirmLabel = options.confirmLabel ?? '';
  confirmDialog.danger = options.danger ?? false;
  confirmDialog.busy = false;
  confirmDialog.visible = true;
  confirmAction = action;
}

function closeConfirm() {
  if (confirmDialog.busy) return;
  confirmDialog.visible = false;
  confirmAction = null;
}

async function runConfirm() {
  const action = confirmAction;
  if (!action || confirmDialog.busy) return;
  confirmDialog.busy = true;
  try {
    await action();
    confirmDialog.visible = false;
    confirmAction = null;
  } finally {
    confirmDialog.busy = false;
  }
}

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
const activeGroupName = computed(() => (isRecentView.value ? t('最近使用') : activeGroup.value?.name ?? ''));
const hasActiveGroup = computed(() => Boolean(activeGroup.value));
const isSearching = computed(() => searchQuery.value.trim().length > 0);
const totalItemCount = computed(() =>
  config.groups.reduce((sum, group) => sum + (group.items?.length ?? 0), 0)
);
const collapsedGroupIds = computed<string[]>(() => {
  const ids = config.settings.collapsed_groups;
  return Array.isArray(ids) ? ids : [];
});
// F3: a folded group still shows how many hits it holds while searching.
const searchHitCounts = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {};
  for (const hit of searchHits.value) {
    counts[hit.group_id] = (counts[hit.group_id] ?? 0) + 1;
  }
  return counts;
});

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

// Any dialog on screen keeps the window from hiding itself on blur (file pickers, alt-tab).
const anyModalOpen = computed(
  () =>
    settingsOpen.value ||
    itemModal.visible ||
    groupModal.visible ||
    startMenuModal.visible ||
    contextMenu.visible
);

watch(
  anyModalOpen,
  (open) => {
    void bridge.setModalOpen(open).catch(() => undefined);
  },
  { flush: 'post' }
);

const contextMenuItems = computed<ContextMenuItem[]>(() => {
  if (contextMenu.targetType === 'item' && contextMenu.entry) {
    const entry = contextMenu.entry;
    const items: ContextMenuItem[] = [
      { id: 'launch', label: t('启动'), icon: 'edit' },
      { id: 'edit-item', label: t('编辑'), icon: 'edit' },
      {
        id: 'toggle-admin',
        label: entry.item.admin ? t('取消管理员身份运行') : t('以管理员身份运行'),
      },
      { id: 'open-location', label: t('打开所在文件夹'), icon: 'folder' },
      { id: 'copy-path', label: t('复制目标路径') },
    ];
    if (missingTargets.value.has(entry.item.id)) {
      // The file moved or was renamed; picking the new target keeps the entry usable.
      items.splice(2, 0, { id: 'relocate-item', label: t('更新路径…'), icon: 'folder' });
    }
    if (entry.groupId === activeGroupId.value) {
      items.push({ id: 'move-up', label: t('上移') });
      items.push({ id: 'move-down', label: t('下移') });
    }
    items.push({ id: 'delete-item', label: t('删除'), icon: 'trash', danger: true, dividerBefore: true });
    return items;
  }
  if (contextMenu.targetType === 'group' && contextMenu.group) {
    return [
      { id: 'rename-group', label: t('重命名分组'), icon: 'rename' },
      { id: 'group-up', label: t('分组上移') },
      { id: 'group-down', label: t('分组下移') },
      { id: 'delete-group', label: t('删除分组'), icon: 'trash', danger: true, dividerBefore: true },
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
  toasts.value = toasts.value.filter((item) => item.id !== id);
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

// Pending reveal bookkeeping: which group to jump back to, and which item ids existed before the
// add, so the newly added entry can be identified once the config arrives.
let pendingRevealGroupId = '';
let revealBaselineIds = new Set<string>();
let revealPendingItemId: string | null = null;

/** revealSelection selects the last item of a group, or the entry matching pendingItemId. */
function revealSelection(groupId: string, pendingItemId: string | null) {
  const group = config.groups.find((g) => g.id === groupId);
  if (!group || group.items.length === 0) return;
  const index = pendingItemId ? group.items.findIndex((item) => item.id === pendingItemId) : -1;
  selectedIndex.value = index >= 0 ? index : group.items.length - 1;
  void gridRef.value?.ensureSelectionVisible();
}

/** iconSignature changes only when an item's icon does, so unrelated edits keep the cache URLs. */
function iconSignature(groups: Group[]): string {
  return groups.map((group) => group.items.map((item) => item.id + ':' + item.icon).join(',')).join('|');
}

function applyConfig(next: Config) {
  const settings = { ...defaultSettings, ...next.settings };
  config.settings = settings;
  const nextGroups = Array.isArray(next.groups) ? next.groups : [];
  if (iconSignature(nextGroups) !== iconSignature(config.groups)) {
    iconCacheBuster.value = Date.now();
  }
  config.groups = nextGroups;
  configError.value = '';
  // The file exists again (or was never the problem) once a usable config arrives.
  configRemoved.value = false;
  setLanguage(settings.language);

  const stillExists =
    activeGroupId.value === RECENT_GROUP_ID || config.groups.some((g) => g.id === activeGroupId.value);
  if (!stillExists) {
    selectGroup(config.groups[0]?.id ?? '', { silent: true });
  } else if (activeGroupId.value && activeGroupId.value !== RECENT_GROUP_ID) {
    void bridge.setActiveGroup(activeGroupId.value);
  }

  void refreshRecent();
  void refreshUndoLabel();
  void refreshMissingTargets();

  // The pending reveal from an add/drop/import can only be resolved now that the new items exist.
  if (pendingRevealGroupId) {
    const groupId = pendingRevealGroupId;
    pendingRevealGroupId = '';
    const group = config.groups.find((g) => g.id === groupId);
    const added = group?.items.find((item) => !revealBaselineIds.has(item.id));
    revealPendingItemId = added?.id ?? null;
    void nextTick(() => {
      revealSelection(groupId, revealPendingItemId);
    });
  }
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

// Signature of the group data the last missing-target probe ran against. Anything that cannot move
// a target (a settings save, folding a group, a window show) must not trigger a full os.Stat sweep.
let missingTargetsSignature = '';
let missingTargetsFailureReported = false;

/**
 * C7 refreshes the set of broken targets; config changes can fix or break any of them. The probe
 * stats every target, so it only runs when the groups actually changed.
 */
async function refreshMissingTargets() {
  const signature = JSON.stringify(config.groups);
  if (signature === missingTargetsSignature) return;
  try {
    missingTargets.value = new Set(await bridge.getMissingTargets());
    missingTargetsSignature = signature;
    missingTargetsFailureReported = false;
  } catch (err) {
    // A failed probe keeps the previous set: forgetting broken entries would hide them again.
    if (!missingTargetsFailureReported) {
      missingTargetsFailureReported = true;
      showToast({ type: 'error', message: t('检查失效目标失败: {error}', { error: errorText(err) }) });
    }
  }
}

let searchTimer = 0;
// Only the newest query may write into the grid; slower answers are dropped instead of applied.
let searchSeq = 0;

function handleSearchInput(value: string) {
  searchQuery.value = value;
  selectedIndex.value = 0;
  window.clearTimeout(searchTimer);
  const seq = ++searchSeq;
  if (!value.trim()) {
    searchHits.value = [];
    searching.value = false;
    return;
  }
  searching.value = true;
  searchTimer = window.setTimeout(async () => {
    try {
      const hits = await bridge.searchItems(value);
      if (seq !== searchSeq) return;
      searchHits.value = hits;
    } catch (err) {
      if (seq !== searchSeq) return;
      searchHits.value = [];
      showToast({ type: 'error', message: t('搜索失败: {error}', { error: errorText(err) }) });
    } finally {
      // An outdated request must not clear the spinner of the query that replaced it.
      if (seq === searchSeq) searching.value = false;
    }
  }, SEARCH_DEBOUNCE_MS);
}

// --- launching ------------------------------------------------------------

function entryKey(entry: DisplayItem): string {
  return `${entry.groupId}:${entry.item.id}`;
}

async function launchEntry(entry: DisplayItem) {
  const key = entryKey(entry);
  if (launchingIds.value.has(key)) return;
  const inFlight = new Set(launchingIds.value);
  inFlight.add(key);
  launchingIds.value = inFlight;

  try {
    await bridge.launchItem(entry.groupId, entry.item.id);
    void refreshRecent();
  } catch (err) {
    showToast({ type: 'error', message: t('启动失败: {error}', { error: errorText(err) }) });
  } finally {
    // The guard outlives the call on purpose: a double click delivers its second event right after.
    window.setTimeout(() => {
      const remaining = new Set(launchingIds.value);
      remaining.delete(key);
      launchingIds.value = remaining;
    }, LAUNCH_GUARD_MS);
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

/**
 * storedItem returns the item as the config holds it. Search hits and recent entries carry a
 * stripped-down item (no args, no work_dir), so those objects must never reach an update call.
 */
function storedItem(groupId: string, itemId: string): Item | null {
  return config.groups.find((g) => g.id === groupId)?.items.find((i) => i.id === itemId) ?? null;
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
      // Open the dialog for the group the item really lives in, with every field filled in.
      itemModal.item = { ...(storedItem(entry.groupId, entry.item.id) ?? entry.item) };
      itemModal.groupId = entry.groupId;
      itemModal.visible = true;
      return;
    }

    if (actionId === 'toggle-admin' && entry) {
      // A dedicated endpoint flips the flag only; sending the partial display item would erase
      // the arguments and working directory that were saved for it.
      const nextAdmin = !entry.item.admin;
      await bridge.setItemAdmin(entry.groupId, entry.item.id, nextAdmin);
      showToast({
        type: 'success',
        message: nextAdmin ? t('已设为以管理员身份运行') : t('已取消管理员身份运行'),
      });
      return;
    }

    if (actionId === 'open-location' && entry) {
      await bridge.openItemLocation(entry.item.path);
      return;
    }

    if (actionId === 'relocate-item' && entry) {
      const picked = await bridge.relocateItem(entry.groupId, entry.item.id);
      if (picked) {
        missingTargetsSignature = '';
        void refreshMissingTargets();
      }
      return;
    }

    if (actionId === 'copy-path' && entry) {
      await navigator.clipboard.writeText(entry.item.path);
      showToast({ type: 'success', message: t('已复制目标路径') });
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
        showToast({ type: 'info', message: t('{label}（Ctrl+Z）', { label: undoLabel.value }) });
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
      const itemCount = group.items.length;
      askConfirm(
        {
          title: t('删除分组'),
          message:
            itemCount > 0
              ? t('分组「{name}」包含 {count} 个项目，删除后可以按 Ctrl+Z 撤销。', { name: group.name, count: itemCount })
              : t('确定删除分组「{name}」吗？可以按 Ctrl+Z 撤销。', { name: group.name }),
          confirmLabel: t('删除'),
          danger: true,
        },
        async () => {
          await bridge.deleteGroup(group.id);
          await refreshUndoLabel();
          if (undoLabel.value) {
            showToast({ type: 'info', message: t('{label}（Ctrl+Z）', { label: undoLabel.value }) });
          }
        }
      );
    }
  } catch (err) {
    showToast({ type: 'error', message: t('操作失败: {error}', { error: errorText(err) }) });
  }
}

// --- items ----------------------------------------------------------------

/**
 * D7: an added item must be visible right away. A leftover search filter or another active group
 * would hide it, so the grid goes back to the group the item landed in and scrolls to its end,
 * where the new entry was appended.
 */
function revealItems(groupId: string) {
  if (isSearching.value) {
    handleSearchInput('');
  }
  if (groupId && groupId !== activeGroupId.value) {
    selectGroup(groupId);
  }
  // The item is not in `config` yet: the backend answers with a config:updated event afterwards.
  // Selecting the last entry now would point at the previous item, so the id is remembered and
  // consumed by applyConfig once the new list is in.
  pendingRevealGroupId = groupId;
  revealPendingItemId = null;
  // A new item is invisible under an active filter; clearing it shows what was just added.
  searchQuery.value = '';
  const existing = config.groups.find((g) => g.id === groupId);
  revealBaselineIds = new Set(existing?.items.map((item) => item.id) ?? []);
  void nextTick(() => {
    revealSelection(groupId, revealPendingItemId);
  });
  void refreshMissingTargets();
}

function openCreateItemModal() {
  if (!activeGroup.value) {
    showToast({ type: 'warning', message: t('请先创建一个分组') });
    openAddGroupModal();
    return;
  }
  itemModal.item = null;
  itemModal.groupId = activeGroup.value.id;
  itemModal.visible = true;
}

/** The item dialog reports picker failures inline; the shell mirrors them once as a toast. */
function handleItemModalError(message: string) {
  if (message) {
    showToast({ type: 'error', message });
  }
}

async function handleSaveItem(request: ItemSaveRequest) {
  try {
    if (request.isNew) {
      await bridge.addItem(request.groupId, request.item);
      revealItems(request.groupId);
    } else {
      await bridge.updateItem(request.groupId, request.item);
    }
    itemModal.visible = false;
  } catch (err) {
    showToast({ type: 'error', message: t('保存项目失败: {error}', { error: errorText(err) }) });
  } finally {
    // Always report back: on failure the modal stays open and must accept another attempt.
    request.done();
  }
}

async function handleDroppedPaths(paths: string[]) {
  const groupId = activeGroup.value?.id ?? config.groups[0]?.id;
  if (!groupId) {
    showToast({ type: 'warning', message: t('请先创建一个分组再拖入文件') });
    openAddGroupModal();
    return;
  }
  try {
    await bridge.addDroppedFiles(groupId, paths);
    revealItems(groupId);
  } catch (err) {
    showToast({ type: 'error', message: t('添加拖入文件失败: {error}', { error: errorText(err) }) });
  }
}

// --- groups & settings ----------------------------------------------------

function openAddGroupModal() {
  groupModal.mode = 'add';
  groupModal.groupId = '';
  groupModal.initialName = '';
  groupModal.visible = true;
}

async function handleGroupModalSubmit(name: string, done?: () => void) {
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
    showToast({ type: 'error', message: t('分组操作失败: {error}', { error: errorText(err) }) });
  } finally {
    done?.();
  }
}

/**
 * F3: folding a group is persisted through the dedicated quiet endpoint instead of saveSettings,
 * which would rewrite every setting and re-render the list that was just clicked.
 */
async function handleToggleGroupCollapsed(groupId: string) {
  const next = new Set(collapsedGroupIds.value);
  if (next.has(groupId)) {
    next.delete(groupId);
  } else {
    next.add(groupId);
  }
  const ids = [...next];
  config.settings = { ...config.settings, collapsed_groups: ids };
  try {
    await bridge.setCollapsedGroups(ids);
  } catch (err) {
    showToast({ type: 'error', message: t('保存分组折叠状态失败: {error}', { error: errorText(err) }) });
  }
}

async function handleExportConfig() {
  try {
    const target = await bridge.exportConfig();
    if (target) {
      showToast({ type: 'success', message: t('配置已导出到 {target}', { target }) });
    }
  } catch (err) {
    showToast({ type: 'error', message: t('导出失败: {error}', { error: errorText(err) }) });
  }
}

async function handleImportConfig() {
  try {
    const count = await bridge.importConfig();
    if (count > 0) {
      showToast({ type: 'success', message: t('已导入 {count} 个分组', { count }) });
    }
  } catch (err) {
    showToast({ type: 'error', message: t('导入失败: {error}', { error: errorText(err) }) });
  }
}

async function handleReloadConfig() {
  try {
    await bridge.reloadConfig();
  } catch (err) {
    showToast({ type: 'error', message: t('重载配置失败: {error}', { error: errorText(err) }) });
  }
}

async function handleOpenConfigFile() {
  try {
    await bridge.openConfigFile();
  } catch (err) {
    showToast({ type: 'error', message: t('打开配置文件失败: {error}', { error: errorText(err) }) });
  }
}

async function handleOpenBaseDir() {
  try {
    await bridge.openBaseDir();
  } catch (err) {
    showToast({ type: 'error', message: t('打开数据目录失败: {error}', { error: errorText(err) }) });
  }
}

/** E10: writes the in-memory configuration back to the deleted config.json. */
async function handleRebuildConfigFile() {
  try {
    await bridge.rebuildConfigFile();
    configRemoved.value = false;
    configError.value = '';
    showToast({ type: 'success', message: t('已重建 config.json') });
  } catch (err) {
    showToast({ type: 'error', message: t('重建配置文件失败: {error}', { error: errorText(err) }) });
  }
}

async function handleResetWindowState() {
  try {
    await bridge.resetWindowState();
    showToast({ type: 'success', message: t('已重置窗口位置和大小') });
  } catch (err) {
    showToast({ type: 'error', message: t('重置失败: {error}', { error: errorText(err) }) });
  }
}

async function handleMoveItem(payload: { groupId: string; fromGroupId: string; itemId: string }) {
  try {
    await bridge.moveItem(payload.fromGroupId, payload.itemId, payload.groupId, 0);
    const target = config.groups.find((g) => g.id === payload.groupId);
    showToast({ type: 'success', message: t('已移动到「{name}」', { name: target?.name ?? t('未命名分组') }) });
  } catch (err) {
    showToast({ type: 'error', message: t('移动失败: {error}', { error: errorText(err) }) });
  }
}

async function handleClearRecent() {
  try {
    await bridge.clearRecentItems();
    recentItems.value = [];
    showToast({ type: 'success', message: t('最近记录已清空') });
  } catch (err) {
    showToast({ type: 'error', message: t('清空失败: {error}', { error: errorText(err) }) });
  }
}

// --- start menu import ----------------------------------------------------

async function openStartMenuImport() {
  startMenuModal.visible = true;
  await loadStartMenuEntries();
}

async function loadStartMenuEntries() {
  startMenuModal.loading = true;
  startMenuModal.error = '';
  try {
    startMenuModal.entries = await bridge.scanStartMenu();
  } catch (err) {
    startMenuModal.entries = [];
    startMenuModal.error = errorText(err);
    showToast({ type: 'error', message: t('扫描开始菜单失败: {error}', { error: errorText(err) }) });
  } finally {
    startMenuModal.loading = false;
  }
}

async function handleStartMenuImport(lnkPaths: string[]) {
  const groupId = activeGroup.value?.id;
  if (!groupId) {
    showToast({ type: 'warning', message: t('请先选择一个分组') });
    return;
  }
  startMenuModal.busy = true;
  try {
    await bridge.importStartMenuItems(groupId, lnkPaths);
    startMenuModal.visible = false;
    revealItems(groupId);
  } catch (err) {
    showToast({ type: 'error', message: t('导入失败: {error}', { error: errorText(err) }) });
  } finally {
    startMenuModal.busy = false;
  }
}

// --- undo -----------------------------------------------------------------

async function handleUndo() {
  try {
    const name = (await bridge.getUndoLabel()).startsWith('group:')
      ? await bridge.undoDeleteGroup()
      : await bridge.undoDeleteItem();
    if (name) {
      showToast({ type: 'success', message: t('已恢复「{name}」', { name }) });
    } else {
      showToast({ type: 'info', message: t('没有可撤销的删除') });
    }
    await refreshUndoLabel();
  } catch (err) {
    showToast({ type: 'error', message: t('撤销失败: {error}', { error: errorText(err) }) });
  }
}

// --- keyboard -------------------------------------------------------------

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable;
}

function isOverlayOpen(): boolean {
  return (
    settingsOpen.value ||
    itemModal.visible ||
    groupModal.visible ||
    startMenuModal.visible ||
    confirmDialog.visible ||
    contextMenu.visible
  );
}

/**
 * The settings dialog owns its close path: it rolls back a language preview and refuses to close
 * while a save is in flight. Esc therefore goes through it instead of flipping the flag here.
 */
function closeSettings() {
  settingsRef.value?.handleClose();
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
      closeSettings();
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

  // Everything below is a global shortcut, so a focused text field is checked first: inside an input
  // Ctrl+Z must undo the text, Tab must move focus, and the arrows must move the caret. Only Enter
  // (launch the selection) and Escape (handled above) are taken away from it.
  if (isTypingTarget(event.target)) {
    if (event.key === 'Enter') {
      event.preventDefault();
      void launchSelected();
    }
    return;
  }

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

  // Alt+Left/Right switches groups (the header pill names the shortcut); Tab stays free for focus.
  if (
    event.altKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    (event.key === 'ArrowLeft' || event.key === 'ArrowRight')
  ) {
    event.preventDefault();
    cycleGroup(event.key === 'ArrowLeft' ? -1 : 1);
    return;
  }

  if (event.key === 'Tab') {
    // Tab keeps moving focus: hijacking it trapped keyboard users inside the grid. Ctrl+Tab (and
    // Ctrl+Shift+Tab) still switches groups for people used to the old behaviour.
    if (event.ctrlKey) {
      event.preventDefault();
      cycleGroup(event.shiftKey ? -1 : 1);
    }
    return;
  }

  if (event.key === 'F5') {
    event.preventDefault();
    void refreshRecent();
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
      showToast({ type: 'info', message: t('{label}（Ctrl+Z）', { label: undoLabel.value }) });
    }
  } catch (err) {
    showToast({ type: 'error', message: t('删除失败: {error}', { error: errorText(err) }) });
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
        configError.value = payload?.message || payload?.error || t('config.json 解析错误');
      }
    }),
    // E10: deleting config.json is not an edit the UI can render, so it gets its own banner.
    bridge.eventsOn('config:removed', () => {
      configRemoved.value = true;
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
      // The panel is a launcher: reopening it should start from the unfiltered grid.
      searchQuery.value = '';
      // A dialog that survived the window being hidden keeps the focus; stealing it would pull the
      // caret out of the form the user was in the middle of filling.
      if (!anyModalOpen.value) {
        void topBarRef.value?.focusSearch();
      }
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
    configError.value = t('加载配置失败: {error}', { error: errorText(err) });
  } finally {
    // D11: only now may the grid render an empty state instead of its skeleton.
    configReady.value = true;
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
  // Leaving the modal hint set would keep the backend from hiding the window on blur.
  void bridge.setModalOpen(false).catch(() => undefined);
  for (const unsub of unsubscribers) unsub();
});
</script>

<template>
  <div class="w-screen h-screen select-none wails-drop-zone">
    <div
      class="w-full h-full rounded-lg glass-panel border jd-border grid grid-rows-[auto_auto_minmax(0,1fr)_auto] overflow-hidden"
      :style="shellStyle"
    >
      <!-- Top Bar -->
      <TopBar
        ref="topBarRef"
        :search-query="searchQuery"
        :active-group-name="activeGroupName || undefined"
        :item-count="displayItems.length"
        :total-count="totalItemCount"
        @update:search-query="handleSearchInput"
        @add-item="openCreateItemModal"
        @open-settings="settingsOpen = true"
        @hide="bridge.hideWindow()"
      />

      <!-- Banners: one wrapper keeps the grid rows stable whether one or both are shown -->
      <div class="shrink-0">
        <!-- Non-crashing Config Error Banner -->
        <Transition name="fade">
          <div
            v-if="configError"
            role="alert"
            class="px-4 py-2 bg-rose-500/20 border-b border-rose-400/30 text-xs flex items-center justify-between gap-3 shrink-0"
          >
            <div class="flex items-center gap-2 min-w-0">
              <svg
                class="w-4 h-4 text-rose-300 shrink-0"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                aria-hidden="true"
              >
                <circle cx="12" cy="12" r="9" />
                <path stroke-linecap="round" d="M12 8v5m0 3h.01" />
              </svg>
              <span class="truncate jd-text select-text" :title="configError">
                {{ t('配置文件异常: {error}', { error: configError }) }}
              </span>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <button
                type="button"
                class="px-2.5 py-1 rounded-lg bg-rose-400/25 hover:bg-rose-400/35 border border-rose-300/30 text-rose-50 transition-colors"
                @click="handleOpenConfigFile"
              >
                {{ t('打开 config.json') }}
              </button>
              <button
                type="button"
                :aria-label="t('关闭提示')"
                :title="t('关闭提示')"
                class="p-1 rounded-lg text-rose-200/80 hover:text-white hover:bg-white/10 transition-colors"
                @click="configError = ''"
              >
                <svg
                  class="w-3.5 h-3.5"
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
          </div>
        </Transition>

        <!-- E10: config.json was deleted while running -->
        <Transition name="fade">
          <div
            v-if="configRemoved"
            role="alert"
            class="px-4 py-2 bg-amber-500/20 border-b border-amber-400/30 text-xs flex items-center justify-between gap-3 shrink-0"
          >
            <div class="flex items-center gap-2 min-w-0">
              <svg
                class="w-4 h-4 text-amber-300 shrink-0"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                aria-hidden="true"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 3l9 16H3l9-16z" />
                <path stroke-linecap="round" d="M12 9v4m0 3h.01" />
              </svg>
              <span class="truncate jd-text select-text">
                {{ t('config.json 已被删除，当前仍在用内存中的配置') }}
              </span>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <button
                type="button"
                class="px-2.5 py-1 rounded-lg bg-amber-400/25 hover:bg-amber-400/35 border border-amber-300/30 text-amber-50 transition-colors"
                @click="handleRebuildConfigFile"
              >
                {{ t('重建配置文件') }}
              </button>
              <button
                type="button"
                class="px-2.5 py-1 rounded-lg bg-amber-400/15 hover:bg-amber-400/25 border border-amber-300/25 text-amber-50 transition-colors"
                @click="handleOpenBaseDir"
              >
                {{ t('打开数据目录') }}
              </button>
              <button
                type="button"
                :aria-label="t('关闭横幅')"
                :title="t('关闭横幅')"
                class="p-1 rounded-lg text-amber-100/80 hover:text-white hover:bg-white/10 transition-colors"
                @click="configRemoved = false"
              >
                <svg
                  class="w-3.5 h-3.5"
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
          </div>
        </Transition>
      </div>

      <!-- Workspace Body -->
      <div class="flex min-h-0 overflow-hidden">
        <GroupSidebar
          :groups="config.groups"
          :active-group-id="activeGroupId"
          :collapsed="sidebarCollapsed"
          :show-recent="config.settings.show_recent"
          :recent-count="recentItems.length"
          :collapsed-group-ids="collapsedGroupIds"
          :search-active="isSearching"
          :hit-counts="searchHitCounts"
          @select="selectGroup"
          @add-group="openAddGroupModal"
          @toggle-collapse="sidebarCollapsed = !sidebarCollapsed"
          @toggle-group-collapse="handleToggleGroupCollapsed"
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
          :has-groups="config.groups.length > 0"
          :selected-index="selectedIndex"
          :click-mode="config.settings.click_mode"
          :loading="searching"
          :config-ready="configReady"
          :launching-ids="launchingIds"
          :missing-targets="missingTargets"
          @launch="launchEntry"
          @contextmenu="selectEntry"
          @select="selectedIndex = $event"
          @clear-search="handleSearchInput('')"
          @add-item="openCreateItemModal"
          @add-group="openAddGroupModal"
          @open-start-menu-import="openStartMenuImport"
        />
      </div>

      <!-- Status Bar -->
      <footer class="h-7 px-3 flex items-center justify-between gap-3 border-t jd-border jd-surface-soft text-[11px] jd-subtle shrink-0">
        <div class="flex items-center gap-3 min-w-0">
          <span class="truncate">
            {{ config.settings.hotkey }}
            <span v-if="!hotkeyStatus.registered" class="text-rose-300">{{ t('(未注册)') }}</span>
          </span>
          <span v-if="undoLabel" class="truncate jd-muted select-text">{{ undoLabel }} · Ctrl+Z</span>
        </div>
        <div class="flex items-center gap-3 shrink-0">
          <span>{{ t('{count} 个分组', { count: config.groups.length }) }}</span>
          <span v-if="isSearching">{{ t('{count} 个结果', { count: displayItems.length }) }}</span>
          <button
            type="button"
            class="px-1.5 py-0.5 rounded jd-hover transition-colors"
            :aria-label="t('导入开始菜单')"
            :title="t('导入开始菜单')"
            @click="openStartMenuImport"
          >
            {{ t('导入') }}
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
      :group-id="itemModal.groupId"
      :cache-buster="iconCacheBuster"
      @close="itemModal.visible = false"
      @save="handleSaveItem"
      @error="handleItemModalError"
    />

    <SettingsModal
      ref="settingsRef"
      :visible="settingsOpen"
      :settings="config.settings"
      :hotkey-status="hotkeyStatus"
      :app-info="appInfo"
      @close="settingsOpen = false"
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
      :busy="startMenuModal.busy"
      :error="startMenuModal.error"
      :group-name="activeGroupName"
      @close="startMenuModal.visible = false"
      @refresh="loadStartMenuEntries"
      @import="handleStartMenuImport"
    />

    <ConfirmDialog
      :visible="confirmDialog.visible"
      :title="confirmDialog.title"
      :message="confirmDialog.message"
      :confirm-label="confirmDialog.confirmLabel"
      :danger="confirmDialog.danger"
      :busy="confirmDialog.busy"
      @confirm="runConfirm"
      @close="closeConfirm"
    />

    <!-- Transient Toast Container -->
    <ToastContainer :toasts="toasts" @dismiss="dismissToast" />
  </div>
</template>
