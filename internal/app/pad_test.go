package app

import (
	"os"
	"path/filepath"
	"strings"
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
		{library.Game{PlatformKey: "SEGA", RomPath: "SEGA/DC/foo.gdi"}, "dc"},
		{library.Game{PlatformKey: "SEGA", RomPath: "SEGA/Model 2/foo.zip"}, "arcade"},
		{library.Game{PlatformKey: "任天堂", RomPath: "任天堂/3DS/foo.cia"}, "3ds"},
		{library.Game{PlatformKey: "微软", RomPath: "微软/XBOX/foo.iso"}, "xbox"},
		{library.Game{PlatformKey: "微软", RomPath: "微软/360/foo.iso"}, "xbox360"},
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
	pads := map[int]map[int]bool{0: {prof["b"].Pad - 1: true}}
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
	a.bindPad("fc", 0, "a", "key", bKey, 0, 0)

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
	a.bindPad("cps2", 0, "a", "pad", 0, 15, 0) // LT

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

// TestSecondPlayerKeepsItsOwnButtons: the second player's defaults are the
// same buttons on controller two, not a different button of controller one. A
// pad button is an index into one controller's buttons; shifting it by four
// would land on Start/L3 instead.
func TestSecondPlayerKeepsItsOwnButtons(t *testing.T) {
	p2 := defaultPadBinds2()
	for id := range padRetroIds {
		b := p2[id]
		if b.Key != "" || b.VK != 0 {
			t.Errorf("%s has a keyboard of its own: %+v", id, b)
		}
		want := defaultPads[id]
		if want == 0 {
			continue
		}
		if b.Pad != want || b.Ctrl != 1 {
			t.Errorf("%s reads controller %d button %d, want controller 1 button %d",
				id, b.Ctrl, b.Pad, want)
		}
	}
}

// TestSecondPlayerKeyboardReachesItsPort: player two can play on the keyboard
// alone; the key lands in player one's mask only if player one bound it too.
func TestSecondPlayerKeyboardReachesItsPort(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "fc"}
	a.bindPad("fc", 1, "a", "key", 0x50, 0, 0) // P
	keys := map[int]bool{0x50: true}
	if held := a.padMask(1, keys, nil); !held[padA] {
		t.Errorf("player two did not read its key: %v", held)
	}
	if held := a.padMask(0, keys, nil); held[padA] {
		t.Errorf("player one read player two's key: %v", held)
	}
}

// TestPlayersReadTheirOwnPads: two pads press the same button numbers at the
// same time; each player's mask reads only the controller its binding names.
func TestPlayersReadTheirOwnPads(t *testing.T) {
	a := &App{pads: map[string]map[string]padBind{}, padLive: "fc"}
	l := padLayoutFor("fc")
	p1, p2 := a.padProfile("fc"), a.padProfile2("fc")
	pads := map[int]map[int]bool{
		0: {p1["a"].Pad - 1: true},
		1: {p2["b"].Pad - 1: true},
	}
	h1 := maskOf(l, p1, nil, pads)
	h2 := maskOf(l, p2, nil, pads)
	if !h1[padA] || h1[padB] {
		t.Errorf("player one read %v, want only A", h1)
	}
	if !h2[padB] || h2[padA] {
		t.Errorf("player two read %v, want only B", h2)
	}
}

// TestSameKeyCanDriveBothPlayers: a key both players bind is not a conflict:
// each player's mask reads it, so both ports see their own button.
func TestSameKeyCanDriveBothPlayers(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "fc"}
	a.bindPad("fc", 0, "a", "key", 0x50, 0, 0) // P
	a.bindPad("fc", 1, "a", "key", 0x50, 0, 0) // P again
	l := padLayoutFor("fc")
	keys := map[int]bool{0x50: true}
	if h := maskOf(l, a.padProfile("fc"), keys, nil); !h[padA] {
		t.Errorf("player one did not read the shared key: %v", h)
	}
	if h := maskOf(l, a.padProfile2("fc"), keys, nil); !h[padA] {
		t.Errorf("player two did not read the shared key: %v", h)
	}
}

