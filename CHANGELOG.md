# Changelog

面向开发与维护者，记录行为、接口、配置与维护方式的变化。目前尚无正式版本，
以下为「开发中」的累计变化。

## 开发中

### 搜索、键盘与最近使用

#### 新增

- 跨分组搜索：输入即搜（120ms 防抖），结果标注所属分组；排序为「前缀 > 词首 > 子串」，
  同分时使用次数多的优先。评分逻辑在 `internal/search`。
- 键盘操作：呼出后搜索框自动聚焦，支持 `↑/↓`、`Enter` 启动、`Ctrl+Z` 撤销删除、`Ctrl+F`
  聚焦搜索、`Ctrl+,` 打开设置、`Alt+←/→` 或 `Ctrl+Tab` 切组、`Alt+数字` 直接启动、
  直接打字即搜索；`Esc` 隐藏。搜索框为空时 `Home/End` 跳到首尾、`Delete` 删除选中项；
  框内有内容时这些键仍然编辑文本。`Tab` 留给焦点移动，不被全局快捷键占用。
- 「最近使用」虚拟分组与使用次数统计，存放在 `usage.json`（与 `config.json` 分开，
  避免每次启动都触发配置热更新）。
- 分组支持拖动排序与跨分组移动（`MoveGroup`），落盘顺序即显示顺序；项目的排序走右键菜单的
  上移/下移（`MoveItem`），网格内的拖动排序尚未接线，见 `TODO.md`。
- 左侧分组栏支持长按拖动排序：按住约 0.3 秒后该行放大浮起（阴影 + 描边），跟手移动时
  其它行按 40px 槽位让位、形成落点空隙，松手调用 `MoveGroup` 落盘；短按仍是切换分组，
  靠近上下边缘自动滚动，折叠按钮不触发拖动。
- 开始菜单导入：扫描用户与全局两处「开始菜单」的 `.lnk`，可搜索、按范围筛选、多选导入，
  自动跳过重复条目。
- 删除项目或分组后可用 `Ctrl+Z` 撤销；撤销快照保留到下一次删除或程序退出（不设时限），
  分组删除前显示包含的项目数。
- 单击还是双击启动可在设置里切换（`settings.click_mode`，默认双击）。
- 管理员启动（`item.admin`）：通过 UAC 提权打开目标，取消提权不算失败也不计入最近使用。

#### 修复

- 删除分组后 `Ctrl+Z` 能真正恢复整组。前端不再从「撤销删除分组「x」」这类中文文案里猜类型，
  改用新增的 `App.PendingUndoKind`；此前判断恒为假，分组删除一律走条目撤销并报
  「没有可撤销的删除」，整组只能从 `config.json.bak` 手工恢复。
- 搜索框聚焦时键盘导航恢复可用：`↑/↓`、`Alt+数字`、`Ctrl+,`、`Ctrl+Tab` 不再被
  「输入框内不接管全局键」的守卫吞掉；框内为空时 `Home/End`、`Delete`、`Ctrl+Z` 也按导航处理，
  有内容时它们仍然编辑文本。此前呼出后搜索框必定获得焦点，这些键全部静默无效。

### 配置与数据安全

#### 新增

- `config.json` 每次保存前轮转为 `config.json.bak`；设置页提供导出/导入（导入前校验 JSON）。
- 启动时把日志写入 `<baseDir>/logs/app.log`（超过 2 MiB 轮转为 `.1`），设置页与托盘都能打开日志。
- 异常退出检测：`logs/.running` 标记在正常退出时删除，下次启动若发现残留则提示「上次没有正常退出」。
- 图标缓存按目标路径哈希命名，命中即复用；启动时补齐缺失图标，并清理无人引用的孤儿 PNG。
- 图标提取遍历 `PrivateExtractIcons` → `ExtractIconEx` → `SHGetFileInfo` 三级回退，并拒绝全透明
  的占位图；`.bat`/`.lnk`/`.html`/文件夹等非 exe 目标也走同一套提取，不再因扩展名不是图片就
  退回内置图标。
