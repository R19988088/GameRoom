# GameRoom

用 Go + [MyGo](https://github.com/egoist/mygo) 自绘 UI 的模拟器前端，**以游戏厅（街机）为主场，同时覆盖家用机 / 掌机 / PC**。
它替代了原来的 simUI：只看封面、双击即玩、默认套 CRT 滤镜。

- 顶部浮动工具条：`街机 / 家用机 / 掌机 / PC` 四层货架切换
- 封面横版展示、保持原始比例（街机 flyer 不裁切），没有视频、GIF、右侧介绍面板
- 数据自己管：一份 SQLite，核心只存名字，换系统自动拼 `.dll / .so / .dylib`
- 默认 CRT 滤镜来自 **[脑浆油条](https://space.bilibili.com/)** 的 PhosphorLine 系列
- 第一版只启动 RetroArch，其它模拟器只做提示

## 运行时的目录

根目录保持干净，只有四项（外加可选源码）：

```
GameRoom.exe
模拟器/
  RetroArch/            retroarch.exe、cores/、system/、shaders/ 同级
rom/
  arc/  fc/  sfc/  ps/  SEGA/  掌机/  任天堂/  微软/  majiang/  3DO/
数据/
  launcher.db           SQLite：平台、分类、游戏、模拟器、设置
  config.json           simuiRoot（仅首次导入用）、crtShader
  covers/<平台>/        封面副本（约 735 张 / 153 MB）
  retroarch-shader.cfg  每次启动生成的滤镜覆盖文件
```

`模拟器/RetroArch` 与 `rom/*` 可以是 Windows 目录联接（junction）或 macOS/Linux 的软链接，
指向已有的大文件目录，本仓库不占空间，也不分发任何 ROM、BIOS 与模拟器本体。

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

## 启动一个游戏时发生什么

1. `rom/<平台>/<相对路径>` 拼成绝对路径，`sim.exe`（相对工程根或绝对）定位模拟器
2. `sim.core` → `模拟器/RetroArch/cores/<core>_libretro.dll`
3. `sim.args` 里的 `{RomFullPath}` `{RomDir}` `{RomName}` `{RomBase}` 展开
4. 读出 `数据/config.json` 的 `crtShader`，写 `数据/retroarch-shader.cfg`，用 `--appendconfig` 交给 RetroArch
5. 工作目录设为 RetroArch 所在目录，拉起进程

## CRT 滤镜

默认采用 **脑浆油条** 的 **PhosphorLine**（预设 `shaders_slang/202603.slangp`）：

- 滤镜本体放在 `模拟器/RetroArch/shaders/shaders_slang/`，**不随本仓库分发**（体积与授权原因）
- 想换滤镜：改 `数据/config.json` 的 `crtShader`，如 `"shaders_slang/crt/crt-lottes.slangp"`
- 想关掉：把 `crtShader` 留空
- RetroArch 侧若已有 `config/global.slangp`，会优先于这里的设置

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
