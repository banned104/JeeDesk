# Changelog

面向开发与维护者，记录行为、接口、配置与维护方式的变化。目前尚无正式版本，
以下为「开发中」的累计变化。

## 开发中

### 搜索、键盘与最近使用

#### 新增

- 跨分组搜索：输入即搜（120ms 防抖），结果标注所属分组；排序为「前缀 > 词首 > 子串」，
  同分时使用次数多的优先。评分逻辑在 `internal/search`，有独立单测。
- 键盘操作：呼出后搜索框自动聚焦，支持 `↑/↓`、`Home/End`、`Enter` 启动、`Delete` 删除、
  `Ctrl+Z` 撤销删除、`Ctrl+F` 聚焦搜索、`Ctrl+,` 打开设置、`Tab` 切组、`Alt+数字` 直接启动、
  直接打字即搜索；`Esc` 隐藏。
- 「最近使用」虚拟分组与使用次数统计，存放在 `usage.json`（与 `config.json` 分开，
  避免每次启动都触发配置热更新）。
- 项目与分组支持拖动排序和跨分组移动（`MoveItem` / `MoveGroup`），落盘顺序即显示顺序。
- 开始菜单导入：扫描用户与全局两处「开始菜单」的 `.lnk`，可搜索、按范围筛选、多选导入，
  自动跳过重复条目。
- 删除项目或分组后 5 秒内可撤销（`Ctrl+Z`）；分组删除前显示包含的项目数。
- 单击还是双击启动可在设置里切换（`settings.click_mode`，默认双击）。
- 管理员启动（`item.admin`）：通过 UAC 提权打开目标，取消提权不算失败也不计入最近使用。

### 配置与数据安全

#### 新增

- `config.json` 每次保存前轮转为 `config.json.bak`；设置页提供导出/导入（导入前校验 JSON）。
- 启动时把日志写入 `<baseDir>/logs/app.log`（超过 2 MiB 轮转为 `.1`），设置页与托盘都能打开日志。
- 异常退出检测：`logs/.running` 标记在正常退出时删除，下次启动若发现残留则提示「上次没有正常退出」。
- 图标缓存按目标路径哈希命名，命中即复用；启动时补齐缺失图标，并清理无人引用的孤儿 PNG。
- 失效目标检测：目标文件不存在的条目在网格里显示警示点。
- 同一份 `config.json` 只允许一个实例；单实例名由数据目录哈希派生，因此便携副本可以与安装版并存。

#### 修复

- 保存设置时先注册快捷键、成功后再落盘：注册失败（被别的程序占用）不再写入 `config.json`，
  设置页保持打开并说明「仍在使用旧快捷键」。
- `config.json` 被外部修改后立刻在界面里操作，不再用内存快照覆盖外部改动。
- 一次读失败不再永久锁死写入：瞬时占用会在下次写入前重试自愈。
- 低级鼠标钩子被系统摘掉后由看门狗重建；`SetSwallowInput` 可关闭「吞掉鼠标按键」。
- 单实例退出不再可能挂死：唤醒消息带超时，互斥体句柄释放。
- 删除/改名 `config.json` 后界面显示可操作横幅（重建文件、打开数据目录），不再静默。
- 鼠标呼出取值非法时回退到 `none`，并在界面提示原始值。
- 自定义图标接口只接受本地绝对路径，拒绝 UNC 与设备路径。

### 外观、设置与无障碍

#### 新增

- 主题 `dark`/`light`/`system`、窗口不透明度、字号缩放、窗口尺寸与位置记忆；
  无保存位置时面板出现在鼠标所在显示器中央。
- 开机自启（HKCU Run）与静默启动（`settings.auto_start` / `settings.start_hidden`）。
- 语言切换 `zh`/`en`：前端文案通过 `frontend/src/i18n.ts` 的 `t()` 与各组件
  `registerTranslations` 提供英文，缺失翻译回退中文。
- 设置页新增「关于/更新」：版本、数据目录、日志路径、检查更新（只提示并打开下载页）、
  清理图标缓存。
- 弹窗打开时通知后端（`App.SetModalOpen`），避免切走窗口时丢掉正在编辑的内容。
- 严格失焦隐藏（`settings.hide_on_blur_strict`）：光标停在面板上也随失焦隐藏。
- 分组折叠状态持久化（`settings.collapsed_groups`），侧栏可逐组折叠。

#### 修复

- 弹窗具备 `role="dialog"`/`aria-modal`/焦点捕获与关闭后焦点归位；Toast 容器 `aria-live`；
  纯图标按钮补 `aria-label`；错误与路径文本可选中复制。
- 右键菜单在滚动与窗口大小变化时关闭。
- 编辑弹窗在 150% 缩放与小窗口下不再溢出，页脚按钮始终可点。
- 拖入非文件内容不再显示「松开即可添加」遮罩。
- 浏览器 mock 增加延迟、假路径与 `?hotkeyFail=1` / `?missing=1` 失败开关，顶栏显示 `mock` 角标。

#### 变更

- `config.json` 新增字段（均有默认值，旧文件无需迁移）：`mouse_swallow`、`click_mode`、
  `auto_start`、`start_hidden`、`theme`、`language`、`show_recent`、`collapsed_groups`、
  `window_opacity`、`window_width/height`、`window_x/y`、`font_scale`、`hide_on_blur_strict`；
  `Item` 新增 `admin`。`frontend/src/types/config.ts` 与 `bridge.ts` 已同步。
