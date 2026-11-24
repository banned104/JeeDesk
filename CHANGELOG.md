# Changelog

面向开发与维护者，记录行为、接口、配置与维护方式的变化。目前尚无正式版本，
以下为「开发中」的累计变化。

## 开发中

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
  图标是抠掉参考图背景的透明底成品，刀身孔与护手环孔保留为透明，任意底色与深浅主题下都适用。
- `README.md` 改写为概览式项目介绍：徽章、特性表、使用步骤、构建与验证、目录结构与已知限制，
  配置、图标机制与 GeekDesk 迁移说明保留，并补致谢。

#### 变更

- `.agents/`、`_scratch/` 与 `wails build` 生成的 `frontend/package.json.md5`
  不纳入版本管理。
- 图标有两个来源：应用图标以 `build/appicon.png`（源在 `assets/`）为准；`go run build/genicon.go`
  仍会用 `internal/tray` 的内置托盘图标覆盖 `build/windows/icon.ico`。
