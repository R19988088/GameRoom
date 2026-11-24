package app

import (
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
)

// threeDumps is one card with two dumps filed under it by hand, as a Ctrl drag
// leaves them: the base 1, and 2 and 3 pointing at it.
func threeDumps() *library.Library {
	return &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "真侍魂", RomPath: "arc/samsho2.zip"},
			{ID: 2, PlatformKey: "arc", Title: "真侍魂 sp", RomPath: "arc/samsho2sp.zip", Family: 1},
			{ID: 3, PlatformKey: "arc", Title: "真侍魂 斩红郎无双剑", RomPath: "arc/samsho3.zip", Family: 1},
		},
	}
}

// TestSplitTheBaseTakesItOutAndKeepsTheRest: 独立运行 on the card's own base dump
// gives it a card of its own, and the dumps that were filed under it move on to
// whoever is left instead of following it out.
func TestSplitTheBaseTakesItOutAndKeepsTheRest(t *testing.T) {
	a := testApp(threeDumps(), "arcade", "arc")
	a.buildVersions()
	a.refilter()
	if len(a.shown) != 1 || a.shown[0].ID != 1 {
		t.Fatalf("shown = %v, want the one card of 1", titles(a.shown))
	}

	a.splitDump(1)
	a.runQueue()
	if g := a.lib.Game(1); g == nil || !g.Solo || g.Family != 0 {
		t.Fatalf("game 1 = %+v after 独立运行, want it solo", g)
	}
	if a.verFam[2] != a.verFam[3] {
		t.Errorf("games 2 and 3 fell apart: %q %q", a.verFam[2], a.verFam[3])
	}
	if a.verFam[1] == a.verFam[2] {
		t.Errorf("game 1 still shares the family of 2: %q", a.verFam[1])
	}
	if len(a.shown) != 2 {
		t.Fatalf("shown = %v, want the split card and the family left", titles(a.shown))
	}
}

// TestSplitAVersionTakesOnlyItOut: 独立运行 on a dump that is not the base leaves
// the base's card as it was.
func TestSplitAVersionTakesOnlyItOut(t *testing.T) {
	a := testApp(threeDumps(), "arcade", "arc")
	a.buildVersions()
	a.refilter()

	a.splitDump(2)
	a.runQueue()
	if g := a.lib.Game(2); g == nil || !g.Solo {
		t.Fatalf("game 2 = %+v after 独立运行, want it solo", g)
	}
	if a.verFam[1] != a.verFam[3] {
		t.Errorf("the family lost game 3 as well: %q %q", a.verFam[1], a.verFam[3])
	}
	if len(a.shown) != 2 {
		t.Fatalf("shown = %v, want two cards", titles(a.shown))
	}
}
