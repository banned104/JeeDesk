# JeeDesk

<p align="center">
  <img src="assets/appicon.png" width="132" alt="JeeDesk 图标">
</p>

<p align="center">
  <b>一个按键，把桌面应用摊在眼前。</b><br>
  Windows 快捷启动面板 · 无边框置顶 · 便携运行
</p>

<p align="center">
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white" alt="Go 1.27"></a>
  <a href="https://wails.io"><img src="https://img.shields.io/badge/Wails-v2-E4002B" alt="Wails v2"></a>
  <a href="https://vuejs.org"><img src="https://img.shields.io/badge/Vue-3-42B883?logo=vuedotjs&logoColor=white" alt="Vue 3"></a>
  <img src="https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white" alt="TypeScript 5">
  <img src="https://img.shields.io/badge/Tailwind-3-06B6D4?logo=tailwindcss&logoColor=white" alt="Tailwind CSS 3">
  <img src="https://img.shields.io/badge/Windows-10%20%7C%2011-0078D6?logo=windows11&logoColor=white" alt="Windows 10 / 11">
</p>

> 🥚 感谢 **欧公子** 与 **MistralAI** 送来的赛博鸡蛋。

## 它是什么

JeeDesk 是一个常驻托盘的小启动面板：按一下全局快捷键（默认 `Alt+G`），无边框的窗口贴着屏幕弹出，
按分组排列的应用图标一览无余，点一下就启动，失焦或 `Esc` 自动收起。

配置只有一个 `config.json`，图标缓存在 `icons/`：把 `JeeDesk.exe` 和这几个文件放进任意目录，
数据就跟着走。没有安装包、没有注册表、没有后台服务。

## 特性

| | 能力 | 说明 |
| --- | --- | --- |
| ⚡ | 一键呼出 | 全局快捷键注册在独立的钩子线程，可随时更换；默认 `Alt+G` |
| 🖱️ | 鼠标触发 | 可选鼠标中键或侧键呼出，默认关闭 |
| 🗂️ | 分组 | 分组切换即时生效，新建分组后直接落到新分组 |
| 🎯 | 拖拽添加 | 把文件或文件夹拖进窗口即加入当前分组，`.lnk` 自动解析 |
| 🖼️ | 图标提取与缓存 | 首次从目标提取，之后读磁盘缓存，列表滚动不再重复调用 Win32 |
| 🧩 | 内置类型图标 | `.bat` / `.cmd` / `.ps1` 与文件夹用内置图标兜底 |
| 🔁 | 配置热更新 | 外部改动 `config.json` 会被监听并立即生效，无需重启 |
| 📦 | 便携 | 运行数据就在 exe 旁边，整个目录拷走即可 |
| 🧊 | 系统圆角 | Windows 11 下使用 DWM 原生圆角，Acrylic 底不会从四角漏出 |
| 🧷 | 单实例 | 重复启动只唤醒已运行的窗口，不会开出第二个面板 |
| 🛡️ | 配置保护 | 磁盘上的配置读不出来时不写入、只提示，原文件保持原样 |
| 🚚 | GeekDesk 迁移 | 提供完整的迁移路径与字段映射，见 [docs/geekdesk-migration.md](docs/geekdesk-migration.md) |

## 使用

1. **呼出**：按 `Alt+G`（或设置里选的鼠标键），面板出现在当前屏幕。
2. **添加**：点「添加」手动填写，或把目标拖进窗口。
3. **启动**：单击图标。按设置可在启动后隐藏面板。
4. **收起**：失焦、`Esc` 或再次触发快捷键。
5. **设置**：快捷键、鼠标触发、隐藏行为都在设置页；保存后写入 `config.json`。

## 从源码构建

需要 Go 1.27、Node.js/npm 与 Wails CLI v2（`wails` 不在 PATH 时位于 `%USERPROFILE%\go\bin`）。

