package library

import "testing"

// shelfFor builds a library with one game per line, then splits it, so the
// chips it ends up with are the ones the bar would draw.
func shelfFor(t *testing.T, platforms []Platform, games []Game, sims []Sim) *Library {
	t.Helper()
	l := &Library{Platforms: platforms, Games: games, Sims: sims}
	l.split()
	return l
}

// TestArcadeSplitsByMaker: the 街机 shelf files its games under the
// manufacturer, taking the publisher first and then the folder it came from,
// and puts the games it cannot place last.
func TestArcadeSplitsByMaker(t *testing.T) {
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

	menus := l.MenusOf("arc")
	want := []string{"SNK", "CAPCOM", "SEGA", "未标注厂商"}
	if len(menus) != len(want) {
		t.Fatalf("menus = %v, want %v", names(menus), want)
	}
	for i, m := range menus {
		if m.Name != want[i] {
			t.Errorf("menu %d = %q, want %q", i, m.Name, want[i])
		}
	}
	if l.Games[3].Menu != "SEGA" {
		t.Errorf("stv game filed under %q, want SEGA", l.Games[3].Menu)
	}
	if l.Games[4].Menu != "未标注厂商" {
		t.Errorf("genre-only game filed under %q", l.Games[4].Menu)
	}
}

// TestMahjongSplitsByYear: the 麻将 shelf files its games under the year, and
// the timeline wins over popularity — an older year with more games still
// comes after a newer one — with the unknown year last.
func TestMahjongSplitsByYear(t *testing.T) {
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

	menus := l.MenusOf("majiang")
	if got := names(menus); len(got) != 3 || got[0] != "2022" || got[1] != "1996" || got[2] != "年份未知" {
		t.Errorf("menus = %v, want [2022 1996 年份未知]", got)
	}
}

// TestHandheldSplitsByMachine: the 掌机 shelf names the machine its core
// emulates, not the core.
func TestHandheldSplitsByMachine(t *testing.T) {
	l := shelfFor(t,
		[]Platform{{Key: "掌机", Name: "掌机", Group: "handheld"}},
		[]Game{
			{ID: 1, PlatformKey: "掌机", Title: "gba game"},
			{ID: 2, PlatformKey: "掌机", Title: "psp game", SimID: 2},
		},
		[]Sim{
			{ID: 1, PlatformKey: "掌机", Name: "mGBA", Core: "mgba", IsDefault: true},
			{ID: 2, PlatformKey: "掌机", Name: "PPSSPP", Core: "ppsspp"},
		})

	menus := l.MenusOf("掌机")
	got := names(menus)
	if len(got) != 2 || got[0] != "GBA" || got[1] != "PSP" {
		t.Errorf("menus = %v, want [GBA PSP]", got)
	}
}

// TestConsoleHasNoMenus: the 家用机 shelf is split by the platform itself, so
// the old series menus are dropped.
func TestConsoleHasNoMenus(t *testing.T) {
	l := shelfFor(t,
		[]Platform{{Key: "fc", Name: "FC", Group: "console"}},
		[]Game{
			{ID: 1, PlatformKey: "fc", Title: "a", Menu: "冒险岛"},
			{ID: 2, PlatformKey: "fc", Title: "b", Menu: "格斗"},
		},
		nil)

	if menus := l.MenusOf("fc"); len(menus) != 0 {
		t.Errorf("fc menus = %v, want none", names(menus))
	}
	for _, g := range l.Games {
		if g.Menu != "" {
			t.Errorf("%s still filed under %q", g.Title, g.Menu)
		}
	}
}

func names(menus []Menu) []string {
	out := make([]string, 0, len(menus))
	for _, m := range menus {
		out = append(out, m.Name)
	}
	return out
}