- 图标缓存带版本号（`icons/.version`），提取逻辑变化后启动时一次性丢弃旧缓存，避免旧的空白
  PNG 一直被命中。
- `/custom-icon` 的允许目录包含每个条目自身所在文件夹，D 盘、其它盘上的项目图标照常显示。
- 失效目标检测：目标文件不存在的条目在网格里显示警示点；右键可直接「更新路径…」
  （`App.RelocateItem`，弹出文件选择框并同步路径与派生图标）。
- 同一份 `config.json` 只允许一个实例；单实例名由数据目录哈希派生，因此便携副本可以与安装版并存。
  旧版本使用的固定互斥体名仍会检查，升级后不会与旧进程同时运行。

#### 修复

- 保存设置时先注册快捷键、成功后再落盘：注册失败（被别的程序占用）不再写入 `config.json`，
  设置页保持打开并说明「仍在使用旧快捷键」。
- `config.json` 被外部修改后立刻在界面里操作，不再用内存快照覆盖外部改动。
- 一次读失败不再永久锁死写入：瞬时占用会在下次写入前重试自愈。
- 低级鼠标钩子被系统摘掉后由看门狗重建；`SetSwallowInput` 可关闭「吞掉鼠标按键」。
- 单实例退出不再可能挂死：唤醒消息带超时，互斥体句柄释放。
- `start_hidden` 与 `--silent` 启动时面板不再闪过：Wails 在 `OnDomReady` 之后才显示 WebView，
  现在 DOM 就绪后显式隐藏一次，用户主动打开时不会再被隐藏。
- 删除/改名 `config.json` 后界面显示可操作横幅（重建文件、打开数据目录），不再静默。
- 鼠标呼出取值非法时回退到 `none`，并在界面提示原始值。
- 自定义图标接口只接受本地绝对路径，拒绝 UNC 与设备路径；并且只从数据目录与当前用户
  配置目录读取，`config.json` 无法让它读取 `%WINDIR%` 或别人的目录。
- 管理员项目只启动一次：此前 `LaunchItem` 先按普通进程启动再提权，会留下一个未提权的副本。
- 鼠标钩子的看门狗不再自我阻塞：以前重建时会在持锁状态下等待自己退出，每次重建卡 2 秒；
  同时死钩判定改用取模比较，开机超过 49.7 天后不再失效（补 `inputAgeMillis` 单测）。
- 配置监听在 `Close()` 之后不会再被重启唤醒，`Close()` 也不再无限等待循环退出。
- 面板重新显示时会重新应用鼠标触发，Windows 在面板隐藏期间摘掉的钩子能自愈；
  隐藏时补发 `window:hidden` 事件。
- 图标缓存按目标修改时间校验，命中缓存但目标已被替换时重新提取，不再显示旧图标。
- 数据目录不可写时回退到 `%LOCALAPPDATA%\JeeDesk`，避免启动即失败。
- 「清理图标缓存」在配置读取失败或没有任何分组时跳过，不会清掉仍被引用的图标。
- 启动时检测到上次异常退出、单实例唤醒、日志写入都在主实例确认后执行，副实例不再写日志或默认配置。

### 外观、设置与无障碍

#### 新增

- 主题 `dark`/`light`/`system`、窗口不透明度、字号缩放、窗口尺寸与位置记忆；
  无保存位置时面板出现在鼠标所在显示器中央。
- 开机自启（HKCU Run）与静默启动（`settings.auto_start` / `settings.start_hidden`）。
- 语言切换 `zh`/`en`：前端文案通过 `frontend/src/i18n.ts` 的 `t()` 与各组件
  `registerTranslations` 提供英文，缺失翻译回退中文。
- 设置页新增「关于/更新」：版本、数据目录、日志路径、检查更新（只提示并打开下载页）、
  清理图标缓存、补齐缺失图标。
