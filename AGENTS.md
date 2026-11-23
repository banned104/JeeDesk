# JeeDesk 工作流与项目约定

本文件是本项目的约定入口，只维护项目级约定与必要入口。通用流程统一引用
`git-small-project-workflow` Skill，不在此另抄一套设计检查、Git、验证、发布或收尾流程；
本文件没有另行约定的部分按该 Skill 处理。

## 项目概览

- JeeDesk 是 Windows 桌面快捷启动器：无边框置顶窗口、系统托盘、全局快捷键与鼠标按键呼出、
  拖拽添加项目、`config.json` 热更新。
- 技术栈为 Wails v2 + Go + Vue 3 + TypeScript + Tailwind CSS，仅面向 Windows（Win32 API、WebView2）。

| 位置 | 职责 |
| --- | --- |
| `main.go` | Wails 装配、运行目录判定（`resolveBaseDir`）、图标资源中间件 |
| `app.go` | `App`：Wails 绑定方法、窗口显隐与失焦隐藏、配置与钩子状态 |
| `single_instance.go`、`win_helpers.go` | 单实例互斥与唤醒、Win32 窗口辅助 |
| `internal/config` | `config.json` 结构、原子读写与热更新监听 |
| `internal/hook` | 全局快捷键与鼠标触发钩子 |
| `internal/launcher` | 启动目标、打开文件与所在位置 |
| `internal/resolver` | 拖入路径与 `.lnk` 解析、图标提取与缓存 |
| `internal/tray` | 系统托盘与内置图标数据 |
| `frontend/src` | Vue 界面。`api/bridge.ts` 封装后端调用：自行声明 `WailsGoApp` 接口并直接访问 `window.go.main.App`，同时内置脱离 Wails 在浏览器中开发用的 mock |
| `frontend/wailsjs` | Wails 构建时自动生成并纳入版本管理，不手工编辑；前端源码不引用它 |

运行时数据是 `config.json` 与 `icons/` 图标缓存，目录由 `resolveBaseDir` 决定：当前工作目录
已有 `config.json` 时直接采用该目录；否则用可执行文件所在目录（`wails dev` 下即
`build/bin/`）；仅当可执行文件位于临时目录（`go run`）时回退到当前工作目录。
两者均被 `.gitignore` 忽略。

## 通用流程来源

- 执行实现、提交、分支、CHANGELOG、版本、TODO、PR、回滚、发布或部署相关任务前，读取
  `git-small-project-workflow` 的 `SKILL.md`。
- 复杂任务记录按需参考该 Skill 的 `references/task-records.md`；本文件只维护项目级约定。
- 最新用户指令和适用的仓库明确规则按各自权限层级执行，既有授权在原适用范围内继续有效。
- 本文件不新增审批，也不扩大授权；仅完成计划、创建本地提交或标记归档都不产生后续操作授权。

该 Skill 尚未安装到 Agent 的 skills 目录，按实际位置读取（2026-11-24 核实，路径变化时更新本表）：

| 文件 | 位置 |
| --- | --- |
| `SKILL.md` | `F:\Codes\git_cooperation\Git协作和工程规范\SKILLS\中小工程Git工作流Skills\git-small-project-workflow\SKILL.md` |
| `references/task-records.md` | 同目录下 `references\task-records.md` |

## 项目约定

