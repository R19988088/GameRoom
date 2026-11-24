package app

import (
	"strconv"
	"strings"
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// manyMakers is a shelf of sixteen arcade games, two under each of eight
// publishers, half of them CPS2 and half MVS so the 其他 page has two boards to
// label.
func manyMakers() *library.Library {
	l := &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
	}
	ids := 0
	for i, maker := range []string{"CAPCOM", "SNK", "TAITO", "SEGA", "KONAMI", "NMK", "BANPRESTO", "JALECO"} {
		for k := 0; k < 2; k++ {
			ids++
			board := "capcom/cps2.cpp"
			if i%2 == 1 {
				board = "snk/neogeo.cpp"
			}
			l.Games = append(l.Games, library.Game{
				ID: ids, PlatformKey: "arc", Title: maker + " Game",
				RomPath: "arc/g" + maker + ".zip", Publisher: maker, Board: board,
			})
		}
	}
	return l
}

// TestBarKeepsFiveAndFoldsTheRestInOther: only five values fit on the bar, and
// 其他 counts the ones it did not list.
func TestBarKeepsFiveAndFoldsTheRestInOther(t *testing.T) {
	a := testApp(manyMakers(), "arcade", "arc")
	if len(a.chips) != 8 {
		t.Fatalf("chips = %d, want 8", len(a.chips))
	}
	if got := len(a.topChips()); got != 5 {
		t.Errorf("bar lists %d chips, want 5", got)
	}
	if a.otherTotal != 6 {
		t.Errorf("其他 holds %d games, want the three makers left out (6)", a.otherTotal)
	}
}

// TestOtherIsFiledByHardware: 其他's body labels the machine or the board, not
// the category the bar left out, and holds exactly the games the five chips did
// not.
func TestOtherIsFiledByHardware(t *testing.T) {
	a := testApp(manyMakers(), "arcade", "arc")
	a.chooseSpecial(specialOther)
	if len(a.shown) != a.otherTotal {
		t.Fatalf("其他 shows %d games, want %d", len(a.shown), a.otherTotal)
	}
	for _, g := range a.shown {
		if a.topNames()[g.Publisher] {
			t.Errorf("%q is on a bar chip, so 其他 must not hold it", g.Title)
		}
	}
	// One header per board, each of them the board, never a publisher.
	if len(a.sections) != 2 {
		t.Fatalf("sections = %+v, want two boards", a.sections)
	}
	for _, s := range a.sections {
		if s.name != "CPS2" && s.name != "MVS" {
			t.Errorf("header = %q, want a board", s.name)
		}
	}
}

// barTexts is what the bar draws: the text of the top row, by name, with the
// leftmost x it was drawn at. The body's headers start at topGap (74), so
// anything above 60 is a chip.
func barTexts(tt *ui.Tester) map[string]float32 {
	out := map[string]float32{}
	for _, info := range tt.AccessAll() {
		if info.Name == "" || info.Bounds.H == 0 || info.Bounds.Y >= 60 {
			continue
		}
		// A pill's name joins its label and its count ("CAPCOM 2"); the chip is
		// the label.
		name := info.Name
		if i := strings.LastIndex(name, " "); i > 0 {
			if _, err := strconv.Atoi(name[i+1:]); err == nil {
				name = name[:i]
			}
		}
		if x, ok := out[name]; !ok || info.Bounds.X < x {
			out[name] = info.Bounds.X
		}
	}
	return out
}

// TestBarDrawsFiveChipsAndOther: on screen the bar holds 全部, the five busiest
// values and 其他 — nothing beyond the five, and 其他 last.
func TestBarDrawsFiveChipsAndOther(t *testing.T) {
	a := testApp(manyMakers(), "arcade", "arc")
	tt := ui.NewTester(a.view, 1500, 940)
	tt.Frame()
	bar := barTexts(tt)

	var listed []string
	for _, c := range a.topChips() {
		listed = append(listed, c.Name)
		if _, ok := bar[c.Name]; !ok {
			t.Errorf("%s has a chip but is not drawn on the bar (%v)", c.Name, bar)
		}
	}
	// The ones the bar left out are nowhere above the body.
	for _, c := range a.chips[5:] {
		if _, ok := bar[c.Name]; ok {
			t.Errorf("%s is drawn on the bar, but the bar has room for five (%v)", c.Name, bar)
		}
	}
	if x, ok := bar["其他"]; !ok {
		t.Errorf("其他 is not drawn (%v)", bar)
	} else if x < bar[listed[len(listed)-1]] {
		t.Errorf("其他 sits left of the fifth chip (%v)", bar)
	}
}

