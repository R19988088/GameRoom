# AGENTS.md

给接手这个工程的人（或 AI）看的说明。GameRoom 是用 Go + MyGo 写的一个游戏前端，
用来替代原来的 simUI：只看封面、默认套 CRT 滤镜、第一版只启动 RetroArch。

## 目录约定

根目录保持干净，默认只有这四项，外加源码和本文件：

```
模拟器/        模拟器本体。RetroArch 放在 模拟器/RetroArch/（retroarch.exe 与 cores/、system/ 同级）
rom/           游戏文件，按平台分子目录：rom/arc、rom/fc、rom/sfc、rom/ps、rom/SEGA、
               rom/掌机、rom/任天堂、rom/微软、rom/majiang、rom/3DO
数据/          launcher.db（SQLite）、config.json、covers/<平台>/（封面副本，735 张 / 153 MB）
exe/           构建产物 simui-d.exe
源码/          Go 源码（go.mod、main.go、internal/）
```

`模拟器/RetroArch` 与 `rom/*` 目前是 Windows 目录联接（junction），指向原 simUI 安装里的
大文件（RetroArch 8.7GB、rom 86GB），所以本目录不占空间。换机器时：

- Windows：`New-Item -ItemType Junction -Path rom\arc -Target <原目录>`
- macOS / Linux：`ln -s <原目录> rom/arc`（junction 是 Windows 专有）
- 也可以把文件真搬进来，路径不用改。

## 构建与运行

源码仓库是 `github.com/R19988088/GameRoom`，本地放在 `E:\game\GameRoom`；
`E:\game\Sim ui_D\源码` 是指向它的目录联接，所以从 `源码` 里改代码等于改仓库。

```powershell
cd "E:\game\GameRoom"
$env:GOPROXY="https://goproxy.cn,direct"; $env:GOSUMDB="off"   # 国内网络
go build -o "E:\game\Sim ui_D\exe\GameRoom.exe" .
```

运行 `exe\GameRoom.exe`（进程名 `GameRoom`，窗口标题 "GameRoom"）。程序会自己往上找带
`rom/`、`模拟器/`、`数据/` 的目录作为根，所以从 `源码` 里 `go run .` 也能跑。

第一次运行会把原 simUI 的库导入 `数据/launcher.db`。原库位置写在 `数据/config.json`：

```json
{ "simuiRoot": "E:/game/0_RetroArch Sim ui", "crtShader": "shaders_slang/202603.slangp" }
```

导入完成后 `simuiRoot` 可以留空，不再读它（封面已复制进 `数据/covers/`）。
`crtShader` 是默认的 CRT 滤镜（脑浆油条 PhosphorLine 的 `202603` 预设），留空则不套滤镜。

## 数据模型（`数据/launcher.db`）

一张表一件事，键与路径分开：

| 表 | 内容 |
| --- | --- |
| `platform` | `key`（rom 下的目录名，如 `arc`）、`name`、`grp`（货架：`arcade` / `console` / `handheld` / `pc`）、`exts`、`sort` |
| `menu` | 某平台下的分类：`platform_key` + `name` |
| `game` | `title`（中文展示名）与 `rom_path`（相对 `rom/` 的文件路径）分开；`size` 为字节数（INTEGER）；`cover` 为**相对 `数据/`** 的路径（`covers/arc/dino.png`）；`favorite` 布尔；`core` 是这个游戏单独指定的核心（空 = 用 sim 的）；`sim_id`（0 = 用平台默认模拟器）|
| `sim` | 模拟器：`exe`（相对工程根或绝对路径）、`core`（RetroArch 核心名，如 `geolith`）、`args`（其余参数，含 `{RomFullPath}` 占位符）、`is_default` |
| `setting` | `schema`（库版本）、`imported`、`imported_at`、`imported_from` |

导入（`internal/library/import.go`）对旧库做的整理：

- `rom.name` 是展示名、`rom_path` 是文件名，拆成 `title` 与 `rom_path`；
- rom 路径统一成相对 `rom/` 的 `平台/子目录/文件`（`arc/SNK/xxx.neo`）；
- `menu` 里 `_7b9` 这类脏值归到“未分类”（`menu = ''`）；
- 平台扩展名以**实际 rom 的扩展名**为准，不再沿用旧库那串把 RetroArch 全部支持格式抄进来的表；
- **只搬封面**：命中的图复制进 `数据/covers/<平台>/`，`cover` 记相对路径，旧安装可以删；
  视频、GIF、doc、desc 等三十多个媒体目录一律不搬；
- `size` 用 `os.Stat` 读真实字节数，读不到才退回解析旧的 `"3.93MB"` 文本；
- 旧库 `sim_conf` 那段 JSON（每个游戏单独指定核心）解析成 `game.core`，只认能确定的，
  互相矛盾的条目宁可留空用 sim 的核心；
- 模拟器路径改写到新布局：`retroarch.exe` 一律记为 `模拟器/RetroArch/retroarch.exe`，
  其余（Windows 独占的旧模拟器）保留绝对路径，仅供提示；
- 同一 `(平台, rom 路径)` 只留一条（`game_rom` 是 UNIQUE 索引，数据库也把住）。

**库版本**：`setting.schema = '3'`。表结构不兼容时 `Open` 返回 `ErrOldSchema`，
`app.open` 会在有 `simuiRoot` 的前提下删库重建（重新导入一次），不会半读半崩。

