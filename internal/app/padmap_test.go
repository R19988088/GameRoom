package app

import (
	"testing"

	"github.com/R19988088/GameRoom/internal/core"
)

// says builds a core's announced buttons: id then name, as SET_INPUT_DESCRIPTORS
// hands them over (port 0, joypad, index 0).
func says(pairs ...any) []core.InputDesc {
	out := make([]core.InputDesc, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, core.InputDesc{Port: 0, Device: 1, Index: 0, ID: pairs[i].(int), Name: pairs[i+1].(string)})
	}
	return out
}

// idsOf reads one panel's resolved ids back as what the panel's buttons press.
func idsOf(ids map[string][]int, spot string) []int {
	return ids[spot]
}

// TestFBNeoStreetFighterOrder: FBNeo reads a six-panel cabinet — Y/X/L are the
// three punches, B/A/R the three kicks — so the panel's 1…6 have to move onto
// those ids. The player's test of 街霸II is what pinned it down: the launch had
// read FBNeo's table too early (it announces after the first frame, not during
// load), so the panel kept its drawn order and 1 came out as 4 (B is button 4 on
// this panel model). The launch asks again once the core has spoken.
func TestFBNeoStreetFighterOrder(t *testing.T) {
	// What fbneo announces for sf2ce, read out of the core itself.
	descs := says(
		2, "Coin", 3, "Start",
		1, "Weak Punch", 9, "Medium Punch", 10, "Strong Punch",
		0, "Weak Kick", 8, "Medium Kick", 11, "Strong Kick",
	)
	l := padPanelFor(padLayoutFor("cps1"), descs)
	ids := padIDsFor(l, descs)
	want := map[string][]int{
		"b":   {padY},             // 1 轻拳
		"y":   {padX},             // 2 中拳
		"a":   {padL},             // 3 重拳
		"x":   {padB},             // 4 轻脚
		"l":   {padA},             // 5 中脚
		"r":   {padR},             // 6 重脚
		"abc": {padY, padX, padL}, // the three punches still, wherever they moved
		"def": {padB, padA, padR},
	}
	for spot, w := range want {
		got := idsOf(ids, spot)
		if len(got) != len(w) {
			t.Errorf("%s presses %v, want %v", spot, got, w)
			continue
		}
		for i := range w {
			if got[i] != w[i] {
				t.Errorf("%s presses %v, want %v", spot, got, w)
				break
			}
		}
	}
}

// TestFBNeoNeoGeoOrder: a NeoGeo panel names its buttons A to D, and FBNeo reads
// A from B, B from A, C from Y, D from X — the panel's letters are the machine's,
// not the retropad's.
func TestFBNeoNeoGeoOrder(t *testing.T) {
	descs := says(
		2, "Coin", 3, "Start",
		0, "Button A", 8, "Button B", 1, "Button C", 9, "Button D",
	)
	ids := padIDsFor(padLayoutFor("mvs"), descs)
	want := map[string][]int{"b": {padB}, "y": {padA}, "a": {padY}, "x": {padX}}
	for spot, w := range want {
		if got := idsOf(ids, spot); len(got) != 1 || got[0] != w[0] {
			t.Errorf("%s presses %v, want %v", spot, got, w)
		}
	}
}

// TestFBNeoUnnumberedMachine: a machine that names no number at all — dino's
// "Attack" and "Jump" — is read in the order the core announced.
func TestFBNeoUnnumberedMachine(t *testing.T) {
	descs := says(2, "Coin", 3, "Start", 0, "Attack", 8, "Jump")
	ids := padIDsFor(padPanelFor(padLayoutFor("cps1"), descs), descs)
	if got := idsOf(ids, "b"); len(got) != 1 || got[0] != padB {
		t.Errorf("button 1 presses %v, want B (Attack)", got)
	}
	if got := idsOf(ids, "y"); len(got) != 1 || got[0] != padA {
		t.Errorf("button 2 presses %v, want A (Jump)", got)
	}
}