// TestSecondPlayerBindingsAreFiledApart: player two's source lives under the
// 2p: prefix, so writing it leaves player one's bare entry alone, and resetting
// player two leaves player one alone.
func TestSecondPlayerBindingsAreFiledApart(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "fc"}
	a.bindPad("fc", 0, "a", "key", 0x50, 0, 0)
	a.bindPad("fc", 1, "a", "pad", 0, 11, 1) // A on controller two

	if _, ok := a.pads["fc"]["a"]; !ok {
		t.Fatalf("player one's entry disappeared")
	}
	if _, ok := a.padProfile("fc")["2p:a"]; ok {
		t.Fatalf("player two's entry leaked into player one's profile")
	}
	if b, ok := a.pads["fc"]["2p:a"]; !ok || b.Pad != 11 || b.Ctrl != 1 {
		t.Fatalf("player two's entry is %+v, want controller 1 button 11", b)
	}

	a.resetPadPlayer("fc", 1)
	if _, ok := a.pads["fc"]["2p:a"]; ok {
		t.Fatalf("player two's entry survived its reset")
	}
	if _, ok := a.pads["fc"]["a"]; !ok {
		t.Fatalf("player one's entry was cleared by player two's reset")
	}
}

// TestSecondPlayerCaptureWritesItsOwnPrefix: a button clicked on the right half
// waits for a source and files it under 2p:, carrying the controller it was
// pressed on.
func TestSecondPlayerCaptureWritesItsOwnPrefix(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "fc"}
	a.cap, a.capP = "a", 1
	a.capVK, a.capPad = map[int]bool{}, map[int]map[int]bool{}
	a.captureTick(nil, map[int]map[int]bool{1: {10: true}}) // A on controller two

	b, ok := a.pads["fc"]["2p:a"]
	if !ok || b.Pad != 11 || b.Ctrl != 1 {
		t.Fatalf("the capture filed %+v, want controller 1 button 11", b)
	}
	if _, ok := a.pads["fc"]["a"]; ok {
		t.Fatalf("player two's capture wrote player one's entry")
	}
}

// TestPadPanelLeavesRoomForBothColumns: the panel is exactly two models plus
// the gap between them and the padding around them, so neither half is clipped.
func TestPadPanelLeavesRoomForBothColumns(t *testing.T) {
	const padding, gap = 12, 8
	if padModelW*2+gap+padding*2 != panelW {
		t.Fatalf("panel %d, want two %d models, a %d gap and %d padding",
			panelW, padModelW, gap, padding)
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
	a.capP = 0
	a.capVK = map[int]bool{}
	a.capPad = map[int]map[int]bool{}
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
	a.captureTick(nil, map[int]map[int]bool{0: {9: true}}) // RB

	if b := a.padProfile("cps2")["r"]; b.Pad != 10 || b.VK != 0 {
		t.Fatalf("6 reads %+v, want the pad's RB only", b)
	}
}

// TestCaptureIgnoresHeldKeysAndCancel: a key held when the wait began is not a
// press, and Esc gives up without binding anything.
func TestCaptureIgnoresHeldKeysAndCancel(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "cps2"}
	a.cap = "b"
	a.capP = 0
	a.capVK = map[int]bool{0x50: true}
	a.capPad = map[int]map[int]bool{}
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
		a.bindPad(c.layout, 0, c.macro, "key", 0x50, 0, 0) // P
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

// TestPlayStationButtonsFollowTheCore: the core reads the Sony diamond by its
// position — X up (△), Y left (□), A right (○), B down (×) — so the model has to
// press the same ids, or the shapes would lie about which button is which.
func TestPlayStationButtonsFollowTheCore(t *testing.T) {
	cases := []struct {
		layout, label, want string
		id                  int
	}{
		{"ps", "△", "x", padX}, {"ps", "□", "y", padY},
		{"ps", "○", "a", padA}, {"ps", "×", "b", padB},
		{"ps2", "×", "b", padB}, {"ps3", "○", "a", padA},
		{"psp", "□", "y", padY}, {"psv", "△", "x", padX},
	}
	for _, c := range cases {
		l := padLayoutFor(c.layout)
		if l == nil {
			t.Fatalf("%s has no panel", c.layout)
		}
		for _, s := range l.Spots {
			if s.Label != c.label {
				continue
			}
			if s.ID != c.want {
				t.Errorf("%s %s reads %q, want %q", c.layout, c.label, s.ID, c.want)
			}
			a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}}
			a.bindPad(c.layout, 0, s.ID, "key", 0x50, 0, 0)
			held := maskOf(l, a.padProfile(c.layout), map[int]bool{0x50: true}, nil)
			if !held[c.id] {
				t.Errorf("%s %s does not press the core's id %d", c.layout, c.label, c.id)
			}
			goto next
		}
		t.Errorf("%s has no %s button", c.layout, c.label)
	next:
	}
}