平台归到哪个货架由 `import.go` 的 `groupOf` 决定（旧库没有这个概念）：
`arc`/`majiang` → 街机，`掌机` → 掌机，`微软` → PC，其余 → 家用机。

封面按 `title > poster > snaps > thumbs > background` 顺序找，文件名取 rom 文件的主名
（`dino.zip` ↔ `title/dino.png`），扩展名优先 png > webp > jpg > jpeg。

## 行为约定

- **只显示封面**：不出视频、不出 GIF、不做右侧介绍面板。
- **顶部浮动工具条**（Apple TV 味）：左侧四个货架（街机 / 家用机 / 掌机 / PC）做成分段控件，
  右侧是“平台 · 数量”和搜索框，整条悬浮在封面之上（`Absolute().Top(18)` + 圆角 + 阴影）。
  内容列因此从 `Padding(98, 26, 40, 26)` 开始——**注意 `Padding` 的参数顺序是 上/右/下/左**，
  写反了工具条就会压在分类 chip 上（`internal/app/layout_test.go` 专门盯着这件事）。
- 工具条下面是平台 chip 和该平台的分类 chip（可横向滚动），再下面是封面网格。
- 封面用 `Fit(Contain)` 保持原始比例（街机 flyer 不裁切），卡片本身 16:9 左右。
- **第一版只启动 RetroArch**：`internal/launch` 只认 `retroarch.exe`；其它模拟器只在状态栏
  提示“本版只启动 RetroArch”。
- 启动参数支持引号、占位符 `{RomFullPath}` `{RomDir}` `{RomName}` `{RomBase}`；
  工作目录设为 exe 所在目录，RetroArch 才能找到 `cores/`、`system/`。
- **核心只存名字**：`sim.core = geolith / mame / fbneo …`，启动时才拼成
  `cores/<core>_libretro.<dll|so|dylib>`（Windows `.dll`、Linux `.so`、macOS `.dylib`，
  见 `internal/launch` 的 `coreFile`）。所以同一份 `launcher.db` 拿到别的系统上，
  核心路径会自动对上，不用改库。旧库导入时 `-L cores\xxx_libretro.dll` 会被拆成
  `core` + 其余参数。
- **游戏可以覆盖核心**：`game.core` 非空时用它的，空则用 sim 的（旧库给个别街机
  游戏指定过 `mame2010`/`mame2016`，导入时保留成这一列）。
- macOS 上传 `.app` 会自动指向 `Contents/MacOS/<名字>`。
- 网格 3–6 列随窗口宽度自适应；Enter 或双击启动；搜索框按标题过滤。

## 跨平台核对（2026-11 查 buildbot.libretro.com）

| 核心 | Windows x64 | Linux x64 | macOS arm64 | macOS x64 |
| --- | --- | --- | --- | --- |
| `fbneo`（FBNeo，现行） | 有 | 有 | 有 | 有（2023 旧构建）|
| `fbalpha`（旧库 sim 15 “FBA”） | 有，2022-10 起停更 | **无** | **无** | **无** |
| `fbalpha2012` | 有 | 有 | 只有 neogeo 变体 | 有 |
| `mame` | 有 | 有 | 有 | 目录停更于 2023 |
| `mame2010` / `mame2016` | 有 | 有 | 有 | 有 |
| `mamearcade`（旧库 sim 19） | 仅本机有，buildbot 无 | 无 | 无 | 无 |

结论：**MAME 与 FBNeo 三平台都有**，跨平台没问题；隐患是旧库里两个老核心：

1. `FBA`（`fbalpha_libretro`）Windows 独占且已停更 → 跨平台前把它换成 `fbneo_libretro`
   （同一批 rom，FBNeo 是 FBA 的后继）；
2. `MAME Arcade`（`mamearcade_libretro`）不在 buildbot 上 → 换成 `mame_libretro`；
3. `NAOMI` 用 `flycast_libretro`，三平台都有，可保留。

换核心只需改 `sim` 表的 `core` 字段，`game.sim_id` 不用动；核心文件放在
`模拟器/RetroArch/cores/`，各平台换成对应后缀即可。

## 与 simUI 的关系

代码和数据上都已经和 simUI 脱钩了：UI 是 MyGo 自绘、数据是自己建的 SQLite、启动自己拼命令，
封面也复制进了 `数据/covers/`（735 个文件 / 约 153 MB），走相对路径。

simUI 只剩**一次性导入**这一处：第一次启动读它的 `data.dll` 和 `games/*/title` 等美术目录。
导入完成后 `数据/config.json` 里的 `simuiRoot` 可以留空，`E:\game\0_RetroArch Sim ui`
整个目录都可以删（`模拟器/RetroArch` 那条 junction 除外——那是复用同一份 RetroArch 安装）。

RetroArch 本身不是 simUI 的一部分。

## 源码约定

- `internal/library` 管数据：模型、建表、导入。
- `internal/launch` 管启动：解析参数、起进程、按平台分文件（`proc_windows.go` / `proc_other.go`）。
- `internal/app` 管界面：状态、筛选、封面异步解码、`view.go` 里的布局。
- 图片必须自己读文件再解码（`ui.DecodeBitmapScaled`），MyGo 的 `ui.Image` 只吃 `*Bitmap`。
- 界面配色在 `buildTheme()`：近黑底、暖橙 accent，改主题只改这一处。
