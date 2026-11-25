# TODO

已知债务与暂缓项。按影响排序；完成一项就删除对应条目，结论记入 `CHANGELOG.md`。

## 需要先定政策

- **版本来源仍有两处**：`build/windows/info.json` 的可执行文件版本资源，与 `app.go` 的
  `appVersion` 变量（可用 `-ldflags -X main.appVersion=…` 覆盖）。发布流程确定后应改为单一来源
  并在构建脚本里同时写入，否则"关于"里的版本会和文件属性不一致。
- **更新检查指向 GitHub Releases**：仓库目前没有正式发布，`CheckUpdates` 只会返回"尚未发布任何版本"。
  有了第一个 Release 之后需要复核 tag 命名（当前按 `v1.2.3` / `1.2.3` 都能比较）

## 缺陷（2026-11-25 全量审查确认）

- **撤销快照在写盘前就清空**：`app_features.go` 的 `UndoDeleteItem` / `UndoDeleteGroup` 先取走
  `deletedItem` / `deletedGroup` 再调 `mgr.Update`，写入失败（如 `ErrWriteBlocked`）时不归还快照，
  已删除内容再无恢复入口。验收：写入失败后快照仍在，`Ctrl+Z` 可以重试。
- **图标缓存版本升级后不重建**：`app_diagnostics.go` 的 `syncIconCacheVersion` 会删光
  `icons/*.png`，而 `EnsureIcons` 跳过 `item.Icon != ""` 的条目，于是所有图标退成内置默认图且不再重建。
  验收：改动 `iconCacheVersion` 后启动，缓存图标自动补齐（前端 `ItemIcon.vue` 命中过 404）。
- **网格内的项目拖动排序未接线**：`ItemGrid.vue` 只有 `@dragstart` 设置载荷，没有 tile 级
  `dragover`/`drop`，目前只能右键上移/下移。验收：拖动 tile 到新位置后顺序落盘。
- **分组拖动后可能吞掉一次点击**：`GroupSidebar.vue` 的 `suppressClick` 只在「松手落在同一行」时消费，
  拖到别行或空隙后标志位残留，下一次短按（含折叠按钮）被静默忽略。验收：任何拖动结束后首次短按都生效。
- **窗口坐标与文档不符**：`settings.window_x/window_y` 在 `schema.go` 里声明为位置坐标，
  `flushWindowState` 存的是物理像素、`moveWindowTo` 也按物理像素复原（自洽但与 DIP 文档不一致，
  混缩放屏之间会偏移）。验收：坐标同样按 DIP 存取。
- **配置界面有三处「有哪些弹窗」清单**：`App.vue` 的 `anyModalOpen`、`isOverlayOpen`、Esc 链已漂移
  （`anyModalOpen` 漏了 `confirmDialog`，确认框打开时切走窗口仍会失焦隐藏）。验收：三处收敛为一处判断。
- **桥接接口缺两个方法**：`bridge.ts` 的 `WailsGoApp` 未声明 `Quit` 与 `ToggleWindow`
  （生成的 `frontend/wailsjs/go/main/App.d.ts` 里有），违反 AGENTS.md 的绑定同步约定。前端目前未调用。
- **`ErrElevationCancelled` 无人消费**：`app.go` 的启动路径原样返回错误，前端照弹「启动失败: 已取消管理员授权」，
  与「取消提权不算失败」的规格相反。验收：取消提权不弹错误、不计入最近使用。

## 功能缺口

- **多显示器 DPI 混合缩放**：`font_scale` 按 96 DPI 计算，主副屏缩放不同时面板尺寸会跳变。
- **分组折叠状态没有上限**：`settings.collapsed_groups` 里会留下已删除分组的 ID，目前靠 `Normalize`
  不报错来容忍；条目多了应做一次差集清理。
- **语言只覆盖前端文案**：后端 toast（如"已添加 3 个项目"）仍是中文，未接入 `settings.language`；
  `emitToast` 的文案硬编码在 Go 里，英文界面会混中文。
- **没有虚拟滚动**：300 项以内靠 `content-visibility` 够用；上千项需要窗口化。
- **托盘图标只嵌入了 32×32**：高 DPI 托盘会由系统放大，必要时补 16/20/24/40/48 多尺寸 ICO。
- **`internal/search` 与 `internal/usage` 没有测试**：评分排序与使用次数统计是纯函数逻辑，最容易补单测。

## 结构债务（不改变行为，逐步收敛）

- `internal/resolver` 一包四职（图标缓存、`.lnk` 解析、开始菜单扫描、环境变量展开），建议按职责拆包。
- `internal/hook` 的 `hotkey.go` 与 `mouse.go` 各写一套「LockOSThread + 消息循环 + 超时 Stop」生命周期，
  且 `mouse.go` 借用 `hotkey.go` 里的 Win32 过程声明。
- 「临时文件 + rename」的原子写在 `schema.go`、`resolver/icon.go`、`usage.go` 各写一遍，应抽成一处。
- 焦点陷阱在四个弹窗里逐字节重复，`ConfirmDialog` 已漂移；应抽 `ModalShell` / `ToggleSwitch`。
- `bridge.ts` 的 50 个方法各自重复「走真实调用、否则 mock」分支，应收进一个 `call(real, mock)` 助手；
  同一份 mock 与真实后端的返回值已经漂移（`getUndoLabel` 恒返回空）。
- `App` 结构体约 45 个字段同时承担窗口、配置、钩子、撤销、对话框状态（`app.go`）。

## 已知限制（不做）

- 不做云同步、不做多进程拆分、不引 UI 组件库与状态管理库。
- 不做静默自动更新：只提示并打开下载页。
- `Tab` 不用于切组（留给焦点移动），切组用 `Alt+←/→` 或 `Ctrl+Tab`。
