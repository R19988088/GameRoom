# GameRoom

用 Go + [MyGo](https://github.com/egoist/mygo) 自绘 UI 的模拟器前端，**以游戏厅（街机）为主场，同时覆盖家用机 / 掌机 / PC**。
它替代了原来的 simUI：只看封面、双击即玩、默认套 CRT 滤镜。

- 顶部是三个分开的胶囊（不是一条大横条），封面从它们后面滚过去、被磨砂模糊掉：
  - 左：货架按钮（`街机 267 ▾`），点开是 街机 / 麻将 / 家用机 / 掌机 / PC 的菜单
  - 中：当前货架的机种 chip（默认只显示局数最多的 5 个，其余收进 `更多 N`）＋ 该机种的分类 chip，
    两者合成一条可横向滚动；单机种的货架（街机、麻将）只显示分类
  - 右：搜索框，Liquid Glass 里再嵌一层 Liquid Glass
- 封面横版展示、保持原始比例（街机 flyer 不裁切），没有视频、GIF、右侧介绍面板
- 数据自己管：一份 SQLite，核心只存名字，换系统自动拼 `.dll / .so / .dylib`
- 默认 CRT 滤镜来自 **[脑浆油条](https://space.bilibili.com/)** 的 PhosphorLine 系列
- 第一版只启动 RetroArch，其它模拟器只做提示

## 运行时的目录

根目录保持干净，只有四项（外加可选源码）：

```
GameRoom.exe
模拟器/
  RetroArch/            retroarch.exe + cores/ system/ shaders/ config/ saves/ states/
  Magpie/               给外部程序套 CRT 的捕获层（可选，见下）
rom/
  arc/  fc/  sfc/  ps/  SEGA/  掌机/  任天堂/  微软/  majiang/  3DO/
数据/
  launcher.db           SQLite：平台、分类、游戏、模拟器、设置
  config.json           simuiRoot（仅首次导入用）、crtShader
  covers/<平台>/        封面副本（约 735 张 / 153 MB）
```

`rom/*` 可以是 Windows 目录联接（junction）或 macOS/Linux 的软链接，指向已有的大文件目录，
本仓库不占空间，也不分发任何 ROM、BIOS 与模拟器本体。

### 自带的 RetroArch 是怎么放的

把一份装了几年、被 simUI 反复改过的 RetroArch 收干净，只留跑游戏要用的东西（约 5.7 GB）：

| 保留 | 内容 |
| --- | --- |
| `retroarch.exe` + 根目录 dll | 主程序与运行库 |
| `cores/` | **只留** `<名字>_libretro.dll`，52 个核心；清掉 `xxx.dll-`、`xxx.dll 2406`、`*.cpp` 这类历史遗留，也把误放在根目录的 `geolith_libretro.dll` 归位 |
| `system/` | BIOS 与 MAME 需要的资源：`neogeo.zip`、`scph*.bin`、`system/mame`、`system/fbneo`、`system/bios`、`system/DC` 等 |
| `saves/` `states/` | 存档与即时存档（原样带走，别丢） |
| `shaders/` | 收成一个预设 + 两个依赖目录（见下节） |
| `config/` | 手柄映射、核心选项（`Geolith/Geolith.opt` 等） |
| `assets/` `autoconfig/` | 界面资源与手柄自动配置 |

去掉的是纯缓存与现成用不到的大件：`cache*/`、`database/`、`thumbnails/`、`screenshots/`、
`recordings/`、`playlists/`、`logs/`、`downloads/`、`overlays/`、`shaders 精简/`，
以及 `system/pcsx2*`、`system/xenia`、`system/dolphin-emu`（库里没有对应游戏）。

## 它由什么组成

| 部分 | 内容 |
| --- | --- |
| 前端程序 | Go 1.27 单文件可执行，纯 Go 无 cgo，可交叉编译 |
| 界面 | MyGo v0.10 原生 GPU 自绘，无 WebView、无 Sciter |
| 数据库 | `modernc.org/sqlite`（纯 Go），5 张表，WAL |
| 启动器 | `internal/launch`：按核心名拼路径、展开占位符、写滤镜覆盖、拉起进程 |
| 导入器 | `internal/library`：读旧前端 `data.dll`，清洗成上面的模型 |
| 模拟器 | RetroArch（libretro 核心：geolith / mame / fbneo / flycast …） |

### 代码结构

```
main.go
internal/app/      窗口、货架、网格、搜索、主题
internal/launch/   命令行解析与进程启动（含滤镜覆盖文件）
internal/library/  数据模型、SQLite、旧库导入、封面搬迁
```

## 数据模型

| 表 | 说明 |
| --- | --- |
| `platform` | `key`（`rom/` 下的目录名）、`name`、`grp`（货架：`arcade`/`console`/`handheld`/`pc`）、`exts`、`sort` |
| `menu` | 某平台下的分类（SNK / CAPCOM / 彩京 …） |
| `game` | `title` 与 `rom_path` 分开；`size` 字节数、`cover` 相对 `数据/` 的路径、`favorite`、`core`（本游戏单独指定的核心）、`sim_id` |
| `sim` | 模拟器：`exe`、`core`（核心名）、`args`（其余参数，含 `{RomFullPath}`）、`is_default` |
| `setting` | `schema`（库版本）、`imported` 等 |

几个刻意的设计：

- **核心只存名字**：`sim.core = geolith`，启动时才拼 `cores/<core>_libretro.{dll,so,dylib}`，
  同一份库拿到 Linux / macOS 上不用改。
- **游戏可以覆盖核心**：`game.core` 非空时用它，空则用 sim 的。
- **`(平台, rom 路径)` 唯一**：UNIQUE 索引，重复的 rom 进不来。
- **封面自带**：导入时把命中的封面复制进 `数据/covers/`，旧前端目录可以整个删掉。
- **库版本**：`setting.schema = '3'`，结构不兼容时删库重建，不会半读半崩。

## 每个货架一个固定的分类标准

顶栏中间那颗胶囊里的第二组 chip 是「分类」，它的含义按货架固定，不随游戏变：

| 货架 | 按什么分 | 举例 |
| --- | --- | --- |
| 街机 | 厂商 | CAPCOM 51 / SNK 46 / IGS 23 / PSIKYO 13 / SEGA 13 … 认不出的归「未标注厂商」 |
| 麻将 | 年份 | 2022 → 2006 → … → 1986，倒序时间线，未知年份排最后 |
| 掌机 | 机种 | GBA / PSP / NGP，按它跑的核心里映出的机器名，不直接显示核心名 |
| 家用机 | 机种＝平台本身 | FC / SFC / SEGA / Playstation …，所以这一排只有机种 chip，没有第二组 |
| PC | 同上 | 只有机种 chip |

实现是 `internal/library/split.go` 的 `menuAxis` + `(*Library).split()`：载入库之后按上面
的轴重写每个游戏的 `menu`，再重建菜单，所以 chip 和筛选永远读同一个字段。旧前端那些
把系列、类型、硬件名混在一起的菜单（`冒险岛`、`ACT`、`STV`）不再使用。

- 厂商取自 `game.publisher`（ADB 抓的），没有就退回它原来的目录名（`SNK`、`CAPCOM`），
  `彩京 → PSIKYO`、`STV/NAOMI/MODEL2/MODEL3 → SEGA`，`ACT/STG/OTHERS/古老的游戏` 这类
  不是厂商的名字统一落「未标注厂商」；`CAPCOM / ARIKA` 这种合写只取第一个。
- 年份轴按时间线排，不按游戏数排——1996 有 12 个也不会跑到 2022 前面。

## 封面与厂商年份：Arcade Italia

街机封面来自 [adb.arcadeitalia.net](https://adb.arcadeitalia.net)：不需要 key、不校验 referer，
`media/mame.current/titles/<rom>.png?release=209` 就是盒子图，`ingames/<rom>.png` 是游戏内截图，
romset 不存在就 404。同名的 `dettaglio_mame.php?game_name=<rom>` 是详情页，厂商 / 年份 / 类型
都在 `table_caption` + `dettaglio` 的结构里，顺手一起抓。

- 界面右下角状态栏有「补全封面 N」按钮，只在有缺口时出现，点一下后台抓、抓完自动刷新网格。
- 无窗口跑（同一份 exe，必须用 exe 跑，`go run` 找不到运行根）：

  ```
  GameRoom.exe -fetch-covers [-limit N]
  GameRoom.exe -fetch-info   [-limit N]
  ```

- 抓到的图落到 `数据/covers/<平台>/<rom>.png`，同时写回 `game.cover`；厂商年份写回
  `game.publisher` / `game.year` / `game.genre`。
- **ADB 会限流**：一口气抓十几张之后连 TLS 握手都会失败，隔一阵才恢复。所以客户端默认
  每张之间停 400ms，网络错误重试三次（2s / 4s 退避）。批量抓请一批一批来。
- 有些 romset 就是没图（`ssf2tnl`、`wofaha2`、`deathsm2` 只有 0.6KB 占位图），这类会一直留在
  「补全封面」的计数里，属于正常缺口。

## 启动一个游戏时发生什么

1. `rom/<平台>/<相对路径>` 拼成绝对路径，`sim.exe`（相对工程根或绝对）定位模拟器
2. `sim.core` → `模拟器/RetroArch/cores/<core>_libretro.dll`
3. `sim.args` 里的 `{RomFullPath}` `{RomDir}` `{RomName}` `{RomBase}` 展开
4. 读 `数据/config.json` 的 `crtShader`，把预设转写进 RetroArch 的 `config/global.slangp`
5. 工作目录设为 RetroArch 所在目录，拉起进程

## CRT 滤镜

默认采用 **脑浆油条** 的 **PhosphorLine**：`模拟器/RetroArch/shaders/202603.slangp`。

**它没法真的只留一个文件**——slang 管线里每个 pass 都是一支独立的 `.slang` 程序、各自占一张
framebuffer，RetroArch 不支持把 8 个 pass 合进同一个文件。能做的是把预设收成一个文件、
把依赖收进两个小目录，不再是一整包两千多个预设：

```
shaders/
  202603.slangp        唯一的预设：8 pass 的管线 + 全部参数（可直接改数值调口味）
  passes/              14 支 .slang（phosphor-line v2.0 的 pass）
  fsr/                 4 支（FSR 边缘平滑 pass + 两个头文件）
```

- **怎么生效**：RetroArch 1.17 不认命令行传进来的 `video_shader`（实测仍回落到 stock shader），
  它只认 `config/` 下的 per-content 全局预设。所以 GameRoom 启动游戏前把
  `shaders/202603.slangp` 转写成 `config/global.slangp`（pass 路径加 `../shaders/` 前缀）——
  日志可见 `Specific shader preset found at ...config/global.slangp` 与 8 支 pass 编译、0 错误。
- 想换滤镜：把新的 `.slangp` 放进 `shaders/`，改 `数据/config.json` 的 `crtShader` 即可
- 想关掉：`crtShader` 留空
- 滤镜本体不随本仓库分发（体积与授权原因）；找不到预设时不会拦着游戏启动，只是不套滤镜
- 建议同时关掉 RetroArch 自己的 `auto_shaders_enable`（或至少别留别的 `config/global.slangp`），免得两边打架


## 家用机 / 掌机：一个系统一个核心

旧库里同一个平台会散着好几种模拟器（一个 FC 里既有 VirtuaNES 也有 RetroArch，SEGA 里 Mega Drive
和 Model 2 混在一条 sim 里）。GameRoom 一律走 RetroArch，按**目录优先、扩展名其次**选核心，
所以同一个系统的每局游戏都吃同一套 shader：

| 平台 | 目录 | 系统 | 核心 |
| --- | --- | --- | --- |
| fc | 全部 | FC / NES | `mesen` |
| sfc | 全部 | SFC / SNES | `snes9x` |
| 3DO | 全部 | 3DO | `opera` |
| ps | `ps/` | PlayStation | `duckstation` |
| ps | `ps2/`（或 .iso） | PlayStation 2 | `pcsx2` |
| SEGA | `MD/`（其余兜底） | Mega Drive / Master System | `genesis_plus_gx` |
| SEGA | `DC/`（.gdi/.cdi） | Dreamcast | `flycast` |
| SEGA | `SS/` | Saturn | `mednafen_saturn` |
| SEGA | `Model 2/` `Model 3/` `STV/` | 街机基板 | `mame` |
| 任天堂 | `3ds/`（.cia/.cci） | 3DS | `citra` |
| 任天堂 | `wii/`（.rvz/.wbfs） | Wii / GameCube | `dolphin` |
| 任天堂 | `GB/`（.gb/.gbc） | Game Boy | `gearboy` |
| 任天堂 | `switch/`（.nsp/.xci） | Switch | Ryujinx（外部，无 libretro 核心） |
| 掌机 | `GBA/`（.gba） | GBA | `mgba` |
| 掌机 | `PSP/`（.iso/.cso） | PSP | `ppsspp` |
| 掌机 | `NGP/`（.ngc） | Neo Geo Pocket | `mednafen_ngp` |

导入时 `library.NormalizeConsoles` 会执行这套规则：清掉旧库的按游戏核心覆盖、把 sim 收成
「每（平台, 核心）一条」、把没人用的 sim（指向旧 simUI 目录的 VirtuaNES、Snes9x 等）删掉、
每个平台留一个默认。街机（`arc`、`majiang`）和 PC（`微软`）不动——街机的手工 MAME / FBNeo
选择比一律归一更合理，PC 留待后面处理。

## 外部程序的滤镜（PC 游戏 / 非 RetroArch 的模拟器）

RetroArch 里的游戏直接吃 shader，画面最准。跑在 RetroArch 之外的程序（Dolphin、Citra、
Xenia、直接双击的 PC 游戏）拿不到 shader，所以由一份外部捕获层补上：

- 工具是 [Magpie](https://github.com/Blinue/Magpie)（GPL-3.0，约 10 MB），放在 `模拟器/Magpie/`，
  不随仓库分发，需要自行下载 release 解压进去
- 它用 Windows 图形捕获拿窗口画面，**不往游戏目录写任何文件**，32/64 位程序都能抓
- 启动外部程序后 GameRoom 会：等它的窗口出现（30 秒超时，兼容启动器移交给子进程的情况）→
  把窗口置前 → 起 Magpie（已起就复用，并把它自己的窗口最小化）→ 发一次 Magpie 的缩放热键
  （默认 `Win+Shift+A`）
- GameRoom 会把 Magpie 的默认缩放模式改成内置的 **CRT-Geom**（曲线 + 荫罩 + 扫描线），
  这样热键一按就是 CRT，而不是默认的 Lanczos

**一致性的边界**：Magpie 的 CRT 是 CRT-Geom 家族，观感和 PhosphorLine 同路但不逐像素相同。
要做到完全一致有两条路：

1. 把能上 RetroArch 核心的模拟器换回核心（`citra`/`dolphin`/`pcsx2`/`dosbox` 在 `cores/` 里都有），
   这样它们和街机走同一条 shader 管线，效果 100% 一致；
2. 把 `phosphor-line` 的 pass 移植成 MagpieFX（HLSL compute），需要逐个 pass 改写，工作量不小。

## 从 simUI 迁移（一次性）

旧前端的数据其实是个 SQLite 文件 `data.dll`。启动时若 `数据/launcher.db` 还没有库，
程序会读 `config.json` 的 `simuiRoot`，把旧库清洗一遍：

- `rom.name`（展示名）与 `rom_path`（文件名）拆开
- rom 路径改成相对 `rom/` 的 `平台/子目录/文件`
- `_7b9` 这类脏菜单归到“未分类”
- 平台扩展名按**实际 rom 的扩展名**重新生成
- 只搬封面，视频 / GIF / doc / desc 全部丢弃
- `sim_conf` 里的 JSON（每个游戏单独指定核心）解析成 `game.core`
- 同一 `(平台, rom 路径)` 去重
- 导入后核对一遍“库里点名的核心”是否真在 `cores/` 下存在：本例 `mame2016` 没有 dll
  （3 个街机游戏 + 1 条 sim），统一改成 `mame`；`xenia` 的覆盖改空，回落到 sim 的
  `xenia_launcher`。核对后 25 个核心名全部命中

导入完成后 `simuiRoot` 可以留空。

## 构建

```powershell
$env:GOPROXY="https://goproxy.cn,direct"   # 国内网络
go build -o ..\exe\GameRoom.exe .
```

依赖：Go 1.27+。MyGo 与 sqlite 都是纯 Go，交叉编译不需要 cgo。

## 跨平台

程序本身三平台通吃（Windows / Linux / macOS），UI 与数据库无平台代码（仅进程属性分文件）。
核心是否齐就看 buildbot：

| 核心 | Windows x64 | Linux x64 | Linux arm64 | macOS arm64 | macOS x64 |
| --- | --- | --- | --- | --- | --- |
| `geolith`（Neo Geo） | 有 | 有 | 有 | 有 | 目录 2023-10 停更 |
| `mame` | 有 | 有 | 有 | 有 | 仅旧构建 |
| `fbneo` | 有 | 有 | 有 | 有 | 有 |
| `fbalpha` | 有（2022 停更） | 无 | 无 | 无 | 无 |

geolith 只认 `.neo`，并且要 `system/neogeo.zip`（MVS）或 `aes.zip`（AES）。

## 免责声明

本仓库只有前端代码：不包含、不提供任何 ROM、BIOS、模拟器本体或封面素材。
请仅用于自己合法拥有的游戏备份。

## 致谢

- [MyGo](https://github.com/egoist/mygo) — 自绘 UI 框架
- [RetroArch](https://www.retroarch.com/) 与 libretro 各核心
- **脑浆油条** — 默认 CRT 滤镜 PhosphorLine
- simUI — 数据结构的起点（本项目已不再依赖它的运行时）
