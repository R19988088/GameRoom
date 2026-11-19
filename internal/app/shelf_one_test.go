package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/library"
)

// twoShelves is a 街机 shelf holding ARCADE and a 麻将 shelf holding 麻将, the
// two one-platform shelves.
func twoShelves() *library.Library {
	lib := &library.Library{}
	groups := []struct {
		key, name, group string
		n                int
	}{
		{"arc", "ARCADE", "arcade", 3},
		{"majiang", "麻将", "mahjong", 2},
	}
	id := 1
	for i, g := range groups {
		lib.Platforms = append(lib.Platforms, library.Platform{
			ID: i + 1, Key: g.key, Name: g.name, Group: g.group, Sort: i,
		})
		for n := 0; n < g.n; n++ {
			lib.Games = append(lib.Games, library.Game{
				ID: id, PlatformKey: g.key, Title: g.name + " game", RomPath: g.key + "/g.zip",
			})
			id++
		}
	}
	return lib
}

// TestSinglePlatformShelfHasNoChip: a shelf of one platform shows no
// platform chip, because its games are right under the bar already.
func TestSinglePlatformShelfHasNoChip(t *testing.T) {
	for _, shelf := range []struct{ group, platform, chip string }{
		{"arcade", "arc", "ARCADE"},
		{"mahjong", "majiang", "麻将"},
	} {
		a := newTestApp(twoShelves(), shelf.platform)
		a.group = shelf.group
		a.refilter()

		tt := ui.NewTester(a.view, 1400, 900)
		tt.Frame()
		for _, info := range tt.AccessAll() {
			// Anything on the top row (y under the capsules) named after the
			// platform would be a chip; the shelf button carries 街机/麻将.
			if info.Bounds.Y < 82 && strings.HasPrefix(info.Name, shelf.chip) && !strings.Contains(info.Name, "▾") {
				t.Errorf("shelf %s draws a %q chip on the top row: %q", shelf.group, shelf.chip, info.Name)
			}
		}
		if !tt.HasText("搜索游戏名") {
			t.Errorf("shelf %s lost the search field", shelf.group)
		}
	}
}

// TestMahjongIsItsOwnShelf: 麻将 games belong to the 麻将 shelf and the arcade
// shelf counts only the arcade's.
func TestMahjongIsItsOwnShelf(t *testing.T) {
	a := newTestApp(twoShelves(), "arc")
	if a.counts["arc"] != 3 {
		t.Errorf("arcade shelf has %d games, want 3", a.counts["arc"])
	}
	a.chooseGroup("mahjong")
	if a.platform != "majiang" {
		t.Errorf("麻将 shelf chose platform %q", a.platform)
	}
	if len(a.shown) != 2 {
		t.Errorf("麻将 shelf shows %d games, want 2", len(a.shown))
	}
}