- 弹窗打开时通知后端（`App.SetModalOpen`），避免切走窗口时丢掉正在编辑的内容。
- 严格失焦隐藏（`settings.hide_on_blur_strict`）：光标停在面板上也随失焦隐藏。
- 分组折叠状态持久化（`settings.collapsed_groups`），侧栏可逐组折叠。
- 删除分组、清理图标等不可逆操作统一走 `ConfirmDialog`，不再使用浏览器 `confirm`。
- 设置页新增「补齐缺失图标」，与「清理图标缓存」并列。
- 顶栏品牌角标与 `JeeDesk.exe` 图标同源（`build/appicon.png` 系列）。

#### 修复

- 面板重新显示时清空搜索框并回到未过滤的网格；新增、拖入或导入条目会退出搜索态，
  避免新条目被过滤条件藏起来。
- `config:updated` 只有在条目图标真的变化时才刷新图标 URL（`App.vue` 的 `iconSignature`）。
- 组件卸载时调用 Wails 的 `OnFileDropOff()`，不再把拖放监听留在运行时里。
- 弹窗具备 `role="dialog"`/`aria-modal`/焦点捕获与关闭后焦点归位；Toast 容器 `aria-live`；
  纯图标按钮补 `aria-label`；错误与路径文本可选中复制。
- 右键菜单在滚动与窗口大小变化时关闭。
- 编辑弹窗在 150% 缩放与小窗口下不再溢出，页脚按钮始终可点。
- 拖入非文件内容不再显示「松开即可添加」遮罩。
- 浏览器 mock 增加延迟、假路径与 `?hotkeyFail=1` / `?missing=1` 失败开关，顶栏显示 `mock` 角标。

#### 变更

- `config.json` 新增字段（均有默认值，旧文件无需迁移）：`mouse_swallow`、`click_mode`、
  `auto_start`、`start_hidden`、`theme`、`language`、`show_recent`、`collapsed_groups`、
  `window_opacity`、`window_width/height`、`window_x/y`、`window_positioned`、
  `font_scale`、`hide_on_blur_strict`；
  `Item` 新增 `admin`。`frontend/src/types/config.ts` 与 `bridge.ts` 已同步。
- 托盘图标改为与窗口、可执行文件一致的刀形标（内嵌 32×32 ICO），托盘菜单新增
  「打开数据目录」「查看日志」。
- `JEEDESK_BASE_DIR` 可覆盖数据目录，`JEEDESK_DEV=1` 显式声明开发运行；`wails dev` 通过
  Wails 设置的 `assetdir` 变量被直接识别，不再只靠二进制路径位于 `%TEMP%` 猜测；`--silent`
  启动时不显示面板（供开机自启使用）。
- 新增绑定方法 `App.OpenURL`（只允许 http/https）、`App.SetItemAdmin`、
  `App.CheckUpdates`、`App.EnsureIcons`、`App.SetModalOpen`、`App.RelocateItem`；
  `frontend/src/api/bridge.ts` 的 `WailsGoApp` 与 `frontend/src/types/config.ts` 已同步。
- 设置保存不再接受前端传来的窗口几何：窗口位置由 `App.WindowPositioned` 在后端记录，
  前端无法覆盖内存中的几何值。
- 窗口几何变化不再触发 `config.json` 备份轮转，仅内容类修改会备份。
- WebView2 用户数据目录固定到 `<baseDir>/webview`，便携副本不会共用同一个缓存目录。
- 托盘图标改为刀形标后，`internal/tray` 的内置图标与 `build/appicon` 系列保持一致。

### 配置管理

#### 修复

- 磁盘上的 `config.json` 无法读取或解析期间，`config.Manager.Update` 返回
  `ErrWriteBlocked` 且不再落盘。此前应用会用内存里的配置（启动时即默认配置）覆盖原文件；
  现在需先修复文件，监听器或手动重载成功后自动恢复可写。
- 「重建 `config.json`」与「导入配置」不再被上面的写入锁拦住：两者走新增的
  `config.Manager.Replace`，是用户明确要求替换该文件的入口。此前文件一旦损坏，
  应用变成只读，连界面提供的恢复按钮也失败。损坏的原文件仍会留存为 `config.json.bak`。
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

- 手动重载配置会重试尚未生效的快捷键，并且强制重新加载（`ReloadForce`），
  即使文件内容与内存一致也会重新注册钩子。

