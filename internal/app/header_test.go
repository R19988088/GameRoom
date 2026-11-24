package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSectionHeadingIsOnlyALabel: clicking the heading — its words or the strip
// it sits on — must change nothing. It used to file the shelf by that section's
// value, and a click there collapsed the shelf to that one group, which reads as
// the shelf leaving. The chip above is the way to filter; the heading is a label.
func TestSectionHeadingIsOnlyALabel(t *testing.T) {
	a := testApp(fakeLibrary(), "arcade", "arc")
	a.buildVersions()
	a.refilter()
	a.stale = true
	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()

	if len(a.lines) == 0 || a.lines[0].head < 0 {
		t.Fatalf("no heading line to click: %+v", a.lines)
	}
	label, ok := tt.Find("SNK")
	if !ok {
		t.Fatalf("the heading has no label: %v", tt.Texts())
	}
	y := a.lines[0].top + 15
	before := len(a.shown)

	// 横条中间的空白。
	tt.ClickAt(900, y)
	tt.Frame()
	if a.menu != "" || a.special != "" || len(a.shown) != before {
		t.Errorf("a click on the blank strip did something: menu=%q special=%q shown=%d",
			a.menu, a.special, len(a.shown))
	}

	// 标题本身。
	tt.ClickAt(label.X+label.W/2, label.Y+label.H/2)
	tt.Frame()
	if a.menu != "" || a.special != "" || len(a.shown) != before {
		t.Errorf("a click on the heading did something: menu=%q special=%q shown=%d",
			a.menu, a.special, len(a.shown))
	}
}