- 托盘图标改为与窗口、可执行文件一致的刀形标（内嵌 32×32 ICO），托盘菜单新增
  「打开数据目录」「查看日志」。
- `JEEDESK_BASE_DIR` 可覆盖数据目录，`JEEDESK_DEV=1` 显式声明开发运行。

### 配置管理

#### 修复

- 磁盘上的 `config.json` 无法读取或解析期间，`config.Manager.Update` 返回
  `ErrWriteBlocked` 且不再落盘。此前应用会用内存里的配置（启动时即默认配置）覆盖原文件；
  现在需先修复文件，监听器或手动重载成功后自动恢复可写。
- 空白的 `config.json`（只有空白或 BOM）视为不存在并写入默认配置，不再报解析错误。

#### 变更

- 配置错误弹窗改为如实说明：修复之前 JeeDesk 不会写入该文件，原内容保持不变。

### 快捷键与鼠标呼出

#### 修复

- 切换快捷键失败后，改回仍在生效的快捷键时设置页立即显示已注册，不再需要手动重载。
  `app.go` 的 `applyHotkey` 与 `syncHooks` 改为以 `hook.HotkeyManager.ActiveSpec()`
  （钩子线程实际持有的快捷键）为准。
- 同一个快捷键注册失败不再反复提示：保存设置后由文件监听器回声触发的重试不再弹提示。
  用户在应用内的操作（保存设置、手动重载）失败时仍会提示。

#### 变更

- 手动重载配置会重试尚未生效的快捷键。

### 分组与项目

#### 修复

- 新建分组后新分组被选中并保持选中。此前配置更新到达时，前端会把选择改回原来的分组，
  拖入的文件会落到用户没有看到的分组里。
- 添加或编辑项目时保存失败，提交按钮恢复可用并保留已填内容，可以直接重试。
- 把文件或文件夹拖进窗口会添加到当前分组。此前根元素没有挂 `--wails-drop-target`，
  Wails 的拖放管线不接管，WebView2 按默认行为把拖入项当作网址导航，弹出预览窗口。

#### 变更

- Wails 绑定方法 `App.AddGroup` 的返回值由 `error` 变为 `(新分组 ID, error)`；
  `frontend/src/api/bridge.ts` 已同步。

### 设置与界面

#### 新增

- 内置默认图标（vscode-icons，MIT）：`.bat`/`.cmd`/`.ps1` 条目直接显示对应类型图标，不再依赖
  外壳提取出的通用图标；图标提取失败或未设置时，按目标是否为文件夹回退到内置文件/文件夹图标。
  用户自选图标始终优先。素材在 `frontend/public/defaults/`。

#### 修复

- 设置页的「录制」按钮点击后可以捕获按键：点击不再把焦点移出输入框，开启录制时聚焦输入框。
- 无边框窗口在 Windows 11 使用系统圆角（DWM `DWMWA_WINDOW_CORNER_PREFERENCE`），去掉面板
  圆角外的 2px 缝隙、面板圆角改为与系统 8px 一致：Acrylic 底不再从四角露出来。

### 启动与运行目录

#### 修复

- `wails dev` 下运行目录是项目根，与 `main.go` 的注释一致；`config.json` 与 `icons/`
  不再生成在 `build/bin/`。可执行文件位于 `<工作目录>/build/bin` 时视为开发环境。
- 当前工作目录里的 `config.json` 只有属于 JeeDesk（或无法解析）时才被采用。属于其他程序的
  同名文件不再被当成自己的配置，也就不会被覆盖。
- `app.go` 中 `ctx` 的访问加了锁，`cfgMgr` 改在任何其他 goroutine 出现之前创建，
  消除了启动期间再次启动实例时的数据竞争。

### 启动器

#### 修复

- PATH 中没有 PowerShell 时，`.ps1` 项目改用 `%SystemRoot%` 下的完整路径启动
  （`internal/launcher` 的 `findPowerShell`）。

### 工程与仓库

#### 新增

- 根目录 `README.md` 与 `docs/geekdesk-migration.md`（GeekDesk 迁移过程与字段映射的完整记录）。
- 根目录 `AGENTS.md` 作为项目约定入口。
- 应用图标换成红宝石 M9 多普勒，替换掉 Wails 模板的 `W`：`build/appicon.png` 与
  `build/windows/icon.ico`（16/24/32/48/64/128/256 七档）；1024 图与原始素材放 `assets/`。
  成品由 `assets/source/Gemini_Generated_Image_kks1ackks1ackks1.jpg` 的绿幕原图抠出，
  透明底，刀身圆孔与护手环孔保留为透明，任意底色与深浅主题下都适用。
- 顶栏左上角的品牌角标从蓝色四宫格换成同一把刀的缩略图（`frontend/public/brand-m9.png`），
  与可执行文件图标一致。
- `README.md` 改写为概览式项目介绍：徽章、特性表、使用步骤、构建与验证、目录结构与已知限制，
  配置、图标机制与 GeekDesk 迁移说明保留，并补致谢。

#### 变更

- `.agents/`、`_scratch/` 与 `wails build` 生成的 `frontend/package.json.md5`
  不纳入版本管理。
- 图标有两个来源：应用图标以 `build/appicon.png`（源在 `assets/`）为准；`go run build/genicon.go`
  仍会用 `internal/tray` 的内置托盘图标覆盖 `build/windows/icon.ico`。
