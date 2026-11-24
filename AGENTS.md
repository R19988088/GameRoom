# AGENTS.md

给接手这个工程的人（或 AI）看的说明。GameRoom 是用 Go + MyGo 写的一个游戏前端，
用来替代原来的 simUI：只看封面、默认套 CRT 滤镜、第一版只启动 RetroArch。

## 目录约定

根目录保持干净，默认只有这四项，外加源码和本文件：

```
模拟器/        模拟器本体。RetroArch 放在 模拟器/RetroArch/（retroarch.exe 与 cores/、system/ 同级）
rom/           游戏文件，按平台分子目录：rom/arc、rom/fc、rom/sfc、rom/ps、rom/SEGA、
               rom/掌机、rom/任天堂、rom/微软、rom/majiang、rom/3DO、rom/dos、rom/pc98
数据/          launcher.db（SQLite）、config.json、covers/<平台>/（封面副本）
exe/           构建产物 GameRoom.exe
源码/          Go 源码（go.mod、main.go、internal/）
```

`rom/dos` 是**原样的 zip**（游戏自带 `play.BAT` 与 `dosbox.conf`，DOSBox Pure 直接挂载它），
`rom/pc98` 是**一个游戏一个目录、目录里是镜像**（`rom/pc98/<游戏名>/MEI.hdi`）：PC-98 的镜像
在不同游戏里常同名（`NANPA2.hdi`、`SHIKUMI.hdi`），扁平放会互相覆盖，NP2kai 也只认镜像、不认 zip。

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
go build -ldflags "-s -w -H windowsgui" -o "E:\game\Sim ui_D\exe\GameRoom.exe" .
```

**`-H windowsgui` 不能省**（2026-11-24 踩过）：不带它 Go 默认建控制台子系统（PE 头
`subsystem=3`），双击出来会拖一个控制台窗口，看着像「打不开」；带上才是纯窗口程序
（`subsystem=2`，20.4MB）。`-s -w` 剥符号表，去掉是 29.6MB。核对：
`(Get-Subsystem "exe\GameRoom.exe")` 读 PE 头 `+0x44` 那两字节，要 2。

**一次只开一个**：`app.Run()` 第一行是 `mygo.App.RequestSingleInstanceLock()`。上一个实例
**还在**（哪怕窗口已经关了、只剩个没有窗口的僵尸进程）时，新开的这一个会一直等在启动握手
上——窗口永远不出来，进程 CPU 近 0、内存十几 MB。表现就是「点了没反应 / 在等什么」。
诊断：`Get-Process GameRoom` 有两个 PID、新的那个 `MainWindowTitle` 空。清掉旧实例
（`Invoke-CimMethod -InputObject (Get-CimInstance Win32_Process -Filter "ProcessId=<旧>") -MethodName Terminate`）
后，等在半路的那一个会自己把窗口画出来。

**再点一次图标要“亮”起来**（2026-11-24）：第一个实例活着时会话接得住命令行，
`Run()` 里 `mygo.App.OnSecondInstance(func([]string,string){ raiseWindow() })` 把它
`Restore` + `Show` + `Focus` 提到前面；没有这一条，第二个实例交完命令行就退出，屏幕上什么
都不动，用户以为「点了没反应 / 没有窗口」。`boot()` 把 `running` 指向自己的 App，供它用。

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
| `game` | `title`（展示名，导入时是 rom 文件名）与 `rom_path`（相对 `rom/` 的文件路径）分开；`name` 是**目录里查到的大名**（ADB 的「Short name」，如 `Alien vs. Predator`），有它卡片就显示它；`size` 为字节数（INTEGER）；`cover` 为**相对 `数据/`** 的路径（`covers/arc/dino.png`）；`favorite` 布尔；`family` 是手工归到哪张卡下面（0 = 按标题自动）；`solo` 布尔 = 取消合并（标题相同也单独一张卡）；`ips` 是这个 rom 打开的补丁组（`数据/ips` 里的组名，`|` 分隔，见「IPS 补丁」）；`core` 是这个游戏单独指定的核心（空 = 用 sim 的）；`sim_id`（0 = 用平台默认模拟器）|
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
`arc` → 街机（`arcade`）、`majiang` → 麻将（`mahjong`，**自成货架，不并进街机**）、
`掌机` → 掌机（`handheld`）、`微软` → PC（`pc`），其余 → 家用机（`console`）。

封面按 `title > poster > snaps > thumbs > background` 顺序找，文件名取 rom 文件的主名
（`dino.zip` ↔ `title/dino.png`），扩展名优先 png > webp > jpg > jpeg。

## 每个货架的分类标准（`internal/library/split.go`）

chip 里的「分类」按货架固定，由 `menuAxis` 决定，`(*Library).split()` 在 `Load` 末尾顺序执行：
先按轴重写每个游戏的 `menu`，再重建 `l.Menus`（**`store.go` 不再读 `menu` 表**，旧前端那套
把系列 / 类型 / 硬件名混在一起的菜单全部作废）。

| 货架 | 轴 | 取值 |
| --- | --- | --- |
| 街机 | `publisher` | `PublisherOf`：先 `game.publisher`（ADB 抓的，`makerName` 大写去空白、`/` 取第一段）+ `makers` 别名表（彩京→PSIKYO、STV/NAOMI/NAOMI2/MODEL2/MODEL3→SEGA），再退回原目录名，`notAMaker`（ACT/STG/OTHERS/古老的游戏/不能运行/未分类）→ `未标注厂商` |
| 麻将 | `year` | `yearOf`，0 → `年份未知` |
| 掌机 | `core` | `machineOf`：mgba→GBA、ppsspp→PSP、mednafen_ngp→NGP、gambatte→GB、mesen→FC，否则核心名，空→`未知机种` |
| DOS&PC98 | `machine` ⇄ `publisher` | 机种=平台（DOS / PC98）；厂商=`dosMaker`，**用游戏自己的 `publisher`，不改大写**（松岗 / 欢乐盒 / elf 这类中文发行商），空→`未标注厂商` |
| 家用机 / PC | 空 | 平台本身即机种，`menu` 清空、不产生菜单 |

排序：`unknownRank`（`未标注厂商` / `年份未知` / `未知机种` 及 `未标注` 前缀）一律排最后；
**年份轴按时间线降序，不看游戏数**；其余轴按游戏数降序，同数按名字。
测试：`split_test.go`（街机厂商折叠与未标注排末、麻将时间线优先于数量、掌机 GBA/PSP、家用机无菜单）。

## 封面与厂商年份（`internal/covers` + `internal/app/covers.go`）

街机封面与元数据来自 `adb.arcadeitalia.net`（无 key、不校验 referer，未知 romset 404）：

- `media/mame.current/titles/<rom>.png?release=<Release>`：盒子图；`Cover()` 拿不到再退
  `ingames/<rom>.png`；`get()` 校验 PNG 魔数 + ≥1024 字节（0.6KB 的占位图不算），
  404 归 `ErrNoCover`，网络错误重试三次（2s / 4s）。
- `dettaglio_mame.php?game_name=<rom>`：详情页，`adbValue("Manufacturer"|"Year"|"Genre")`
  解析 `table_caption` 后紧跟的 `dettaglio` 文本；`adbValue("Short name")` 是**游戏标题**
  （`Alien vs. Predator`，不带区域/版本后缀），写进 `game.name`——只在那一行的 `title` 还是
  rom 文件名时才写，已有中文名的保持中文（`missingInfo` 里算 `untitled`，`SetName` 才落库）。
- **限流是真的**：ADB 抓十几张后连 TLS 握手都会失败，静置一阵才恢复 → `Pause` 默认 400ms，
  批量抓要一批一批来；`ADB.page()` 读详情页也有三次重试（2s / 4s），批量时先小 `-limit` 试水；
  `ADB.Base` 非空时指向测试服务器（`adb_test.go` 用 httptest 桩）。
- 落盘：`数据/covers/<平台>/<rom>.png` + `library.SetCover`；元数据走 `library.SetInfo`
  （`CASE WHEN` 保护空值，不会把已有的值擦掉），标题走 `library.SetName`。
- 目标：`missingCovers` 只认 `Group == "arcade"`；`missingInfo` 认街机 + 麻将里
  `publisher = ''` / `year = 0` / `board = ''` **或者标题还是 rom 文件名**的。
  **DOS&PC98 不走 ADB**（ADB 只有 MAME romset）：封面来自老 DOS 合集的 `theme/title.dos`、
  `theme/title.PC98`（640×400 的标题画面，横的，正是卡片要的比例），盒图 `boxart.dos` 作后备，
  统一转成 `数据/covers/<平台>/<rom 文件名>.png`。
- 入口：界面状态栏「补全封面 N」（只在有缺口时出现，`a.fetching` 期间显示 `补全中 n/N · 已新增 m`）
  和无窗口 CLI `-fetch-covers` / `-fetch-info` / `-limit`（**必须跑 exe**，`go run` 的
  `findRoot()` 找不到 `rom/模拟器/数据`）。
- 现状缺口（ADB 确实没有图）：`ssf2tnl`、`wofaha2`、`wofsgzb`、`dead smile 2`（占位图被过滤）、
  `Paca Paca Passion Special.cue`（名字对不上 romset）。

## 行为约定

- **只显示封面**：不出视频、不出 GIF、不做右侧介绍面板。
- **顶栏是平铺的一条，不套层**（Apple TV 味）：`toolbar()` 一条 `Row`（`Left/Right 20`、`Top 14`、
  `Height 46`、`Gap 10`）摆三样东西，**没有任何一样把 chip 包在里面**：
  1. 左：货架按钮，一整颗（`a.glass(el)`：`Radius(14)` + `Material(glass.Glass{Clarity: 0.5,
     Tint: "#0d0f168f"})` + `Border(1, #ffffff1f)` + `Shadow(0, 10, 26, 0, #00000099)`），`Height(46)`、
     `PaddingX(16)`，显示"货架名 + 局数 + `▾`"，点开 `ui.PopoverBase` 列出五个货架，当前项橙色高亮。
  2. 中：`ScrollHorizontal(...).Grow(1).MinWidth(0).Height(46).ClipX()` —— **一个看不见的滚动条**，
     自己没有任何玻璃；里面 `Row` 直接铺 chip。`contentRow()` 输出：**分类标准切换药丸**
     （`axisSwitch()`：`⇄` + 当前标准名，点击 `nextAxis()` 轮换；`AxesOf` 只有一个轴时——麻将——不画）
     + 1px 细线 + 分类 chip（`全部` + `Chips`）。机种轴下 chip 就是平台 chip（`topPlatforms(5)`，
     多的进 `更多 N`，只摆局数最多的五个，选中的机种保证有一个）。
     **单平台货架不画机种 chip**；**机种轴下没有分类，连 `全部` 都不画**，只剩机种 chip。
  3. 右：搜索框，`a.glass(...)` 一层药丸（`Width(300)`、`Height(46)`、`PaddingX(14)`），里面直接是
     `TextInputBase`（无样式控件，不带自己的底），所以只有一层玻璃。
  chip 本身（`chip()` → `chipBase()`）：`ButtonBase` + `Height(32)`、`Radius(16)`、
  `Material(glass.Glass{Clarity: 0.45, Tint: "#0d0f16a6"})` + `Border(1, #ffffff24)` +
  `Shadow(0,8,20,0,#00000066)`；选中换 `Tint #e8763a3b` / 描边 `#e8763aa6` + 600 字重。
  **小药丸一律走 `chipBase()`**（状态栏的「补全封面 N」也是），这样它们都是同一层玻璃。
  **加东西时别再引入一层容器药丸**：`layout_test.go` 的 `TestTopBarIsFlat` 断言 chip 高度 ≤ 34
  而货架按钮 ≥ 42，就是为了钉死"chip 不在盘子里"。
- **瀑布流是自建的分节列表**：`body()` 用 `ui.Scroll`（`Left/Right 26`、`Top(0)`、`Bottom(34)`、
  `TrackScroll(&a.scroll)`），里面一个 `Box().Height(a.contentH)`，标题行与卡片行都是
  `Absolute().Left(0).Right(0).Top(ln.top)`。`layout(w)` 把版面一次算进 `a.lines`：从 `topGap`
  起，每节先出一个标题行（`headH + 10`），再每 `cols` 局一行（`cardH + cardGap`），节末
  `secGap`。**只构建视窗内的行**（`from = Y-300`、`to = Y+h+120`），936 局也只建一屏。
  常量在 `view.go` 顶部：`cardH 172 / cardMin 200 / cardGap 8 / headH 30 / secGap 18 / topGap 74`。
- **裁剪线在窗口顶，不在药丸下面**：滚动容器从 `Top(0)` 起，封面一滚就穿到窗口最上沿、从药丸
  之间露出来（用户要的"封面透到玻璃下面"）。旧 `GridView` 的 `Padding` 会把裁剪线推到 padding
  的下沿，于是滚起来某一行只剩标题、封面被切一条带 —— 那就是"顶部封面被挡住"的根因，别再退回
  `GridView`。
- **顶栏三颗胶囊一模一样**：左边的货架按钮（`shelfSwitch`）和右边的搜索框（`searchField`）都走
  `chipBase(c, 32, …)`，跟中间的 chip 同形同高（32）、同半径（16）、同一层玻璃。顶栏 `Row` 仍是
  `Height(46)`，三颗矮胶囊居中摆在里面；**不要**再给左右两侧套 46 高的整块玻璃——那会立刻又
  「套层」。`layout_test.go` 的 `TestTopBarIsFlat` / `TestTopBarPillsAreOneHeight` 钉着这条。
- **搜索框左边是「刷新」**（2026-11-24）：`refreshPill` 也是 `chipBase(c, 32, …)` 的同款胶囊，和搜索框
  一起包在一条 `Gap(6)` 的小 `Row` 里贴在最右端（不和 chip 混在一起）。按一下 → 下一帧开头先
  `library.Rescan` 扫 rom 目录，再 `a.reload` 重读 `launcher.db`（详见下面「刷新不只重读库」）。
- **搜索同时比游戏名和 rom 名**（2026-11-24）：`versionMatches` 以前只比 `Title`，现在也比 `romFile(RomPath)`
  整份文件名（含扩展名），所以「hsf2」「mjelctrn」这种文件上写的名字、连「hsf2.zip」这种带后缀的写法
  都能砸中整张卡（族里任一份命中即可，`searchrom_test.go` 钉着）。注意是**整份文件名**，搜 "zip" 会
  把库里的 zip 全捞出来，这是有意的。搜索框的提示文字写「搜索游戏名 / rom 名」
  （`view.go` 的 `Placeholder`），无障碍 `Label` 仍是「搜索游戏名」——测试按这个 label 找框，别一起改。
- **同名不同容器要露扩展名**（2026-11-24）：版本列表里「同标题的两份」本来靠 `romStem` 区分，可
  `hsf2.7z` 和 `hsf2.zip` 的 stem 都是 `hsf2`，两行会一模一样。新的 `romTag(g)` 先看族里有没有
  别的成员**stem 相同**：有就写整份文件名（`hsf2.7z` / `hsf2.zip`），没有才回到 stem。
  `versionLabel`（卡片按钮上的尾巴）和 `versionName`（列表整行）都改用它。
  `reload()` 现在先看 `a.db == nil` 再 `library.Load`——以前没库就一路走到 `QueryRow` 上空指针崩掉
  （UI 测试里踩过）；`refresh_test.go` 钉着「在搜索框左边」和「按下去真的重读（没库时报读库失败）」。
- **刷新不只重读库，先扫 rom 目录**（2026-11-24 实测）：`library.Rescan(db, root)` 走一遍 `rom/<平台>`，
  把库里没有的 rom 加进来。两个坑：
  1. **`rom/arc`、`rom/SEGA`、`rom/3DO` 是指向旧安装的 junction**。`filepath.WalkDir` **不跟链接**，会把
     它们当成三个“文件”（ext 为空）就跳过——这就是用户往 `rom/arc/CAPCOM/STREET FIGHTER/` 丢了
     `hsf2.zip` 后按刷新什么也没发生的原因。所以这里用自己的递归走法：`os.Stat` 跟链接、进目录，
     `EvalSymlinks` 记一份 visited 防环。
  2. **同一个名字换容器 = 同一张卡的另一个版本**，不是第二张卡：`hsf2.zip` 与库里已有的 `hsf2.7z`
     同 stem，插进来时抄它的 title/menu/board/core/sim 并把 `family` 指向它的 id——卡片不变，只是
     「版本 2」。没见过的名字才新建卡，`title` 先取文件 stem，`menu` 取 rom 下面那一层目录名
     （`arc/CAPCOM/STREET FIGHTER/hsf2.zip` → `CAPCOM`；`majiang/无作弊版/x.zip` → `无作弊版`），
     所以目录判断是 `len(parts) > 1`，不是 `> 2`。
  - 只加不删：rom 被移走、卡片留着（星标、封面、手改的名字不会因为整理目录就丢）。
  - 实测一次：`games 1019 → 1055`（街机货架 267→279、CAPCOM 50→52、SEGA 13→14、其他 130→139），
    `hsf2.zip` 成了「超级街头霸王Ⅱ 15周年纪念版」的 `family=4874` 成员。
  - 跑之前先备份库：`Copy-Item 数据\launcher.db 数据\launcher.db.bak-<时间>`（本机这次是 `-1443`）。
- **组内按游戏名排序**：`file()` 先按 chip 名次分组，组内用 `titleLess`/`naturalKey` 按名字排——
  数字按数值比（街霸2 在 街霸10 前面），其余按码位比，中文顺序稳定。机种轴那种只有一组的货架
  也要排序（`len(a.chips)==0` 的分支里也要排一遍）。
- **卡片右键菜单**（`ui.ContextMenuBase`）：每张卡用自己的 open 指针（`a.opens map[int]*bool`），
  否则弹层会锚到最后建出来的那张卡上。菜单先列五个字段——机种/基板、厂商、游戏类型、核心、游戏名，
  值在右边：**单击**换到「选择值」列表（值取该货架已有的，`a.values`），**双击**换到输入框
  （回车或「保存」写入）。`游戏名` 能改，但它没有值列表：**单击**就直接进输入框，写 `name` 列
  （`library.SetName`）并同步内存里的 `Title`；点了**不筛**——一筛整个货架只剩一张卡。
  街机的「厂商」写 `publisher`；家用机/掌机的「厂商」是机子厂商（由平台/核心推出来的，不给改），
  只有「机种」能改：掌机改机种=改 `core`，家用机改机种=改 `platform_key`。
  字段下面还有一条 **收藏/取消收藏**（`favoriteRow`）：写 `favorite` 列（`library.SetFavorite`），
  星标同时出现在卡片标题旁与顶栏的「收藏」页。卡片上的星在**标题左边**（`card()` 里标题行先放
  一个宽 10 的星位，没收藏也占着，收藏与否标题都对齐），`TestFavoriteStarSitsLeftOfTheTitle`
  比两段文字的 X 盯着这条。
  - 版式（2026-11-24 按用户要求收的）：标题行**右上角是 rom 文件名、不带扩展名**
    （`romNameOf` = `romStem(RomPath)`，同一标题下挂好几个改版时靠它认是哪一份），
    原来的「右键」提示去掉；字段下面的说明行（「单击选择 · 双击输入 · 拖入改版可并入」）
    整条删掉——菜单只留字段和按钮。收藏那一行按状态写：没收藏 `☆ 添加收藏`，收藏了
    `★ 取消收藏`（以前两边都只写「收藏」）。版本弹层里那一行的右键菜单同款：右上角 rom 名、
    底下的「移出这个标题，单独一张卡」删掉。`cardmenu_test.go` 钉着这几条。
  - 版本弹层（`versionPanel`）每一行**最右边写着这一份的 rom 名**（`a.romTag(m)`，同名不同容器
    时带扩展名），依次是 标题（Grow）→ 日版/美版 → rom 名 → ✓（2026-11-24 用户圈出右侧空白要求
    "右侧显示一下 rom 名字"）。这样两行标题一模一样时也能认出是哪一份。
    `versionpick_test.go` 断言弹层里同时出现 `samsho2` 与 `samsho2sp`。
  - 卡片菜单最底下是**「rom 目录」「封面目录」两个按钮，左右并排**（`folderRow`，各占一半、
    字居中，2026-11-24 用户要求）：点了用 `explorer.exe` 打开 `rom/<RomPath 的上一级>` 和
    `数据/covers`；目录不在就只写状态行「目录不存在：…」，不弹窗、不改数据——换 rom、换封面
    这些事交给资源管理器做。`cardmenu_test.go`（`TestCardMenuOpensBothFolders`）钉着两件事：
    两个按钮同一行且 rom 目录在左，缺目录只落状态行。
- **改数据的动作排到下一帧**（`a.later`/`runQueue`，队列在 `view()` 第一行跑）：卡片菜单是在
  **这一帧的** `a.lines` 里画的，菜单里一改筛选或元数据，后面的卡还按老列表取下标，就
  `index out of range` 把窗口弄死（`TestNameRowIsTypedIn`/`TestValueClickRewritesTheGame`
  盯着这两条，`sections_test.go` 里直接调 `assign` 的用例要补 `a.runQueue()`）。
  写入照旧走 `library.SetField` / `SetPlatform` / `SetName`，改完 `SplitGroup` + `refilter`
  并保留滚动位置。
- **瀑布流按分类分组**：`refilter()` 先筛 `a.shown`，再按 `menu` 切成 `a.sections`；每节开头插一个
  标题行（组名 + 灰色计数）。`chips` 为空（家用机/PC 的机种轴：机种就是平台）时只有一节且不画标题。
  **标题条是纯标签，不是按钮**（2026-11-24 用户反馈"点击中间的横条会退出"、"点击左面这个小标题
  还是会退出 你不能用裁切来隐藏"）：以前 `header()` 的 `ui.Row` 是 `Left(4).Right(0)` 拉满整宽、
  `Clicked()` 挂在这条上，点空白或点组名都会把整个货架收到那一家，看起来像货架突然退出。中途只把
  点击缩小到 `label` 小 Row 被用户否掉——**用户明确反对用裁切/缩小点击区掩盖问题**——所以现在整条
  连 `Clicked()` 和 hover 背景都没有，只有两个 `ui.Text`（组名、计数），筛选用顶栏的 chip。
  `header_test.go`（`TestSectionHeadingIsOnlyALabel`）钉着：点标题文字与点横条空白都不改
  `a.menu`/`a.special`/`a.shown`。
- **封面缓存要上锁，否则窗口会静默消失**（2026-11-24 复查"退出"时实测）：`a.covers`/`a.icons`/
  `a.pending` 和 `a.jobs`/`a.loading` 由 `a.imgMu`（`sync.Mutex`）保护。解码在 goroutine 里收尾，
  有窗口时 `a.win.Update(done)` 投到 UI 线程，但没有窗口时（headless 测试）`done()` 直接在解码
  goroutine 上写 map，同时这一帧正在读同一张 map —— Go 报 `fatal error: concurrent map read and
  map write`，fatal 不走 recover、不写 Application 事件日志，进程就那么没了（与"点一下窗口自己关掉、
  事件日志里什么都没有"吻合）。`image()` 拿锁进，`pumpImagesLocked()` 在锁内派活，`done()` 在锁内
  写缓存并接着排下一张，`reload()` 换新 map 也拿锁；`imagequeue_test.go` 反复催 30 张封面盯着这条。
- **顶栏只列五个分类，其余进「其他」**：`topChips()` 取 `a.chips` 前五（机种轴取 `topPlatforms(5)`），
  `a.otherTotal = shelfTotal - 前五之和`，第六个往后全靠「其他」一个 chip（`otherChip`）。进「其他」
  时顶栏**不再列任何具体分类**，正文改用 `fileByMachine()`：标题按硬件分小组（街机/麻将优先用基板
  `BoardNameOf`，没基板就落平台名；掌机用核心映出的机器；家用机就是平台），组内仍按游戏名排。
  `a.special` 记着当前是不是这两个页面（""=普通 chip，"fav"/"other"）——切 chip、切轴、切货架、
  换平台都要把它清空（`chooseMenu/choosePlatform/chooseAxis/chooseGroup`）。原来的「更多」弹层已撤掉。
- **改版（版本族）**：`buildVersions` 先按标题归一（`normTitle` 去掉括号注释与 sp/特别版/中文版等后缀，`versionKeyOf` 取最短前缀，
  `sequelRemainder` 挡住 2/III/Kids 这类续作），把同一平台的改版归到一个 `verFam` 键上，卡片只出**基准那一张**，
  其余挂在卡片下的「版本 N」弹层里。**手工归并/拆分写在库里，不动磁盘**：`game.family`（= 基准卡的 id）让一个标题
  不同的改版也归到那张卡下，`game.solo`（= 取消合并）让标题相同的改版单独一张卡；两者都在标题归一之后覆盖，
  所以手工结论不会被 `longer/distinct` 那条规则再拆开。货架内才生效（跨平台的 family 指针忽略）。
  - 归并：**从资源管理器拖 zip/7z 到卡片上**（多选拖、不用按 Ctrl/⌘），`card()` 里 `b.DroppedFiles()` 收路径，
    `gameByFile` 按文件名（或去扩展名）在同平台里找那一行 → `library.FileDump(db, id, baseID, false)`；
    同名的 `zip`/`7z` 都算同一个 rom。**库里没有的文件只报数，不自动导入**（入库要复制 rom，属于导入流程）。
  - 归并（卡片到卡片，2026-11-24）：**按住 Ctrl 把一张卡拖到另一张卡上**，两张合成一张——被拖的那局成为那张卡的
    改版，**名字完全不同也照并**（`JoJo's Venture` 并进 `JOJO的奇妙冒险` 就是这一条）。`card()` 里
    `if c.Modifiers()&ui.Ctrl != 0 { b.Drag(cardDrag{id}) }` 才让卡片成为拖的来源（不按 Ctrl 拖动不会抓走卡片），
    目标卡 `ui.DragOver[cardDrag]` 时描一圈橙色，松手 `ui.Drop[cardDrag]` → `mergeCards` → `later` → `mergeInto`
    （落到 `versionBase(*dst)`，族不套族；跨平台只提示不写库）。和拖文件那条一样，改完
    `buildVersions` + `recount` + `refilter` 并保留滚动位置。测试：`carddrag_test.go`（不带 Ctrl 的拖动什么都不并、
    带 Ctrl 拖过去并成一张卡）。
  - 拆分：卡片右键菜单的「取消合并」（`splitRow`，族里不止一张时才出现，行右侧写「只取消这一个」）和版本弹层里**每一行的右键菜单**
    （`ui.ContextMenuBase` + `a.vmenu` 每行一个 open 指针，别复用卡片的 `a.opens`）→ `library.FileDump(db, id, 0, true)`。
    `setSolo` 里**先把还指着这张卡的改版改指到族里剩下的那张**（`baseOf(remaining)`），再把这局写成 solo：
    族里的成员是按 `family` 记的 id 挂的，拆的正好是基准卡时，别的改版还写着「跟着它」，
    `buildVersions` 又把它们归回去——看上去就是「点了取消合并没反应、拆不开」（2026-11-24 用户报的）。
    测试：`split_test.go`（拆基准卡后它单独一张、剩下两张仍是一张卡；拆普通改版只动它自己）。
  - 两个动作都排到下一帧（`mergeDumps`/`mergeCards`/`splitDump` → `later` → `mergeFiles`/`mergeInto`/`setSolo`），改完要
    `buildVersions` + `recount` + `refilter` 并保留滚动位置。
  - **版本弹层是单击**（2026-11-24 用户要求，原来是双击）：`versionPanel` 里一行 `row.Clicked()` → `pickVersion`
    （`setVersionDefault` 写 `版本:<fam>` 这条设置，再 `verSel`、关弹层）。弹层里原来那条「双击设为默认」
    提示也删了。`setVersionDefault` 里 `db == nil` 要先跳过（`library.SetSetting` 收 nil db 会直接崩）。
    测试：`versionpick_test.go` 的 `TestOneClickPicksAVersion`。
  - **每行写完整名字**（2026-11-24）：`versionName(m)` = 整个标题（同名两份才补 `· rom名`），基准那份也写自己的
    全名——不再有「主 ROM」这一行，是不是正在跑只靠行尾那个 `✓` 和橙色底。卡面上那个小按钮仍旧用短标签
    `versionLabel`（只写尾巴，如 `霸王丸地狱变 sp`），那里面子小，写全名会溢出。
  - **封面跟着默认版本走**：`card()` 里画封面用 `a.cardCover(g)` —— `versionTarget(g).Cover`，那一版没封面
    才回落到基准卡自己的。切默认版本，标题下那张图跟着换（`TestCardCoverFollowsTheDefaultVersion`）。
- **IPS 补丁**（2026-11-24，`internal/ips` + `internal/app/ipsrow.go`）：rom 的 `.ips` 补丁由前端自己套，
  核心只管读前端交过去的文件。
  - **位置是 `数据/ips/<rom 文件名去扩展名>/`**（如 `数据/ips/jojojr1/`），一个目录一个 set，跟 MAME Plus
    的 ips 目录同名同序；也认 `数据/ips/<平台 key>/<rom 名>/` 和 `数据/ips/<游戏标题>/`（手工整理的目录）。
    三种都扫，同名组只认第一个。
  - **两种格式都认**：`.dat` 是一组（MAME Plus 的清单：`目标文件<TAB>补丁名<TAB>CRC(D1E4836D)`，补丁名省略
    `.ips`；`[zh_CN]` 这种段落是说明不是补丁）；没有 `.dat` 认领的 `.ips` 自己成一组，改哪个文件靠文件名
    的尾巴（`组名-ch-a-604.u17.ips` → 目标 `ch-a-604.u17`），单文件 set 就改那一个。IPS 与 IPS32 两种头
    都读（`PATCH` 3 字节偏移、`IPS32` 4 字节偏移），`长度=0` 是 RLE、`EOF` 后面可以再跟新长度。
  - **开关在卡片右键菜单最下面**：`fieldList` 最后（`folderRow` 之下）调 `ipsSection`——一行 `ips` 标题 +
    `N 组`，再每组一行，行尾 `ui.Switch`。目录里没有这个 rom 的补丁就**整段都不画**（用户要求「没有就不显示」）。
    版本弹层里每一行的右键菜单（`splitMenuPanel`）底下也挂同一段。点击开关当场写库，不排下一帧（列表不变）。
  - **状态记在 `game.ips`**（`|` 分隔的组名，`library.SetIPS`，`Game.IPSOn`/`IPSGroups` 由
    `splitList` 拆），启动时只认「库里开着 且 目录里还在」的组。
  - **启动不碰 `rom/`**：`ips.Prepare(rom, groups, 模拟器/RetroArch/cache/ips)` 把补丁套到一份副本上
    （`<stem>-<8 位签名>/<原文件名>`；签名 = 组名 + 每个补丁的大小与修改时间，改过补丁就换目录），
    `.dat` 里的 CRC 与目标对不上直接报错、不启动。zip 里没被补丁改的成员用 `CreateRaw` 原样搬，改过的
    重新 deflate；不是 zip 的 rom 整份打。副本目录里把 rom 原目录的兄弟文件硬链接进去（`linkSiblings`），
    父集/BIOS 才找得到。
  - **副本怎么交给核心，两套走法**（`launchGame`）：
    - `fbneo` / `fbneo_plus`（`isFBNeoCore`）：**把副本放到 `<核心收到的 system>/fbneo/patched/<set>.zip`**
      （`stagePatchedSet`，硬链接、跨盘则拷贝），rom 参数照给原 rom，同时用 `core.Start(..., options...)`
      传 `fbneo-allow-patched-romsets=enabled`。核心读到这份就按名字取成员、**完全不查 CRC**，
      日志会打 `[FBNeo] Patched romset found at …`。
      实测证据：往 `patched\dino.zip` 里塞一个改了 1 字节（CRC 必然不符）的 `cde_23a.8f`，
      核心照常起画面且画面确实变了（mean 0.60 → 3.83）——所以走过的就是这份文件。
      直接把重打包 zip 当 rom 递进去是**不行**的（`ROM … is missing`），别再试。
    - 其他核心（`mame` 等）：直接把副本当 rom 给（`playRom = patched`），MAME 本来就忽略 CRC。
    - 这次没开补丁 → `clearPatchedSet` 把这个 set 在 `fbneo/patched` 里的残留删掉，
      否则关掉开关后 FBNeo 还会捡上次那份跑。
  - **标题栏报组数**：启动时 `SetWindowTitle(ipsTitle(g.Title, n))` → `JOJO的奇妙冒险    ips启用 2 组`，
    没开补丁就是原名；退出仍回 `GameRoom`。
  - 测试：`internal/ips/ips_test.go`（.dat 分组、zip 成员被打/其他成员不动且原 zip 未被写、CRC 不符拒补、
    平文件、IPS32 与 RLE）；`internal/app/ips_test.go`（有补丁才画且在最下面、没补丁不画、开关点击写库、
    计数进标题）。
- **收藏排在前五之前**：`a.favTotal` 是整货架（当前 group）的星标数，>0 时顶栏第一颗就是「收藏」，
  再是「全部」、前五、「其他」。收藏页 `special == specialFav`，正文同样按硬件分组；在收藏页里取消
  收藏，那张卡当场消失。星的写入排到下一帧（`toggleFavorite` → `later` → `setFavorite`），
  与改元数据同一条队列。机种轴的货架（chip 就是机台）**平时不画「全部」**——选中的那台机器就是
  全部——只有 收藏/其他 页面上才画一颗「全部」（计数用 `platformTotal`）当退路；在这两个页面上
  再点一次亮着的那颗，也会退回货架（`chooseSpecial` 自己就是开关）。
- 位置由 `internal/app/layout_test.go` 的 `TestTopBarIsFlat` 盯着：三样东西同一行（比中心点）、
  两两之间有空隙、chip 比货架按钮矮（不套层）、整行底边在 70 以内、卡片不钻到顶栏下面；
  `bar_test.go` 还盯着货架在最左、`其他` 不压搜索框；`shelf_test.go` 确认菜单点得开；
  `shelf_one_test.go` 确认单机种货架不画机种 chip、麻将自成货架；`render_test.go` 采样像素确认
  搜索框的玻璃真的画出来了。
- **封面一张一张解码**：`a.image()` 只把路径塞进 `a.jobs` 队列（`pending` 去重），
  `pumpImages()` 一次只起一个 goroutine 读文件 + `DecodeBitmapScaled`，解完在 `win.Update` 里写入
  `covers` / `icons` 并**再调一次 `pumpImages()` 拉下一张**。滚动再快也只有一个解码器在跑。
- **分类标准（axis）是按货架定的、可切换的**：`library.AxesOf(group)` 给出这个货架有哪几套口径
  （街机 `[maker, board]`、麻将 `[year]`、家用机/掌机/PC `[machine, maker]`），第一个是默认。
  `SplitGroup(group, axis)` 把整货架每个游戏的 `menu` 按该轴重写，`Chips` 再汇总；切轴就重写一遍，
  chip 与筛选读的永远是 `menu` 这一个字段。`menu` 表不再读。车库里 `a.axis/a.chips/shelfTotal`
  记着当前口径，`chooseAxis/nextAxis` 改它，`recount` 重算 chip，`refilter` 重算列表。
- **分类口径**：厂商 = `publisher` 归一（`PublisherOf`/`makerName`，认不出落「未标注厂商」）；
  基板 = `board`（MAME driver source）经 `BoardOf` 折成牌子名（`MVS`/`CPS1`/`CPS2`/`ST-V`/`NAOMI`…
  没收录就大写源名兜底）；年份按时间线降序、未知排末（`unknownRank`/`yearRank`）；机种在
  家用机就是平台本身（无分类），在掌机按核心映出的机器（`machineOf`）。
- 街机默认开厂商而非基板：`board` 历史上只有 124/267 抓得到（ADB 没页的 romset 就没有 driver source），
  默认开基板会让一半游戏落进「未标注基板」。2026-11-24 用 `cmd/boardfill` 从 MAME 机器表补齐后街机只剩 2 个空的，
  想默认开基板再说，先看一遍 `go run ./cmd/boards -platform arc`。
- 进一个货架、或切到机种轴时，选的是**局数最多**的那台机器（`biggestPlatform`），不是平台表第一行
  ——否则家用机会开局停在只有 1 个游戏的 3DO。
- 封面用 `Fit(Contain)` 保持原始比例（街机 flyer 不裁切），卡片本身 16:9 左右。
- **第一版只启动 RetroArch**：`internal/launch` 只认 `retroarch.exe`；其它模拟器只在状态栏
  提示“本版只启动 RetroArch”。
- **「双击或 Enter」两种都要能开**：卡片自己的 `DoubleClicked()` 是一路；Enter 挂在**卡片**上
  （`b.Shortcut(0, ui.KeyEnter)`），而不是挂在最外层 `ui.Scroll` 上——键只递给「焦点链」里的元素，
  点过一张卡它就有了焦点，Enter 才收得到。挂在 Scroll 上按 Enter 是没反应的（踩过）。
  框架的 `DoubleClicked()` 在真窗口里**不一定会报**（元素每帧重建，按 id 计数的配对会丢），
  所以 `card()` 自己也记时间：同一张卡 650ms 内第二次 `Click()` 就启动（`lastClick`/`lastClickAt`）。
- **滤镜别把预设写坏**：`InstallShader` 只改写 `shaderN = "路径"` 这一种行；预设第一行
  `shaders = "8"` 是**数量**，被当成路径改写成 `"../shaders/8"` 会让 RetroArch 在 shader 编译阶段
  直接崩（`0xC0000005`，游戏一闪而退，看着像"打不开游戏"）。`launch_test.go` 钉着这条。
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
- 主页 3–8 列随窗口宽度自适应（`cardMin 200` 保底宽，最多 8 列）；Enter 或双击启动；搜索框按标题过滤。

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

### 滤镜面板：两组参数并排，底部按卡片管组（2026-11-23）

- `filterPanel` 不再按预设分四颗胶囊：**一组参数**按原来的左右双列排（`knobs` 对半切），
  上面只有一个滤镜开关，底部是**已保存组的卡片 + 三颗按钮**（见下）。
- 面板里**不再有** `边框尺寸`（`BORDER_SIZE`）、`边框圆角大小`（`B_CORNER`）、`边框圆角曲率`
  （`FILLET_D`）三行：前两个本来就没参与着色器的几何（`B_CORNER`/`FILLET_D` 是死变量），
  画面圆角走的是 `CORNER`/`圆角`；`BORDER_SIZE` 一直为 0，删掉后画面尺寸的算式里不再乘
  `(1-BORDER_SIZE)`。要恢复就把三行加回 `knobs`，并在 `crtShaderSrc` 里接上。
- 版式：面板宽 `filterPanelW=720`，内边距 `filterPanelPad=48`（原 16 的三倍）**只管左右**，
  上下是 `filterPanelEdge=16`（标题和 ✕ 贴玻璃自己的上沿，底部按钮同理），两列间距
  `filterColumnGap=120`（原 24 的五倍），行高 `filterRowH=26`、行距 `filterRowGap=8`；
  两列各 252 = 标签位 `filterLabelW=144`（原 204 的七成）+ 8 + 控件 `filterPillW=100`。
  **720 是上限，不是硬宽**：`filterPanelWidth(c)` 取 `min(720, 窗口宽-2×16)`，窗口不够宽时
  先压缩两列（标签固定 144、控件 `Grow(1)` 吃掉剩下的宽度），面板右沿永远留得住那 16 的
  内边距——之前写死 720 时窗口稍窄，右侧控件就会贴到窗口边、被裁掉。29 个参数 = 15/14 行，
  768×800 的窗口整张放得下。
- `上下间距 (%)`（`PAD_Y`）只决定**画面可以占多大的盒子**（每边留 0–10%，盒子最大缩到 80%）；
  画面本身按下面的扫描线锁定取整，所以它变大的时候黑边一格一格长，画面不会跟着缩成小数倍。
  它不是「图像尺寸」那种硬缩放，也不是遮黑边。默认 0（=现在的画面）。`crt_test.go` 编译整段 shader 守着它。
- **扫描线锁在画面的比例上：一个游戏像素 = k 个屏幕像素，k 是 1–8 的整数**（2026-11-23）：
  - 画面高度是**核心自己的整条行数的整数倍**：`k = floor(盒高/帧高*(1+INT_TOL))`，再夹到窗口本身
    （宽也要夹）、夹到 1–8。于是每条游戏行固定占 k 个输出行，一条行的亮/暗变化在每一行上都落在
    同样的位置；剩下的不足一行的黑留在边框里。`INT_SCALE`（强制整数缩放）默认 1，就是这一条；
    关掉才回到按盒子收小的小数倍（那才会花）。
  - **取消了「双数」**（同一天下午）：先按偶数取整时，27" 4K 上只有 4×（画面 1194×896，中间巴掌大）
    和 8×（1792，几乎满屏）两档，中间什么都没有。奇数 k 一样是锁死的——锁靠的是「pitch 整除行」，
    不是「行分成两半」，k=3/5/7 每行仍是一个相同的 3/5/7 行花纹。
  - **扫描线坐标用画面自己的像素，不是窗口的**：`dp := vec2(suv.x*pw, suv.y*ph)`。以前是
    `suv*窗口尺寸`，画面只占窗口一半时 pitch 就按两倍的窗口行去数，每行都读到花纹的不同位置 ——
    这是「花掉」的主因，不是 pitch 本身。
  - **pitch 从整条行起算，且必须整除它（或整倍于它）**：`pitch = sc` 起手（k 为奇数时只有它是约数，
    旧写法先摆 2 会得到一个不整除的 pitch），`扫描线间距`（`LINE_PITCH_COEF`）读成「每条行几条扫描线」，
    再在约数里取最接近的（1 和 2 之间取 2；>1.25 倍时取整倍），所以花纹和行永远同一个相位。
  - 行边上 `mod(dp.y, pitch)` 在正好整除时会从两个方向读成不同的相位（暗行变成另一色的荧光条纹），
    所以相位取了 `dp.y + 0.0001`；pitch 小于 3 时没有第三行可翻，绿线的反相也不做，否则整幅发绿。
  - **孔径蒙版的单元按列宽算**：`mcell = k*帧高*GAME_ASPECT/帧宽/3`。4:3 的 384×224 是宽像素，
    按行高算会让三色单元细到半个像素、整幅起彩色噪点；按列宽算才是一列游戏像素一个三色组。
    方形像素平台（掌机/任天堂）上它正好退化成原来的 `k*0.3333`。
  - 横向仍是 4:3 的正确形状，所以一个游戏像素横着是 `0.933*k` 个物理像素，不是整的。
    要严格「k×k 方块」就得按 320×224 的像素自己摆（画面宽 7%，像 RetroArch 的 square pixel），没做。
  - 自检（CPU 光栅器，行均值统计）：k=3 → 画面 896×672、行均值 224 条的标准差 1.6；
    k=4 → 1194×896、1.7；k=6 → 1792×1344、1.7。旧算法（`INT_SCALE=0`）同一帧整幅粉绿大块。
    对照图留在项目目录：`扫描线-改前.png` / `扫描线-改后.png` / `扫描线-3倍-放大.png` / `扫描线-6倍-放大.png`。

- 开关只改 `FilterLevel` 的 `off` 与 `normal`：关掉不丢 `CRTTune`，再打开参数还在；`crtEffect` 每帧读
  uniform，所以开关、拖动、切换都在下一帧生效，不用重启游戏。
- **组管理是卡片**（`tune.go`）：`数据/filters/tune-<n>.json` 一组一张卡，格式
  `{"level": "...", "tune": {...}}`，卡片是底部左侧那条横向滚动条上的小玻璃片，显示组号，
  正在听的那张点亮（`tuneIdx`）。`config.json` 的 `filterTune` 记住最后选中/保存的文件名。
  - 没选中卡片时只有一颗 `保存` = `newTune()`：取最大编号 + 1 新建一组，写完才把卡片加上并点亮，
    写失败不会留空卡。
  - 选中卡片后按钮换成 `更新` / `另存` / `删除`：`updateTune()` 覆盖选中的那组；`newTune()`（另存）
    再开一张新卡并点亮；`deleteTune()` 删掉选中文件、点亮顶上来那张（删的是最后一张就点亮前一张），
    删光则回到 `保存`、参数留在滑杆上不丢。
  - 点卡片 = `selectTune(i)`：读文件，同时套用该组的 `level` 与全部参数（一组是一整套观感）。
  - 按钮失败的信息写在卡片条最左边（`tuneNote`，宽 150），卡片照旧可点；成功后这行清空。
- 面板一打开会重新 `scanTunes()`，手工丢进 `数据/filters` 的文件不用重启就能变成一张新卡。
- 测试：`tune_test.go` 盯“保存建一张卡、再更新还是那一张、另存才多一张”“点卡片套用 level+参数、
  删除后点亮邻居、删光回退”；`filter_test.go` 在 720p 里量两列起点距离 ≥300、最后一颗参数
  （`上下间距 (%)`）仍在窗内、底部不再有说明文字、有卡片时是 `更新/另存/删除` 且卡片在按钮左边、
  700 宽窗口下面板不越界，并钉住 720 / 48 / 120 / 144 / 100 / 26 / 8 这组版式数字。

### 存档面板与按帧回退（2026-11-23）

- 播放界面右上角多一颗 `存档` 胶囊（`saveButton`，在 `控制器` 和 `滤镜` 之间；只在进程内跑核心的游戏上出现，
  交给 RetroArch 自己窗口的游戏没有状态可存）。点开是同一套玻璃面板（`savepanel.go`，宽 596、内边距 24），
  上半是 **3×3 九格截图票位**，下半是 **时间轴 + 读取/删除**。
- **打开面板 = 暂停**：`toggleSaves()` 与 `filtersOpen`/`padsOpen` 互斥，并 `a.core.Pause(true)`；关闭
  （✕ / Esc / `继续` / 再点胶囊）走 `closeSaves()`：清空回退行、`Pause(false)`。暂停由核心自己实现
  （`session_windows.go` 的 `paused`/`stepOnce`）：暂停时仍走 timing 和 `onFrame`，所以页面照常重绘、
  任务队列照常被吃掉；`Step()` 只放一帧。
- **票位**（`saves.go`）：一个槽 = 三个文件，落在 `数据/存档/<平台>-<rom名>/` 下，文件名 `save-<n>.state`
  （核心的整份序列化）、`save-<n>.png`（拍照，`shrink` 按块取均值缩到 ≤320 宽，不引 `x/image`）、
  `save-<n>.json`（`{"at":"RFC3339","frames":n}`）。`scanSaves()` 在面板打开时重读，手工丢进来的档也能用；
  缩略图按路径缓存在 `a.shots`，点 `删除` 时释放。空位显示 `＋ 建档`，**点空位 = 立刻存档**；
  有图的位置点一下只是**选中**（`slotSel`），读取/删除要再按按钮——一次点击不会既存档又覆盖。
  `读取` = `loadFrom()`：`LoadState` 进去 + `Step()` 一帧让玻璃显示那一瞬，并把回退行重置。
- **回退行**（`rewind.go`）：`OnFrame` 里每帧调 `captureRewind()`，在核心线程、`retro_run` 之后调
  `SaveStateNow()` 存一份。**分段以 100ms 计，不是按帧**：`rewindPlan(size,fps,cost)` 里
  `step = round(fps × 0.1)`（60fps = 6 帧、50fps = 5 帧），`max = 2 秒 / step`（两条夹：128MB 与 240 条）。
  成本超预算时**整段往上抬到下一个 100ms**（一个状态要 15ms 写不完一次 100ms，就变 12 帧 = 200ms），
  所以读数永远是整百毫秒，不会出现 133ms 这种碎数。注意 `Do()` 一次往返固定 ≈16ms（一个帧边界的延迟），
  预算只看核心线程内的 raw 成本。
- 时间轴（`rewindTrack`）：一条与条目等距的条，读数 `已回退 200 ms`、旁边 `覆盖 2.0 秒 · 每格 100 ms`
  （`Span` 按整段算：20 段 = 2 秒，不是首尾相差的 1.9 秒），`继续` 回到实时。**拖动就是回退**：
  按住轨道往左拖，按 `dx / (轨宽/(条数-1))` 映射到条目（`scrubDrag` 记起点条目与累计位移），
  每落到一条就 `rewindTo(i)` = `LoadState` + `Step()`。往回拖到底是最早的一条，往右拖到底回到最新。
- **不能存档的核心，格子上就是一个 ✕**（`canSave()` / `savable()`）：核心 `retro_serialize_size` 答 0，
  或者第一次捕获就 PB 回来的空格子（回退行记成 `planned && step<=0`），九格全画 ✕ + `不支持`，
  点它只在底部说一句 `这个核心不能存档`，不写任何文件；没有游戏时（`a.core == nil`）不判这一条。
- **核心给不给存档，是量的不是猜的**：`retro_serialize` / `retro_unserialize` 是
  `(void *data, size_t size)`，两个参数都得传。少传 `size` 时寄存器里是个野值，核心会答“没写出”
  （`mednafen_psx` 就是这么被骗的），别的核心运气好才成——`state_windows.go` 里两处都补上了 `uintptr(len(buf))`。
- 实测（`cmd/zzstate` 探针，Harmful Park，120 帧后连存六次 + 暂停后的队列往返）：

  | 核心 | 声明 | raw 存 | raw 读 | 队列（暂停时）|
  | --- | --- | --- | --- | --- |
  | `fceumm` | 22KB | ~0 | ~0 | 16ms 一个帧边界 |
  | `snes9x2010` | 826KB | 225µs | 456µs | 同上 |
  | `mednafen_psx`（PSX 现行） | 16MB | 10–15ms | 0.5–1ms | 18ms 存 / 17ms 读，字节数对得上 |
  | `pcsx_rearmed` | 4.4MB | 1.5–7ms | 1.5ms | 18ms |
  | `duckstation` | — | 在本进程里 `retro_run` 直接崩（GL 上下文），所以 PSX 走 `mednafen_psx` | | |

  PSX 裁剪后一份 3.6MB、成本 10–15ms，所以分段抬到 200ms：10 条、覆盖 2 秒，占 36MB。
  `fceumm` / `snes9x2010` 都是 100ms × 20 条。
- **快照的体积**（`state_windows.go` + `saves.go`）：
  - **先裁掉核心没写的那截**。`retro_serialize_size` 是“最多可能写多大”，不是“写了多大”：
    `mednafen_psx` 声明 16MB，实际只写 **3.75MB**。所以存档前 `clear(buf)`，写完后
    `trimState()` 剥掉尾部全零的填充，状态就只留核心真的写过的那段；读档时按核心声明的
    大小用零补齐再喂 `retro_unserialize`（补的是零 = 存的时候那截就是零，内容完全一致）。
    实测“裁剪后的块直接喂进去也能读”（`load trimmed=<nil>`），但补齐更稳妥。
  - **落盘再 deflate**（`writeStateFile`/`readStateFile`，前置标记 `GRZ1`，老的无标记文件
    仍按裸块读）：`flate.BestSpeed` 就够，面板是暂停状态下写的。

    | 核心 | 声明 | 裁完 | 落盘（deflate1） | 存/读耗时 |
    | --- | --- | --- | --- | --- |
    | `mednafen_psx` | 16MB | 3.75MB | **379KB（10%）** | 8ms / 12ms |
    | `pcsx_rearmed` | 4.4MB | 4.38MB | 1.24MB（28%） | 18ms / 25ms |
    | `fceumm` | 22KB | 22KB | 2.6KB（12%） | ~0 |
    | `snes9x2010` | 826KB | 826KB | ~? | 225µs |

    也就是 PSX 的一个存档从 16MB 变成 379KB（42 倍）。
  - 回退行里的状态**不压**：那是每段一次的热路径，deflate 的 8ms 加在 10ms 的采集上
    会把 PSX 拖慢；裁剪后的 3.6MB × 10 条 ≈ 36MB，仍在 `rewindBytes` 的 128MB 以内。
  - 副作用：`rewindPlan` 用的是裁剪后的长度，16MB 不再撑满 128MB 上限，PSX 从“8 条、
    被字节上限卡住”变成“10 条、每段 200ms、正好 2 秒”——体积小下来之后历史够用了。
- 退游戏时 `releaseGame()` 清 `savesOpen/slots/shots/slotSel` 并 `a.rew.reset()`；`launchGame` 开局前同样重置，
  回退行永远只属于当前这颗核心。
- 测试：`rewind_test.go` 盯 `rewindPlan` 的三种核心（22KB / 16MB / 6ms 成本）、行的裁剪与 `−N 帧` 读数、
  `freeze` 与 `reset`；`savepanel_test.go` 盯九格布局（`建档` 数 = 9 − 已存档、槽位时间、`时间轴/继续/读取/删除`
  都在窗内、✕ 不越过玻璃）、点存档位选不建档、无游戏时点空位只说原因不落文件、无存档核心的九格全叉，
  以及 `fileSafe`/`shrink`/`savable`。

### 存档与时间轴分开判（2026-11-24 实测）

**九格存档**只看核心有没有状态：`retro_serialize_size` 答 0，或者第一次捕获写出来是空的，就画 ✕。
**时间轴**还要看一份状态多大、写一份多慢：`rewindPlan` 现在先判两条门槛——一份状态超过 `rewindMaxState`
（64MB，即字节预算的一半）或写一份超过 `rewindCostCeiling`（300ms）就直接答 0/0，一条都不留；
`max < 2`（两秒的跨度里凑不出两条）也答没有。这样 100MB 级的状态（Dolphin 107MB、1.2 秒一次）
不会每段卡一下机器。**超界的核心只丢时间轴，九格照旧**：`rewind.noState` 只为「核心根本没状态」置位，
`savable()` 读它，而不是读 `step <= 0`。

`cmd/corecheck` 逐核心量的结果（一份状态 = 核心声明的大小；存/读都在 `Do` 的任务里量；秒数是捕获一次的开销）：

| 系统 | 核心 | 起画面 | 存档 | 一份状态 | 存 / 读 | 时间轴 |
| --- | --- | --- | --- | --- | --- | --- |
| FC | `fceumm` | ok | 是 | 22KB | 0ms / 0ms | 100ms × 20 |
| SFC | `snes9x` | ok | 是 | 823KB | 14 / 19 | 100ms × 20 |
| GB | `gearboy` | ok | 是 | 385KB | 17 / 64 | 100ms × 20 |
| GBA | `mgba` | ok | 是 | 528KB | 16 / 17 | 100ms × 20 |
| NGP | `mednafen_ngp` | ok | 是 | 31KB | 16 / 16 | 100ms × 20 |
| MD | `genesis_plus_gx` | ok | 是 | 145KB | 19 / 17 | 100ms × 20 |
| SS | `mednafen_saturn` | ok | 是 | 4.7MB | 19 / 17 | 200ms × 10 |
| PS1 | `mednafen_psx` | ok | 是 | 3.75MB（声明 16MB） | 104 / 13 | 900ms × 10 |
| PS2 | `pcsx2` | ok（约 2 秒后出画） | 是 | 50MB | 114 / 69 | 1.1s × 2 |
| PSP | `ppsspp` | ok（约 2 秒后出画） | 是 | 36MB | 102 / 66 | 500ms × 3 |
| DC / NAOMI / Atomiswave | `flycast` | ok（NAOMI 先出 GD-ROM 标） | 是 | 27–59MB | 100–365 / 62–182 | 1s × 2 |
| 3DO | `opera`（730KB 2026-11 构建） | ok（约 3 秒后出画） | 是 | 6.6MB | 24 / 33 | 200ms × 10 |
| DOS | `dosbox_pure` | ok | 是 | 1.4MB | 17 / 31 | 200ms × 10 |
| PC98 | `np2kai` | ok | 是 | 10.7MB | 77 / 62 | 500ms × 3 |
| 3DS | `citra` | ok（约 2 秒后出画） | 是 | 12.7MB | 531 / 1036 | **没有**（只有九格） |
| Wii / GC | `dolphin` | ok | 是 | 107MB | 1232 / 935 | **没有**（只有九格） |
| 街机（FBNeo 认识的 romset） | `fbneo` / `fbneo_plus` | ok | 是 | 17–415KB | 16 / 17 | 100ms × 20 |
| 街机（MAME 认识的 romset） | `mame` | ok | 是 | 0.5–2.4MB | 17–34 / 16–21 | 300ms × 10 |

- **FBNeo 的「白屏」是它自己的错误画面**：`Romset is unknown.`，640×480、124 色、`declare=0`。
  所以量 FBNeo 不能只看 `status=ok`，要 `declare > 0` 才算真的进游戏。
- `mame2010`：`retro_serialize_size` 答 0（没有存档），`retro_unload_game` 会挂住，退出游戏时不再用。
- `mame2016`：`retro_load_game` 之后第一次跑就 `0xc0000005`，整个进程一起没，已停用。
- `mamearcade`：buildbot 上没有这个核心，只能在这台机器上用，启动 19 秒，已停用。
- `fbalpha`：能用、能存档（269KB / 16ms），但 Windows 独占且已停更，逐步让位给 `fbneo`。
- **关不掉的核心**：`flycast`、`mame2010` 的卸载能卡几十秒。`Session.Close` 把 `retro_unload_game`/`retro_deinit`
  放到独立 goroutine，4 秒没回来就交还窗口（核心线程泄漏，但不再把前端冻在播放页）。
- **MAME 系核心读 rom 路径像命令行**：`rom/majiang/80年/7toitsu.zip` 会被读成系统名「80年」外加一个杂项参数
  （日志 `System not found: 80年` / `Error: unknown option`）。`core.Start` 里 `asciiRomPath` 先把它硬链接到
  `<盘符>\GAMEROOM\roms\<名字>-<路径哈希>\<文件名>`（同一份字节，文件名就是 MAME 要的 driver 名），再交给核心；
  同一文件夹里的其他文件（父集、BIOS）也一起链接进这个子目录，因为 MAME 只从“被交出去的那个文件所在目录”找父集。
  带空格或非 ASCII 的 MAME rom 都走这条（`Sim ui_D` 本身带空格，所以放在库里 `rom\merge` 下的成品也走镜像）。
  别的核心不镜像。两个同名补丁（都叫 `xymg.zip`）靠路径哈希分到不同子目录。
- **缺父 romset 的街机/麻将**：MAME 日志 `xxx NOT FOUND (tried in <clone> <parent>)` + `Required files are missing`
  说明这个 zip 只是 clone，父集（如 `mgakuen.zip`、`mjgottsu.zip`、`qsound.zip`）不在库里，任何核心都起不来；
  这是 rom 数据缺件，不是核心没配好。先到本机三个老包（`0_ARCADE265`、`groovymame`、`安卓麻将`）里找父集，
  找到就按 `rom\merge` 的规矩合并；找不到才是真缺。

#### 街机与麻将所有权的重排（2026-11-24）

旧库把 267 个街机游戏摊在 `fbalpha / fbneo / fbneo_plus / mame / mame2010 / mame2016 / mamearcade /
flycast / geolith / mednafen_saturn / opera` 十一个核上，`sim` 表的 `name` 和 `core` 还对不上
（名为 `MAME 2010` 的 sim 其实是 `mame`，名为 `fbneo_plus` 的 sim 其实是 `mame2016`），`arc` 没有默认 sim，
于是 130 个没有 `board` 的游戏落到哪颗核心全看取第一条 sim 的顺序。重排的依据是逐游戏实测：

- 判定标准：`status=ok` 不够，`declare > 0` 才算真的进游戏。FBNeo 不认识 romset 或 rom 缺文件时
  也回 `status=ok`，画的是自己的 FBNeo Error 白屏（`Romset is unknown.` / `missing files for THIS VERSION`），
  `declare=0` 正好可以把它和真游戏分开。
- 结果：`arc` 267 个 → `fbneo` 174、`mame` 57、`flycast` 26（NAOMI/Atomiswave 一律 flycast，
  MAME 对这批只认 `mvsc2`/`vf3` 两个）、`fbalpha` 4、`fbneo_plus` 2、`mame2003_plus` 2、`mamearcade` 1、`geolith` 1。
  `majiang` 81 个 → 全部 `mame`（**75 个能起，6 个底包全机器都没有**）。
- 为救回 9 个 `fbneo`/`mame` 都起不来的改版，留了四颗“补位核”：`fbneo_plus` 接 CPS3 的 `sfiii3`、STG 的 `hachamfb`；
  `fbalpha` 接 `wofch`/`alienchac`/`samsho4sp`/`pwrinst2`；`mame2003_plus` 接 `galpans2`/`volfied`；
  `mamearcade` 接 `kov2nl_302cn`。这九款都实测能存（一份状态 166KB–1.9MB）。`fbalpha` 是 Windows 独占且停更，
  `mamearcade` 不在 buildbot 上，只为这几款留着，别当默认核。

### 哪些核心“忽略 CRC”（2026-11-24 实测，改核前先看这张表）

- `mame` / `mame2010` / `mame2003_plus` / `mamearcade` / `mame2003_midway`：**忽略CRC**。CRC 不符只打一条
  `WRONG CHECKSUMS` 警告，照常跑；真正致命的是 `NOT FOUND` / `Required files are missing` / `WRONG LENGTH`。
  所以“文件齐但 CRC 不同”的汉化包、bootleg 在这几颗上能起，缺件不能。
- `fbneo` / `fbneo_plus`：默认**不忽略 CRC**，但选项文件 `config\FinalBurn Neo\FinalBurn Neo.opt` 里
  `fbneo-allow-patched-romsets = "enabled"` 一直开着，等于对“补丁包”放行 CRC；缺文件、文件名不对仍然白屏。
  要临时试开关用环境变量 `GAMEROOM_CORE_VAR='fbneo-allow-patched-romsets=true'`，不必改代码。
  注意：那份 `.opt` 实际上**从来没被加载**——`core.loadCoreOptions` 兜底用 `name+"_"` 前缀匹配，
  FBNeo 的键是 `fbneo-`（连字符）。所以跑 ips 补丁时是 `launchGame` 通过 `core.Start(...)` 的变参
  逐个传 `fbneo-allow-patched-romsets=enabled`（见「IPS 补丁」一节），不靠 opt 文件。
- `fbalpha` / `fbalpha2012` / `fbalpha2012_neogeo`：**不忽略 CRC**，且 `fbalpha2012` 对不认识的 set 名直接
  `retro_load_game` 返回 false。

### 缺件靠“合并底包”补，不换核

- `rom\merge\<平台>\<游戏>\<驱动名>.zip` 是合并出来的成品：补丁包（`无作弊版\xxxxnc.zip`）+ 底包
  （`90年\xxx.7z` 等）解出来叠在一起再压回去，文件名用驱动名（MAME 用文件名当系统名）。
- 需要父集的（`7toitsu`←`mgakuen`、`bakuhatu`←`mjgottsu`、`punisherbz`←`punisher`）就把父集放在同一目录。
- 底包不止 `Sim ui_D\rom` 里有：`E:\game\0_RetroArch_ARCADE265\RetroArch\games\ARCADE`、`0_emu\groovymame\roms`、
  `0_emu\安卓麻将\...\RetroArch\roms` 里还有完整 set（`vf`、`tekken3`、`galpani4`、`ssf2t`、`mgakuen`、`mjgottsu`、
  `wlcc`、`mjelct3` 等），先在这三个包里找一遍再判“缺件”。
- 库里这些游戏的 `rom_path` 已指到成品（`menu`/`title`/封面没动，货架不变）。
- `mame2016` 已改名为 `mame2016_libretro.dll.disabled-20261124`（它 `retro_load_game` 后第一次跑就
  `0xc0000005`，整个进程一起没）；`mame2010` 不再被任何游戏指向。
- `sim` 表按 (platform, core) 重建（`cmd/fixsims`）：名字跟核心走、`exe` 指回 `模拟器/RetroArch/retroarch.exe`、
  `args` 统一 `{RomFullPath}`，不再有指向错核心的 sim；`arc` 默认 `fbneo`，`majiang` 默认 `mame`。
- 维护工具：`cmd/audit`（平台×核心×sim 的账，`MISSING` 会点出库里不存在的 rom）、`cmd/corecheck`
  （单核心单 rom 的启动/存档实测，`-png` 存一帧）、`cmd/coreplan`（按 plan.tsv 改归属，`-ps` 把 PS1 换成
  mednafen_psx）、`cmd/fixsims`（按 plan 重建 sim 表和归属）、`cmd/boards`（按 board 分组的账）。

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

## DOS&PC98 货架（`internal/library/dos.go` + `-import-dos`）

老合集 `E:\game\0_GALGAMEonDOSUI` 里有 **6 套独立 DOSBox**（X / X32 / ECE / GOG / Staging / SVN Daum）
＋ `np2fmgen` ＋ 自带一份 RetroArch（`dosbox_core`、`dosbox_pure`、`np2kai` 三支核心）＋ 一个
`loader.bat` 按 7 种 `dostype` 拼命令行。这套东西只在 Windows 上成立，而且没有 shader、没有窗口接管，
所以**一套都不进本工程**，全部换成 RetroArch 的两支跨平台核心：

| 机种 | 平台 key | 核心 | 内容 | 老合集里的对应 |
| --- | --- | --- | --- | --- |
| DOS | `dos` | `dosbox_pure` | 原样 zip | `RA-DOS-pure`（`sim` 表里本来就有这条） |
| PC98 | `pc98` | `np2kai` | `.hdi/.fdi/.d88` 镜像 | `RA-core-NP2kai` |

导入：`exe\GameRoom.exe -import-dos "E:\game\0_GALGAMEonDOSUI"`

- **只动这一个货架**：`ImportDOS` 删的是 `platform_key in ('dos','pc98')` 的行，
  `library.Import`（读 `data.dll` 的那个）会 `DELETE FROM game/menu/sim/platform` 全表重写，
  **不能拿它导 DOS**，否则 936 局街机/家用机一起没了。重复跑 `-import-dos` 安全。
- 元数据来自 `list.csv`（**UTF-8 带 BOM，不是 GBK**；列名 `文件名,游戏名称,类型,发行日期,发行商,地区…`），
  按 `文件名` 去掉扩展名匹配 rom 文件名；`title` 存 rom 名、`name` 存中文标题（和街机那套一致）。
- 表里没有的游戏（`list.csv` 只有 83 行里的 81 行）从文件名里挖 `(1994)(HimeyaSoft)`：
  `fromRomName` 补年份与厂商，`(TW)/(JP)` 这类两位大写地名不当厂商；`titleFromRomName` 把
  `Ash. (1994)(HimeyaSoft)(TW)[cr]` 收成 `Ash.` 当显示名（`title` 列仍存完整 rom 名）；
  `makerAlias` 把 `天堂鳥→天堂鸟` 这类简繁两种写法并成一个 chip。
- PC98 的 zip 必须**先解出镜像**（`7z x -o"rom\pc98\<游戏名>" *.hdi *.fdi *.d88`），
  `imageIn()` 从游戏目录里挑镜像——所以一个游戏一个目录。
- `np2kai` 要 `模拟器/RetroArch/system/np2kai/`（`bios.rom`、`FONT.ROM`、`font.bmp`、
  `2608_*.wav`、`np2kai.cfg`）。老合集那份 `np2kai.cfg` 里硬写了别人的
  `HDD1FILE = D:\DOSBox\...\MEI.hdi`（会把某款游戏强行挂在 C 盘）和 `fontfile` 绝对路径，
  **已清空并改到本工程路径**。
- PC98 六局的实机状态（各跑 30～70 秒截图核对）：`NANPA2.hdi`（Doukyuusei 2 PC98 版，进到日文
  字体检测+状态栏）、`FUKEI.fdi`、`MEI.hdi`（`2003 WWW.AGAGOLF.COM` 开场）、`MFIGHT.hdi`、
  `SHIKUMI.hdi` 都起来了；只剩 `TFKIKOU.hdi` 仍是纯黑。注意 **np2kai 的帧是居中渲染**的，
  按 DOS 那种左上角去裁剪会误判成黑屏。np2kai 会**直接写镜像**存档（本工程没有 overlay）。
- DOSBox Pure 默认起来是它自己的 START MENU（列出 zip 里的 exe）。本工程不用这套：**双击直启**
  靠 `internal/library/doslaunch.go` 在导入时往每个 zip 根目录写一份 `DOSBOX.BAT`
  （Pure 官方机制：内容里有 `DOSBOX.BAT` 就**跳过菜单直接执行**，前端无关）。
  批处理正文来自该 zip 自带的根级 `dosbox.conf` 的 `[autoexec]`——老合集作者给每款游戏调好的
  启动命令（`cd IF2` / `call PLAY.BAT` / `imgmount D .\CD\BG.cue -t cdrom` / TSR 串），
  只去掉 `mount C .`（Pure 自己把 zip 挂在 C:）和 `exit`（会把会话关掉）。
  - 结果：**77/77** 双击直接进游戏（已实机验过 `3x3 EYES`、`Cobra Mission`、`Blue Garnet` 的
    `imgmount`、`Tenchi Muyou`、`Doukyuusei 2 Special Disc`）。
  - 导入时**静态校验**批处理指向的文件真的在 zip 里（`checkCommands`），指空就**不注入**，
    宁可留在菜单也不给一个死命令行；`call CM` 这种会自动补 `.BAT/.COM/.EXE` 再找。
  - 没有 `dosbox.conf` 的包按老合集 `loader.bat` 的那几条规则挑命令（顺序一致）：
    `play.*`（bat/exe/com，任意目录）→ `<目录>/<目录>.bat` → 全包唯一一个 exe/com →
    全包唯一一个 bat，`install/setup/readme/help/inst/dos4gw` 不算。这两条规则是从
    `E:\game\0_GALGAMEonDOSUI\loader.bat` 原样搬过来的，老项目就是靠它们给没有 conf 的包配音命令。
  - 启发式也猜不出来的包，命令写在 `doslaunch.go` 的 `launchOverrides`（按 zip **小写文件名**
    作 key，值是命令行切片），一次一个游戏地补。现在只有一条：`Doukyuusei 2 Special Disc`
    是 DOS/V 游戏，要先 `dosvfont -f:font.dat -tj` 再 `play5v` 才轮到
    `ai5v start.mes H x C`——这三行没有一条能从包里读出来。
  - **Pure 的 zip 驱动只认 store / deflate**：7-Zip 用 LZMA（method 14）压的老包，核心挂上去后
    那些文件在 `dir` 里**根本不存在**，游戏看起来"一个 exe 都没有"。`Doukyuusei 2 Special Disc`
    374 个条目里 241 个是 LZMA，所以它连菜单里都找不到 exe。现在 `unreadableEntries` 在导入时
    检查并报出来（不注入死命令），这个包已经用 deflate 重新打包（内容不变，条目数不变）。
  - 幂等：zip 里已有 `DOSBOX.BAT` 就跳过；改 zip 用 `zip.Writer.CreateRaw` 原样搬压缩字节，
    **每个条目 CRC 不变**（只有新增的 60 字节批处理是新的），重打包 1.6 GB 也不重压缩。
  - 注意：注入会改 zip 内容，用户重新从老合集拷原包回来会把这层"直启"覆盖掉——
    重跑一次 `-import-dos` 就补回来。

## 外部程序的滤镜（`takeover_windows.go`）

RetroArch 之外的程序没有 shader，靠 `模拟器/Magpie` 这份捕获层补：

1. `launch.Start` 起进程后调用 `launch.TakeOver(pid, exeName, …)`（goroutine，别放在 UI 线程）：
   **RetroArch 也走这条路**，只是不带 Magpie（`MagpieFilter{}`）——它自己用 shader 出 CRT，
   但它同样需要**被置前并拿到键盘**。不置前时 RetroArch 起在库窗口后面，玩家看到的"点了才动"
   "键盘鼠标没反应"，其实是按键全进了 GameRoom 的窗口。
2. `front` 置前不是只 `SetForegroundWindow`：非前台进程直接调它，Windows 会**静默拒绝**。
   现在是 `ShowWindow(restore)` → `BringWindowToTop` → `AttachThreadInput(当前线程, 目标线程)`
   → `SetForegroundWindow` → `SetFocus` 再解挂；这是 Windows 认的"我是故意的"写法。
3. 另：`retroarch.cfg` 里 `input_auto_mouse_grab = "true"`，鼠标类 DOS 游戏进画面就抓住指针，
   不用再点一下窗口。
4. 非 RetroArch 的分支再补 Magpie：
   `waitWindow` 每 250 ms 枚举一次顶层窗口，30 秒超时；先按 PID 匹配，再按**同名的进程**匹配——
   Windows 11 的 Notepad、以及各种启动器会把窗口移交给另一个进程，光看 PID 会抓空。
   置前之后 `scaleHotkey` 用 `keybd_event` 发 `Win+Shift+A`（Magpie 的默认缩放热键，
   全局钩子，所以要发在游戏窗口置前之后）。Magpie 没运行时先 `configureMagpie`
   （把 `%LOCALAPPDATA%\Magpie\config\v5\config.json` 的 `profiles[0].scalingMode` 指到
   `scalingModes` 里名字为 `CRT-Geom` 的那一项），再启动它，等 2.5 秒把 Magpie 自己的窗口
   最小化，免得盖住游戏。找不到 Magpie 也不报错：只做窗口置前，游戏照常跑。

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



## 控制器面板（`internal/app/pad.go` + `padview.go`）

播放页右上角两颗胶囊：`控制器`（跟着当前游戏，显示机种/基板名）和 `滤镜`。面板是**图形化手柄**，
点模型上的某一个按键就进入抓键态，按键盘键或手柄键即写入绑定；**每个平台一份通用配置**（`数据/controls.json`），
该平台所有游戏默认共用，也可以给某个游戏另存一份专属配置（见下）。

- 平台名就是货架里的**基板名规范化后的 id**：`CPS1/CPS2/CPS3→cps1/cps2/cps3`（CPS3 面板显示 `CPSIII`）、
  `MVS→mvs`、`ST-V→st-v`、`NAOMI GD→naomi-gd`；认不出的基板名回落 `arcade`。
  `layoutID()` 把小写、空格换 `-`，`library.BoardKnown()` 判断这名字到底是不是一块基板。
- 按钮数由基板决定：CPS1 六键（街霸 II 就是三拳三脚，CPS1 也吃六键面板）、MVS 四键（`A+B`/`C+D` 组合）、其余街机六键（`ABC`/`DEF` 组合）。
  组合键是一个 spot 按多个 RetroPad id（`padSpot.Drives`），不是额外的物理按键。
  **MVS 的四个键排成实机那种阶梯**（`arcadeFour()`：A 最低在左，B、C 各上一级，D 与 C 平齐靠右，
  每级 x+40 / y-23；2026-11-24 玩家画图定的），不是 2×2 方阵；画面里的四键非 SNK 机台（三国战纪那类）走 `arcadeNumbered(4)`，仍是 1/2/3 下排 + 4 上排。
  **CPS1 里的六键机台（街霸 II）会在启动时按核心公告升级成六键面板，见「按键顺序按核心来」。**
- 主机：FC/SFC/GB/GBA/NGP/MD（六键）/SS（六键，L/R 走 L2/R2）/PS1/PSP/3DO。
  SEGA 文件夹按路径分辨：`/ss/`→SS，`/model 2|3/`、`/stv/`、DC→`arcade`，其余→MD。
- 六键不是按 RetroPad 的“直觉”叠的，要看核心：MD 的 A 读 Y、C 读 A、X/Y/Z 读 L/X/R；
  SS 的 A 读 B、B 读 A、C 读 R、Z 读 L。改动这些前先核对 `genesis_plus_gx` / `mednafen_saturn`。
  **这条现在有了机器可读的来源，见下节：能问核心就别再靠手写记忆。**
- 输入：`pushPad()` 每帧把两份掩码交给 `core.Session.SetPads`（port 0/1），核心按 `retro_set_input_state` 的 `port` 回答；
  面板自己读键用 `GetAsyncKeyState`、读手柄用 `XInputGetState`（`xinput1_4` → `9_1_0`/`1_3` 回退），四个手柄都读。
- 面板不自己切平台：顶部只显示当前机种名（居中），没有切换条；Esc 关面板。

### 游戏专属配置（2026-11-23）

- 控制器面板在标题下多了一行 `配置`：`+ 专属` 给当前运行的游戏复制一份自己的绑定，旁边的 `通用` 开关决定面板和核心读哪一份。
  `通用` 开 = 这台机器所有游戏共用的旧配置；`通用` 关 = 当前游戏的专属配置。没有专属配置时开关不会伪造一份，只提示先按 `+`。
- `controls.json` 在 `layouts` 之外新增 `games`（按 `game:<game.ID>` 存该游戏的绑定，1P/2P 的裸 id 与 `2p:` 规则不变）
  和 `generic`（`game:<id>: true` 表示这个游戏仍读机器通用配置）。旧文件只有 `layouts`，照原样加载，不需要迁移。
- `padProfile` / `padProfile2` 都走 `padBinds(layout)`：有专属且 `通用` 关时读 `games`，否则读 `layouts`；
  `bindPad` / `resetPadPlayer` 也只写当前选中的那一份。`savePads` 推两张键盘表，所以切 `通用` 或改专属绑定都会在下一帧到达运行中的核心。
- 加专属时复制机器通用那一份，之后互不影响；`+` 不会重复建第二份，已有专属时只切回它。新游戏启动后若已有专属且 `通用` 关，会直接读专属。
- 测试：`pad_test.go` 新增“复制通用后互不影响”“通用开关切回原配置”“两个游戏各存各的”“重启后专属与通用选择都还在”。

### 按键顺序按核心来（2026-11-24）

**别再用一张手写表赌核心的顺序。** 核心加载后会通过 `SET_INPUT_DESCRIPTORS`（环境命令 **11**）把自己每个
RetroPad id 对应的**机器按键名**交上来，这才是权威。`core.Session` 把它收进 `descs`，`Session.InputDescs()` 取用；
`GAMEROOM_PADMAP=1` 跑 `cmd/corecheck` 会把这张表打到 stderr（`padmap port=0 device=1 index=0 id=1 Weak Punch`）。

实测（都与 `padLayout` 的手写表对过）：

| 核心 / 机种 | 实际顺序 |
|---|---|
| `fbneo` / `fbneo_plus` / `fbalpha`（**公告晚到**） | 公告的是一台「6 面板」机台：轻拳=Y、中拳=X、重拳=L、轻脚=B、中脚=A、重脚=R；MVS 是 A=B、B=A、C=Y、D=X；`dino` 这种无名机台按公告顺序。**它的 `SET_INPUT_DESCRIPTORS` 在第一帧之后才给**，启动那一刻读到的是空表——所以启动后必须再读一次（`padLate`），否则面板照自己画的顺序发，1 会变成 4（2026-11-24 玩家实测：按 123 出 4、1、5） |
| `mame` | 只报 RetroPad 名（A/B/X/Y/L/R），**位置映射**，面板原样 |
| `mame2003_plus`（街机默认核，街霸就在这里跑） | 报的是带编号的机器名：`B1: Jab Punch…B6: Roundhouse Kick`。**核心自己把 2/3 接反了**：`B1=B(0)`、`B3=Y(1)`、`B2=A(8)`、`B4=X(9)`、`B5=L`、`B6=R`；方向键名也不是 Up/Down 而是 `Jump(4)/Crouch(5)/Left(6)/Right(7)` |
| `fceumm` / `snes9x` / `genesis_plus_gx` / `mgba` / `pcsx_rearmed` | 报的是机器自己的名字，面板原有顺序**本来就对**（md 的 A=Y、C=A、X=L、Y=X、Z=R；PS 的 △=X、□=Y、○=A、×=B） |
| `mednafen_ngp` | **A=B(0)、B=A(8)**，面板原来画反了；`Option`=3（Strat），没有 Start |

- `padIDsFor(padLayout, descs)` 在**每次启动**算一张 `spot id → 要按的 retro id`（`map[string][]int`），
  存 `a.padIDs`，`maskOf` 按它出掩码；`nil` 表示核心的说法不可用（无公告/只报 RetroPad 名）→ 照旧用面板自己写的 id。
  解析规则：**先看名字里的机器编号**（`buttonNumberOf`：`B1:` / `Button 2` / `Fire 3` / `P1_BUTTON1`），数字 spot 直接按编号取 id
  ——这条是给 `mame2003_plus` 兜的，它把 B2/B3 接反，只有编号能对齐；`Buttons 3x Punch` 是三键组合、不是 Button 3，正则要放过。
  编号取不到才按名字找（`Weak Punch…`、MVS 的字母 `Button A…`）；都对不上但核心有按钮名 → 按公告顺序依次填数字 spot（`dino` 那种）；
  组合键（ABC/DEF、A+B、C+D）按它引用的那几个 spot 解析后的 id 去按，所以“三个拳”永远还是三个拳。
- **公告的时机也是坑。** `mame2003_plus` 在 `retro_load_game` 里就给了表，`fbneo` 要等第一帧；所以启动时读到的表可能是空的，
  必须在帧回调里再读一次重算（`padLate`：只在第一次拿到非空表时算，重写 `数据/按键自检.txt` 的表头）。
- 结算原则：**玩家的实测永远高于核心的自述，但先确认自己读到的是不是真的那张表。**
- 诊断入口：`数据/按键自检.txt`，每次启动覆盖写一份（游戏、核心、核心公告、面板每个键的键盘/手柄绑定、要按的 id），
  之后每按一下记一行「命中哪个 spot（pad/vk）→ 发给核心 held 」。玩家说“按键错”时先要（或直接读）这个文件，别再靠猜。
- **公告顺序只在核心一个编号都没报时才能用。** `mame2003_plus` 的公告里混着 `Jump/Crouch`，把它们算进顺序会让
  面板的 3、4 键变成“跳”和“蹲”（玩家看到的就是“按键映射到方向了”，2026-11-24 踩过）；有编号的核走编号，这条就不会触发。
- `padPanelFor` 用核心公告**决定画几个键，两个方向都跟**：`sf2ce` 六键机台把 CPS1 的三键面板撑成六键，
  MVS 的四键也压得住一块被错标成六键的基板（2026-11-24 玩家实测：fnbeo 下 SNK 游戏画的是六键）。改编后的面板 id/名字仍是原基板的，
  绑定沿用；`arcadeNumbered(n)` 画 1…n（1 2 3 下排，4 5 6 上排）。
  核心按字母报键（`Button A…D`，或 NeoGeo CD 那种裸 `A/B/C/D`——C、D 是 RetroPad 没有的字母）→ 直接上 MVS 四键（`A+B`/`C+D`），
  名字改叫 `MVS / NEOGEO`；报 `A B C X Y Z` 的 MD 不会被误判（见到 X/Y/Z/L/R 就不算 NeoGeo）。
  `fireButtons()` 只认 `Button N`/`fire N`/拳脚名/带机器编号名（`B1:`），FCEUmm 的 `(FDS) Disk Side Change`、`Turbo A`、`A+B` 之类不算，
  否则一台 NES 会被撑成街机六键面板。
- 面板画的是 `a.padPanelNow()`（本次启动的面板，没有游戏时回落 `padLayoutFor(a.padLive)`）；
  `maskOf`、`padPanel` 都用它，否则六键面板画出来了、键却按不出来。
- 想加新平台/新核心：先 `GAMEROOM_PADMAP=1 go run ./cmd/corecheck -core <core> -rom <rom> -secs 2`，
  看 id 与名字，再决定要不要写进 `padLayout`；能由 `SET_INPUT_DESCRIPTORS` 决定的，就别写死。
- 测试：`padmap_test.go` 里 `TestFBNeoStreetFighterOrder` / `TestFBNeoNeoGeoOrder` / `TestFBNeoUnnumberedMachine` /
  `TestMAME2003PlusStreetFighterNumbersItsButtons` / `TestMAMEKeepsTheDrawnOrder` / `TestNeoGeoPocketOrder` /
  `TestNESPadsStayNES` / `TestStreetFighterGetsSixOnACPS1Board` / `TestConsolesAlreadyAgree` /
  `TestNeoGeoGameWearingSixKeys` / `TestNeoGeoCDPanelIsFourKeys` / `TestFourButtonCabinetGetsFourKeys` /
  `TestThreeButtonCabinetGetsThreeKeys` / `TestMegaDriveStaysSixOfItsOwn`，用的都是实测抓下来的表。

### 每个 rom 的基板：`cmd/boardfill`（2026-11-24）

`board` 一列原来是空的就一直是空的（ADB 只补「缺厂商/年份」的那些），空基板 → 面板回落 `arcade` 六键，
于是 SNK 的四键机台被画成六键。现在按 MAME 自己的机器表来填：

```
mame64 -listxml > E:\game\tmp\mame-listxml.xml      # 0_ARCADE 里那份 MAME 带 35088 个 romset，约 208MB / 30 秒
go run ./cmd/boardfill -xml E:\game\tmp\mame-listxml.xml -dry      # 先看会改成什么
go run ./cmd/boardfill -xml E:\game\tmp\mame-listxml.xml            # 写库（只填空的，加 -force 才覆盖）
go run ./cmd/boardfill -xml E:\game\tmp\mame-listxml.xml -online   # MAME 不认识的再问 Arcade Italia
```

- MAME 不认识的（改版/盗版/汉化）按名字推断：**名的前缀是 MAME 的 romset，最多剩 4 个字母**
  （`wofh→wof`、`kof97pla→kof97`、`doubledrbh→doubledr`），推断结果会打出来（`（按 wof 推断）`），
  前缀剩太多的一律不猜（`Samurai Shodown II X` 不会被当成 `samurai`）。`.neo` = NeoGeo CD；`.cue/.gdi` 不猜（那是 DC/NAOMI 镜像）。
- 写入的是 MAME 的 `sourcefile` 原样（`neogeo.cpp`、`capcom/cps1.cpp` 混着都行）：显示和分组都走 `library.BoardOf()`，
  只按 basename 认牌子，所以 `neogeo.cpp` 与 `snk/neogeo.cpp` 都是 `MVS`。
- 2026-11-24 一次：街机 153 个空基板 → 151 个填上（MAME 127、家族推断 19、ADB 4、`.neo` 1），
  剩 2 个是 `Gunbird 2 (Japan).cue` / `Paca Paca Passion Special.cue` —— 是 Dreamcast 镜像被放进了 `arc`，该挪去 SEGA。
- 改库前备份 `数据/launcher.db`（本轮备份 `launcher.db.bak-20261124-2010`）。


### 双人（2026-11-23）

- `controls.json` 仍是一台机器一份；1P 的 id 保持裸名字，2P 的写在 `2p:` 前缀下，旧文件不用迁移。
  `padProfile()` 跳过 `2p:` 条目，`padProfile2()` 剥掉前缀覆盖到 2P 默认表上。
- `padBind` 多了 `ctrl`：XInput 控制器序号（0 是第一只手柄，1 是第二只）。`Pad` 仍是**一只柄上的按钮序号 + 1**，
  2P 默认只把 `ctrl` 改成 1，**不能给 `Pad` 加 4** —— 那会从 A 跳到 LT，之前 `8af7c3c` 的 `+4` 已改正。
  2P 默认无键盘、按钮集与 1P 相同，所以两只手柄不会互读。
- 核心 `Session` 的 `keys` / `binds` / `pads` 都是按端口四份（`keyMasks` / `bindTables` / `padMasks`）：
  `SetKeyBinds(port, table)` 分别写，`pollKeys()` 每个玩家各产一份（同一个 VK 两边都按会同时进两个端口），
  `SetPads([4]map)` 一次写全，`inputCallback` 的 joypad 和 keyboard 都读自己 `port`；`Start` 已向 0/1 两个端口 announce 手柄。
- 面板 `panelW=1000`：左右两列各 484，间距 8，内边距 12；标题居中，两列分别标 `1P` / `2P`。
  点哪一列的键就抓哪一列（`capP` 记玩家），`capPad` 是 `map[int]map[int]bool`（控制器 → 按钮），
  写回的绑定带控制器号；每列自己的“恢复默认”只清自己那一半。
- 测试：`pad_test.go` 的 `TestSecondPlayerKeepsItsOwnButtons`、`TestPlayersReadTheirOwnPads`、
  `TestSameKeyCanDriveBothPlayers`、`TestSecondPlayerBindingsAreFiledApart`、`TestSecondPlayerCaptureWritesItsOwnPrefix`；
  `session_windows_test.go` 盯 joypad / keyboard 的端口回答和同一按键进两个端口。

### 本轮补充（2026-11-20）

- 主机补齐：DC、XBOX、XBOX 360、PS2、PS3、PSV、3DS。SEGA 的 DC 不再回落到街机（`padLayoutOf` 返回 `dc`）；
  `ps` 平台按核心再分 `pcsx2/pcsx_rearmed→ps2`、`rpcs3→ps3`；任天堂按 `citra`/`.cia`→`3ds`；微软按 `xenia_launcher`→`xbox360`，其余 `xbox`。
- **PS 系按钮按位置对**：核心读 `△→X(上)`、`□→Y(左)`、`○→A(右)`、`×→B(下)`（Beetle PSX / pcsx2 的 SET_INPUT_DESCRIPTORS），
  以前写反的 `△→Y/□→X/○→B/×→A` 已改；PSV/PS2/PS3/PSP 共用 `psFaces()`。
- 十字键：一条竖臂 + 一条横臂拼的实心十字（同一不透明色 `#2b303c`，避免叠加处出现亮方块），
  四臂各一个三角 `▲▼◀▶`；**没有中心方块、没有下面的圆**；三角下面只在玩家另外绑了键时才写绑定字。
- 摇杆：有摇杆的机种（PS2/PS3/PSP/PSV/DC/XBOX/XBOX360/3DS）在十字键右侧画一个圆形摇杆井，
  四向三角单独成 spot，id 为 `lsup/lsdown/lsleft/lsright`，默认读 XInput 左摇杆的索引 16..19（`padNames` 追加「摇杆↑↓←→」），
  与十字键的 0..3 分开存，**两套方向互不影响**；不给 WASD 默认键（那是第二套方向键，属多余）。
- **长按是摇杆井中心的一颗独立键 `lshold`**（显示「长按」），不是面板开关：
  按住它时若摇杆正偏向某方向，就锁住该方向（松手后继续按着）；空白时按它则解除锁定。
  它本身不进游戏（`padRetroIds["lshold"]=padNothing = -1`，`maskOf` 跳过负数 id）。

### 画面比例：按平台定玻璃，不信核心（2026-11-21）

CPS2/CPS3 的 384×224 以前被画成 1.71 的宽画面，因为进程内跑核心时读的是
`retro_get_system_av_info` 的 `geometry.aspect`，而这批核心**普遍报 0**（实测 `fbneo_plus`、`fbneo`、
`fbalpha`、`opera`(3DO)、`snes9x`、`dosbox_pure`、`mame2010`），前端就落到帧自身的像素比——
384/224=1.71、320×200=1.6、256×224=1.14、256×240=1.07，全都不是 4:3。

规则写在 `internal/app/aspect.go`：

- `tubeGlass` 里的平台（`arc`、`majiang`、`fc`、`sfc`、`SEGA`、`3DO`、`ps`、`dos`、`pc98`）一律按 **4:3**；
  帧高大于帧宽的竖屏（纵向射击）按 **3:4**；像素比超出 1/3..3 的怪板（Darius 三屏）保留自身比例。
- 不在表里的平台（`掌机`、`任天堂`）是真方像素：GBA 240×160=3:2、GB 160×144、PSP 480×272、3DS 400×240，
  帧比例本来就对，不动。
- 只有一个数 `pictureRatio()`：`gamePicture` 的黑边框、`crtEffect` 的 `GAME_ASPECT`、`input_windows.go`
  的鼠标映射都读它，三处不会各算一套。

**`ui.Image` 默认是 Contain**（保原始比例），所以不开滤镜那条路必须 `.Fit(ui.FillBox)` 才会把 1.71 的帧
拉进 4:3 的框；只改外框尺寸会把画面缩一圈、比例还是错的（踩过）。开滤镜时画面由着色器的 `GAME_ASPECT`
决定，同一条数。

验收：`jojobajr1`（CPS3）关滤镜截图，画面框 2119×1589 = 1.334；改前同一局是 2119×1235 = 1.716。

### 核心读的路径：先 UTF-8，再 ANSI；zip 由前端拆（2026-11-21）

两个老问题（FC 打不开、中文名 rom 打不开）根子都在 `retro_load_game` 拿到的那个路径/内容上。

- **路径编码没有统一答案**。多数核心走 libretro-common 的 `fopen_utf8`（见 `compat/fopen_utf8.c`：
  UTF-8 → UTF-16 → `_wfopen`），要**UTF-8**；少数老核心直接 `fopen`，要系统 **ANSI**（GBK）。
  实测同一台机器：`fceumm`+中文名 `.nes` 用 UTF-8 才开得了、用 ANSI 必失败；`snes9x`+`马里奥医生.zip`
  反过来，只有 ANSI 能开。所以 `Start` 里先按 UTF-8 传，失败再换 `ansiBytes()` 传一次，不再挑一个赌。
- 目录（system / saves / core 路径）统一按 **UTF-8** 给，和 rom 的第一次尝试一致。
- **zip 是容器**：核心自己读压缩包的（`blockExtract=true`，FBNeo/MAME 这类街机）原样把 zip 交过去；
  其余核心看 `need_fullpath`——要路径就把包内最大的 rom 解到 `模拟器/RetroArch/cache/roms/<平台目录>/<zip名>/`
  再传那个路径（同名稳定，存档能跟着找回来），能吃内存就用 `gi.data/gi.size` 直接喂字节，一个文件都不落盘。
  `FCEUmm` 这版核心自己读不了 zip（`need_fullpath` 为真），就是靠这一步跑起来的。

**FC 换核心**：`mesen` 在 `retro_init` 里 `0xc0000005` 崩（env 走完 52/16/35/65 就崩，全返回 1 也崩，
同一个 dll 在 RetroArch 里正常），修不动，所以 `fc` 的规范核心改成 `fceumm`（`coreNames`/`CanonicalCore`、
db 的 `sim` 表都已换，mesen 的 dll 仍留在 cores 里）。`fceumm`/`nestopia` 都能过 init，选 fceumm 是因为
它认 zip。

### 画面旋转：核心要求转，前端自己转（2026-11-24 实测）

麻将的像管是倒装的、纵版射击的像管是侧装的。MAME 在 `mame_rotation_mode = "libretro"`（`config\MAME\MAME.opt`
里的默认）下**不自己转画面**，而是通过 libretro 的环境命令 `SET_ROTATION`（`cmd=1`）请前端转。以前前端
`switch` 里没有 `case 1`，于是「麻将 电子基盘」整屏倒 180°、纵版游戏整屏侧躺——不是某个 drv 的事，是全部
"像管装反/装侧"的基板一起错。

- 载荷是**四分之一圈数**，不是角度：0/1/2/3 = 0°/90°/180°/270°。MAME 的日志写 `Screen rotation: 180deg`
  而传过来的是 **2**；「武装飞鸟」（psikyo）传 **1**。一开始按角度读（`*(*uint32)` 当 180）读出 0，白改一轮。
- 方向也别猜：`1` 要用 **270°（顺时针系）**也就是逆时针转，画面才正；先按 90 转过，武装飞鸟的
  `Work RAM OK` 是侧着的。`turnDegrees()` 就是这张表，`internal/core/turn_test.go` 钉住。
- 实现在 `internal/core/session_windows.go`：`Session.turn`（`atomic.Int32`）、`Rotation()`、`turnFrame()`、
  纯函数 `turnRGBA(img, deg)`；`case 1` 存 `turn`，四条提交路径（Run 的 pend、videoCallback 的 vk / hw 直读、
  software）末尾各调一次 `turnFrame()`。180 原地转（行交换 + 每行反转），90/270 新建 `image.NewRGBA(h,w)` 逐像素
  落位——每帧都转，属于已知取舍（重负载时可按需搬到 GPU 着色器）。
- 验收不看窗口也能看：`GAMEROOM_DUMP=<png 路径>` 让核心在**转过之后**的第 90 帧落一张 PNG（每进程只落一次，
  `dumpPath`/`dumped`）。实测：电子基盘 `2 quarters = 180 degrees`，图里 `MAHJONG ELECTRON BASE` 正着；
  天开眼 `0 quarters`（同是 dynax.cpp 但驱动不要求转，本来就不倒）；武装飞鸟 `1 quarters = 270 degrees`，224×320
  竖图、`Work RAM OK` 正着。
- MAME 启动时还会在画面下方压一行 `Error: Load state file <rom>\<rom>\auto.sta not found`——这是它自己的
  autosave（`MAME.opt` 里 `mame_auto_save = "enabled"`、`mame_saves = "game"`，还会读自己的 mame.ini），
  和旋转无关；本机 `saves\mame\states\<rom>\` 里已有别的 .sta，说明它照常存档，只是本前端**强杀进程**，
  这一局永远存不上。改 GET_VARIABLE 覆盖无效（实测 `var: mame_auto_save -> disabled` 后照样报），
  要消掉只能改那份 opt/ini。