// TestNeoGeoGameWearingSixKeys: the case the player hit on mslug2. FBNeo draws
// the panel of the board the shelf filed the romset under, and a NeoGeo cartridge
// is usually filed under no board at all — so it was drawn as a six-button
// cabinet. FBNeo's own table says what the machine is: Button A…D, four of them.
// The panel then takes the NeoGeo's four buttons, whatever board it was filed
// under, and keeps the labels the machine uses.
func TestNeoGeoGameWearingSixKeys(t *testing.T) {
	descs := says(2, "Coin", 3, "Start", 0, "Button A", 8, "Button B", 1, "Button C", 9, "Button D")
	l := padPanelFor(padLayoutFor("arcade"), descs)
	if buttonsOf(l) != 4 {
		t.Fatalf("the NeoGeo panel came out with %d buttons, want 4", buttonsOf(l))
	}
	if l.ID != "arcade" {
		t.Errorf("the panel lost the board's id: %q", l.ID)
	}
	// Four buttons labelled A…D, each pressing the id FBNeo reads that letter from.
	labels := map[string]string{"b": "A", "y": "B", "a": "C", "x": "D"}
	for _, s := range l.Spots {
		if want, ok := labels[s.ID]; ok && s.Label != want {
			t.Errorf("%s is labelled %q, want %q", s.ID, s.Label, want)
		}
	}
	ids := padIDsFor(l, descs)
	want := map[string]int{"b": padB, "y": padA, "a": padY, "x": padX}
	for spot, one := range want {
		if got := idsOf(ids, spot); len(got) != 1 || got[0] != one {
			t.Errorf("%s presses %v, want %d", spot, got, one)
		}
	}
	if got := idsOf(ids, "cd"); len(got) != 2 || got[0] != padY || got[1] != padX {
		t.Errorf("C+D presses %v, want C and D", got)
	}
}

// TestFourButtonCabinetGetsFourKeys: a core that numbers four buttons — a NeoGeo
// romset on MAME 2003 Plus, a four-button cabinet of any maker — draws four keys,
// not the six of the board it happened to be filed under.
func TestFourButtonCabinetGetsFourKeys(t *testing.T) {
	descs := says(0, "B1: Button 1", 8, "B2: Button 2", 1, "B3: Button 3", 9, "B4: Button 4",
		4, "Jump", 5, "Crouch", 2, "Coin", 3, "Start")
	l := padPanelFor(padLayoutFor("arcade"), descs)
	if buttonsOf(l) != 4 {
		t.Fatalf("a four-button cabinet came out with %d buttons", buttonsOf(l))
	}
	ids := padIDsFor(l, descs)
	for spot, one := range map[string]int{"b": padB, "y": padA, "a": padY, "x": padX} {
		if got := idsOf(ids, spot); len(got) != 1 || got[0] != one {
			t.Errorf("%s presses %v, want %d", spot, got, one)
		}
	}
}

// TestThreeButtonCabinetGetsThreeKeys: the same the other way — three buttons on
// the core's table is a three-button cabinet.
func TestThreeButtonCabinetGetsThreeKeys(t *testing.T) {
	descs := says(2, "Coin", 3, "Start", 1, "Button 1", 9, "Button 2", 10, "Button 3")
	l := padPanelFor(padLayoutFor("arcade"), descs)
	if buttonsOf(l) != 3 {
		t.Fatalf("a three-button cabinet came out with %d buttons", buttonsOf(l))
	}
}

// TestNeoGeoCDPanelIsFourKeys: the NeoGeo CD core names its buttons plainly A,
// B, C, D — C and D being letters the retropad never uses — so a NeoGeo CD game
// is a four-button machine too, not the six the shelf's unknown board drew.
func TestNeoGeoCDPanelIsFourKeys(t *testing.T) {
	descs := says(4, "Up", 5, "Down", 6, "Left", 7, "Right",
		0, "A", 8, "B", 1, "C", 9, "D", 2, "Select/Coin", 3, "Start",
		10, "C+D", 11, "A+B", 12, "B+C", 13, "A+B+C")
	l := padPanelFor(padLayoutFor("arcade"), descs)
	if buttonsOf(l) != 4 {
		t.Fatalf("the NeoGeo CD panel came out with %d buttons", buttonsOf(l))
	}
	ids := padIDsFor(l, descs)
	for spot, one := range map[string]int{"b": padB, "y": padA, "a": padY, "x": padX} {
		if got := idsOf(ids, spot); len(got) != 1 || got[0] != one {
			t.Errorf("%s presses %v, want %d", spot, got, one)
		}
	}
}

