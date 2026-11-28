# 项目拖动排序：现状与重做交接

本文是「网格内拖动项目」这个功能的交接说明：现在是什么样、为什么这么做、哪里不够用、
下一步怎么改，以及改动时必须守住的契约。写给接手的人或 Agent，不写给最终用户。

- 适用范围：`frontend/src/components/ItemGrid.vue` 的 tile 拖动；侧栏分组长按拖动
  （`GroupSidebar.vue`）是参考实现，不是本任务对象。
- 基线：`main` 的 `b8d5579`（发布 `v1.0.1`）。
- 现状（2026-11-28 重做后）：已按第 5 节的「插入位」方案实现——ghost 固定定位不被裁切、
  左右半区决定插入位、tile 间空隙与末尾空白都能落、其余 tile 让位动画、边缘自动滚动、
  跨组落点高亮。第 1 节表里的八个缺口逐条对照见第 3 节，第 7 节给验收状态。

## 1. 用户反馈与已知问题

先分清事实与推断，接手后**先向用户确认再动手**，不要照着推断直接重写。

已确认的事实：

- 用户明确否定了现在的拖动体验（原话大意：「根本不灵活」），没有给出更细的复现步骤。
  先按第 1 节的行为缺口推进，同时保留向用户确认「到底哪里不灵活」的义务。
- 交互模型与侧栏分组长按拖动不一致：侧栏有浮起、让位、边缘自动滚动，网格内什么都没有。

从代码可以确定的行为缺口（不需要用户确认，都是读代码就能看到的）：

| 缺口 | 具体表现 | 位置 |
| --- | --- | --- |
| 没有让位动画 | 拖动时其它 tile 岿然不动，看不出会落到哪里 | `ItemGrid.vue` 的 `tileDragStyle` 只改被拖 tile |
| 没有占位/插入标记 | 松手前不知道目标位置 | 同上；`dropIndex` 只用于加一圈 ring |
| 拖动中的 tile 会被容器裁掉 | 上移出网格顶部、或越过滚动区边界时看不到 | ghost 是网格内的 `<button>`，父级 `main` 是 `overflow-y-auto`（`ItemGrid.vue:336`） |
| 不能拖到末尾/空白处 | 落在最后一排以下的空白区、或列表底部，`dropIndex` 为 `null`，直接不生效 | `handleTilePointerMove` 的 `elementFromPoint` 只认 tile 与分组行 |
| 没有边缘自动滚动 | 条目一多，想拖到看不见的位置只能先滚动再拖 | 网格内没有 `scroll` 逻辑；侧栏有 `autoScrollTimer`（`GroupSidebar.vue:205` 起） |
| 跨组没有目标高亮 | 拖到侧栏分组行上时没有任何视觉指示 | `dropGroupId` 只参与落点计算，不传回 `GroupSidebar` |
| 只在分组视图可拖 | 搜索、最近使用的列表里按下不动 | `handleTilePointerDown` 开头的 `props.mode !== 'group'` |
| 落点语义不直观 | 拖到目标 tile 上是「占它的位置」，末尾没法追加到最后一个之后 | 见下节索引换算 |

推断（需要与用户确认，不要当成结论）：

- 「不灵活」可能指手感：ghost 直接是原 tile 的 `transform`，与指针有固定偏移、没有缩放阴影层感，
  移动时会被内容重排/裁切干扰。
- 也可能指命中太苛刻：必须精确落在某个 tile 上才生效，落在 tile 之间的 `gap` 里不生效
  （`elementFromPoint` 拿到的是 grid 容器本身，`closest('[data-tile-index]')` 为 `null`）。

## 2. 为什么不用 HTML5 拖放

`main.go:116` 设了 `DisableWebViewDrop: true`，Wails 因此调用
`chromium.AllowExternalDrag(false)`（`internal/frontend/desktop/windows/frontend.go:497`），
也就是 WebView2 的 `ICoreWebView2Controller4.AllowExternalDrop = false`。

观测事实：上一版 grid 用的是 HTML5 拖放（`draggable` + `dragstart`/`dragover`/`drop`），
用户实机上**完全拖不动**；改成指针事件后能拖了。所以在本项目的配置下，页面内 HTML5 拖放
不可用，tile 拖动只能用指针事件（`pointerdown`/`pointermove`/`pointerup`）。

