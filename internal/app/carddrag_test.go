package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestCtrlDragFilesOneCardUnderAnother: holding Ctrl and dragging one card onto
// another files the dragged game under that card, so the two become one card
// with two versions. A drag without Ctrl picks nothing up.
func TestCtrlDragFilesOneCardUnderAnother(t *testing.T) {
	a := testApp(twoHacks(), "arcade", "arc")
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	src, ok := tt.Find("sf2ce")
	if !ok {
		t.Fatalf("the hack has no card on screen: %v", titles(a.shown))
	}
	dst, ok := tt.Find("街头霸王2")
	if !ok {
		t.Fatalf("the base card is not on screen: %v", titles(a.shown))
	}

	tt.Press(dst.X+dst.W/2, dst.Y+dst.H/2)
	tt.Frame()
	tt.Move(dst.X+dst.W/2+16, dst.Y+dst.H/2+8)
	tt.Move(src.X+src.W/2, src.Y+src.H/2)
	tt.Frame()
	tt.Release(src.X+src.W/2, src.Y+src.H/2)
	tt.Frame()
	if g := a.lib.Game(2); g == nil || g.Family != 0 {
		t.Fatalf("game 2 = %+v after a plain drag, want it filed nowhere", g)
	}

	// 按着 Ctrl 再按卡片：卡片这时才是拖的来源（Tester 的按键事件不带
	// 修饰键，所以按着之后再 HoldModifiers 一次，让下一帧按 Ctrl 建卡）。
	tt.Press(src.X+src.W/2, src.Y+src.H/2)
	tt.HoldModifiers(ui.Ctrl)
	tt.Frame()
	tt.Move(src.X+src.W/2+16, src.Y+src.H/2+8)
	tt.Frame()
	tt.Move(dst.X+dst.W/2, dst.Y+dst.H/2)
	tt.Frame()
	tt.Release(dst.X+dst.W/2, dst.Y+dst.H/2)
	tt.Frame() // 落下的那一帧收下 Drop，并把它排到下一帧
	tt.Frame() // view 的第一行就是 runQueue
	if g := a.lib.Game(2); g == nil || g.Family != 1 || g.Solo {
		t.Fatalf("game 2 = %+v after the Ctrl drag, want it filed under 1", g)
	}
	if len(a.shown) != 1 {
		t.Fatalf("shown = %v, want the one card", titles(a.shown))
	}
	if a.status == "" {
		t.Error("the merge said nothing")
	}
}