| 项目项 | 约定 |
| --- | --- |
| 分支 | 主分支 `main`（2026-11-24 初始化）；任务分支与 Commit 沿用 Skill 默认。 |
| 版本 | 唯一现有来源是 `build/windows/info.json`（`fixed.file_version` 与 `info.ProductVersion`，均为 `1.0.0`），`wails build` 用它生成可执行文件的版本资源。`wails.json` 未配置 `info.productVersion`，`frontend/package.json` 的 `version` 只属于前端包。尚无正式发布、Tag 与兼容性政策：普通任务不改版本，发布时按 Skill 先确认约定。 |
| CHANGELOG | 尚未建立。出现符合 Skill 记录条件的变化时在根目录创建 `CHANGELOG.md`，维护主题届时按维护职责设置。 |
| 债务 | 根目录 `TODO.md`（Skill 默认），出现已确认债务时创建，目前不存在。 |
| 任务记录 | 见[复杂任务记录与归档](#复杂任务记录与归档)。 |
| 正式文档 | 尚无 README 及接口、配置、架构、部署文档。当前事实以代码为准：配置结构见 `internal/config/schema.go`；后端绑定见 `app.go` 的导出方法与 `frontend/src/api/bridge.ts` 的 `WailsGoApp` 接口。 |
| 发布与运行 | 目标环境是 Windows 桌面会话。截至 2026-11-24 没有远程仓库、CI 和发布流程；Push、Tag、Release、部署按 Skill 需用户明确授权。 |

## 构建与验证

环境：Windows，`go.mod` 声明 `go 1.27.4`，Wails CLI v2，Node.js 与 npm。`go`、`wails` 不在 PATH 时，
默认分别位于 `%ProgramFiles%\Go\bin` 与 `%USERPROFILE%\go\bin`。项目未配置独立 Lint
（无 golangci-lint、ESLint、Prettier 配置）。

在仓库根目录执行：

| 目的 | 命令 |
| --- | --- |
| 格式 | `gofmt -l .`（无输出为通过） |
| 静态检查 | `go vet ./...` |
| 行为测试 | `go test ./... -count=1` |
| 前端类型检查 | `cd frontend && npm run typecheck` |
| 前端构建 | `cd frontend && npm install && npm run build` |
| 完整构建 | `wails build`（产物为 `build/bin/JeeDesk.exe`） |
| 开发运行 | `wails dev` |

- **全新克隆先生成前端产物**：`main.go` 以 `//go:embed all:frontend/dist` 嵌入前端构建结果，而
  `frontend/dist/` 不入库。克隆后先执行 `wails build`、`wails dev` 或前端构建，否则根包的
  `go build`、`go vet`、`go test` 会报 `pattern all:frontend/dist: no matching files found`
  （`internal/...` 各包不受影响）。
- **前后端契约靠人工同步**：前端不引用 `frontend/wailsjs`，改动 `App` 的导出方法签名或
  `config.json` 结构时，必须同步 `frontend/src/api/bridge.ts` 与 `frontend/src/types/config.ts`。
  类型检查发现不了这类不一致，需要实际运行验证。
- **测试的环境影响**：`go test ./...` 会短暂注册全局快捷键 `Ctrl+Alt+Shift+F20`~`F23`、
  安装低级鼠标钩子、创建消息窗口，并调用 PowerShell 生成 `.lnk` 夹具、从 `notepad.exe`
  提取图标，需要 Windows 桌面会话。
- **按风险选验证**（对应 Skill 的 C、D 两级）：改动 `internal/hook`、`internal/launcher`、
  `internal/tray`、单实例或窗口显隐与焦点逻辑（`single_instance.go`、`win_helpers.go`、
  `app.go`）时，单元测试之外还需实际运行应用验收，并与离线验证分别记录。

## 复杂任务记录与归档

- 任务记录目录是项目根的 `_scratch`。先查找并复用同一任务的既有计划与状态入口，不另开同义目录。
- 新任务使用 `_scratch/YYYYMMDD-short-topic/Plan.md` 与 `Progress.md`；`YYYYMMDD` 取项目时区的
  本地日期（初始化时本机为 UTC+8）。不创建空壳计划。
- 整体满足 Skill 的归档条件后，在同层为原目录名加 `_done_` 前缀
  （`_scratch/_done_YYYYMMDD-short-topic/`），并修正指向旧路径的链接。
  阶段完成、计划写完、会话结束都不等于整体完成；后续阶段待授权时保持活动目录并注明恢复条件。
- 任务目标本身只是交付计划时，才可按“计划交付完成”归档，不能据此暗示产品已经实现。
- Git 跟踪策略：`_scratch` 未被 `.gitignore` 忽略。脱敏后的计划、进度和专题文档默认纳入版本管理，
  提交时逐个文件精确暂存；临时程序、缓存、大日志、安装包、真实数据、个人配置和秘密不提交。
