package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// twoHacks is a shelf of two arcade dumps whose titles share nothing: the
// 街霸2 card and a hack named after its file, as an imported romset starts.
func twoHacks() *library.Library {
	return &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "街头霸王2", RomPath: "arc/sf2.zip"},
			{ID: 2, PlatformKey: "arc", Title: "sf2ce", RomPath: "arc/sf2ce.zip"},
		},
	}
}

// TestHandFilingWinsOverTitles: a dump filed under a card stays under it, even
// when its own title would have filed it on a card of its own.
func TestHandFilingWinsOverTitles(t *testing.T) {
	l := twoHacks()
	a := testApp(l, "arcade", "arc")
	a.buildVersions()
	if len(a.versionsOf(a.shown[0])) != 0 {
		t.Fatalf("the two titles are their own games before the hand filing")
	}

	l.Games[1].Family = 1
	a.buildVersions()
	a.refilter()
	if len(a.shown) != 1 {
		t.Fatalf("shown = %v, want the one card", titles(a.shown))
	}
	versions := a.versionsOf(a.shown[0])
	if len(versions) != 2 || versions[1].ID != 2 {
		t.Fatalf("versions = %v, want the base and the hack", versions)
	}
}

// TestSoloKeepsItsOwnCard: a dump that matched by title is sent out with
// 独立运行 and keeps a card of its own again.
func TestSoloKeepsItsOwnCard(t *testing.T) {
	l := &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "真侍魂", RomPath: "arc/samsho2.zip"},
			{ID: 2, PlatformKey: "arc", Title: "真侍魂 sp", RomPath: "arc/samsho2sp.zip"},
		},
	}
	a := testApp(l, "arcade", "arc")
	a.buildVersions()
	a.refilter()
	if len(a.shown) != 1 {
		t.Fatalf("shown = %v, want one card for the family", titles(a.shown))
	}

	l.Games[1].Solo = true
	a.buildVersions()
	a.refilter()
	if len(a.shown) != 2 {
		t.Fatalf("shown = %v, want the dump on a card of its own", titles(a.shown))
	}
}

// TestDropFilesTheDumpUnderTheCard: a zip dropped onto a card is filed under it
// by its file name, and a file the library does not know is only reported.
func TestDropFilesTheDumpUnderTheCard(t *testing.T) {
	a := testApp(twoHacks(), "arcade", "arc")
	a.buildVersions()
	a.mergeFiles(1, []string{`C:\Users\x\Downloads\sf2ce.zip`, `C:\Users\x\Downloads\nope.zip`})
	if g := a.lib.Game(2); g == nil || g.Family != 1 || g.Solo {
		t.Fatalf("game 2 = %+v after the drop, want it filed under 1", g)
	}
	if len(a.shown) != 1 {
		t.Fatalf("shown = %v, want the one card", titles(a.shown))
	}
	if a.status == "" {
		t.Error("the drop said nothing")
	}
}

// TestDropAFolderFilesItsRoms: dropping the folder the hacks sit in files every
// rom of it the library knows, which is the "choose the folder" way of doing it.
func TestDropAFolderFilesItsRoms(t *testing.T) {
	a := testApp(twoHacks(), "arcade", "arc")
	a.buildVersions()
	dir := t.TempDir()
	for _, name := range []string{"sf2ce.zip", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a.mergeFiles(1, []string{dir})
	if g := a.lib.Game(2); g == nil || g.Family != 1 {
		t.Fatalf("game 2 = %+v after the folder was dropped, want it filed under 1", g)
	}
}

// TestVersionRowMenuOffersSplit: a dump in the card's version list opens its own
// menu on the right button, with 取消合并.
func TestVersionRowMenuOffersSplit(t *testing.T) {
	l := &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "真侍魂", RomPath: "arc/samsho2.zip"},
			{ID: 2, PlatformKey: "arc", Title: "真侍魂 sp", RomPath: "arc/samsho2sp.zip"},
		},
	}
	a := testApp(l, "arcade", "arc")
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	// 点开版本弹层（本用例只关心行上的右键菜单，弹层怎么开由 versionBar 的另一条用例管）。
	*a.versionOpen(1) = true
	a.stale = true
	tt.Frame()

	var row *ui.Rect
	for _, info := range tt.AccessAll() {
		if info.Bounds.H >= 34 && info.Bounds.H <= 38 && strings.Contains(info.Name, "sp") {
			r := info.Bounds
			row = &r
		}
	}
	if row == nil {
		for _, info := range tt.AccessAll() {
			t.Logf("%q %v", info.Name, info.Bounds)
		}
		t.Fatal("no row for the hack in the version list")
	}
	tt.RightClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	if !tt.HasText("取消合并") {
		t.Error("the row's menu does not offer 取消合并")
	}

	// And the button in it is the split: clicking it files the dump on its own.
	var split *ui.Rect
	for _, info := range tt.AccessAll() {
		if strings.Contains(info.Name, "取消合并") {
			r := info.Bounds
			split = &r
		}
	}
	if split == nil {
		t.Fatal("the menu's 取消合并 has no box to click")
	}
	tt.ClickAt(split.X+split.W/2, split.Y+split.H/2)
	tt.Frame() // view 的第一行就是 runQueue
	if g := a.lib.Game(2); g == nil || !g.Solo {
		t.Fatalf("game 2 = %+v after the menu's 取消合并, want it solo", g)
	}
}

// TestSplitDumpGivesACard: 取消合并 takes the dump out of the family and puts it
// on a card, and the card's own menu offers the button while it has versions.
func TestSplitDumpGivesACard(t *testing.T) {
	l := &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "真侍魂", RomPath: "arc/samsho2.zip"},
			{ID: 2, PlatformKey: "arc", Title: "真侍魂 sp", RomPath: "arc/samsho2sp.zip"},
		},
	}
	a := testApp(l, "arcade", "arc")
	a.buildVersions()
	a.refilter()
	base := a.shown[0]
	if len(a.verMembers[a.verFam[base.ID]]) < 2 {
		t.Fatal("the family has no versions to split")
	}

	a.splitDump(base.ID) // 与改元数据一样排到下一帧
	a.runQueue()
	if g := a.lib.Game(base.ID); g == nil || !g.Solo {
		t.Fatalf("game %d = %+v after 独立运行, want it solo", base.ID, g)
	}
	if len(a.shown) != 2 {
		t.Fatalf("shown = %v, want the split dump on its own card", titles(a.shown))
	}
}
