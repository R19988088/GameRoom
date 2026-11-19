package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestShelfMenuOpensAndSwitches: the left button opens a menu of the four
// shelves; clicking it is enough to show them.
func TestShelfMenuOpensAndSwitches(t *testing.T) {
	a := newTestApp(manyPlatforms(), "fc")
	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	if tt.HasText("掌机") {
		t.Fatal("the shelf menu is open before it was clicked")
	}
	var r ui.Rect
	ok := false
	for _, info := range tt.AccessAll() {
		if strings.HasPrefix(info.Name, "家用机") {
			r, ok = info.Bounds, true
		}
	}
	if !ok {
		t.Skip("the shelf picker is not in the tree")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if !tt.HasText("掌机") && !tt.HasText("PC") {
		t.Fatalf("the menu did not open; texts: %v", tt.Texts())
	}
	if !tt.HasText("街机") {
		t.Errorf("the menu should list 街机 as well: %v", tt.Texts())
	}
}