留一个待复核点（不要盲目相信上面这条机制解释）：`AllowExternalDrop` 按字面只是「外部内容」
能否落入 WebView，理论上不该影响页面内拖放。重做前值得花十分钟做最小复现——在面板里
拖一个 `draggable` 的 `div`，看 `dragstart` 是否触发。如果其实可用，HTML5 路线在
「拖出窗口/滚动手感」上比手写指针事件省事得多，值得回头评估。

与文件拖入的关系：文件拖入仍依赖 Wails 的 `--wails-drop-target` 管线（`ItemGrid` 根元素的
`wails-drop-zone` 与 `main.ts` 的 `preventDefault`），与 tile 拖动互不影响。
**不要为了拖动去关掉 `DisableWebViewDrop`**：那会让 WebView2 按默认行为把拖入文件当网址导航
（见 `CHANGELOG.md` 1.0.0「把文件或文件夹拖进窗口会添加到当前分组」）。

## 3. 现在的实现（重做后）

### 数据与状态

```ts
interface TileDrag {          // ItemGrid.vue
  groupId: string; itemId: string; originIndex: number;
  startX: number; startY: number;   // 手势起点
  curX: number; curY: number;
  width: number; height: number;    // 拿起时的 tile 尺寸，ghost 沿用
  offsetX: number; offsetY: number; // 指针在 tile 内的偏移，ghost 不会跳到光标下
}

interface GridMetrics {       // 拖动开始时一次性测量
  slotWidth: number;          // 一个槽的宽度（含 gap），让位动画的单位
  slotHeight: number;
  rows: GridRow[];            // 每行的 top/bottom/left/width
  scrollTop: number;          // 拿起时的滚动位置，用来把页面坐标换算成当前视口
}
```

- `tileDrag: Ref<TileDrag | null>`：拖动是否进行中。
- `insertAt: Ref<number | null>`：**落点的唯一真相**，语义「插到 `items[insertAt]` 之前」，
  取值 `0..items.length`。`null` 表示指针正压在侧栏分组行上（跨组追加）。
- `dropGroupId: Ref<string | null>`：命中的侧栏分组 ID，变化时才 `emit('dragGroup')`。
- `metrics` / `pressIndex` / `pressStart` / `lastPointer` / `scrollDir` / `shiftTouched`：
  普通 `let`，不进响应式（每帧都要写，走响应式是白扔开销）。

### 生命周期

1. tile 的 `@pointerdown` → `handleTilePointerDown`：记下起点，在 `window` 上挂
   `pointermove`/`pointerup`/`pointercancel`/`blur`。监听挂在 window 上，所以指针移出 tile
   甚至移出网格仍然跟手；`blur` 负责窗口失焦（`Alt+G` 隐藏面板、切到别的程序）时收尾。
2. 移动超过 `DRAG_THRESHOLD_PX = 6` 才进入拖动：记录 tile 的 `getBoundingClientRect()`、
   指针内偏移，`measureGrid()` 拍一张行几何快照，给 `document.body` 加 `select-none`。
3. 拖动中每次移动：先看 `event.buttons`——按钮已在窗口外松开就直接复位。
   用 `elementFromPoint` **只**找侧栏分组行；命中分组行则 `insertAt = null`，
   否则 `insertAtFromPointer()` 用几何快照算插入位。被拖的 tile 是 `pointer-events-none`
   + `opacity-0`，作为占位留在布局里。
4. 松手 → `handleTilePointerUp`：命中别的分组 → `reorder(toIndex: 1<<30)`（`MoveItem` clamp 到末尾）；
   否则把插入位换算成「移除之后」的下标（见下），换算结果等于原下标时不发事件。
   随后 `suppressClick` 置真、下一个 tick 复位，网格的 `@click.capture` 用它在松手后吞一次点击。
   右键、`pointercancel`、组件卸载都走 `interruptTileDrag()` → `resetTileDrag()` 的同一条收尾路径。
5. 视觉：原 tile 变占位，其余 tile 让位；ghost 是 `Teleport to="body"` 的 `position: fixed`
   副本（尺寸取自拿起时的 rect，位置 = 指针 − 内偏移，`scale(1.04)` + 阴影 + ring）。

