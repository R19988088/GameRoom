package library

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The standards a shelf can file its games by. Every shelf offers one or two
// of them, and the bar shows a switch to move between them:
//
//	街机   → 基板（CPS2 / MVS / NAOMI …）⇄ 厂商（CAPCOM / SNK …）
//	家用机 → 机种（FC / SFC / PS …）    ⇄ 厂商（NINTENDO / SONY …）
//	掌机   → 机种（GBA / PSP …）        ⇄ 厂商（NINTENDO / SONY / SNK …）
//	麻将   → 年份（2022 … 1986），只有这一种
const (
	AxisMachine = "machine" // 机种
	AxisMaker   = "maker"   // 厂商
	AxisBoard   = "board"   // 基板
	AxisYear    = "year"    // 年份
)

// AxesOf lists the standards a shelf offers, the first one being its default.
// 街机 opens on 厂商, because every arcade game has one, while the 基板 is only
// known for the romsets whose page has been read; the switch still offers it.
func AxesOf(group string) []string {
	switch group {
	case "arcade":
		return []string{AxisMaker, AxisBoard}
	case "mahjong":
		return []string{AxisYear}
	}
	return []string{AxisMachine, AxisMaker}
}

// AxisName is what the switch calls a standard.
func AxisName(axis string) string {
	switch axis {
	case AxisMachine:
		return "机种"
	case AxisMaker:
		return "厂商"
	case AxisBoard:
		return "基板"
	case AxisYear:
		return "年份"
	}
	return axis
}

// boards is what MAME's driver source is known as on a cabinet. Anything not
// listed keeps its source name, uppercased — that is still the hardware family
// MAME groups the romset by.
var boards = map[string]string{
	"neogeo":        "MVS",
	"neogeo_mv1":    "MVS",
	"cps1":          "CPS1",
	"cps2":          "CPS2",
	"cps3":          "CPS3",
	"psikyo":        "PSIKYO",
	"psikyosh":      "PSIKYO SH2",
	"stv":           "ST-V",
	"model1":        "MODEL1",
	"model2":        "MODEL2",
	"model3":        "MODEL3",
	"naomi":         "NAOMI",
	"naomigd":       "NAOMI GD",
	"pgm":           "PGM",
	"pgm2":          "PGM2",
	"taito_f2":      "TAITO F2",
	"taito_f3":      "TAITO F3",
	"taito_z":       "TAITO Z",
	"taitowlf":      "TAITO WOLF",
	"gx":            "KONAMI GX",
	"hornet":        "HORNET",
	"system16":      "SYSTEM16",
	"segas32":       "SEGA S32",
	"hikaru":        "HIKARU",
	"m72":           "M72",
	"m92":           "M92",
	"m107":          "M107",
	"kaneko16":      "SUPER KANEKO",
	"cave":          "CAVE",
	"toaplan1":      "TOAPLAN1",
	"toaplan2":      "TOAPLAN2",
	"nmk16":         "NMK16",
	"megasys1":      "MEGASYS1",
	"deco32":        "DECO32",
	"deco16ic":      "DECO16",
	"midtunit":      "MIDWAY T-UNIT",
	"midwunit":      "MIDWAY W-UNIT",
	"midxunit":      "MIDWAY X-UNIT",
	"midyunit":      "MIDWAY Y-UNIT",
	"konamigx":      "KONAMI GX",
	"namcos12":      "SYSTEM 12",
	"namcos11":      "SYSTEM 11",
	"namcos21":      "SYSTEM 21",
	"namcos2":       "SYSTEM 2",
	"zn":            "SONY ZN",
	"ssv":           "SSV",
	"dc_atomiswave": "ATOMISWAVE",
	"fuukifg3":      "FUUKI FG3",
	"segaic16":      "SEGA SYSTEM C",
	"segahang":      "SEGA HANG-ON",
	"galaxian":      "GALAXIAN",
	"pacman":        "PAC-MAN",
	"mappy":         "MAPPY",
}

// BoardOf names the hardware of a romset from its driver source:
//
//	"snk/neogeo.cpp"     -> MVS
//	"capcom/cps2.cpp"    -> CPS2
//	"universal/mrdo.cpp" -> MRDO
func BoardOf(source string) string {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(source, "\\", "/")))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if b, ok := boards[base]; ok {
		return b
	}
	if base == "" {
		return "未标注基板"
	}
	return strings.ToUpper(strings.ReplaceAll(base, "_", " "))
}

// makers renames what the old library filed under a folder name into the
// manufacturer, so the 厂商 chips do not mix hardware (stv, naomi) with makers.
var makers = map[string]string{
	"彩京":     "PSIKYO",
	"STV":    "SEGA",
	"NAOMI":  "SEGA",
	"NAOMI2": "SEGA",
	"MODEL2": "SEGA",
	"MODEL3": "SEGA",
}

// notAMaker is what the old library filed as a genre, a state or nothing.
var notAMaker = map[string]bool{
	"": true, "ACT": true, "STG": true, "OTHERS": true, "UNKNOWN": true,
	"古老的游戏": true, "不能运行": true, "未分类": true, "未标注": true,
}

// PublisherOf names the manufacturer of an arcade game: its own publisher
// when the import filled one, else the folder it was filed under (SNK,
// CAPCOM …), else 未标注厂商.
func PublisherOf(g Game) string {
	if m := makerName(g.Publisher); m != "" {
		return m
	}
	menu := strings.ToUpper(strings.TrimSpace(g.Menu))
	if m, ok := makers[menu]; ok {
		return m
	}
	if !notAMaker[menu] {
		return menu
	}
	return "未标注厂商"
}