// TestMegaDriveStaysSixOfItsOwn: a Mega Drive announces A B C X Y Z, and the C is
// a real Mega Drive button — the NeoGeo rule must not claim a six-button console.
func TestMegaDriveStaysSixOfItsOwn(t *testing.T) {
	descs := says(0, "B", 8, "C", 9, "Y", 1, "A", 10, "X", 11, "Z", 2, "Mode", 3, "Start")
	if l := padPanelFor(padLayoutFor("md"), descs); l != padLayoutFor("md") {
		t.Errorf("the Mega Drive pad was turned into %q", l.Name)
	}
}

// TestNESPadsStayNES: FCEUmm announces its own extras (insert coin, disk side
// change, turbo A) beside B and A, and none of that may turn a two-button pad
// into an arcade cabinet of six.
func TestNESPadsStayNES(t *testing.T) {
	descs := says(
		0, "B", 8, "A", 14, "A+B", 2, "Select", 3, "Start",
		13, "(VSSystem) Insert Coin", 10, "(FDS) Disk Side Change", 9, "Turbo A",
	)
	if l := padPanelFor(padLayoutFor("fc"), descs); l != padLayoutFor("fc") {
		t.Errorf("the NES pad was turned into %q (%d buttons)", l.Name, buttonsOf(l))
	}
	ids := padIDsFor(padLayoutFor("fc"), descs)
	if got := idsOf(ids, "b"); len(got) != 1 || got[0] != padB {
		t.Errorf("B presses %v, want B", got)
	}
	if got := idsOf(ids, "a"); len(got) != 1 || got[0] != padA {
		t.Errorf("A presses %v, want A", got)
	}
}

// TestStreetFighterGetsSixOnACPS1Board: a CPS1 board carries six-button games —
// 街头霸王 II is three punches and three kicks — so its panel is the six of a
// cabinet, not the three it was once written as. A game that reads fewer keeps
// the extra keys dead.
func TestStreetFighterGetsSixOnACPS1Board(t *testing.T) {
	descs := says(
		1, "Weak Punch", 9, "Medium Punch", 10, "Strong Punch",
		0, "Weak Kick", 8, "Medium Kick", 11, "Strong Kick",
	)
	l := padPanelFor(padLayoutFor("cps1"), descs)
	if buttonsOf(l) != 6 || l.ID != "cps1" {
		t.Errorf("the CPS1 panel came out as %q with %d buttons", l.ID, buttonsOf(l))
	}
	// Six buttons on screen, and the names say where: 1→Y, 2→X, 3→L, 4→B, 5→A, 6→R.
	ids := padIDsFor(l, descs)
	want := map[string]int{"b": padY, "y": padX, "a": padL, "x": padB, "l": padA, "r": padR}
	for spot, one := range want {
		if got := idsOf(ids, spot); len(got) != 1 || got[0] != one {
			t.Errorf("%s presses %v, want %d", spot, got, one)
		}
	}
}