// TestFavoriteComesFirstOnTheBar: once a card is starred, 收藏 has a chip left
// of 全部 and of the five.
func TestFavoriteComesFirstOnTheBar(t *testing.T) {
	a := testApp(manyMakers(), "arcade", "arc")
	a.menuGame = a.shown[0].ID
	a.toggleFavorite(a.shown[0].ID)
	a.runQueue()
	a.stale = true

	tt := ui.NewTester(a.view, 1500, 940)
	tt.Frame()
	bar := barTexts(tt)
	xFav, ok := bar["收藏"]
	if !ok {
		t.Fatalf("收藏 has a chip but is not drawn (%v)", bar)
	}
	for _, name := range []string{"全部", a.topChips()[0].Name} {
		if xAll, ok := bar[name]; ok && xAll < xFav {
			t.Errorf("%s sits left of 收藏 (%v)", name, bar)
		}
	}
}

// TestOtherPageListsNoSingleCategory: on 其他 the bar keeps only 全部 and 其他,
// and the headers below name the boards instead.
func TestOtherPageDrawsNoCategory(t *testing.T) {
	a := testApp(manyMakers(), "arcade", "arc")
	a.chooseSpecial(specialOther)
	tt := ui.NewTester(a.view, 1500, 940)
	tt.Frame()
	bar := barTexts(tt)
	for _, c := range a.topChips() {
		if _, ok := bar[c.Name]; ok {
			t.Errorf("%s is still on the bar on 其他 (%v)", c.Name, bar)
		}
	}
	if _, ok := bar["其他"]; !ok {
		t.Errorf("其他 is not on the bar on its own page (%v)", bar)
	}
	if !tt.HasText("CPS2") || !tt.HasText("MVS") {
		t.Errorf("the 其他 page does not label its boards")
	}
}

// TestConsoleBarHasNoAllUntilAPage: a shelf whose chips are its machines needs
// no 全部 at rest — the chosen machine is the shelf — and gets one back only on
// 收藏 or 其他, where a second click on the lit chip also returns.
func TestConsoleBarHasNoAllUntilAPage(t *testing.T) {
	a := newTestApp(manyPlatforms(), "fc")
	a.stale = true
	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	bar := barTexts(tt)
	if _, ok := bar["全部"]; ok {
		t.Errorf("全部 is on the bar of a machine shelf at rest (%v)", bar)
	}
	if _, ok := bar["3DO"]; ok {
		t.Errorf("3DO is drawn, but the sixth machine is under 其他 (%v)", bar)
	}
	if _, ok := bar["其他"]; !ok {
		t.Errorf("其他 is not drawn (%v)", bar)
	}

	a.chooseSpecial(specialOther)
	a.stale = true
	tt = ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	if _, ok := barTexts(tt)["全部"]; !ok {
		t.Error("其他 has no 全部 to go back to")
	}

	a.chooseSpecial(specialOther)
	if a.special != "" {
		t.Errorf("the second click left %q showing, want the shelf", a.special)
	}
	if len(a.shown) != 40 {
		t.Errorf("back on the shelf it shows %d games, want the 40 of fc", len(a.shown))
	}
}

// TestFavoriteRowStarsTheCard: the menu's 收藏 button writes the star, counts it
// on the bar, and makes 收藏 a page of its own.
func TestFavoriteRowStarsTheCard(t *testing.T) {

	a := testApp(manyMakers(), "arcade", "arc")
	g := a.shown[0]
	a.menuGame = g.ID
	a.toggleFavorite(g.ID)
	a.runQueue() // 收藏也是排到下一帧写的
	if got := a.lib.Game(g.ID); got == nil || !got.Favorite {
		t.Fatalf("game %d = %+v after the star", g.ID, got)
	}
	if a.favTotal != 1 {
		t.Errorf("收藏 counts %d, want 1", a.favTotal)
	}

	a.chooseSpecial(specialFav)
	if len(a.shown) != 1 || a.shown[0].ID != g.ID {
		t.Fatalf("收藏 shows %v, want just %q", titles(a.shown), g.Title)
	}

	// Taking the star away from 收藏's own page removes the card there.
	a.menuGame = g.ID
	a.toggleFavorite(g.ID)
	a.runQueue()
	if a.favTotal != 0 || len(a.shown) != 0 {
		t.Errorf("收藏 counts %d and shows %d after the star came off, want 0 0",
			a.favTotal, len(a.shown))
	}
}
