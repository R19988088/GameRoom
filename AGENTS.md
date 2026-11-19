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

`rom/*` 目前是 Windows 目录联接（junction），指向原 simUI 安装里的
大文件（rom 86GB），所以本目录不占空间；`模拟器/RetroArch` 已经是收干净的实体拷贝（5.7GB）。换机器时：

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
{ "simuiRoot": "E:/game/0_RetroArch Sim ui", "crtShader": "202603.slangp" }
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
- **顶部浮动工具条**（Apple TV 味，一条三区）：左区是一个货架按钮（显示当前货架名 + 局数 + `▾`），
  点开才是四个货架的菜单；中区是当前货架的机种 chip，**默认只摆局数最多的五个**，其余的进右侧
  `更多 N` 菜单（选中的机种永远保证有一个 chip，否则它下面的游戏会"无主"）；右区是搜索框，
  底材用 `plugins/glass` 的 `glass.Glass{}`（真正的背景模糊 + 边缘高光，D3D/CPU 都有 shader，
  不依赖 macOS），所以是"液态"的磨砂玻璃而不是半透明色块。
  两个下拉都用 `ui.PopoverBase(c, anchor, &a.shelfOpen, …)`：位置自动贴着按钮、点外面或 Esc 自动关，
  panel 用 `Background/Radius/Border/Shadow` 自己画（`a.shelfOpen`、`a.moreOpen` 是状态位）。
  内容列因此从 `Padding(98, 26, 40, 26)` 开始——**注意 `Padding` 的参数顺序是 上/右/下/左**，
  写反了工具条就会压在分类 chip 上（`internal/app/layout_test.go` 专门盯着这件事）。
- 工具条下面是**当前机种**的分类 chip（可横向滚动），再下面是封面网格。
- 顶部三区的位置由 `internal/app/bar_test.go` 盯着：货架按钮在最左、`更多` 在它右边、搜索框在最右且
  不出窗口；`shelf_test.go` 确认货架菜单点得开；`render_test.go` 采样像素，确认搜索框的玻璃真的画出来了。
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
- `mame2016` 在 2026-11 核对时本机并没有这个核心，3 个街机游戏和 1 条 sim 引用了它 →
  已统一改成 `mame`；`4B4E07EF_DDR_Universe_3` 的 `game.core='xenia'` 同样没有对应 dll，
  已置空改用 sim 的核心。改完核对：`sim` 里 25 个不同的核心名，`cores/` 下 52 个
  `*_libretro.dll` 全部命中，无缺失。
- macOS 上传 `.app` 会自动指向 `Contents/MacOS/<名字>`。
- 网格 3–6 列随窗口宽度自适应；Enter 或双击启动；搜索框按标题过滤。

## 滤镜怎么落到 RetroArch 里

`shaders` 只留一个预设：`shaders/202603.slangp`（脑浆油条 PhosphorLine，8 pass），
依赖是 `shaders/passes/`（14 支 `.slang`）和 `shaders/fsr/`（FSR 平滑 pass + 头文件）。
**不能真的并成一个文件**：每个 pass 是独立的 `.slang` 程序，各占一张 framebuffer。

RetroArch 1.17 实测不认命令行里的 `video_shader`（加了 `--appendconfig` 也回落到 stock
shader），只认 `config/` 下的 per-content 全局预设。所以 `launch.InstallShader`
在启动前把预设转写一份到 `config/global.slangp`：`shaderN = "passes/x.slang"`
改成 `"../shaders/passes/x.slang"`（config 目录到 shaders 目录的相对路径）。
日志会出现 `Specific shader preset found at ...config/global.slangp` 并编译全部 pass，
可作为验证手段。预设不存在时直接跳过，不拦游戏启动。

`数据/config.json` 的 `crtShader` 只是预设名（`202603.slangp`），换滤镜就把新 `.slangp` 丢进
`shaders/` 并改这里；留空则不套滤镜。

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

## 核心标准化（`internal/library/cores.go`）

旧库让每局游戏各自记一个模拟器，于是同一个平台里混着七八种。现在统一到「一个系统一个核心」：

- `CanonicalCore(platform, romPath)` 是唯一判据，**目录优先、扩展名其次**。一个平台目录常装多个系统：
  `SEGA` 里同时有 `MD/`、`DC/`、`SS/`、`Model 2/`、`Model 3/`、`STV/`；`ps` 里同时有 PS1 和 PS2；
  `掌机` 里同时有 GBA、PSP、NGP。所以先按路径段判断，扩展名只做兜底。
