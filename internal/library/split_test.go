package library

import (
	"testing"
)

// shelfFor builds a library, splits one shelf on one standard, and hands it
// back so the chips can be read as the bar would draw them.
func shelfFor(t *testing.T, platforms []Platform, games []Game, sims []Sim) *Library {
	t.Helper()
	l := &Library{Platforms: platforms, Games: games, Sims: sims}
	l.index()
	return l
}

func names(chips []Chip) []string {
	out := make([]string, 0, len(chips))
	for _, c := range chips {
		out = append(out, c.Name)
	}
	return out
}

// TestAxesOfShelves: every shelf offers the standards the switch moves
// between, arcade 基板 ⇄ 厂商, console 机种 ⇄ 厂商, 麻将 年份 only.
func TestAxesOfShelves(t *testing.T) {
	for _, tc := range []struct {
		group string
		want  []string
	}{
		{"arcade", []string{AxisMaker, AxisBoard}},
		{"mahjong", []string{AxisYear}},
		{"console", []string{AxisMachine, AxisMaker}},
		{"handheld", []string{AxisMachine, AxisMaker}},
		{"pc", []string{AxisMachine, AxisMaker}},
	} {
		got := AxesOf(tc.group)
		if len(got) != len(tc.want) {
			t.Fatalf("%s axes = %v, want %v", tc.group, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s axis %d = %q, want %q", tc.group, i, got[i], tc.want[i])
			}
		}
	}
}

