package app

import (
	"os"
	"path/filepath"
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
	// The title and its ✕ sit on the glass's own top edge, 16 in, not in the
	// columns' 48.
	if title, ok := tt.Find("脑浆油条 参数"); !ok {
		t.Fatalf("the panel's title is missing")
	} else if cy := title.Y + title.H/2; cy > 16+16+filterHeaderH {
		t.Fatalf("the title's middle is at %.0f, too far below the panel's top edge", cy)
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

// TestFilterPanelFitsANarrowWindow: the glass never runs past the window's
// edges. A window too narrow for the 720 leaves the panel what is between the two
// insets, and the columns give way instead — the bars get narrower rather than
// being cut off at the right side.
func TestFilterPanelFitsANarrowWindow(t *testing.T) {
	a := &App{root: t.TempDir(), theme: buildTheme(), crt: defaultCRT()}
	a.cfg.FilterLevel = "normal"
	var got float32
	tt := ui.NewTester(func(c *ui.Context) {
		got = a.filterPanelWidth(c)
		a.filterPanel(c, &a.theme)
	}, 700, 800)
	tt.Frame()
	if got > 700-2*edgeInset {
		t.Fatalf("the panel is %.0f wide on a 700 window, want at most %d", got, 700-2*edgeInset)
	}
	if last, ok := tt.Find("上下间距 (%)"); !ok {
		t.Fatalf("the last knob is missing on a narrow window")
	} else if right := last.X + filterLabelW + 8 + 60; right > 700-edgeInset-filterPanelPad {
		t.Fatalf("the right column ends at %.0f, past the glass at %d", right, 700-edgeInset-filterPanelPad)
	}
}

// TestFilterPanelFooterCards: the saved groups are cards on the footer's left.
// With nothing saved the footer offers one 保存; with a card lit it offers 更新 /
// 另存 / 删除 and no 保存, and the cards sit to the left of those buttons.
func TestFilterPanelFooterCards(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "数据", "filters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tune-1.json", "tune-2.json"} {
		if err := os.WriteFile(filepath.Join(dir, name),
			[]byte(`{"level":"normal","tune":{"scan":0.4}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{root: root, theme: buildTheme(), crt: defaultCRT()}
	a.cfg.FilterLevel = "normal"
	a.cfg.FilterTune = "tune-1.json"
	a.scanTunes()
	tt := ui.NewTester(func(c *ui.Context) { a.filterPanel(c, &a.theme) }, 1200, 720)
	tt.Frame()

	if _, ok := tt.Find("保存"); ok {
		t.Fatalf("保存 is drawn beside a lit card, want 更新 / 另存 / 删除")
	}
	update, ok := tt.Find("更新")
	if !ok {
		t.Fatalf("更新 is missing from the footer of a lit card")
	}
	for _, label := range []string{"另存", "删除"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("%s is missing from the footer of a lit card", label)
		}
	}
	card, ok := tt.Find("2")
	if !ok {
		t.Fatalf("the second saved group has no card")
	}
	if card.X >= update.X {
		t.Fatalf("the cards start at %.0f, not left of the buttons at %.0f", card.X, update.X)
	}

	// Deleting the last card leaves nothing to update, so 保存 comes back.
	if err := a.deleteTune(); err != nil {
		t.Fatal(err)
	}
	if err := a.deleteTune(); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("保存"); !ok {
		t.Fatalf("保存 is missing with no saved group")
	}
	for _, label := range []string{"更新", "另存", "删除"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("%s is still drawn with no lit card", label)
		}
	}
}