### 分组与项目

#### 修复

- 新建分组后新分组被选中并保持选中。此前配置更新到达时，前端会把选择改回原来的分组，
  拖入的文件会落到用户没有看到的分组里。
- 添加或编辑项目时保存失败，提交按钮恢复可用并保留已填内容，可以直接重试。
- 把文件或文件夹拖进窗口会添加到当前分组。此前根元素没有挂 `--wails-drop-target`，
  Wails 的拖放管线不接管，WebView2 按默认行为把拖入项当作网址导航，弹出预览窗口。

#### 变更

- 新增绑定方法 `App.PendingUndoKind`（返回 `item`、`group` 或空串），供 `Ctrl+Z` 选择恢复目标；
  `frontend/src/api/bridge.ts` 与 `frontend/wailsjs` 已同步。
- Wails 绑定方法 `App.AddGroup` 的返回值由 `error` 变为 `(新分组 ID, error)`；
  `frontend/src/api/bridge.ts` 已同步。

### 设置与界面

#### 新增

- 内置默认图标（vscode-icons，MIT）：`.bat`/`.cmd`/`.ps1` 条目直接显示对应类型图标，不再依赖
  外壳提取出的通用图标；图标提取失败或未设置时，按目标是否为文件夹回退到内置文件/文件夹图标。
  用户自选图标始终优先。素材在 `frontend/public/defaults/`。

#### 修复

- 设置页的「录制」按钮点击后可以捕获按键：点击不再把焦点移出输入框，开启录制时聚焦输入框。
- `config.json` 被外部修改后不再误报「文件已删除」：监听器在 150ms 后确认目标仍未出现才提示。
- 保存设置时若快捷键注册失败，界面保持打开的设置页并显示失败项，可以改完直接重试。
- 无边框窗口在 Windows 11 使用系统圆角（DWM `DWMWA_WINDOW_CORNER_PREFERENCE`），去掉面板
  圆角外的 2px 缝隙、面板圆角改为与系统 8px 一致：Acrylic 底不再从四角露出来。

### 启动与运行目录

#### 修复

- 窗口尺寸按 DIP 存取。此前把 `GetWindowRect` 的物理像素写进 `window_width/height`，
  启动时又被当成 DIP 传给 Wails，缩放 150% 的屏幕上每重启一次面板就放大 1.5 倍。
  换算在 `flushWindowState` 里用 `GetDpiForWindow` 完成。
- `wails dev` 下运行目录是项目根，与 `main.go` 的注释一致；`config.json` 与 `icons/`
  不再生成在 `build/bin/`。可执行文件位于 `<工作目录>/build/bin` 时视为开发环境。
- 当前工作目录里的 `config.json` 只有属于 JeeDesk（或无法解析）时才被采用。属于其他程序的
  同名文件不再被当成自己的配置，也就不会被覆盖。
- `app.go` 中 `ctx` 的访问加了锁，`cfgMgr` 改在任何其他 goroutine 出现之前创建，
  消除了启动期间再次启动实例时的数据竞争。
- 单实例判定提前到读取配置之前：第二个实例不再使用默认配置覆盖磁盘上的 `config.json`，
  也不会因为日志初始化失败而留下半初始化的状态。
- `ctx` 尚未就绪时收到的唤醒请求会记为 `pendingShow`，就绪后补一次显示，不再丢失。
- 退出时停止图标补齐定时器，避免退出过程中还去写缓存目录。

### 启动器

#### 修复

- PATH 中没有 PowerShell 时，`.ps1` 项目改用 `%SystemRoot%` 下的完整路径启动
  （`internal/launcher` 的 `findPowerShell`）。
- 自定义协议链接（`obsidian://`、`vscode://` 等）恢复按系统协议打开：URL 判定改为
  「合法 scheme 且不是盘符」，不再限制在白名单内。
- 开始菜单导入时指定了不存在的分组会返回错误，而不是把条目丢进第一个分组。

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
- 内存中的窗口几何（`window_x/y`、`window_positioned`）不再由前端回写；
  需要记录位置时调用 `App.WindowPositioned`。
