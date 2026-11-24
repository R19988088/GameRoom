package app

import (
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// twoContainers is one game dumped twice into two containers: the same title,
// the same rom name, only the extension differs — the case a refresh creates
// when hsf2.zip lands beside hsf2.7z.
func twoContainers() *library.Library {
	return &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "超级街头霸王Ⅱ 15周年纪念版",
				RomPath: "arc/CAPCOM/STREET FIGHTER/hsf2.7z"},
			{ID: 2, PlatformKey: "arc", Title: "超级街头霸王Ⅱ 15周年纪念版",
				RomPath: "arc/CAPCOM/STREET FIGHTER/hsf2.zip", Family: 1},
		},
	}
}

// TestSameNameShowsTheExtension: two dumps both called hsf2 would be two rows
// saying the same thing, so the row that tells them apart carries the whole file
// name — extension included.
func TestSameNameShowsTheExtension(t *testing.T) {
	a := testApp(twoContainers(), "arcade", "arc")
	a.buildVersions()
	a.refilter()
	a.stale = true

	if got := a.versionName(a.lib.Games[0]); got != "超级街头霸王Ⅱ 15周年纪念版 · hsf2.7z" {
		t.Errorf("the .7z row reads %q", got)
	}
	if got := a.versionName(a.lib.Games[1]); got != "超级街头霸王Ⅱ 15周年纪念版 · hsf2.zip" {
		t.Errorf("the .zip row reads %q", got)
	}

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	*a.versionOpen(1) = true
	a.stale = true
	tt.Frame()
	if !tt.HasText("超级街头霸王Ⅱ 15周年纪念版 · hsf2.zip") {
		t.Errorf("the list does not tell the two apart: %v", tt.Texts())
	}
}

// TestSearchFindsARomName: the search asks for what is written on the file as
// well as the title — "hsf2" finds the card of a game whose title is Chinese.
func TestSearchFindsARomName(t *testing.T) {
	l := twoContainers()
	l.Games = append(l.Games, library.Game{
		ID: 3, PlatformKey: "arc", Title: "恐龙新世纪", RomPath: "arc/dino.zip",
	})
	a := testApp(l, "arcade", "arc")
	a.buildVersions()
	a.query = "hsf2"
	a.refilter()
	if len(a.shown) != 1 || a.shown[0].ID != 1 {
		t.Fatalf("search 游戏名? shown = %v", titles(a.shown))
	}

	// 搜扩展名也能命中，因为比的是整个文件名。
	a.query = "hsf2.zip"
	a.refilter()
	if len(a.shown) != 1 || a.shown[0].ID != 1 {
		t.Fatalf("search hsf2.zip shown = %v", titles(a.shown))
	}

	// 游戏名照旧能搜。
	a.query = "恐龙"
	a.refilter()
	if len(a.shown) != 1 || a.shown[0].ID != 3 {
		t.Fatalf("search 恐龙 shown = %v", titles(a.shown))
	}

	// 赛选里那份放在 hsf2.7z 里，搜 rom 名也能把整张卡找出来。
	a.query = "hsf2.7z"
	a.refilter()
	if len(a.shown) != 1 || a.shown[0].ID != 1 {
		t.Fatalf("search hsf2.7z shown = %v", titles(a.shown))
	}
}