### 插入位算法

```ts
// insertAtFromPointer：行几何快照 + 滚动修正
const scrolled = scroller.scrollTop - metrics.scrollTop;
const target = metrics.rows.find((row) => y <= row.bottom - scrolled);
if (!target) return items.length;              // 最后一排以下 → 追加
const column = clamp(floor((x - target.left) / slotWidth), 0, 该行槽数 - 1);
const into = target.index + column;
return clamp(x >= 槽左 + slotWidth / 2 ? into + 1 : into, 0, items.length);
```

因为用的是快照而不是命中测试，tile 之间的 `gap`、最后一排右侧的空白、以及列表下方整片空白
都能落。`shiftFor` 用同一个 `insertAt`：严格位于来源与插入位之间的 tile 平移一个槽宽
（往后拖为 `-1`，往前拖为 `+1`），被拖的 tile 不动——它就是被推着走的那道缝。

### 落点索引语义（容易踩错）

`App.handleReorderItems`（`App.vue`）把 `toIndex` 原样交给 `App.MoveItem`
（`app_features.go:234`）。`MoveItem` 的语义是**先移除再插入**：

```go
items = append(items[:itemPos], items[itemPos+1:]...)   // 先拿掉
// 再 clamp 到 [0, len(items)]，然后插到 toIndex
```

因此前端传的必须是「移除之后」的下标，换算只有一行
（在指标之后插入才会因为移除而左移一位）：

```ts
toIndex = insertAt > originIndex ? insertAt - 1 : insertAt;
```

- 把 0 号拖到 2 号右半区：`insertAt = 3` → `toIndex = 2`，移除后 `[B C D]` 插到 2 → `[B C A D]`。
- 把 3 号拖到 0 号左半区：`insertAt = 0` → `toIndex = 0` → `[D A B C]`。
- 拖到末尾空白处：`insertAt = n` → `toIndex = n-1`，移除后长度正是 `n-1`，即追加。
- 原地不动：`insertAt` 等于 `originIndex` 或 `originIndex + 1` → `toIndex === originIndex`，不发事件。

## 4. 必须保住的契约

改动前后这些都成立，否则会连带坏掉别处（不用改的地方就不要动）：

1. `ItemGrid` 对外发两个事件：
   ```ts
   emit('reorder', { fromGroupId, toGroupId, itemId, toIndex })
   emit('dragGroup', groupId | null)   // 仅用于侧栏高亮，可直接驱动 props
   ```
   `App.vue` 的 `handleReorderItems` 只做「调 `bridge.moveItem` + `revealItems`」，不要在前端
   自己改数组：条目顺序的唯一真相是 `config.json`，写盘后配置热更新会回推。
   `dragGroup` 只在命中的分组变化时才发，逐帧重复发会让侧栏白重渲染。
2. `toIndex` 是 `MoveItem`「移除之后」的下标，`1 << 30` 表示追加。`App.MoveItem` 的这一语义由
   `app_test.go` 的 `TestMoveItemUsesPostRemovalIndex` / `TestMoveItemAppendsAcrossGroups` /
   `TestMoveItemRefusesMissingGroup` 锁定；改算法时同步改这三条测试与本节，别只改一边。
3. 侧栏分组长按拖动（`GroupSidebar.vue:159` 起）必须继续可用：它与网格拖动共用
   `document.body.classList.toggle('select-none')` 和各自的 `suppressClick`，
   两者不会同时进行（一个在侧栏、一个在网格），但都不要把 `select-none` 永久留在 body 上。
   侧栏新加的 `dropGroupId` 只参与行高亮，绝不参与它自己的 `rowStyle` / `dropSlot` 计算。
4. 卸载清理：`ItemGrid.vue` 的 `onBeforeUnmount` 会调 `resetTileDrag()`。新增的 window 监听、
   `requestAnimationFrame`、让位用的内联 `translate` 都要在这里清掉；右键、`pointercancel`、
   `blur` 也必须走同一条路径，否则 ghost 和 `select-none` 会留在屏幕上。
