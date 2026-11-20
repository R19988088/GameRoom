package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
)

// TestPadLayoutOfMachines: a game's pad is the board it runs on, or the machine
// its folder holds — SEGA keeps several machines in one folder.
func TestPadLayoutOfMachines(t *testing.T) {
	a := &App{}
	cases := []struct {
		game library.Game
		want string
	}{
		{library.Game{PlatformKey: "arc", Board: "capcom/cps2.cpp"}, "cps2"},
		{library.Game{PlatformKey: "arc", Board: "capcom/cps3.cpp"}, "cps3"},
		{library.Game{PlatformKey: "arc", Board: "capcom/cps1.cpp"}, "cps1"},
		{library.Game{PlatformKey: "arc", Board: "snk/neogeo.cpp"}, "mvs"},
		{library.Game{PlatformKey: "arc", Board: "sega/stv.cpp"}, "st-v"},
		{library.Game{PlatformKey: "arc", Board: "capcom/naomigd.cpp"}, "naomi-gd"},
		{library.Game{PlatformKey: "arc", Board: "unknown.cpp"}, "arcade"},
		{library.Game{PlatformKey: "fc"}, "fc"},
		{library.Game{PlatformKey: "sfc"}, "sfc"},
		{library.Game{PlatformKey: "ps"}, "ps"},
		{library.Game{PlatformKey: "3DO"}, "3do"},
		{library.Game{PlatformKey: "SEGA", RomPath: "SEGA/MD/foo.zip"}, "md"},
		{library.Game{PlatformKey: "SEGA", RomPath: "SEGA/SS/Shinobi X.cue"}, "ss"},
		{library.Game{PlatformKey: "SEGA", RomPath: "SEGA/DC/foo.gdi"}, "arcade"},
		{library.Game{PlatformKey: "SEGA", RomPath: "SEGA/Model 2/foo.zip"}, "arcade"},
		{library.Game{PlatformKey: "dos", RomPath: "dos/foo.zip"}, ""},
		{library.Game{PlatformKey: "pc98", RomPath: "pc98/foo/x.hdi"}, ""},
	}
	for _, c := range cases {
		if got := a.padLayoutOf(c.game); got != c.want {
			t.Errorf("padLayoutOf(%s %s) = %q, want %q", c.game.PlatformKey, c.game.Board, got, c.want)
		}
	}
}

// TestMaskOfReadsBothSources: a button is held when its key is down or its
// gamepad button is, and it reaches the core as the RetroPad id the machine's
// button stands for.
func TestMaskOfReadsBothSources(t *testing.T) {
	l := padLayoutFor("fc")
	prof := defaultPadBinds()

	keys := map[int]bool{prof["a"].VK: true}
	if held := maskOf(l, prof, keys, nil); !held[padA] || held[padB] {
		t.Fatalf("key A: held = %v, want only pad A", held)
	}
	pads := map[int]bool{prof["b"].Pad - 1: true}
	if held := maskOf(l, prof, nil, pads); !held[padB] || held[padA] {
		t.Fatalf("pad B: held = %v, want only pad B", held)
	}
	if held := maskOf(nil, prof, keys, pads); held != nil {
		t.Fatalf("no layout: held = %v, want nil", held)
	}
}

// TestBindPadTakesTheSourceFromTheOtherButton: one key drives one button, so
// binding a button to a key another button had clears that one and keeps the
// rest of its sources.
func TestBindPadTakesTheSourceFromTheOtherButton(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}}
	bKey := defaultPadBinds()["b"].VK
	a.bindPad("fc", "a", "key", bKey, 0)

	prof := a.padProfile("fc")
	if prof["a"].VK != bKey {
		t.Fatalf("A reads %d, want %d", prof["a"].VK, bKey)
	}
	if prof["b"].VK != 0 {
		t.Fatalf("B still reads key %d", prof["b"].VK)
	}
	if prof["b"].Pad == 0 {
		t.Fatalf("B lost its gamepad button too")
	}
	if _, err := os.Stat(a.controlsPath()); err != nil {
		t.Fatalf("bindings not saved: %v", err)
	}
}

