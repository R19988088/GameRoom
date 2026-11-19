package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// menuRow finds the row of a card's menu by its label.
func menuRow(t *testing.T, tt *ui.Tester, prefix string) ui.Rect {
	t.Helper()
	best := ui.Rect{X: -1, W: 0, H: 0}
	for _, info := range tt.AccessAll() {
		if !strings.HasPrefix(info.Name, prefix) {
			continue
		}
		if best.X < 0 || info.Bounds.Y < best.Y {
			best = info.Bounds
		}
	}
	if best.X < 0 {
		t.Fatalf("the menu has no %q row", prefix)
	}
	return best
}

// openMenu right clicks the first card and lays the menu out.
func openMenu(t *testing.T, a *App) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(a.view, 1486, 908)
	tt.Frame()
	tt.RightClickAt(146, 200)
	tt.Frame()
	if !tt.HasText("游戏名") {
		t.Fatal("the menu did not open")
	}
	return tt
}

// TestNameRowIsTypedIn: 游戏名那一行单击就进输入框，改了名字卡片和库里都跟着
// 变。它曾经按名字筛，一筛整个货架只剩一张卡——那不是看名字的意思；更早它
// 还因为在画卡片时重排列表，把窗口直接弄死（index out of range）。
func TestNameRowIsTypedIn(t *testing.T) {
	a := testApp(fakeLibrary(), "arcade", "arc")
	tt := openMenu(t, a)

	before := len(a.shown)
	id := a.shown[0].ID
	row := menuRow(t, tt, "游戏名")
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()

	if a.cardField != "title" || !a.cardEdit {
		t.Fatalf("the menu is on %q edit=%v, want the name being typed in", a.cardField, a.cardEdit)
	}
	if a.query != "" {
		t.Errorf("query = %q, want empty: the name row must not filter", a.query)
	}
	if len(a.shown) != before {
		t.Errorf("shown = %d games, want the %d before the click", len(a.shown), before)
	}

	a.cardText = "New Name"
	a.assign("title", a.cardText)
	a.runQueue()

	g := a.lib.Game(id)
	if g == nil || g.Title != "New Name" || g.Name != "New Name" {
		t.Errorf("the game is %+v, want it renamed", g)
	}
	if !tt.HasText("New Name") {
		// The card is drawn again next frame; the library is what the next
		// frame reads.
		tt.Frame()
		if !tt.HasText("New Name") {
			t.Error("the card still shows the old name")
		}
	}
}

// TestValueClickRewritesTheGame: a click on a value writes it and re-files the
// games — the same mid-frame danger, so this checks the window survives it and
// the card takes the value.
func TestValueClickRewritesTheGame(t *testing.T) {
	a := testApp(fakeLibrary(), "arcade", "arc")
	tt := openMenu(t, a)

	game := a.shown[0]
	// 街机货架的机种就是平台：ARCADE 是这一层里唯一的值。
	row := menuRow(t, tt, "基板")
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	if a.cardField != "machine" {
		t.Fatalf("the menu is showing %q, want machine", a.cardField)
	}

	value := a.values("machine", game)[0]
	tt.Frame()
	row = menuRow(t, tt, value)
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame() // runs the queued write
	tt.Frame()

	if got := a.lib.Game(game.ID); got == nil || got.PlatformKey != "arc" {
		t.Errorf("platform = %q, want arc", got.PlatformKey)
	}
	if value != "ARCADE" {
		t.Errorf("value = %q, want ARCADE", value)
	}
	if a.cardField != "" {
		t.Errorf("the menu is still showing %q", a.cardField)
	}
}