- `NormalizeConsoles(&sims, games)` 在 `Import` 读完库后调用：给每局游戏选核心、清掉旧的按游戏
  `core` 覆盖、把 sim 收成「每（平台, 核心）一条」（复用同核心的旧 sim，缺的按
  `模拟器/RetroArch/retroarch.exe` + `{RomFullPath}` 新建）、每平台按游戏数最多的一条设为默认、
  最后删掉没人引用的 console sim（旧库里指向 `E:/game/0_RetroArch Sim ui/games/simulators/...`
  的 VirtuaNES、Snes9x 这类由此消失）。
- Ryujinx 是唯一保留的外部 exe：RetroArch 没有 Switch 的 libretro 核心。识别方式是 core 为空且
  exe 路径含 `ryujinx`，它的 sim 原样保留、游戏照旧指向它（启动走 Magpie 那条链路）。
- 街机（`arc`、`majiang`）与 PC（`微软`）不参与这套归一：街机手工挑的 MAME / FBNeo 比一律归一更合理。
- 加一个平台：在 `CanonicalCore` 里加分支、在 `coreNames` 里加显示名，并在 `cores_test.go`
  里补两个用例（目录判定 + 扩展名兜底各一条）。

库里已按此校正过：`587` 局 console/handheld 全部改到规范核心、无缺失 dll、无空 sim；唯一外部的是
2 局 Switch（Ryujinx）。改动前备份在 `数据/launcher.db.bak_before_console_std_*`。

## 外部程序的滤镜（`takeover_windows.go`）

RetroArch 之外的程序没有 shader，靠 `模拟器/Magpie` 这份捕获层补：

1. `launch.Start` 起进程后，非 RetroArch 的分支调用 `launch.TakeOver(pid, exeName, MagpieFilter{Exe})`
   （goroutine，别放在 UI 线程）
2. `waitWindow` 每 250 ms 枚举一次顶层窗口，30 秒超时；先按 PID 匹配，再按**同名的进程**匹配——
   Windows 11 的 Notepad、以及各种启动器会把窗口移交给另一个进程，光看 PID 会抓空
3. `front` 把窗口还原并置前，然后 `scaleHotkey` 用 `keybd_event` 发 `Win+Shift+A`
   （Magpie 的默认缩放热键，全局钩子，所以要发在游戏窗口置前之后）
4. Magpie 没运行时先 `configureMagpie`（把 `%LOCALAPPDATA%\Magpie\config\v5\config.json` 的
   `profiles[0].scalingMode` 指到 `scalingModes` 里名字为 `CRT-Geom` 的那一项），再启动它，
   等 2.5 秒把 Magpie 自己的窗口最小化，免得盖住游戏
5. 找不到 Magpie 也不报错：只做窗口置前，游戏照常跑

限制：Magpie 需要 Windows 10 1903+、DirectX 11 级别显卡；独占全屏的游戏抓不到（要跑窗口化）；
UWP 与部分带保护的窗口可能拒绝捕获。观感是 CRT-Geom 家族，与 PhosphorLine 同路但不逐像素相同。

## 与 simUI 的关系

代码和数据上都已经和 simUI 脱钩了：UI 是 MyGo 自绘、数据是自己建的 SQLite、启动自己拼命令，
封面也复制进了 `数据/covers/`（735 个文件 / 约 153 MB），走相对路径。

simUI 只剩**一次性导入**这一处：第一次启动读它的 `data.dll` 和 `games/*/title` 等美术目录。
导入完成后 `数据/config.json` 里的 `simuiRoot` 可以留空，`E:\game\0_RetroArch Sim ui`
整个目录都可以删（RetroArch 已经复制成实体目录，不再依赖那边）。

RetroArch 本身不是 simUI 的一部分。

## 源码约定

- `internal/library` 管数据：模型、建表、导入。
- `internal/launch` 管启动：解析参数、起进程、按平台分文件（`proc_windows.go` / `proc_other.go`）。
- `internal/app` 管界面：状态、筛选、封面异步解码、`view.go` 里的布局。
- 图片必须自己读文件再解码（`ui.DecodeBitmapScaled`），MyGo 的 `ui.Image` 只吃 `*Bitmap`。
- 界面配色在 `buildTheme()`：近黑底、暖橙 accent，改主题只改这一处。
