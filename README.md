# JeeDesk

JeeDesk 是 Windows 桌面快捷启动器:无边框置顶窗口、系统托盘、全局快捷键与鼠标按键呼出、
拖拽添加项目、`config.json` 热更新。技术栈为 Wails v2 + Go + Vue 3 + TypeScript + Tailwind CSS,
仅面向 Windows(Webview2)。

| 目录 | 职责 |
| --- | --- |
| `main.go` | Wails 装配、运行目录判定(`resolveBaseDir`)、图标资源中间件 |
| `app.go` | Wails 绑定方法、窗口显隐、配置与钩子状态 |
| `internal/config` | `config.json` 结构、原子读写与热更新监听 |
| `internal/hook` | 全局快捷键与鼠标触发钩子 |
| `internal/launcher` | 启动目标、打开文件与所在位置(ShellExecute) |
| `internal/resolver` | 拖入路径与 `.lnk` 解析、图标提取与缓存 |
| `internal/tray` | 系统托盘与内置图标 |
| `frontend/src` | Vue 界面;`api/bridge.ts` 自行声明后端接口并内置浏览器开发用 mock |

## 构建与验证

需要 Go 1.27、Node.js/npm、Wails CLI v2。`wails` 不在 PATH 时位于 `%USERPROFILE%\go\bin`。

| 目的 | 命令 |
| --- | --- |
| 格式 | `gofmt -l .`(无输出为通过) |
| 静态检查 | `go vet ./...` |
| 行为测试 | `go test ./... -count=1` |
| 竞态检测 | `CGO_ENABLED=1 go test -race ./... -count=1` |
| 前端类型检查 | `cd frontend && npm run typecheck` |
| 完整构建 | `wails build`(产物 `build/bin/JeeDesk.exe`) |
| 开发运行 | `wails dev`(前端也可 `cd frontend && npm run dev` 使用 mock 独立预览) |

全新克隆需先构建前端(`wails build` 或前端构建):根包以 `//go:embed all:frontend/dist`
嵌入前端产物,`frontend/dist/` 不入库。测试会短暂注册全局快捷键并提取系统图标,
需要 Windows 桌面会话。

## 配置与运行数据

- `config.json`:设置 + 分组结构。字段见 `internal/config/schema.go`
  (`settings`:hotkey / mouse_trigger / hide_on_launch / hide_on_blur / hide_on_esc;
  `groups[]`:id、name、items[](id、name、path、args、work_dir、icon))。
- `icons/`:图标磁盘缓存(见下节)。两者均被 `.gitignore` 忽略。
- 运行目录由 `resolveBaseDir` 决定:本仓库内开发时(`go run`、`wails dev`)是当前工作目录;
  其他情况下当前工作目录已有属于 JeeDesk 的 `config.json` 用它,否则用可执行文件所在目录。
  **把 `JeeDesk.exe` 与 `config.json` 放任意目录(双击或快捷方式启动)即可携带数据**;
  磁盘上的配置无法读取或解析期间,应用不会写入它,并在界面提示。

### 图标机制:为什么 exe 旁边有个 `icons/` 文件夹?

界面渲染在 WebView2 里,前端 `<img>` 只能加载 HTTP 资源,不能直接调 Win32 API 取图标;
而图标提取(`SHGetFileInfoW` / `PrivateExtractIconsW` + HICON 转位图)按次计费几十毫秒且列表会
反复渲染同一条目。因此 JeeDesk 的策略是**按需提取、落盘缓存、每次渲染读静态文件**:

```text
条目保存/渲染 → icon 字段
  ├─ icon 指向 icons/<文件>          → /icons/<文件> 直接返回缓存(icon middleware)
  ├─ icon 指向其他路径、icon=path    → /custom-icon?path=…
  │     ExtractAndCacheIcon(path, iconsDir)
  │       ├─ icons/<IconHash(path)>.png 已存在 → 直接返回(重复渲染不再提取)
  │       └─ 不存在 → Win32 提取 → 写 icons/<hash>.png(原子写)→ 返回
  └─ icon 为空或提取失败             → 内置默认图标(见下)
```

所以 `icons/` 不是"没取到图标才生成"的副产品,它是唯一能承接"首次提取一次、之后零成本"
语义的落地点——直接每次渲染都调 Win32 提取会拖慢列表,把图嵌进 `config.json` 又会让配置
文件膨胀且难以人工编辑。缓存按目标路径哈希命名,路径改了自然重新提取。

内置默认图标(`frontend/public/defaults/`,vscode-icons 项目,MIT):

| 条目 | 图标 | 来源 |
| --- | --- | --- |
| `.bat` / `.cmd`(icon 为空或等于目标路径) | `file_type_bat.svg` | vscode-icons |
| `.ps1` | `file_type_powershell.svg` | vscode-icons |
| 提取失败或未设置图标的目标 | `default_file.svg` | vscode-icons |
| 同上的文件夹目标 | `default_folder.svg` | vscode-icons |

用户在编辑弹窗里自选过的图标(`icons/…` 形式)始终优先于上述类型默认值。

## 从 GeekDesk 迁移

GeekDesk 2.5.14 把数据(分组、条目、设置)整体序列化在安装目录的无扩展名 `Data` 文件里,
格式是 .NET `BinaryFormatter`(无法用文本工具直接读)。完整迁移过程、对象结构与字段映射
见 [docs/geekdesk-migration.md](docs/geekdesk-migration.md),流程概要:

1. **导出**:用小 C# 辅助程序引用 GeekDesk.exe 及 `lib/` 依赖做强类型反序列化,只反射实例字段
   (绕开其属性 getter 的"读取即整包回写盘"副作用),导出为一整份 JSON;
2. **映射**:`menuList[] → groups[]`(id 沿用原 `menuId`),`iconList[] → items[]`
   (`name/path/startArg → name/path/args`,条目 id 用 `sha1(menuId|name|path)` 保持稳定);
   `hotkeyStr/mouseMiddleShow → settings`。图标数据内是统一占位图(逐条目图标不存在),
   故 `icon = path` 交给 JeeDesk 运行时提取;
3. **校验**:转换产物用 JeeDesk 的 `internal/config`(Load + Normalize)实际加载核对
   组数/条目数/空值;
4. **部署**:exe + `config.json` 拷入独立目录即用;迁移后注意退出 GeekDesk(两者热键同为
   Alt+G)并关闭 GeekDesk 自身的开机自启。

迁移工具与记录保留在本地 `_scratch/_done_20261125-geekdesk-migration/`(按项目约定不入库)。

## 许可

- 代码:项目自有(暂未配置 LICENSE 文件)。
- `frontend/public/defaults/*.svg` 四枚图标取自 [vscode-icons](https://github.com/vscode-icons/vscode-icons)
  (MIT,提交 `4cf5fa03`),许可原文随目录附于 `frontend/public/defaults/LICENSE.txt`。
