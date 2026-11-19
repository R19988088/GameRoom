package library

import (
	"sort"
	"strconv"
	"strings"
)

// menuAxis is the one standard a shelf splits its games by, fixed per shelf so
// the chips in the top bar always mean the same thing:
//
//	街机   → 厂商（CAPCOM / SNK / IGS …）
//	麻将   → 年份（2022 / 1996 …）
//	掌机   → 机种（GBA / PSP / NGP，取自它跑在哪个核心上）
//	家用机 → 机种，就是它的平台本身（FC / SFC / PS / SEGA …），所以没有菜单
//	PC     → 同上，没有菜单
//
// 空的 axis 表示这个平台不另分菜单：上面那排机种 chip 已经把它分完了。
func menuAxis(p Platform) string {
	switch {
	case p.Key == "arc":
		return "publisher"
	case p.Key == "majiang":
		return "year"
	case p.Group == "handheld":
		return "core"
	}
	return ""
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
// CAPCOM …), else 未标注厂商. Arcade cabinets are known by who built them, so
// this is the one split the 街机 shelf uses.
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

// yearOf is the year a mahjong table came out, "年份未知" when nobody wrote it.
func yearOf(year int) string {
	if year <= 0 {
		return "年份未知"
	}
	return strconv.Itoa(year)
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

// split rebuilds the menus from the shelf's axis and rewrites each game's
// menu to match, so the chips the bar shows and the filter it applies always
// come from the same field. The library's own menu table is dropped: the old
// frontend's menus mixed makers, hardware, genres and series.
func (l *Library) split() {
	counts := map[string]map[string]int{}
	for i := range l.Games {
		g := &l.Games[i]
		p := l.Platform(g.PlatformKey)
		if p == nil {
			continue
		}
		switch menuAxis(*p) {
		case "publisher":
			g.Menu = PublisherOf(*g)
		case "year":
			g.Menu = yearOf(g.Year)
		case "core":
			g.Menu = machineOf(l.SimFor(*g))
		default:
			g.Menu = ""
		}
		if g.Menu == "" {
			continue
		}
		if counts[p.Key] == nil {
			counts[p.Key] = map[string]int{}
		}
		counts[p.Key][g.Menu]++
	}

	l.Menus = make([]Menu, 0, len(l.Menus))
	for _, p := range l.Platforms {
		axis := menuAxis(p)
		c := counts[p.Key]
		if axis == "" || len(c) == 0 {
			continue
		}
		names := make([]string, 0, len(c))
		for name := range c {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			// What could not be filed goes last: an "unknown" chip with the
			// most games should not push the real makers off the bar.
			if ri, rj := unknownRank(names[i]), unknownRank(names[j]); ri != rj {
				return ri < rj
			}
			if axis == "year" {
				// The year axis is a timeline, not a popularity list: newest
				// table first whatever its game count, the unknown year last.
				if yi, yj := yearRank(names[i]), yearRank(names[j]); yi != yj {
					return yi > yj
				}
				return names[i] < names[j]
			}
			// Busiest first: the chips that matter are the ones with games.
			if c[names[i]] != c[names[j]] {
				return c[names[i]] > c[names[j]]
			}
			return names[i] < names[j]
		})
		for i, name := range names {
			l.Menus = append(l.Menus, Menu{PlatformKey: p.Key, Name: name, Sort: i})
		}
	}
	l.index()
}

// unknownRank is 1 for the buckets that mean "we could not tell", so they
// sort after every chip that carries a name.
func unknownRank(name string) int {
	switch name {
	case "未标注厂商", "年份未知", "未知机种":
		return 1
	}
	if strings.HasPrefix(name, "未标注") {
		return 1
	}
	return 0
}

// yearRank turns a year menu into a number to sort by, 0 for 年份未知.
func yearRank(name string) int {
	n, err := strconv.Atoi(name)
	if err != nil {
		return 0
	}
	return n
}