// TestStickHasItsOwnDirections: a machine with a small stick draws it beside the
// cross, files its four directions under ids of their own, and reads them from
// the gamepad's stick rather than from the cross — two sets of bindings, so the
// player can bind them apart.
func TestStickHasItsOwnDirections(t *testing.T) {
	for _, id := range []string{"ps2", "ps3", "psp", "psv", "dc", "xbox", "xbox360", "3ds"} {
		l := padLayoutFor(id)
		if l == nil {
			t.Fatalf("%s has no panel", id)
		}
		sticks := 0
		for _, s := range l.Spots {
			if s.Kind == "stick" {
				sticks++
				switch s.ID {
				case "lsup", "lsdown", "lsleft", "lsright":
				default:
					t.Errorf("%s stick %s is filed under %q", id, s.Label, s.ID)
				}
			}
			if strings.Contains(strings.ToLower(s.ID), "lshold") && s.Kind != "hold" {
				t.Errorf("%s draws %s as %q, want the hold key", id, s.Label, s.Kind)
			}
		}
		if sticks != 4 {
			t.Errorf("%s draws %d stick directions, want 4", id, sticks)
		}
		hasHold := false
		for _, s := range l.Spots {
			if s.Kind == "hold" {
				hasHold = true
			}
		}
		if !hasHold {
			t.Errorf("%s has no stick hold key", id)
		}
		for _, want := range []string{"lsup", "lsdown", "lsleft", "lsright", "lshold"} {
			if _, ok := padRetroIds[want]; !ok {
				t.Errorf("%s is not an id a binding can be filed under", want)
			}
		}
	}
	// The stick's own defaults sit on XInput's indices 16..19, apart from the
	// cross's 0..3, and take no keys: WASD would only be a second set of arrows.
	if defaultPads["lsup"] != 17 || defaultPads["lsright"] != 20 {
		t.Errorf("the stick's defaults are %v", defaultPads)
	}
	if _, ok := defaultKeys["lsup"]; ok {
		t.Errorf("the stick's directions have keys of their own: %v", defaultKeys)
	}
	if len(padNames) != 20 {
		t.Errorf("%d gamepad names, want 20", len(padNames))
	}
}

// TestStickHoldKeyKeepsADirection: the hold is a key of its own — pressed while
// the stick is tilted it keeps that direction on, pressed with nothing tilted it
// lets it go. It is not a switch on the panel, and it presses nothing itself.
func TestStickHoldKeyKeepsADirection(t *testing.T) {
	a := &App{root: t.TempDir(), pads: map[string]map[string]padBind{}, padLive: "ps2"}
	a.bindPad("ps2", 0, "lshold", "key", 0x54, 0, 0) // T
	a.bindPad("ps2", 0, "lsup", "key", 0x57, 0, 0)   // W
	prof := a.padProfile("ps2")

	tilt := map[int]bool{prof["lsup"].VK: true}
	if held := a.padMask(0, map[int]bool{prof["lshold"].VK: true}, nil); len(held) != 0 {
		t.Fatalf("the hold key alone pressed %v, want nothing", held)
	}

	// Tilt the stick, then press the hold key: up stays on after the stick is let go.
	a.padMask(0, tilt, nil)
	a.padMask(0, map[int]bool{prof["lsup"].VK: true, prof["lshold"].VK: true}, nil)
	if held := a.padMask(0, nil, nil); !held[padUp] {
		t.Fatalf("the held direction was dropped: %v", held)
	}
	// Press the hold key again with nothing tilted: it lets go.
	a.padMask(0, map[int]bool{prof["lshold"].VK: true}, nil)
	if held := a.padMask(0, nil, nil); held[padUp] {
		t.Fatalf("the hold key did not let the direction go")
	}
}

// so every button has to be inside it, no button twice, the four arms of the
// d-pad always there, and the face buttons sitting on shared columns instead of
// wherever they happen to land.
func TestModelsFitTheCanvasAndLineUp(t *testing.T) {
	if len(padLayouts) < 19 {
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
		// Two buttons on the same spot would fight for the click: the cross, the
		// stick's directions and its hold key each have their own patch.
		for i, a1 := range l.Spots {
			for _, b1 := range l.Spots[i+1:] {
				if a1.X < b1.X+b1.W && b1.X < a1.X+a1.W && a1.Y < b1.Y+b1.H && b1.Y < a1.Y+a1.H {
					t.Errorf("%s draws %s over %s", l.ID, a1.Label, b1.Label)
				}
			}
		}
	}
}
