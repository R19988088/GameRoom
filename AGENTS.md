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
| `game` | `title`（展示名，导入时是 rom 文件名）与 `rom_path`（相对 `rom/` 的文件路径）分开；`name` 是**目录里查到的大名**（ADB 的「Short name」，如 `Alien vs. Predator`），有它卡片就显示它；`size` 为字节数（INTEGER）；`cover` 为**相对 `数据/`** 的路径（`covers/arc/dino.png`）；`favorite` 布尔；`core` 是这个游戏单独指定的核心（空 = 用 sim 的）；`sim_id`（0 = 用平台默认模拟器）|
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
- **改数据的动作排到下一帧**（`a.later`/`runQueue`，队列在 `view()` 第一行跑）：卡片菜单是在
  **这一帧的** `a.lines` 里画的，菜单里一改筛选或元数据，后面的卡还按老列表取下标，就
  `index out of range` 把窗口弄死（`TestNameRowIsTypedIn`/`TestValueClickRewritesTheGame`
  盯着这两条，`sections_test.go` 里直接调 `assign` 的用例要补 `a.runQueue()`）。
  写入照旧走 `library.SetField` / `SetPlatform` / `SetName`，改完 `SplitGroup` + `refilter`
  并保留滚动位置。
- **瀑布流按分类分组**：`refilter()` 先筛 `a.shown`，再按 `menu` 切成 `a.sections`；每节开头插一个
  标题行（组名大写 + 右侧灰色计数），点标题 `chooseMenu(该组)` 直接筛选到该组。`chips` 为空
  （家用机/PC 的机种轴：机种就是平台）时只有一节且不画标题。
- 位置由 `internal/app/layout_test.go` 的 `TestTopBarIsFlat` 盯着：三样东西同一行（比中心点）、
  两两之间有空隙、chip 比货架按钮矮（不套层）、整行底边在 70 以内、卡片不钻到顶栏下面；
  `bar_test.go` 还盯着货架在最左、`更多` 不压搜索框；`shelf_test.go` 确认菜单点得开；
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
- 街机默认开厂商而非基板：`board` 只有 124/267 抓得到（ADB 没页的 romset 就没有 driver source），
  默认开基板会让一半游戏落进「未标注基板」。
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
- DOSBox Pure 默认起来是它自己的 START MENU（列出 zip 里的 exe）。本工程不用这套：**双击直启**
  靠 `internal/library/doslaunch.go` 在导入时往每个 zip 根目录写一份 `DOSBOX.BAT`
  （Pure 官方机制：内容里有 `DOSBOX.BAT` 就**跳过菜单直接执行**，前端无关）。
  批处理正文来自该 zip 自带的根级 `dosbox.conf` 的 `[autoexec]`——老合集作者给每款游戏调好的
  启动命令（`cd IF2` / `call PLAY.BAT` / `imgmount D .\CD\BG.cue -t cdrom` / TSR 串），
  只去掉 `mount C .`（Pure 自己把 zip 挂在 C:）和 `exit`（会把会话关掉）。
  - 结果：**76/77** 双击直接进游戏（已实机验过 `3x3 EYES`、`Cobra Mission`、`Blue Garnet` 的
    `imgmount`、`Tenchi Muyou`）；`Doukyuusei 2 Special Disc` 没有 `dosbox.conf`、也没有 `play.BAT`，
    仍是 Pure 菜单（能玩，但要选一次）。
  - 导入时**静态校验**批处理指向的文件真的在 zip 里（`checkCommands`），指空就**不注入**，
    宁可留在菜单也不给一个死命令行；`call CM` 这种会自动补 `.BAT/.COM/.EXE` 再找。
  - 没有 `dosbox.conf` 的包按老合集 `loader.bat` 的那几条规则挑命令（顺序一致）：
    `play.*`（bat/exe/com，任意目录）→ `<目录>/<目录>.bat` → 全包唯一一个 exe/com →
    全包唯一一个 bat，`install/setup/readme/help/inst/dos4gw` 不算。这两条规则是从
    `E:\game\0_GALGAMEonDOSUI\loader.bat` 原样搬过来的，老项目就是靠它们给没有 conf 的包配音命令。
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