// TestPadBindingsLoadAgain: a machine's profile is what was saved, and a
// machine nobody tuned runs with the defaults.
func TestPadBindingsLoadAgain(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "数据"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &App{root: root, pads: map[string]map[string]padBind{}}
	a.bindPad("cps2", "a", "pad", 0, 15) // LT

	b := &App{root: root}
	b.loadPads()
	if got := b.padProfile("cps2")["a"].Pad; got != 15 {
		t.Fatalf("A reads pad %d, want 15", got)
	}
	if got := b.padProfile("cps2")["b"].Pad; got != defaultPads["b"] {
		t.Fatalf("B reads pad %d, want the default %d", got, defaultPads["b"])
	}

	b.resetPads("cps2")
	c := &App{root: root}
	c.loadPads()
	if got := c.padProfile("cps2")["a"].Pad; got != defaultPads["a"] {
		t.Fatalf("after reset A reads pad %d, want the default %d", got, defaultPads["a"])
	}
}

// TestPanelsTakeOneSlot: opening the pad panel closes the filter's, and the
// other way round, so the two models never sit on top of each other.
func TestPanelsTakeOneSlot(t *testing.T) {
	a := &App{pads: map[string]map[string]padBind{}, padsOpen: false, filtersOpen: true}
	a.togglePads()
	if !a.padsOpen || a.filtersOpen {
		t.Fatalf("pads open = %v, filters open = %v, want the pad panel only", a.padsOpen, a.filtersOpen)
	}
	a.toggleFilters()
	if !a.filtersOpen || a.padsOpen {
		t.Fatalf("filters open = %v, pads open = %v, want the filter panel only", a.filtersOpen, a.padsOpen)
	}
}

// waitFor starts a capture with nothing held, the state the panel begins in.
func waitFor(a *App, id string) {
	a.cap = id
	a.capVK = map[int]bool{}
	a.capPad = map[int]bool{}
}

// TestCaptureTakesTheFirstNewKey: a button waiting for a source takes the first
// key pressed after the wait, and the binding lands in that machine's profile.
func TestCaptureTakesTheFirstNewKey(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "fc"}
	waitFor(a, "a")
	a.captureTick(map[int]bool{0x50: true}, nil) // P

	if a.cap != "" {
		t.Fatalf("still waiting after a key was pressed")
	}
	b := a.padProfile("fc")["a"]
	if b.VK != 0x50 || b.Key != "P" || b.Pad != 0 {
		t.Fatalf("A reads %+v, want P only", b)
	}
}

// TestCaptureTakesAButton: a gamepad button pressed during the wait is the
// source instead, filed as XInput's index + 1.
func TestCaptureTakesAButton(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "cps2"}
	waitFor(a, "r")
	a.captureTick(nil, map[int]bool{9: true}) // RB

	if b := a.padProfile("cps2")["r"]; b.Pad != 10 || b.VK != 0 {
		t.Fatalf("6 reads %+v, want the pad's RB only", b)
	}
}

// TestCaptureIgnoresHeldKeysAndCancel: a key held when the wait began is not a
// press, and Esc gives up without binding anything.
func TestCaptureIgnoresHeldKeysAndCancel(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "cps2"}
	a.cap = "b"
	a.capVK = map[int]bool{0x50: true}
	a.capPad = map[int]bool{}
	a.captureTick(map[int]bool{0x50: true}, nil)
	if a.cap != "b" {
		t.Fatalf("a key already held was taken")
	}
	a.captureTick(map[int]bool{0x1B: true}, nil)
	if a.cap != "" {
		t.Fatalf("Esc did not give up the wait")
	}
	if _, ok := a.pads["cps2"]["b"]; ok {
		t.Fatalf("Esc bound a key anyway")
	}
}

// spotByLabel finds the button a machine calls by one of its own names.
func spotByLabel(l *padLayout, label string) (padSpot, bool) {
	for _, s := range l.Spots {
		if s.Label == label {
			return s, true
		}
	}
	return padSpot{}, false
}

// TestSixButtonModelsReadTheCoreLayout: a SEGA six-button pad is not laid over
// the RetroPad the obvious way — the Mega Drive reads A from Y and C from A,
// the Saturn reads A from B and Z from L — so each model has to say which id
// its own buttons stand for.
func TestSixButtonModelsReadTheCoreLayout(t *testing.T) {
	cases := []struct{ layout, label, want string }{
		{"md", "A", "y"}, {"md", "B", "b"}, {"md", "C", "a"},
		{"md", "X", "l"}, {"md", "Y", "x"}, {"md", "Z", "r"},
		{"ss", "A", "b"}, {"ss", "B", "a"}, {"ss", "C", "r"},
		{"ss", "X", "y"}, {"ss", "Y", "x"}, {"ss", "Z", "l"},
	}
	for _, c := range cases {
		s, ok := spotByLabel(padLayoutFor(c.layout), c.label)
		if !ok {
			t.Fatalf("%s has no %s button", c.layout, c.label)
		}
		if s.ID != c.want {
			t.Errorf("%s %s reads %q, want %q", c.layout, c.label, s.ID, c.want)
		}
	}
}

