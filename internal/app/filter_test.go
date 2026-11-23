package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestFilterPanelUsesTwoWideColumns: the parameters are two columns on one page,
// not one narrow scroll. The right column starts far enough to the right of the
// left one that the two halves read as two blocks, every name has its own 144 and
// every control the same 100 at the column's right edge, and the last parameter
// is visible on a 720p window without a second scroll.
func TestFilterPanelUsesTwoWideColumns(t *testing.T) {
	a := &App{
		root:  t.TempDir(),
		theme: buildTheme(),
		crt:   defaultCRT(),
	}
	a.cfg.FilterLevel = "normal"
	tt := ui.NewTester(func(c *ui.Context) { a.filterPanel(c, &a.theme) }, 1200, 720)
	tt.Frame()

	left, ok := tt.Find("模拟伽马") // first knob, left column
	if !ok {
		t.Fatalf("the left column's first label is missing")
	}
	right, ok := tt.Find("曝光") // first knob of the second half, right column
	if !ok {
		t.Fatalf("the right column's first label is missing")
	}
	if right.X-left.X < 300 {
		t.Fatalf("the columns are %.0f apart, want at least 300", right.X-left.X)
	}
	if last, ok := tt.Find("上下间距 (%)"); !ok {
		t.Fatalf("the last knob is clipped off a 720p window")
	} else if last.X < right.X {
		t.Fatalf("the last knob starts at %.0f, left of the right column at %.0f", last.X, right.X)
	}

	if w := filterPanelW; w != 720 {
		t.Fatalf("the filter panel is %d wide, want the 720 the window has room for", w)
	}
	col := (filterPanelW - 2*filterPanelPad - filterColumnGap) / 2
	if want := filterLabelW + 8 + filterPillW; col != want {
		t.Fatalf("each column is %d wide, but a name and its control need %d", col, want)
	}
	if filterPanelPad != 48 || filterColumnGap != 120 {
		t.Fatalf("padding %d and gutter %d, want three times the old padding (48) and five times the old gutter (120)", filterPanelPad, filterColumnGap)
	}
	if filterLabelW != 144 {
		t.Fatalf("the name is %d wide, want 144 (30%% less than the 204 it was)", filterLabelW)
	}

	// A shorter window keeps the same two columns but clips the body into one
	// scroll; the footer must still be on screen, not pushed below the window.
	short := ui.NewTester(func(c *ui.Context) { a.filterPanel(c, &a.theme) }, 1200, 660)
	short.Frame()
	footer, ok := short.Find("保存")
	if !ok {
		t.Fatalf("the 保存 button is missing on a 660p window")
	}
	if footer.Y+footer.H > 660 {
		t.Fatalf("the footer ends at %.0f, below a 660p window", footer.Y+footer.H)
	}
	if _, ok := short.Find("一组参数，保存后可切换"); ok {
		t.Fatalf("the description line is still drawn under the buttons")
	}
}
