package app

import (
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// console: 3DO sorts first but has one game, FC has many — entering the shelf
// must open on FC, not on an empty-looking 3DO.
func TestEnteringAShelfOpensOnItsBusiestPlatform(t *testing.T) {
	l := &library.Library{
		Platforms: []library.Platform{
			{Key: "3DO", Name: "3DO", Group: "console", Sort: 1},
			{Key: "fc", Name: "FC", Group: "console", Sort: 2},
		},
	}
	for i := 0; i < 4; i++ {
		l.Games = append(l.Games, library.Game{PlatformKey: "fc", Title: "fc game"})
	}
	l.Games = append(l.Games, library.Game{PlatformKey: "3DO", Title: "3do game"})

	a := &App{
		lib:     l,
		group:   "arcade",
		theme:   buildTheme(),
		covers:  map[string]*ui.Bitmap{},
		icons:   map[string]*ui.Bitmap{},
		pending: map[string]bool{},
	}
	a.chooseGroup("console")
	if a.platform != "fc" {
		t.Errorf("console shelf opened on %q, want fc", a.platform)
	}
	if a.axis != library.AxisMachine {
		t.Errorf("console axis = %q, want %q", a.axis, library.AxisMachine)
	}

	// Switching to 厂商 keeps a platform on screen that has games.
	a.chooseAxis(library.AxisMaker)
	if a.platform != "fc" {
		t.Errorf("maker axis left platform %q, want fc", a.platform)
	}

	// 街机 opens on 厂商 and switches to 基板.
	a.chooseGroup("arcade")
	if a.axis != library.AxisMaker {
		t.Errorf("arcade axis = %q, want %q", a.axis, library.AxisMaker)
	}
	a.nextAxis()
	if a.axis != library.AxisBoard {
		t.Errorf("next axis = %q, want %q", a.axis, library.AxisBoard)
	}
}

// 麻将 has a single standard (年份) and must not offer a switch.
func TestMahjongHasNoAxisSwitch(t *testing.T) {
	l := &library.Library{
		Platforms: []library.Platform{{Key: "majiang", Name: "麻将", Group: "mahjong"}},
	}
	l.Games = append(l.Games, library.Game{PlatformKey: "majiang", Title: "mj", Year: 2001})

	a := &App{
		lib:     l,
		group:   "arcade",
		theme:   buildTheme(),
		covers:  map[string]*ui.Bitmap{},
		icons:   map[string]*ui.Bitmap{},
		pending: map[string]bool{},
	}
	a.chooseGroup("mahjong")
	if got := a.axes(); len(got) != 1 || got[0] != library.AxisYear {
		t.Errorf("麻将 axes = %v, want only 年份", got)
	}
	a.nextAxis()
	if a.axis != library.AxisYear {
		t.Errorf("麻将 axis changed to %q", a.axis)
	}
}