// TestMacroPressesEveryButton: a combination button is not an extra button of
// the machine but several of them at once, in the order the core knows them.
func TestMacroPressesEveryButton(t *testing.T) {
	cases := []struct {
		layout, macro string
		want          []int
	}{
		{"cps2", "abc", []int{padB, padY, padA}},
		{"cps2", "def", []int{padX, padL, padR}},
		{"mvs", "ab", []int{padB, padY}},
		{"mvs", "cd", []int{padA, padX}},
	}
	for _, c := range cases {
		a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}}
		a.bindPad(c.layout, c.macro, "key", 0x50, 0) // P
		held := maskOf(padLayoutFor(c.layout), a.padProfile(c.layout), map[int]bool{0x50: true}, nil)
		if len(held) != len(c.want) {
			t.Errorf("%s %s presses %v, want %v", c.layout, c.macro, held, c.want)
			continue
		}
		for _, id := range c.want {
			if !held[id] {
				t.Errorf("%s %s does not press id %d", c.layout, c.macro, id)
			}
		}
	}
}

// TestBoardNamesAreNormalised: a board is called and filed by its own name, so
// CPS3 is the CP System III everywhere — the title of the panel, the pill, and
// the entry in controls.json.
func TestBoardNamesAreNormalised(t *testing.T) {
	cases := []struct {
		board, id, name string
		faces           int
	}{
		{"CPS3", "cps3", "CPSIII", 6},
		{"CPS2", "cps2", "CPS2", 6},
		{"CPS1", "cps1", "CPS1", 3},
		{"ST-V", "st-v", "ST-V", 6},
		{"NAOMI GD", "naomi-gd", "NAOMI GD", 6},
		{"TAITO F2", "taito-f2", "TAITO F2", 6},
	}
	for _, c := range cases {
		if got := layoutID(c.board); got != c.id {
			t.Errorf("layoutID(%q) = %q, want %q", c.board, got, c.id)
		}
		l := padLayoutFor(c.id)
		if l == nil {
			t.Fatalf("%s has no panel", c.id)
		}
		if l.Name != c.name {
			t.Errorf("%s is called %q, want %q", c.id, l.Name, c.name)
		}
		if got := padShort(c.id); got != c.name {
			t.Errorf("the pill calls %s %q, want %q", c.id, got, c.name)
		}
		count := 0
		for _, s := range l.Spots {
			if s.Kind == "face" {
				count++
			}
		}
		if count != c.faces {
			t.Errorf("%s draws %d buttons, want %d", c.id, count, c.faces)
		}
	}
	if padLayoutFor("arcade").Name != "街机" {
		t.Fatalf("the fallback panel is called %q", padLayoutFor("arcade").Name)
	}
	if padLayoutFor("") != nil {
		t.Fatalf("a machine with no panel was given one")
	}
}

// TestModelsFitTheCanvasAndLineUp: every machine is drawn on the same canvas,
// so every button has to be inside it, no button twice, the four arms of the
// d-pad always there, and the face buttons sitting on shared columns instead of
// wherever they happen to land.
func TestModelsFitTheCanvasAndLineUp(t *testing.T) {
	if len(padLayouts) < 11 {
		t.Fatalf("only %d models", len(padLayouts))
	}
	for _, l := range padLayouts {
		seen := map[string]bool{}
		arms := 0
		for _, s := range l.Spots {
			if seen[s.ID] {
				t.Errorf("%s has two %q buttons", l.ID, s.ID)
			}
			seen[s.ID] = true
			if s.X < 0 || s.Y < 0 || s.X+s.W > padModelW || s.Y+s.H > padModelH {
				t.Errorf("%s %s is off the canvas: %.0f,%.0f %.0fx%.0f", l.ID, s.Label, s.X, s.Y, s.W, s.H)
			}
			if s.Kind == "dir" {
				arms++
			}
			for _, t2 := range targetsOf(s) {
				if _, ok := padRetroIds[t2]; !ok {
					t.Errorf("%s %s presses unknown id %q", l.ID, s.Label, t2)
				}
			}
		}
		if arms != 4 {
			t.Errorf("%s has %d d-pad arms, want 4", l.ID, arms)
		}
	}
}
