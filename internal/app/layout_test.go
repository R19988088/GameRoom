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

// TestToolbarClearsTheChips lays the window out and checks the floating
// bar really floats above the content: its shelf buttons end before the
// platform and menu pills begin.
func TestToolbarClearsTheChips(t *testing.T) {
	lib := fakeLibrary()
	a := &App{
		root:     t.TempDir(),
		status:   "就绪",
		theme:    buildTheme(),
		lib:      lib,
		group:    "arcade",
		platform: "arc",
		counts:   lib.Counts(),
		covers:   map[string]*ui.Bitmap{},
		icons:    map[string]*ui.Bitmap{},
		pending:  map[string]bool{},
	}
	a.recount()
	a.refilter()

	tt := ui.NewTester(a.view, 1500, 940)
	tt.Frame()

	var barBottom, chipsTop float32 = -1, -1
	for _, info := range tt.AccessAll() {
		switch {
		case strings.HasPrefix(info.Name, "街机"):
			if barBottom < info.Bounds.Y+info.Bounds.H {
				barBottom = info.Bounds.Y + info.Bounds.H
			}
		case strings.HasPrefix(info.Name, "全部"):
			if chipsTop < 0 || info.Bounds.Y < chipsTop {
				chipsTop = info.Bounds.Y
			}
		}
	}
	// The chips row is the content's first line; the bar sits above it.
	if barBottom < 0 || chipsTop < 0 {
		t.Skipf("bar %v chips %v not found", barBottom, chipsTop)
	}
	if barBottom >= chipsTop-6 {
		t.Errorf("the bar ends at %v, the chips start at %v: they overlap", barBottom, chipsTop)
	}
}