| 目的 | 命令 |
| --- | --- |
| 格式 | `gofmt -l .`（无输出为通过） |
| 静态检查 | `go vet ./...` |
| 行为测试 | `go test ./... -count=1` |
| 竞态检测 | `CGO_ENABLED=1 go test -race ./... -count=1` |
| 前端类型检查 | `cd frontend && npm run typecheck` |
| 完整构建 | `wails build`（产物 `build/bin/JeeDesk.exe`） |
| 开发运行 | `wails dev`（前端也可 `cd frontend && npm run dev` 用 mock 独立预览） |

全新克隆需先构建前端（`wails build` 或前端构建）：根包以 `//go:embed all:frontend/dist`
嵌入前端产物，而 `frontend/dist/` 不入库。测试会短暂注册全局快捷键、安装鼠标钩子并提取系统图标，
需要 Windows 桌面会话。

目前没有正式发布、Tag 或 CI；构建产物即 `build/bin/JeeDesk.exe`。

## 配置与运行数据

- `config.json`：设置 + 分组，字段以 `internal/config/schema.go` 为准。`settings` 包含
  呼出与隐藏（`hotkey`、`mouse_trigger`、`mouse_swallow`、`hide_on_launch`、`hide_on_blur`、
  `hide_on_blur_strict`、`hide_on_esc`）、启动（`auto_start`、`start_hidden`）、外观
  （`theme`、`language`、`font_scale`、`window_opacity`、`window_width/height`、
  `window_x/y`、`window_positioned`）、交互（`click_mode`、`show_recent`、`collapsed_groups`）；
  `groups[]` 是 `id`、`name`、`items[]`，条目字段为 `id`、`name`、`path`、`args`、
  `work_dir`、`icon`、`admin`。
- 同目录的 `config.json.bak`（上一次保存的内容）、`icons/`（图标缓存）、`usage.json`
  （启动次数与最近使用）、`logs/app.log`（超过 2 MiB 轮转为 `.1`，`logs/.running` 是异常退出
  标记）、`webview/`（WebView2 用户数据目录）都由运行时生成，全部被 `.gitignore` 忽略。
  便携使用时把整个目录一起拷走即可。
- 运行目录由 `resolveBaseDir` 决定：本仓库内开发时（`go run`、`wails dev`）是当前工作目录；
  其他情况下，当前工作目录已有属于 JeeDesk 的 `config.json` 就用它，否则用可执行文件所在目录。
  **把 `JeeDesk.exe` 与 `config.json` 放任意目录（双击或快捷方式启动）即可携带数据**；
  磁盘上的配置无法读取或解析期间，应用不会写入它，并在界面提示。

### 图标机制：为什么 exe 旁边有个 `icons/` 文件夹？

界面渲染在 WebView2 里，前端 `<img>` 只能加载 HTTP 资源，不能直接调 Win32 API 取图标；
而图标提取（`SHGetFileInfoW` / `PrivateExtractIconsW` + HICON 转位图）按次计费几十毫秒且列表会
反复渲染同一条目。因此 JeeDesk 的策略是**按需提取、落盘缓存、每次渲染读静态文件**：

```text
条目保存/渲染 → icon 字段
  ├─ icon 指向 icons/<文件>          → /icons/<文件> 直接返回缓存(icon middleware)
  ├─ icon 指向其他路径、icon=path    → /custom-icon?path=…
  │     ExtractAndCacheIcon(path, iconsDir)
  │       ├─ icons/<IconHash(path)>.png 已存在 → 直接返回(重复渲染不再提取)
  │       └─ 不存在 → Win32 提取 → 写 icons/<hash>.png(原子写)→ 返回
  └─ icon 为空或提取失败             → 内置默认图标(见下)
```

所以 `icons/` 不是"没取到图标才生成"的副产品，它是唯一能承接"首次提取一次、之后零成本"
语义的落地点——直接每次渲染都调 Win32 提取会拖慢列表，把图嵌进 `config.json` 又会让配置
文件膨胀且难以人工编辑。缓存按目标路径哈希命名，路径改了自然重新提取。

内置默认图标（`frontend/public/defaults/`，vscode-icons 项目，MIT）：

