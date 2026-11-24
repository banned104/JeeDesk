# 从 GeekDesk 迁移到 JeeDesk

本文记录把 GeekDesk 2.5.14 中已保存的快速启动项完整迁移到 JeeDesk 的过程,可复现。
迁移工具与中间产物存放在本地 `_scratch/_done_20261125-geekdesk-migration/`(不入库)。

## 迁移对象的数据格式

GeekDesk 2.5.14 是 .NET Framework WPF 应用,数据全部保存在安装目录的无扩展名文件 `Data`:

- 文件头 `00 01 00 00 00 ff ff ff ff` 是 **.NET `BinaryFormatter`** 序列化格式;
- 反序列化根对象为 `GeekDesk.ViewModel.AppData`(何时对哪些类反序列化,依赖同名程序集);
- 主要成员:`menuList`(分组菜单,ObservableCollection\<MenuInfo\>)、`appConfig`(设置)、
  `hiToDoList`/`todoList`(待办,与启动器无关)。

BinaryFormatter 是二进制格式,手工解析不现实;但 GeekDesk 交付目录自带完整依赖
(`GeekDesk.pdb`、`lib/*.dll`),因此用一个小型 C# 辅助程序强类型反序列化后导出 JSON。

重要发现:**GeekDesk 的 ViewModel 属性 getter 有副作用**。读取 `AppConfig.BlurValue` 这类属性会触发
`OnPropertyChanged` → 尝试用 BinaryFormatter 把整个数据重新写盘(路径是工作目录相对的 `Data`)。
导出时必须只反射**实例字段**(BinaryFormatter 的对象状态本来就是字段),不读任何属性,
并且只在独立工作目录下运行,否则有污染原数据目录的风险。

## 步骤

### 1. 导出 JSON

辅助程序 `_scratch/_done_20261125-geekdesk-migration/gf_dump.cs`:

```powershell
csc -nologo -out:gf_dump.exe `
    -r:"E:\GeekDesk 2.5.14\GeekDesk.exe" `
    -r:"E:\GeekDesk 2.5.14\lib\Newtonsoft.Json.dll" gf_dump.cs
gf_dump.exe "E:\GeekDesk 2.5.14\Data" geekdesk-data.json
```

- `Main` 只装 `AssemblyResolve` 钩子(从 GeekDesk 目录与 `lib/` 解析 DLL),实际逻辑放在
  第二个方法里——否则 Main 方法被 JIT 时会在钩子生效前解析 Newtonsoft.Json,直接失败。
- JSON 序列化用自定义 ContractResolver,只取实例字段,绕开属性 getter 的写盘副作用。
- 导出前后核对原 `Data` 的 mtime/大小,确认未动。

### 2. 结构与映射

导出的对象图(与启动器相关的部分):

| GeekDesk 对象 | 字段 | 语义 |
| --- | --- | --- |
| `AppData.menuList[]` (MenuInfo) | `menuId` / `menuName` / `iconList[]` | 分组 / 组内条目;本次共 10 组 |
| `iconList[]` (IconInfo) | `name` / `path` / `startArg` | 名称 / 启动目标 / 启动参数 |
| | `lnkPath` | 来源桌面 .lnk(仅溯源用,不迁移) |
| | `relativePath` | 便携模式相对路径(已有绝对 `path`,不迁移) |
| | `adminStartUp` | 管理员启动(JeeDesk 无对应字段;本次数据中 0 条) |
| | `imageByteArr` / `defaultImage` | 图标位图 —— **94 条全部相等,即统一占位图**,GeekDesk 实际是运行时从 `path` 提取图标,数据里没有逐条目图标 |
| `AppData.appConfig` | `hotkeyStr` / `hotkeyModifiers` | 主呼出键,本次为 `Alt + G` |
| | `mouseMiddleShow` | 中键呼出开关,本次 false |

| GeekDesk | JeeDesk(config.json) | 说明 |
| --- | --- | --- |
| `MenuInfo` | `groups[]`,id 沿用 `menuId`(UUID) | 名称与数组顺序保持原样 |
| `IconInfo` | `items[]`,id = sha1(menuId\|name\|path) 前 32 位 | 重跑不漂移;重名条目按原样保留 |
| `name` / `path` / `startArg` | `name` / `path` / `args` | 原样 |
| `imageByteArr`(占位图) | `icon = path` | 让 JeeDesk 运行时从目标提取图标,效果与 GeekDesk 一致 |
| `hotkeyStr`=`Alt + G` | `settings.hotkey = "Alt+G"` | 恰为 JeeDesk 默认键 |
| `mouseMiddleShow=false` | `settings.mouse_trigger = "none"` | |
| (无对应配置) | `hide_on_launch/blur/esc = true` | 沿用 JeeDesk 默认 |

### 3. 校验

- 映射脚本 `convert.mjs`(Node.js)生成 `config.json`;
- 用仓库校验程序 `validate_config.go`(`jeedesk/internal/config` 的 `Load` + `Normalize`)
  实际加载:`LOADED OK`,10 组 / 94 条 / 0 空值;
- 本组数据现状:条目名/路径无缺失,2 条带参数(`--src1`),全部 `iconType=1`。

### 4. 部署

- `wails build` 产物 + 转换出的 `config.json` 一并拷贝到独立目录 `F:\Softwares\JeeDesk\`;
- 从该目录启动 EXE 做冒烟:进程存活、干净退出、`config.json` 字节不变
  (JeeDesk 只在用户修改时写盘,启动本身不动配置)。

## 迁移后注意事项

- GeekDesk 主热键与 JeeDesk 相同(Alt+G):使用 JeeDesk 前先**彻底退出 GeekDesk**,
  否则提示热键被占用。
- GeekDesk 设置过开机自启(`selfStartUp=true`):迁移完需要手动关闭 GeekDesk 的自启。
- 未迁移:外观类设置(背景/透明度/尺寸/跟随鼠标)、ToDo 与取色器热键、使用次数统计——
  JeeDesk 没有这些概念。
- 首次打开各分组时图标才逐条提取并缓存到 `icons/`(机制见 README「图标机制」)。
