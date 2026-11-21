package app

import (
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
)

func TestNormTitle(t *testing.T) {
	cases := map[string]string{
		"侍魂Ⅱ 真侍魂 霸王丸地狱变 sp":        "侍魂ii 真侍魂 霸王丸地狱变",
		"麻将 大中华圈 (日版, 无作弊版)":       "麻将 大中华圈",
		"西游释厄传 PLUS":               "西游释厄传",
		"雀神 加强版":                   "雀神",
		"吞食天地2~赤壁之战":               "吞食天地2~赤壁之战",
		"节奏天国[Jctg](简)(JP)(128Mb)": "节奏天国",
	}
	for in, want := range cases {
		if got := normTitle(in); got != want {
			t.Errorf("normTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSequelRemainder(t *testing.T) {
	for _, s := range []string{"3", "iii", "4 进化", "第 2 套", ""} {
		if !sequelRemainder(s) {
			t.Errorf("sequelRemainder(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"霸王丸地狱变", "特别版", "喷射版"} {
		if sequelRemainder(s) {
			t.Errorf("sequelRemainder(%q) = true, want false", s)
		}
	}
}

// games builds a shelf of arcade games with the titles given.
func games(titles ...string) []library.Game {
	out := make([]library.Game, len(titles))
	for i, title := range titles {
		out[i] = library.Game{ID: i + 1, PlatformKey: "arc", Title: title}
	}
	return out
}

func TestVersionFamilies(t *testing.T) {
	a := &App{lib: &library.Library{Games: games(
		"侍魂Ⅱ 真侍魂 X",
		"侍魂Ⅱ 真侍魂 霸王丸地狱变",
		"侍魂Ⅱ 真侍魂 霸王丸地狱变 sp",
		"战国传承",
		"战国传承 3",
		"无敌版",
		"无敌版",
		"西游释厄传",
		"西游释厄传 PLUS",
		"麻将 大中华圈",
		"麻将 大中华圈 (日版, 无作弊版)",
	)}}
	a.buildVersions()

	same := func(x, y int, want bool) {
		t.Helper()
		got := a.verFam[x] == a.verFam[y]
		if got != want {
			t.Errorf("games %d and %d same family = %v, want %v", x, y, got, want)
		}
	}
	same(1, 2, true)
	same(2, 3, true)
	same(4, 5, false) // a sequel is its own game
	same(6, 7, false) // the same label on two different games is not a family
	same(8, 9, true)  // a tag-only variant is a version
	same(10, 11, true)

	if vs := a.versionsOf(a.lib.Games[0]); len(vs) != 3 {
		t.Errorf("versions of 真侍魂 = %d, want 3", len(vs))
	}
}

func TestVersionTarget(t *testing.T) {
	a := &App{lib: &library.Library{Games: games(
		"侍魂Ⅱ 真侍魂 霸王丸地狱变",
		"侍魂Ⅱ 真侍魂 X",
		"侍魂Ⅱ 真侍魂 霸王丸地狱变 sp",
	)}}
	a.buildVersions()

	base := a.lib.Games[0]
	a.verDefault[a.verFam[base.ID]] = 2

	if got := a.versionTarget(base); got.ID != 2 {
		t.Errorf("the base card plays %d, want the chosen version 2", got.ID)
	}
	// A card that is itself a version plays that version, not the default.
	if got := a.versionTarget(a.lib.Games[1]); got.ID != 2 {
		t.Errorf("the version card plays %d, want itself 2", got.ID)
	}
}