| 条目 | 图标 | 来源 |
| --- | --- | --- |
| `.bat` / `.cmd`（icon 为空或等于目标路径） | `file_type_bat.svg` | vscode-icons |
| `.ps1` | `file_type_powershell.svg` | vscode-icons |
| 提取失败或未设置图标的目标 | `default_file.svg` | vscode-icons |
| 同上的文件夹目标 | `default_folder.svg` | vscode-icons |

用户在编辑弹窗里自选过的图标（`icons/…` 形式）始终优先于上述类型默认值。

## 从 GeekDesk 迁移

GeekDesk 2.5.14 把数据（分组、条目、设置）整体序列化在安装目录的无扩展名 `Data` 文件里，
格式是 .NET `BinaryFormatter`（无法用文本工具直接读）。完整迁移过程、对象结构与字段映射
见 [docs/geekdesk-migration.md](docs/geekdesk-migration.md)，流程概要：

1. **导出**：用小 C# 辅助程序引用 GeekDesk.exe 及 `lib/` 依赖做强类型反序列化，只反射实例字段
   （绕开其属性 getter 的"读取即整包回写盘"副作用），导出为一整份 JSON；
2. **映射**：`menuList[] → groups[]`（id 沿用原 `menuId`），`iconList[] → items[]`
   （`name/path/startArg → name/path/args`，条目 id 用 `sha1(menuId|name|path)` 保持稳定）；
   `hotkeyStr/mouseMiddleShow → settings`。图标数据内是统一占位图（逐条目图标不存在），
   故 `icon = path` 交给 JeeDesk 运行时提取；
3. **校验**：转换产物用 JeeDesk 的 `internal/config`（Load + Normalize）实际加载核对
   组数/条目数/空值；
4. **部署**：exe + `config.json` 拷入独立目录即用；迁移后注意退出 GeekDesk（两者热键同为
   `Alt+G`）并关闭 GeekDesk 自身的开机自启。

## 目录结构

| 目录 | 职责 |
| --- | --- |
| `main.go` | Wails 装配、运行目录判定（`resolveBaseDir`）、图标资源中间件 |
| `app.go` | Wails 绑定方法、窗口显隐与失焦隐藏、配置与钩子状态 |
| `internal/config` | `config.json` 结构、原子读写与热更新监听 |
| `internal/hook` | 全局快捷键与鼠标触发钩子 |
| `internal/launcher` | 启动目标、打开文件与所在位置（ShellExecute） |
| `internal/resolver` | 拖入路径与 `.lnk` 解析、图标提取与缓存 |
| `internal/tray` | 系统托盘与内置图标 |
| `frontend/src` | Vue 界面；`api/bridge.ts` 自行声明后端接口并内置浏览器开发用 mock |
| `assets/` | 应用图标的源文件与原始素材 |

## 已知限制

- 仅 Windows：依赖 Win32 API 与 WebView2，没有跨平台计划。
- 没有正式版本、Tag 与自动发布；要用就自己构建。
- 测试会注册全局快捷键并操作外壳图标，需要真实的 Windows 桌面会话，不能跑在无桌面环境里。
- 前后端契约靠人工同步：改动 `App` 导出方法或 `config.json` 结构时，需同时更新
  `frontend/src/api/bridge.ts` 与 `frontend/src/types/config.ts`，类型检查发现不了这类不一致。

## 致谢

- 🥚 **欧公子**、**MistralAI**：赛博鸡蛋，本项目的启动燃料。
- [Wails](https://wails.io)：把 Go 和 WebView2 缝在一起的那层胶水。
- [vscode-icons](https://github.com/vscode-icons/vscode-icons)：内置文件类型图标（MIT）。
- GeekDesk：迁移路径的来源，也是这个项目最初的对照组。

## 许可

- 代码：项目自有（暂未配置 LICENSE 文件）。
- `frontend/public/defaults/*.svg` 四枚图标取自 vscode-icons（MIT，提交 `4cf5fa03`），
  许可原文随目录附于 `frontend/public/defaults/LICENSE.txt`。
- `assets/source/` 下的原始素材为项目自用参考图，不随代码许可分发。
