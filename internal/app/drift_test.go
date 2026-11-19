package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScrollSettlesAtRest: several frames with no input must not move the
// body. A shelf that drifts to the middle of its first page on its own hides
// the section headers and cuts the top row.
func TestScrollSettlesAtRest(t *testing.T) {
	a := testApp(fakeLibrary(), "arcade", "arc")
	tt := ui.NewTester(a.view, 1486, 908)
	for i := 0; i < 6; i++ {
		tt.Frame()
		t.Logf("frame %d: Y=%.0f", i, a.scroll.Y)
	}
	if a.scroll.Y != 0 {
		t.Errorf("Y = %.0f after settling, want 0", a.scroll.Y)
	}
}