5. 文件拖入（`@dragenter/@dragover/@drop` + `isFileDrag`）与 tile 拖动是两条路，别混在一起。
6. `frontend/src/api/bridge.ts` 的浏览器 mock 不需要新增接口：拖动只用到既有的 `moveItem`。
7. 网格容器上的 `content-visibility: auto` 已换成 `contain: layout style`：前者会让离屏行
   的 `getBoundingClientRect()` 量成 0 高度，行几何快照会因此退化，插入位算错。
   需要「长列表别一次画完」时要另找方案，不要把它加回这个容器。

## 5. 重做方案（已实施）

目标：手感对齐侧栏分组长按拖动，并且解决第 1 节那张表里的每个缺口。
**2026-11-28 已按本节实施**，实现细节见第 3 节；本节保留为设计与取舍的依据。

### 5.1 ghost 挪到固定定位层

不要再 `transform` 原来的 tile：

- 拖动开始时记录原 tile 的 `getBoundingClientRect()` 与指针在其内部的偏移；
- 原 tile 留在网格里当**占位**：`opacity-0`（保留尺寸，布局不塌）或
  `opacity-30 ring-1 ring-dashed`，并保持 `pointer-events-none`；
- ghost 用 `<Teleport to="body">` 渲染一个 `position: fixed` 的副本（图标 + 名称，
  可复用 `ItemIcon`），尺寸取原 rect，位置 = `pointer - 内偏移`，
  带 `scale(1.03)`、阴影、`ring`、`pointer-events-none`、高 `z-index`。

好处：不被 `overflow-y-auto` 裁切，也不受 `contain` 影响。实际用的是 `opacity-0` 占位 + `scale(1.04)`。

### 5.2 用「插入位」而不是「目标 tile」

拖动中把光标位置换算成一个**插入位** `insertAt`（0..items.length，语义是「插到第几个元素之前」），
而不是「占第几个元素的位置」：

```
光标下的 tile = t（可能没有）
若没有 t：光标在最后一排之后 → insertAt = items.length
若指针 x < t.rect.left + t.rect.width / 2 → insertAt = t.index
否则 → insertAt = t.index + 1
```

于是 tile 之间的 `gap` 只要落在左右半区判断里就自然生效，不必精确命中元素。

再换成 `MoveItem` 需要的「移除之后」下标：

```
toIndex = insertAt > originIndex ? insertAt - 1 : insertAt
```

- 拖到末尾空白处 → `insertAt = n` → `toIndex = n-1`（移除后长度就是 `n-1`，即追加）。
- 原地不动 → `insertAt` 等于 `originIndex` 或 `originIndex + 1` → `toIndex` 与原值相同，
  此时**不要发事件**，省一次写盘与热更新。

插入位不变时不要重复 emit（现在 `handleTilePointerUp` 只比较 `target === originIndex`，
新算法要比较 `toIndex === originIndex`）。

### 5.3 让位与插入指示

两种做法，选一种即可：

- **让位**（对齐侧栏）：在 `insertAt` 处留出空位，受影响 tile 用 `translate` 平移一个槽位。
  需要知道列数与槽宽（`getBoundingClientRect` 测量 + `getComputedStyle(gap)`），
  本项目的 grid 是 `repeat(auto-fill, minmax(92px, 1fr)) gap-2.5`，列数可算。
- **插入线**（更简单、更稳）：在 `insertAt` 对应位置画一条 3px 高亮竖线，用固定定位层渲染，
  坐标来自目标 tile 的 rect（左边界或右边界），随滚动更新。

先做插入线，手感确实不够再上升为让位；两者都要在 `pointerup`/`pointercancel` 时清掉。

### 5.4 边缘自动滚动

指针进入滚动容器上下边缘约 48px 时按需滚动，参考侧栏的 `autoScrollTimer`：
`requestAnimationFrame` 循环里按 `scrollTop += step` 推进，离开区域或松手立即停。
滚动后必须用**最后一次指针位置**重新算 `insertAt`，否则插入线会停在旧位置。

### 5.5 跨组落点要有反馈

拖动中把命中的分组 ID 交给 `App.vue`，再以 prop 传给 `GroupSidebar`，命中行加高亮
（现在这一整条链路没有）。最省事的接法：