// TestMAME2003PlusStreetFighterNumbersItsButtons: the case the player hit on
// 超级街头霸王 II. MAME 2003 Plus reads Button 1 from B, Button 2 from A and
// Button 3 from Y — read out of the core itself for hsf2 — so a panel that took
// the drawn ids would press 中拳 with the "2" key and 重拳 with the "3". Its names
// carry the machine's number ("B1: Jab Punch"), which is what the panel follows:
// B1→B, B2→A, B3→Y, B4→X, B5→L, B6→R. The same names are also the reason the
// panel must never fall back on the announce order here: that order would put
// Jump and Crouch on the "3" and "4" keys.
func TestMAME2003PlusStreetFighterNumbersItsButtons(t *testing.T) {
	descs := says(
		0, "B1: Jab Punch", 1, "B3: Fierce Punch", 2, "Coin", 3, "Start",
		4, "Jump", 5, "Crouch", 6, "Left", 7, "Right",
		8, "B2: Strong Punch", 9, "B4: Short Kick",
		10, "B5: Forward Kick", 11, "B6: Roundhouse Kick",
	)
	l := padPanelFor(padLayoutFor("cps2"), descs)
	ids := padIDsFor(l, descs)
	want := map[string][]int{
		"b":   {padB},            // 1 轻拳 = B1
		"y":   {padA},            // 2 中拳 = B2
		"a":   {padY},            // 3 重拳 = B3
		"x":   {padX},            // 4 轻脚 = B4
		"l":   {padL},            // 5 中脚 = B5
		"r":   {padR},            // 6 重脚 = B6
		"abc": {padB, padA, padY}, // the three punches, in the order the keys name them
		"def": {padX, padL, padR},
	}
	for spot, w := range want {
		got := idsOf(ids, spot)
		if len(got) != len(w) {
			t.Errorf("%s presses %v, want %v", spot, got, w)
			continue
		}
		for i := range w {
			if got[i] != w[i] {
				t.Errorf("%s presses %v, want %v", spot, got, w)
				break
			}
		}
	}
}

// TestMAMEKeepsTheDrawnOrder: a core that only names the retropad, its numbers
// nowhere, is positional: its buttons are whatever the panel drew, and nothing is
// moved.
func TestMAMEKeepsTheDrawnOrder(t *testing.T) {
	descs := says(
		8, "A", 0, "B", 9, "X", 1, "Y", 10, "L", 11, "R", 2, "Select", 3, "Start",
	)
	if ids := padIDsFor(padLayoutFor("cps2"), descs); ids != nil {
		t.Errorf("the MAME panel was remapped: %v", ids)
	}
}

// TestNeoGeoPocketOrder: mednafen_ngp announces A at B and B at A, so the panel's
// A and B have to be swapped against the way it draws them, and Option is the
// core's Start.
func TestNeoGeoPocketOrder(t *testing.T) {
	descs := says(0, "A", 8, "B", 3, "Option")
	ids := padIDsFor(padLayoutFor("ngp"), descs)
	if got := idsOf(ids, "a"); len(got) != 1 || got[0] != padB {
		t.Errorf("A presses %v, want B (the core's A)", got)
	}
	if got := idsOf(ids, "b"); len(got) != 1 || got[0] != padA {
		t.Errorf("B presses %v, want A (the core's B)", got)
	}
	if got := idsOf(ids, "select"); len(got) != 1 || got[0] != padStart {
		t.Errorf("Option presses %v, want Start", got)
	}
}

// TestConsolesAlreadyAgree: for a console the core names the machine's own
// buttons, and the panel already draws each of them on the id the core reads —
// so what the core says changes nothing there, and the check confirms the drawn
// order instead of moving it.
func TestConsolesAlreadyAgree(t *testing.T) {
	sames := func(layout string, descs []core.InputDesc) {
		l := padLayoutFor(layout)
		ids := padIDsFor(l, descs)
		for _, s := range l.Spots {
			if len(s.Drives) != 1 {
				continue
			}
			drawn, ok := padRetroIds[targetsOf(s)[0]]
			if !ok || drawn < 0 {
				continue
			}
			got := ids[s.ID]
			if len(got) == 0 {
				continue // nothing named this button: the drawn id stands
			}
			if len(got) != 1 || got[0] != drawn {
				t.Errorf("%s: %q is drawn on %d, the core reads it from %v",
					layout, s.Label, drawn, got)
			}
		}
	}
	sames("sfc", says(0, "B", 8, "A", 9, "X", 1, "Y", 10, "L", 11, "R", 2, "Select", 3, "Start"))
	sames("md", says(0, "B", 8, "C", 9, "Y", 1, "A", 10, "X", 11, "Z", 2, "Mode", 3, "Start"))
	sames("md", says(0, "B", 8, "A", 1, "Start", 2, "Select"))
	sames("fc", says(0, "B", 8, "A", 2, "Select", 3, "Start"))
}



