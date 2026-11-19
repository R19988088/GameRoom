package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/library"
)

// manyPlatforms is a console shelf of six platforms with uneven counts.
func manyPlatforms() *library.Library {
	lib := &library.Library{}
	keys := []string{"fc", "sfc", "ps", "SEGA", "任天堂", "3DO"}
	counts := []int{40, 30, 20, 10, 5, 1}
	id := 1
	for i, k := range keys {
		lib.Platforms = append(lib.Platforms, library.Platform{
			ID: i + 1, Key: k, Name: k, Group: "console", Sort: i,
		})
		for n := 0; n < counts[i]; n++ {
			lib.Games = append(lib.Games, library.Game{
				ID: id, PlatformKey: k, Title: k + " game", RomPath: k + "/g.zip",
			})
			id++
		}
	}
	return lib
}

func newTestApp(lib *library.Library, platform string) *App {
	a := &App{
		root:     "root",
		status:   "就绪",
		theme:    buildTheme(),
		lib:      lib,
		group:    "console",
		platform: platform,
		counts:   lib.Counts(),
		covers:   map[string]*ui.Bitmap{},
		icons:    map[string]*ui.Bitmap{},
		pending:  map[string]bool{},
	}
	a.axis = a.defaultAxis()
	lib.SplitGroup(a.group, a.axis)
	a.recount()
	a.refilter()
	return a
}

// TestTopPlatformsShowsFive: the bar carries five chips and 更多 keeps the
// rest.
func TestTopPlatformsShowsFive(t *testing.T) {
	a := newTestApp(manyPlatforms(), "fc")
	top := a.topPlatforms(5)
	if len(top) != 5 {
		t.Fatalf("top = %d platforms, want 5", len(top))
	}
	for i := 1; i < len(top); i++ {
		if a.counts[top[i-1].Key] < a.counts[top[i].Key] {
			t.Errorf("top is not by game count: %v then %v", top[i-1].Key, top[i].Key)
		}
	}
	if rest := a.restPlatforms(5); len(rest) != 1 || rest[0].Key != "3DO" {
		t.Errorf("rest = %+v, want just 3DO", rest)
	}
}

// TestTopPlatformsKeepsTheChosenOne: a quiet platform still gets a chip,
// or the bar would hide the platform whose games are on screen.
func TestTopPlatformsKeepsTheChosenOne(t *testing.T) {
	a := newTestApp(manyPlatforms(), "3DO")
	top := a.topPlatforms(5)
	found := false
	for _, p := range top {
		if p.Key == "3DO" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the chosen 3DO is not among %+v", top)
	}
	if len(top) != 5 {
		t.Errorf("top = %d, want 5", len(top))
	}
}

// TestBarHasThreeParts: the shelf picker on the left, the platform chips in
// the middle with 更多, and the search field on the right, in that order.
func TestBarHasThreeParts(t *testing.T) {
	a := newTestApp(manyPlatforms(), "fc")
	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()

	var shelf, more, search ui.Rect
	var found int
	for _, info := range tt.AccessAll() {
		r := info.Bounds
		switch {
		case strings.HasPrefix(info.Name, "家用机"):
			shelf, found = r, found|1
		case strings.HasPrefix(info.Name, "更多"):
			more, found = r, found|2
		case strings.HasPrefix(info.Name, "搜索"):
			search, found = r, found|4
		}
	}
	if found != 7 {
		t.Skipf("not every part was found: %b", found)
	}
	if shelf.X+shelf.W >= more.X {
		t.Errorf("shelf picker (%v) is not left of 更多 (%v)", shelf, more)
	}
	if more.X+more.W >= search.X {
		t.Errorf("更多 (%v) overlaps the search field (%v)", more, search)
	}
	if search.X+search.W > 1400 {
		t.Errorf("the search field runs past the window: %v", search)
	}
	// All three sit on the bar's one line.
	if shelf.Y < 10 || search.Y+search.H > 62 {
		t.Errorf("the parts are not inside the bar: shelf %v, search %v", shelf, search)
	}
}