// TestBoardOfNamesHardware: MAME's driver source reads as the cabinet's board.
func TestBoardOfNamesHardware(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"snk/neogeo.cpp", "MVS"},
		{"capcom/cps2.cpp", "CPS2"},
		{"sega/model2.cpp", "MODEL2"},
		{"universal/mrdo.cpp", "MRDO"},
		{"", "未标注基板"},
	} {
		if got := BoardOf(tc.source); got != tc.want {
			t.Errorf("BoardOf(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

// TestArcadeSplitByMaker: the 街机 shelf on the 厂商 axis files its games
// under the manufacturer, taking the publisher first and then the folder it
// came from, and puts what it cannot place last.
func TestArcadeSplitByMaker(t *testing.T) {
	l := shelfFor(t,
		[]Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		[]Game{
			{ID: 1, PlatformKey: "arc", Title: "a", Menu: "SNK"},
			{ID: 2, PlatformKey: "arc", Title: "b", Menu: "SNK"},
			{ID: 3, PlatformKey: "arc", Title: "c", Menu: "ACT", Publisher: "CAPCOM"},
			{ID: 4, PlatformKey: "arc", Title: "d", Menu: "stv"},
			{ID: 5, PlatformKey: "arc", Title: "e", Menu: "ACT"},
		},
		nil)

	l.SplitGroup("arcade", AxisMaker)
	chips := l.Chips("arcade", AxisMaker)
	want := []string{"SNK", "CAPCOM", "SEGA", "未标注厂商"}
	if len(chips) != len(want) {
		t.Fatalf("chips = %v, want %v", names(chips), want)
	}
	for i, c := range chips {
		if c.Name != want[i] {
			t.Errorf("chip %d = %q, want %q", i, c.Name, want[i])
		}
	}
	if l.Games[3].Menu != "SEGA" {
		t.Errorf("stv game filed under %q, want SEGA", l.Games[3].Menu)
	}
	if l.Games[4].Menu != "未标注厂商" {
		t.Errorf("genre-only game filed under %q", l.Games[4].Menu)
	}
}

// TestArcadeSplitByBoard: on the 基板 axis the same shelf files games by the
// hardware their page names, and the unknown board goes last.
func TestArcadeSplitByBoard(t *testing.T) {
	l := shelfFor(t,
		[]Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		[]Game{
			{ID: 1, PlatformKey: "arc", Title: "kof98", Board: "snk/neogeo.cpp"},
			{ID: 2, PlatformKey: "arc", Title: "sf2", Board: "capcom/cps1.cpp"},
			{ID: 3, PlatformKey: "arc", Title: "dino", Board: "capcom/cps1.cpp"},
			{ID: 4, PlatformKey: "arc", Title: "ssf2", Board: "capcom/cps2.cpp"},
			{ID: 5, PlatformKey: "arc", Title: "mystery"},
		},
		nil)

	l.SplitGroup("arcade", AxisBoard)
	chips := l.Chips("arcade", AxisBoard)
	got := names(chips)
	if len(got) != 4 || got[0] != "CPS1" || got[len(got)-1] != "未标注基板" {
		t.Errorf("chips = %v, want CPS1 first and 未标注基板 last", got)
	}
	if l.Games[0].Menu != "MVS" {
		t.Errorf("kof98 filed under %q, want MVS", l.Games[0].Menu)
	}
}

// TestMahjongSplitByYear: the 麻将 shelf files its games under the year, and
// the timeline wins over popularity — an older year with more games still
// comes after a newer one — with the unknown year last.
func TestMahjongSplitByYear(t *testing.T) {
	l := shelfFor(t,
		[]Platform{{Key: "majiang", Name: "麻将", Group: "mahjong"}},
		[]Game{
			{ID: 1, PlatformKey: "majiang", Title: "old1", Year: 1996},
			{ID: 2, PlatformKey: "majiang", Title: "old2", Year: 1996},
			{ID: 3, PlatformKey: "majiang", Title: "old3", Year: 1996},
			{ID: 4, PlatformKey: "majiang", Title: "new", Year: 2022},
			{ID: 5, PlatformKey: "majiang", Title: "none"},
		},
		nil)

	l.SplitGroup("mahjong", AxisYear)
	chips := l.Chips("mahjong", AxisYear)
	if got := names(chips); len(got) != 3 || got[0] != "2022" || got[1] != "1996" || got[2] != "年份未知" {
		t.Errorf("chips = %v, want [2022 1996 年份未知]", got)
	}
}

// TestConsoleMachineHasNoMenus: on the 机种 axis the platform already is the
// chip, so the shelf has no values of its own.
func TestConsoleMachineHasNoMenus(t *testing.T) {
	l := shelfFor(t,
		[]Platform{
			{Key: "fc", Name: "FC", Group: "console"},
			{Key: "ps", Name: "Playstation", Group: "console"},
		},
		[]Game{
			{ID: 1, PlatformKey: "fc", Title: "mario", Menu: "冒险岛"},
			{ID: 2, PlatformKey: "fc", Title: "zelda"},
			{ID: 3, PlatformKey: "ps", Title: "tekken"},
		},
		nil)

	l.SplitGroup("console", AxisMachine)
	if chips := l.Chips("console", AxisMachine); len(chips) != 0 {
		t.Errorf("机种 axis chips = %v, want none", names(chips))
	}
	for _, g := range l.Games {
		if g.Menu != "" {
			t.Errorf("%s still filed under %q", g.Title, g.Menu)
		}
	}
}

// TestConsoleMakerAxis: 家用机 on the 厂商 axis files games by who built the
// machine — the platform's maker, not the games' publisher.
func TestConsoleMakerAxis(t *testing.T) {
	l := shelfFor(t,
		[]Platform{
			{Key: "fc", Name: "FC", Group: "console"},
			{Key: "ps", Name: "Playstation", Group: "console"},
			{Key: "SEGA", Name: "SEGA", Group: "console"},
		},
		[]Game{
			{ID: 1, PlatformKey: "fc", Title: "mario"},
			{ID: 2, PlatformKey: "fc", Title: "zelda"},
			{ID: 3, PlatformKey: "ps", Title: "tekken"},
			{ID: 4, PlatformKey: "SEGA", Title: "sonic"},
		},
		nil)

	l.SplitGroup("console", AxisMaker)
	chips := l.Chips("console", AxisMaker)
	if got := names(chips); len(got) != 3 || got[0] != "NINTENDO" {
		t.Errorf("chips = %v, want NINTENDO first then SONY, SEGA", got)
	}
	if l.Games[0].Menu != "NINTENDO" || l.Games[2].Menu != "SONY" || l.Games[3].Menu != "SEGA" {
		t.Errorf("menus = %q %q %q", l.Games[0].Menu, l.Games[2].Menu, l.Games[3].Menu)
	}
}

// TestHandheldMakerFollowsCore: a handheld game is made by the machine its
// core emulates, so GBA reads NINTENDO and PSP SONY.
func TestHandheldMakerFollowsCore(t *testing.T) {
	l := shelfFor(t,
		[]Platform{{Key: "掌机", Name: "掌机", Group: "handheld"}},
		[]Game{
			{ID: 1, PlatformKey: "掌机", Title: "gba game"},
			{ID: 2, PlatformKey: "掌机", Title: "psp game", SimID: 2},
			{ID: 3, PlatformKey: "掌机", Title: "ngp game", SimID: 3},
		},
		[]Sim{
			{ID: 1, PlatformKey: "掌机", Name: "mGBA", Core: "mgba", IsDefault: true},
			{ID: 2, PlatformKey: "掌机", Name: "PPSSPP", Core: "ppsspp"},
			{ID: 3, PlatformKey: "掌机", Name: "NGP", Core: "mednafen_ngp"},
		})

	l.SplitGroup("handheld", AxisMaker)
	chips := l.Chips("handheld", AxisMaker)
	if got := names(chips); len(got) != 3 || got[0] != "NINTENDO" {
		t.Errorf("chips = %v, want NINTENDO first", got)
	}
	if l.Games[1].Menu != "SONY" || l.Games[2].Menu != "SNK" {
		t.Errorf("psp/ngp menus = %q %q", l.Games[1].Menu, l.Games[2].Menu)
	}

	// 掌机 is one platform running several machines, so its 机种 chips come
	// from the cores (GBA / PSP / NGP), not from the platform list.
	l.SplitGroup("handheld", AxisMachine)
	if chips := l.Chips("handheld", AxisMachine); len(chips) != 3 {
		t.Errorf("机种 axis chips = %v, want the platform's machines", names(chips))
	}
}

// TestMachineOfHandheldCore names the machine a core emulates.
func TestMachineOfHandheldCore(t *testing.T) {
	for _, tc := range []struct{ core, want string }{
		{"mgba", "GBA"},
		{"ppsspp", "PSP"},
		{"mednafen_ngp", "NGP"},
		{"gambatte", "GB"},
		{"mesen", "FC"},
		{"", "未知机种"},
	} {
		sim := &Sim{Core: tc.core}
		if tc.core == "" {
			sim = nil
		}
		if got := machineOf(sim); got != tc.want {
			t.Errorf("machine of %q = %q, want %q", tc.core, got, tc.want)
		}
	}
}
