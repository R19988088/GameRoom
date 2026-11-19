package app

import (
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// threeMakers is a shelf of five arcade games: CAPCOM twice, SNK twice, one
// with nobody's name on it.
func threeMakers() *library.Library {
	l := &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
	}
	add := func(id int, title, maker string) {
		l.Games = append(l.Games, library.Game{
			ID: id, PlatformKey: "arc", Title: title, RomPath: "arc/" + title + ".zip",
			Publisher: maker,
		})
	}
	add(1, "Alpha", "CAPCOM")
	add(2, "Bravo", "SNK")
	add(3, "Charlie", "CAPCOM")
	add(4, "Delta", "SNK")
	add(5, "Echo", "")
	return l
}

// TestBodyIsFiledByTheStandard: the cards are grouped the way the bar's
// chips are ordered, each group under its own header.
func TestBodyIsFiledByTheStandard(t *testing.T) {
	a := testApp(threeMakers(), "arcade", "arc")
	if got := len(a.sections); got != 3 {
		t.Fatalf("sections = %d, want 3", got)
	}
	// CAPCOM 2, SNK 2, then the one nobody signed: ties fall to the name.
	if a.sections[0].name != "CAPCOM" || a.sections[0].count != 2 {
		t.Errorf("first section = %+v, want CAPCOM × 2", a.sections[0])
	}
	if a.sections[len(a.sections)-1].name != "未标注厂商" {
		t.Errorf("last section = %q, want 未标注厂商", a.sections[len(a.sections)-1].name)
	}
	// The cards follow the sections: shown's first two are CAPCOM's.
	if a.shown[0].Publisher != "CAPCOM" || a.shown[1].Publisher != "CAPCOM" {
		t.Errorf("shown starts %q %q, want CAPCOM first", a.shown[0].Publisher, a.shown[1].Publisher)
	}
	// And there is a header line before each section's cards.
	a.layout(1500)
	headers := 0
	for _, ln := range a.lines {
		if ln.head >= 0 {
			headers++
		}
	}
	if headers != 3 {
		t.Errorf("%d header lines, want one per section (3)", headers)
	}
}

// TestBodyStartsUnderTheBar: the first line sits below the pills but the
// scroll is clipped at the window's top, so a scrolled card is never cut in a
// band — it runs up behind them.
func TestBodyStartsUnderTheBar(t *testing.T) {
	a := testApp(threeMakers(), "arcade", "arc")
	a.layout(1500)
	if len(a.lines) == 0 {
		t.Fatal("no lines laid out")
	}
	if a.lines[0].top != topGap {
		t.Errorf("first line at %v, want %v", a.lines[0].top, topGap)
	}
	if a.contentH <= topGap {
		t.Errorf("content height %v, want more than the top gap", a.contentH)
	}
	if a.cols < 3 || a.cols > 8 {
		t.Errorf("cols = %d", a.cols)
	}
}

// TestBodyRenderedAtRest: on screen, the first card is below the bar and the
// section's name is drawn above it.
func TestBodyRenderedAtRest(t *testing.T) {
	a := testApp(fakeLibrary(), "arcade", "arc")
	a.sections = []section{{name: "CAPCOM", from: 0, count: len(a.shown)}}
	a.stale = true
	tt := ui.NewTester(a.view, 1500, 940)
	tt.Frame()

	if !tt.HasText("CAPCOM") {
		t.Error("the section's header is not drawn")
	}
	top := float32(1e6)
	for _, info := range tt.AccessAll() {
		if info.Bounds.H > 100 && info.Bounds.W < 260 && info.Bounds.Y < top {
			top = info.Bounds.Y
		}
	}
	if top < 60 {
		t.Errorf("a card starts at %v, under the bar", top)
	}
}
