package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestOneClickPicksAVersion: a single click on a row of the version list takes
// that dump and closes the list — no second click.
func TestOneClickPicksAVersion(t *testing.T) {
	a := testApp(threeDumps(), "arcade", "arc")
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	*a.versionOpen(1) = true
	a.stale = true
	tt.Frame()

	// 每一行写它的完整名字，基准那份不写成「主 ROM」，选中的只是一个 ✓。
	if tt.HasText("主 ROM") {
		t.Error("the list still calls the base 主 ROM")
	}
	if !tt.HasText("真侍魂 斩红郎无双剑") {
		t.Errorf("a row does not show its whole title: %v", tt.Texts())
	}
	if !tt.HasText("真侍魂") {
		t.Error("the base row does not show its title")
	}
	// 每一行右边还写着这一份 rom 的文件名，两份同名标题的改版就靠它分。
	for _, rom := range []string{"samsho2", "samsho2sp"} {
		if !tt.HasText(rom) {
			t.Errorf("the list does not show the rom %q: %v", rom, tt.Texts())
		}
	}

	row, ok := tt.Find("真侍魂 sp")
	if !ok {
		t.Fatalf("the second dump has no row in the list: %v", tt.Texts())
	}
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()

	if *a.versionOpen(1) {
		t.Error("the list stayed open after one click")
	}
	if a.verSel != 2 {
		t.Errorf("verSel = %d after the click, want the dump 2", a.verSel)
	}
	for _, gone := range []string{"双击设为默认"} {
		if tt.HasText(gone) {
			t.Errorf("%q is still in the list", gone)
		}
	}
}

// TestCardCoverFollowsTheDefaultVersion: the picture under the title is the
// cover of the dump that plays, and changes when another dump is made the
// default; a dump without a cover leaves the card its own.
func TestCardCoverFollowsTheDefaultVersion(t *testing.T) {
	l := threeDumps()
	l.Games[0].Cover = "covers/arc/samsho2.png"
	l.Games[1].Cover = "covers/arc/samsho2sp.png"
	a := testApp(l, "arcade", "arc")
	a.buildVersions()
	base := a.lib.Games[0]
	key := a.verFam[base.ID]

	a.verDefault[key] = 2
	if got := a.cardCover(base); got != "covers/arc/samsho2sp.png" {
		t.Errorf("cover = %q with dump 2 chosen, want its own", got)
	}
	a.verDefault[key] = 1
	if got := a.cardCover(base); got != "covers/arc/samsho2.png" {
		t.Errorf("cover = %q with the base chosen, want the base's", got)
	}
	a.lib.Games[1].Cover = ""
	a.buildVersions() // 版本族的成员是快照，改完要重排
	a.verDefault[key] = 2
	if got := a.cardCover(base); got != "covers/arc/samsho2.png" {
		t.Errorf("cover = %q for a dump without a cover, want the base's", got)
	}
	if !strings.HasPrefix(a.cardCover(base), "covers/") {
		// The cover is a path under 数据/封面, not a bitmap: sanity on the form.
		t.Logf("cover path: %q", a.cardCover(base))
	}
}