// makerName cleans a manufacturer up: ADB writes co-productions as
// "CAPCOM / ARIKA", and the chip should carry the first name only.
func makerName(publisher string) string {
	m := strings.ToUpper(strings.TrimSpace(publisher))
	if i := strings.Index(m, "/"); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	if notAMaker[m] {
		return ""
	}
	return m
}

// platformMakers is who built a console: the machine's maker, not the games'.
var platformMakers = map[string]string{
	"fc":   "NINTENDO",
	"sfc":  "NINTENDO",
	"任天堂":  "NINTENDO",
	"ps":   "SONY",
	"SEGA": "SEGA",
	"3DO":  "3DO",
	"微软":   "MICROSOFT",
}

// coreMakers is the maker of the machine a handheld core emulates.
var coreMakers = map[string]string{
	"mgba":         "NINTENDO",
	"gambatte":     "NINTENDO",
	"mesen":        "NINTENDO",
	"ppsspp":       "SONY",
	"mednafen_ngp": "SNK",
}

// machines are the cores of the handheld shelf, named as the machine each
// emulates rather than as the core.
var machines = map[string]string{
	"mgba":         "GBA",
	"ppsspp":       "PSP",
	"mednafen_ngp": "NGP",
	"gambatte":     "GB",
	"mesen":        "FC",
}

// machineOf is the machine a handheld game runs on, taken from its core;
// 未知机种 when the sim names none.
func machineOf(sim *Sim) string {
	if sim == nil {
		return "未知机种"
	}
	if name, ok := machines[strings.ToLower(sim.Core)]; ok {
		return name
	}
	if sim.Core != "" {
		return sim.Core
	}
	return "未知机种"
}

// makerOf names who made the machine a console or handheld game runs on.
func makerOf(l *Library, g Game, p Platform) string {
	if p.Group == "handheld" {
		if sim := l.SimFor(g); sim != nil {
			if m, ok := coreMakers[strings.ToLower(sim.Core)]; ok {
				return m
			}
		}
		if m, ok := platformMakers[p.Key]; ok {
			return m
		}
		return "未标注厂商"
	}
	if m, ok := platformMakers[p.Key]; ok {
		return m
	}
	if m := makerName(p.Name); m != "" {
		return m
	}
	return "未标注厂商"
}

// boardOf is the 基板 of an arcade game: what its page said, else unknown.
func boardOf(g Game) string {
	if b := BoardOf(g.Board); b != "未标注基板" {
		return b
	}
	return "未标注基板"
}

// yearOf is the year a mahjong table came out, "年份未知" when nobody wrote it.
func yearOf(year int) string {
	if year <= 0 {
		return "年份未知"
	}
	return strconv.Itoa(year)
}

// menuName is the chip a game falls under for one axis. On the machine axis a
// console's machine *is* its platform, so there is no menu; a handheld
// platform (掌机) runs several machines, so each game files under its own.
func (l *Library) menuName(g Game, p Platform, axis string) string {
	switch axis {
	case AxisBoard:
		return boardOf(g)
	case AxisMaker:
		if p.Group == "arcade" {
			return PublisherOf(g)
		}
		return makerOf(l, g, p)
	case AxisYear:
		return yearOf(g.Year)
	case AxisMachine:
		if p.Group == "handheld" {
			return machineOf(l.SimFor(g))
		}
		return ""
	}
	return ""
}

// SplitGroup rewrites the menu of every game on a shelf for one axis, so the
// chips the bar draws and the filter it applies always read the same field.
func (l *Library) SplitGroup(group, axis string) {
	for i := range l.Games {
		g := &l.Games[i]
		p := l.Platform(g.PlatformKey)
		if p == nil || p.Group != group {
			continue
		}
		g.Menu = l.menuName(*g, *p, axis)
	}
}

// Chips are the bar's chips for a shelf and an axis: the values it files the
// games by, with how many games each holds.
func (l *Library) Chips(group, axis string) []Chip {
	counts := map[string]int{}
	for _, g := range l.Games {
		p := l.Platform(g.PlatformKey)
		if p == nil || p.Group != group {
			continue
		}
		name := l.menuName(g, *p, axis)
		if name == "" {
			continue
		}
		counts[name]++
	}
	chips := make([]Chip, 0, len(counts))
	for name, n := range counts {
		chips = append(chips, Chip{Name: name, Count: n})
	}
	sort.Slice(chips, func(i, j int) bool {
		// What could not be filed goes last: an "unknown" chip with the most
		// games should not push the real makers off the bar.
		if ri, rj := unknownRank(chips[i].Name), unknownRank(chips[j].Name); ri != rj {
			return ri < rj
		}
		if axis == AxisYear {
			// The year axis is a timeline, not a popularity list: newest
			// table first whatever its game count.
			if yi, yj := yearRank(chips[i].Name), yearRank(chips[j].Name); yi != yj {
				return yi > yj
			}
			return chips[i].Name < chips[j].Name
		}
		if chips[i].Count != chips[j].Count {
			return chips[i].Count > chips[j].Count
		}
		return chips[i].Name < chips[j].Name
	})
	return chips
}

// unknownRank is 1 for the buckets that mean "we could not tell", so they
// sort after every chip that carries a name.
func unknownRank(name string) int {
	switch name {
	case "未标注厂商", "未标注基板", "年份未知", "未知机种":
		return 1
	}
	if strings.HasPrefix(name, "未标注") {
		return 1
	}
	return 0
}

// yearRank turns a year chip into a number to sort by, 0 for 年份未知.
func yearRank(name string) int {
	n, err := strconv.Atoi(name)
	if err != nil {
		return 0
	}
	return n
}
