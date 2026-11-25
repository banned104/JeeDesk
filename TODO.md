# TODO

已知债务与暂缓项。按影响排序；完成一项就删除对应条目，结论记入 `CHANGELOG.md`。

## 需要先定政策

- **版本来源仍有两处**：`build/windows/info.json` 的可执行文件版本资源，与 `app.go` 的
  `appVersion` 变量（可用 `-ldflags -X main.appVersion=…` 覆盖）。发布流程确定后应改为单一来源
  并在构建脚本里同时写入，否则"关于"里的版本会和文件属性不一致。
- **更新检查指向 GitHub Releases**：`v1.0.0` 已是首个正式发布，但 `CheckUpdates` 还没有对着真实
  Release 复核过一遍（tag 命名 `v1.2.3` / `1.2.3` 都能比较，需要确认取到的 `tag_name` 正是这一格式）。
  验收：在 1.0.0 版上点「检查更新」显示「已是最新」，把版本手动改小后能提示 1.0.0。

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
