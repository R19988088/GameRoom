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

// TestGroupIsFiledByNames: inside a group the cards read as a list, by their
// names, with 街霸2 ahead of 街霸10.
func TestGroupIsFiledByNames(t *testing.T) {
	l := threeMakers()
	l.Games = []library.Game{
		{ID: 1, PlatformKey: "arc", Title: "街霸10", Publisher: "CAPCOM"},
		{ID: 2, PlatformKey: "arc", Title: "街霸2", Publisher: "CAPCOM"},
		{ID: 3, PlatformKey: "arc", Title: "Alpha", Publisher: "CAPCOM"},
	}
	a := testApp(l, "arcade", "arc")
	want := []string{"Alpha", "街霸2", "街霸10"}
	for i, w := range want {
		if a.shown[i].Title != w {
			t.Errorf("shown[%d] = %q, want %q (order: %v)", i, a.shown[i].Title, w, titles(a.shown))
		}
	}
}

func titles(gs []library.Game) []string {
	out := make([]string, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.Title)
	}
	return out
}

// TestCardMenuOffersTheFields: the right-click menu lists the hardware, the
// maker, the genre, the core and the name — and only the name is fixed.
func TestCardMenuOffersTheFields(t *testing.T) {
	l := threeMakers()
	l.Games[0].Genre, l.Games[0].Board = "Fighter", "snk/neogeo.cpp"
	a := testApp(l, "arcade", "arc")
	fs := a.fields(a.shown[0])
	var keys []string
	for _, f := range fs {
		keys = append(keys, f.key)
	}
	if len(fs) != 5 || keys[0] != "machine" || keys[4] != "title" {
		t.Fatalf("fields = %v", keys)
	}
	if fs[0].label != "基板" || fs[0].value != "MVS" {
		t.Errorf("hardware row = %+v, want 基板 MVS", fs[0])
	}
	if fs[1].value != "CAPCOM" {
		t.Errorf("maker row = %+v", fs[1])
	}
	if !fs[1].editable {
		t.Error("the 厂商 of an arcade game must be correctable")
	}
	// 游戏名能改，但它没有可选的值列表：改名是自己打进去的。
	if !fs[4].editable || !fs[4].typeIn {
		t.Errorf("游戏名 row = %+v, want editable and typed in", fs[4])
	}
	// Console: 机种 is the platform, and the maker is the machine's maker.
	a = testApp(fakeLibrary(), "console", "fc")
	fs = a.fields(a.shown[0])
	if fs[0].label != "机种" || fs[0].value != "FC" {
		t.Errorf("console hardware row = %+v, want 机种 FC", fs[0])
	}
	if fs[1].editable {
		t.Error("the maker of a console is the machine's, not a column")
	}
}

// TestAssignWritesTheField: a correction lands in the library, in the body, and
// in the file the next start reads.
func TestAssignWritesTheField(t *testing.T) {
	a := testApp(threeMakers(), "arcade", "arc")
	id := a.shown[0].ID
	a.menuGame = id
	a.assign("publisher", "TAITO")
	a.runQueue() // 改元数据是排到下一帧做的，这里把它跑掉
	g := a.lib.Game(id)
	if g == nil || g.Publisher != "TAITO" {
		t.Fatalf("game %d = %+v after the correction", id, g)
	}
	if a.status == "" || a.status == "就绪" {
		t.Error("nothing was said about the correction")
	}
}
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