- `ItemGrid` 新增 `emit('drag-group', groupId | null)`，在 `dropGroupId` 变化时才发；
- `App.vue` 用 `const itemDragGroupId = ref<string | null>(null)` 接住，
  `:drop-group-id="itemDragGroupId"` 传给侧栏；
- `GroupSidebar` 新增可选 prop（默认 `null`），在行 class 里加一条高亮，
  与它自己的 `dragGroupId`（长按排序）互不干扰。

### 5.6 顺带修掉的小问题

- `tileDragStyle` 现在无条件返回 `content-visibility`/`contain-intrinsic-size`，把「性能小抄」
  写进了每个 tile 的内联样式。挪回 class 或删除，别和 ghost 的定位混在一起。
- `@mouseenter="emit('select', index)"` 在拖动经过其它 tile 时会把选中项一路改掉，
  拖动期间应当跳过（`if (tileDrag.value) return`）。
- `document.body.classList.add('select-none')` 与 `remove` 成对出现，异常路径（`pointercancel`、
  组件卸载）也要走到。

## 6. 验证

自动检查（每次改动都跑）：

```bash
cd frontend && npm run typecheck
cd frontend && npm run build     # 或 wails build
```

行为验证只能靠真机，因为 WebView2 的指针事件与裁切行为在浏览器里不完全一致。
手工步骤（Windows 桌面会话；本机部署在 `F:\Softwares\JeeDesk`，`--silent` 启动，`Alt+G` 呼出）：

1. 在同一分组里把第 1 个拖到第 3 个位置：ghost 跟手、插入线在正确位置、松手后顺序正确。
2. 拖到列表最后一排以下的空白处：落到末尾。
3. 条目多到出现滚动条：按住拖到网格上/下边缘，列表自动滚动且插入位跟着走。
4. 拖到侧栏某个分组行上：该行高亮，松手后条目出现在**目标分组末尾**。
5. 原地按下再松手、以及拖动后松手：不误启动、不改变顺序、不写 `config.json`。
6. 拖动过程中切走窗口/`Esc` 隐藏面板，再呼出：不残留 ghost、插入线、`select-none`。
7. 侧栏分组长按拖动仍然正常（回归）。

## 7. 验收标准

| 编号 | 判据 | 状态 |
| --- | --- | --- |
| AC-01 | ghost 固定定位，拖到网格外/滚动区边界不被裁切 | 通过（真机验收 2026-11-28） |
| AC-02 | 插入位按左右半区判定，tile 之间的 gap 也能落 | 通过（真机验收 2026-11-28） |
| AC-03 | 同组内可追加到末尾；`insertAt` 未变化时不 emit、不写盘 | 通过（真机验收 + `app_test.go` 的 `TestMoveItemUsesPostRemovalIndex`） |
| AC-04 | 边缘自动滚动，滚动后插入位正确 | 通过（真机验收 2026-11-28） |
| AC-05 | 跨组拖动时目标分组行高亮，落点为该组末尾 | 通过（真机验收 2026-11-28） |
| AC-06 | 拖动后不误启动；`Esc`/失焦/卸载后无残留状态 | 通过（真机验收 2026-11-28） |
| AC-07 | `npm run typecheck`、`wails build` 通过；侧栏分组长按拖动无回归 | 通过（命令全过 + 真机回归确认） |

AC-01…AC-07 全部通过：离线检查由 `npm run typecheck`、`go vet ./...`、`go test ./... -count=1`、
`wails build` 覆盖；手势部分由用户在真实 WebView2 会话里逐条走完第 6 节的 7 步确认。


## 8. 不做什么

- 不做触摸/触控板专用的长按拖动：保持「按下即拖、6px 阈值」。
- 不做键盘重排（已有右键菜单的上移/下移）。
- 不为拖动打开 `DisableWebViewDrop`。
- 不引入第三方拖放库（项目不引 UI 组件库与状态管理库）。

## 9. 相关记录

- `CHANGELOG.md` 1.0.1「分组与项目」：指针拖动的引入原因与当前口径。
- `TODO.md`：拖动重做这条债务的入口。
- `AGENTS.md`：前后端契约同步要求（`App` 绑定变化必须改 `bridge.ts`）。
- 侧栏参考实现：`frontend/src/components/GroupSidebar.vue` 的 `beginPress` / `rowStyle` /
  `startAutoScroll`。
