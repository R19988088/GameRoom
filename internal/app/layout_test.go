package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/library"
)

// fakeLibrary is a small library in the shape Load returns.
func fakeLibrary() *library.Library {
	lib := &library.Library{
		Platforms: []library.Platform{
			{ID: 1, Key: "arc", Name: "ARCADE", Group: "arcade", Sort: 2},
			{ID: 2, Key: "fc", Name: "FC", Group: "console", Sort: 3},
			{ID: 3, Key: "掌机", Name: "掌机", Group: "handheld", Sort: 8},
			{ID: 4, Key: "微软", Name: "微软", Group: "pc", Sort: 10},
		},
		Menus: []library.Menu{
			{PlatformKey: "arc", Name: "SNK", Sort: 1},
			{PlatformKey: "arc", Name: "CAPCOM", Sort: 2},
		},
	}
	for i := 0; i < 9; i++ {
		lib.Games = append(lib.Games, library.Game{
			ID: i + 1, PlatformKey: "arc", Menu: "SNK",
			Title: fmt.Sprintf("Game %d", i), RomPath: fmt.Sprintf("arc/g%d.zip", i),
		})
	}
	lib.Games = append(lib.Games, library.Game{ID: 99, PlatformKey: "fc", Title: "Mario"})
	return lib
}

func testApp(lib *library.Library, group, platform string) *App {
	a := &App{
		root:     "root",
		status:   "就绪",
		theme:    buildTheme(),
		lib:      lib,
		group:    group,
		platform: platform,
		counts:   lib.Counts(),
		covers:   map[string]*ui.Bitmap{},
		icons:    map[string]*ui.Bitmap{},
		pending:  map[string]bool{},
	}
	a.recount()
	a.refilter()
	return a
}

type rect struct{ x, y, w, h float32 }

// TestTopRowIsThreeCapsules: the shelf picker, the shelf's chips and the
// search sit in one row as three separate capsules, not one long bar.
func TestTopRowIsThreeCapsules(t *testing.T) {
	a := testApp(fakeLibrary(), "arcade", "arc")
	tt := ui.NewTester(a.view, 1500, 940)
	tt.Frame()

	find := func(prefix string) rect {
		for _, info := range tt.AccessAll() {
			if strings.HasPrefix(info.Name, prefix) && info.Bounds.H < 70 {
				return rect{info.Bounds.X, info.Bounds.Y, info.Bounds.W, info.Bounds.H}
			}
		}
		return rect{-1, -1, 0, 0}
	}
	shelf, all, search := find("街机"), find("全部"), find("搜索游戏名")
	if shelf.y < 0 || all.y < 0 || search.y < 0 {
		t.Fatalf("not all three capsules found: shelf %v all %v search %v", shelf, all, search)
	}
	// One row: their middles line up (text nodes sit a few DIPs off a
	// button's box, so compare centres).
	for _, r := range []rect{shelf, all, search} {
		if d := (r.y + r.h/2) - (shelf.y + shelf.h/2); d > 4 || d < -4 {
			t.Errorf("capsule centred at %v, shelf centred at %v: not one row",
				r.y+r.h/2, shelf.y+shelf.h/2)
		}
	}
	// Three capsules, not one: there is a gap between each pair.
	if shelf.x+shelf.w > all.x-4 {
		t.Errorf("shelf ends at %v, chips begin at %v: one bar, not two capsules", shelf.x+shelf.w, all.x)
	}
	if all.x+all.w > search.x-4 {
		t.Errorf("chips end at %v, search begins at %v: one bar, not two capsules", all.x+all.w, search.x)
	}
	// And the row floats: everything in it ends above the covers.
	if all.y+all.h > 82 {
		t.Errorf("the chips capsule ends at %v, covers start at 98:", all.y+all.h)
	}
	var cardTop float32 = 1e6
	for _, info := range tt.AccessAll() {
		if info.Bounds.H > 100 && info.Bounds.W < 260 && info.Bounds.Y < cardTop {
			cardTop = info.Bounds.Y
		}
	}
	if cardTop < 82 {
		t.Errorf("a card starts at %v, under the capsules", cardTop)
	}
}
