package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/R19988088/GameRoom/internal/library"
)

// --- the buttons of a pad, as the cores read them ---

// RetroPad ids (RETRO_DEVICE_ID_JOYPAD_*), the one language every core speaks:
// a machine's own buttons are names laid over these.
const (
	padB      = 0
	padY      = 1
	padSelect = 2
	padStart  = 3
	padUp     = 4
	padDown   = 5
	padLeft   = 6
	padRight  = 7
	padA      = 8
	padX      = 9
	padL      = 10
	padR      = 11
	padL2     = 12
	padR2     = 13
	padL3     = 14
	padR3     = 15
	// padNothing is a key that presses no button of the machine: the stick's hold
	// key keeps a direction on, it does not send one into the game.
	padNothing = -1
)

// padRetroIds names each of them, which is how a binding is filed.
var padRetroIds = map[string]int{
	"b": padB, "y": padY, "select": padSelect, "start": padStart,
	"up": padUp, "down": padDown, "left": padLeft, "right": padRight,
	"a": padA, "x": padX, "l": padL, "r": padR,
	"l2": padL2, "r2": padR2, "l3": padL3, "r3": padR3,
	// The stick's four directions are filed under ids of their own, so a machine
	// that has a stick keeps two sets of direction bindings: the cross's and the
	// stick's. lshold is the stick's hold key: it presses nothing itself, it
	// keeps the direction being held.
	"lsup": padUp, "lsdown": padDown, "lsleft": padLeft, "lsright": padRight,
	"lshold": padNothing,
}

// padBind is where one button of a machine's pad is read from: a key of the
// keyboard, a button of the gamepad, or both. The key is written down with its
// Windows virtual-key code so it reads the same whatever the layout types;
// Pad is the gamepad button's index + 1 in XInput's order, 0 for none.
type padBind struct {
	Key string `json:"key,omitempty"`
	VK  int    `json:"vk,omitempty"`
	Pad int    `json:"pad,omitempty"`
}

// --- the model each machine gets ---

// The canvas is as wide as the panel holds, so the frame of the model lines up
// with the rows above and below it, and every machine is drawn on the same
// canvases: a body with the d-pad on the left and the machine's own buttons on
// the right, on shared columns and rows.
const (
	padModelW = 484
	padModelH = 212
)

// padSpot is one button of the model: the machine's own name for it, the id its
// binding is filed under, the RetroPad ids it presses, and where it sits.
type padSpot struct {
	ID     string  // what a binding is filed under: "a", "start", "abc"…
	Label  string  // the machine's name: "A", "投币", "×"
	X, Y   float32 // top left on the canvas
	W, H   float32
	Kind   string   // "face" a round button, "pill" a bar, "dir" an arm of the d-pad
	Drives []string // the RetroPad ids it presses; nothing means ID itself
}

// targetsOf is what one spot presses: one id usually, several for a macro like
// ABC, which is the three punches at once.
func targetsOf(s padSpot) []string {
	if len(s.Drives) > 0 {
		return s.Drives
	}
	return []string{s.ID}
}

// padLayout is one machine's pad: what the model shows, and which RetroPad ids
// its buttons stand for.
type padLayout struct {
	ID    string
	Name  string
	Spots []padSpot
}

// The canvas is divided once, and every model uses the same divisions: the
// d-pad on the left, three columns of face buttons on the right, a row of bars
// under the d-pad. That is what makes the models line up with one another.
const (
	dpadX, dpadY = 100, 108 // the middle of the cross
	armSize      = 30
	padD         = 32 // the diameter of a round button

	col1, col2, col3 = 322, 366, 410
	row1, row2       = 86, 130

	lbarX, rbarX           = 61, 151
	lbarW, barH            = 84, 26
	barY                   = 164
	shoulderY, shoulderH   = 18, 20
	shoulderW              = 110
	shoulderLX, shoulderRX = 61, 313
	macroX1, macroX2       = 244, 334
	diamondCY              = 102
	diamondR               = 38

	// The small stick sits between the cross and the buttons, with its own four
	// directions: a machine that has one reads them apart from the d-pad, so a
	// player can bind them apart too.
	stickX, stickY = 200, 104
	stickArm       = 20
)

// dirSpots are the four arms of the d-pad, at the same place on every model: a
// triangle on each arm, as the cross of a real pad has.
func dirSpots() []padSpot {
	return []padSpot{
		{"up", "▲", dpadX - 15, dpadY - 45, armSize, armSize, "dir", nil},
		{"down", "▼", dpadX - 15, dpadY + 15, armSize, armSize, "dir", nil},
		{"left", "◀", dpadX - 39, dpadY - 15, armSize, armSize, "dir", nil},
		{"right", "▶", dpadX + 15, dpadY - 15, armSize, armSize, "dir", nil},
	}
}

// stickSpots are the four directions of the small stick, and the key that keeps
// one of them on. They are filed under ids of their own — lsup, lsdown, lsleft,
// lsright — so the stick is its own set of direction bindings, while still
// pressing the directions the core knows. The hold key sits in the middle of the
// well: it is a button of its own, bound like any other, not a switch.
func stickSpots() []padSpot {
	return []padSpot{
		{"lsup", "▲", stickX - 10, stickY - 32, stickArm, stickArm, "stick", []string{"up"}},
		{"lsdown", "▼", stickX - 10, stickY + 12, stickArm, stickArm, "stick", []string{"down"}},
		{"lsleft", "◀", stickX - 32, stickY - 10, stickArm, stickArm, "stick", []string{"left"}},
		{"lsright", "▶", stickX + 12, stickY - 10, stickArm, stickArm, "stick", []string{"right"}},
		{"lshold", "长按", stickX - 11, stickY - 11, 22, 22, "hold", nil},
	}
}

// faceAt is a round button centred on the place it holds on a real pad.
func faceAt(id, label string, cx, cy float32) padSpot {
	return padSpot{ID: id, Label: label, X: cx - padD/2, Y: cy - padD/2, W: padD, H: padD, Kind: "face"}
}

// barAt is a rounded bar: a shoulder button, or two buttons made into a macro.
func barAt(id, label string, x, y, w, h float32, drives ...string) padSpot {
	return padSpot{ID: id, Label: label, X: x, Y: y, W: w, H: h, Kind: "pill", Drives: drives}
}

// padBars are the two bars under the d-pad: Select and Start on a console,
// 投币 and Start on a cabinet.
func padBars(left, right string) []padSpot {
	return []padSpot{
		barAt("select", left, lbarX, barY, lbarW, barH),
		barAt("start", right, rbarX, barY, lbarW, barH),
	}
}

// shoulders are the two bars over the buttons: L and R on a Super Nintendo, L1
// and R1 on a PlayStation, L and R on a Saturn.
func shoulders(left, right string) []padSpot {
	return []padSpot{
		barAt("l", left, shoulderLX, shoulderY, shoulderW, shoulderH),
		barAt("r", right, shoulderRX, shoulderY, shoulderW, shoulderH),
	}
}

// shoulders2 are the two bars of a pad whose six buttons already took L and R:
// the Saturn reads its Z from L and its C from R, so its shoulders are L2 and
// R2, which is what the core asks for.
func shoulders2(left, right string) []padSpot {
	return []padSpot{
		barAt("l2", left, shoulderLX, shoulderY, shoulderW, shoulderH),
		barAt("r2", right, shoulderRX, shoulderY, shoulderW, shoulderH),
	}
}

// macro is a bar that presses several buttons at once, for the combinations a
// fighting game asks for.
func macro(id, label string, x float32, drives ...string) padSpot {
	return barAt(id, label, x, barY, lbarW, barH, drives...)
}

// padModel joins the parts of one machine's model: the d-pad, its own buttons,
// and the bars underneath.
func padModel(parts ...[]padSpot) []padSpot {
	out := make([]padSpot, 0, 32)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// faces is the shorthand every console uses: its buttons, each where it sits.
func faces(list ...padSpot) []padSpot { return list }

// psFaces is the PlayStation diamond.
func psFaces() []padSpot {
	return faces(
		faceAt("x", "△", col2, diamondCY-diamondR),
		faceAt("y", "□", col1, diamondCY),
		faceAt("a", "○", col3, diamondCY),
		faceAt("b", "×", col2, diamondCY+diamondR),
	)
}

// xboxFaces is the diamond both Xbox pads wear: A down, B right, X left, Y up.
func xboxFaces() []padSpot {
	return faces(
		faceAt("y", "Y", col2, diamondCY-diamondR),
		faceAt("x", "X", col1, diamondCY),
		faceAt("b", "B", col3, diamondCY),
		faceAt("a", "A", col2, diamondCY+diamondR),
	)
}

// padLayouts is every machine the panel draws. The ids are what a profile is
// filed under — one profile per machine, shared by every game of it.
var padLayouts = []padLayout{
	{"fc", "FC / NES", padModel(dirSpots(), faces(
		faceAt("b", "B", col2, row2),
		faceAt("a", "A", col3, row1),
	), padBars("Select", "Start"))},
	{"sfc", "SFC / SNES", padModel(dirSpots(), faces(
		faceAt("x", "X", col2, diamondCY-diamondR),
		faceAt("y", "Y", col1, diamondCY),
		faceAt("a", "A", col3, diamondCY),
		faceAt("b", "B", col2, diamondCY+diamondR),
	), shoulders("L", "R"), padBars("Select", "Start"))},
	{"gb", "GB / GBC", padModel(dirSpots(), faces(
		faceAt("b", "B", col1+12, row2),
		faceAt("a", "A", col3-12, row2),
	), padBars("Select", "Start"))},
	{"gba", "GBA", padModel(dirSpots(), faces(
		faceAt("b", "B", col1+12, row2),
		faceAt("a", "A", col3-12, row2),
	), shoulders("L", "R"), padBars("Select", "Start"))},
	{"ngp", "NGP", padModel(dirSpots(), faces(
		faceAt("b", "B", col2, row2+8),
		faceAt("a", "A", col3, row1+8),
	), padBars("Option", "Start"))},
	// Mega Drive's six buttons: A B C along the bottom, X Y Z over them. The
	// core reads A from Y, B from B, C from A, X from L, Y from X, Z from R.
	{"md", "MD / GENESIS", padModel(dirSpots(), faces(
		faceAt("y", "A", col1, row2),
		faceAt("b", "B", col2, row2),
		faceAt("a", "C", col3, row2),
		faceAt("l", "X", col1, row1),
		faceAt("x", "Y", col2, row1),
		faceAt("r", "Z", col3, row1),
	), padBars("Mode", "Start"))},
	// Saturn's six buttons, and its two shoulders. The core reads A from B, B
	// from A, C from R, X from Y, Y from X, Z from L, L from L2, R from R2.
	{"ss", "SS / SATURN", padModel(dirSpots(), faces(
		faceAt("b", "A", col1, row2),
		faceAt("a", "B", col2, row2),
		faceAt("r", "C", col3, row2),
		faceAt("y", "X", col1, row1),
		faceAt("x", "Y", col2, row1),
		faceAt("l", "Z", col3, row1),
	), shoulders2("L", "R"), []padSpot{
		barAt("start", "Start", lbarX, barY, lbarW, barH),
	})},
	// The PlayStation diamond, read the way the cores read it: the pad's X is up
	// (△), its Y is left (□), its A is right (○), its B is down (×). Every Sony
	// machine — PS1, PS2, PS3, PSP, Vita — reads its buttons that way.
	{"ps", "PS1", padModel(dirSpots(), psFaces(), shoulders("L1", "R1"), padBars("Select", "Start"))},
	{"ps2", "PS2", padModel(dirSpots(), stickSpots(), psFaces(), shoulders("L1", "R1"),
		[]padSpot{
			barAt("select", "Select", lbarX, barY, lbarW, barH),
			barAt("start", "Start", rbarX, barY, lbarW, barH),
			barAt("l2", "L2", macroX1, barY, lbarW, barH),
			barAt("r2", "R2", macroX2, barY, lbarW, barH),
		})},
	{"ps3", "PS3", padModel(dirSpots(), stickSpots(), psFaces(), shoulders("L1", "R1"),
		[]padSpot{
			barAt("select", "Select", lbarX, barY, lbarW, barH),
			barAt("start", "Start", rbarX, barY, lbarW, barH),
			barAt("l2", "L2", macroX1, barY, lbarW, barH),
			barAt("r2", "R2", macroX2, barY, lbarW, barH),
		})},
	{"psp", "PSP", padModel(dirSpots(), stickSpots(), psFaces(), shoulders("L", "R"),
		padBars("Select", "Start"))},
	{"psv", "PSV", padModel(dirSpots(), stickSpots(), psFaces(), shoulders("L", "R"),
		padBars("Select", "Start"))},
	// A Dreamcast pad: A down, B right, X left, Y up, and its triggers on L2/R2.
	{"dc", "DC", padModel(dirSpots(), stickSpots(), faces(
		faceAt("b", "A", col2, diamondCY+diamondR),
		faceAt("a", "B", col3, diamondCY),
		faceAt("y", "X", col1, diamondCY),
		faceAt("x", "Y", col2, diamondCY-diamondR),
	), []padSpot{
		barAt("start", "Start", lbarX, barY, lbarW, barH),
		barAt("l2", "LT", rbarX, barY, lbarW, barH),
		barAt("r2", "RT", macroX1, barY, lbarW, barH),
	})},
	// The two Xbox pads: A down, B right, X left, Y up, the bumpers on L and R,
	// the triggers on L2 and R2.
	{"xbox", "XBOX", padModel(dirSpots(), stickSpots(), xboxFaces(), shoulders("LB", "RB"),
		[]padSpot{
			barAt("select", "Back", lbarX, barY, lbarW, barH),
			barAt("start", "Start", rbarX, barY, lbarW, barH),
			barAt("l2", "LT", macroX1, barY, lbarW, barH),
			barAt("r2", "RT", macroX2, barY, lbarW, barH),
		})},
	{"xbox360", "XBOX 360", padModel(dirSpots(), stickSpots(), xboxFaces(), shoulders("LB", "RB"),
		[]padSpot{
			barAt("select", "Back", lbarX, barY, lbarW, barH),
			barAt("start", "Start", rbarX, barY, lbarW, barH),
			barAt("l2", "LT", macroX1, barY, lbarW, barH),
			barAt("r2", "RT", macroX2, barY, lbarW, barH),
		})},
	// A 3DS: A right, B down, X up, Y left, L and R over the screen, and the
	// circle pad beside the cross.
	{"3ds", "3DS", padModel(dirSpots(), stickSpots(), faces(
		faceAt("a", "A", col3, diamondCY),
		faceAt("b", "B", col2, diamondCY+diamondR),
		faceAt("x", "X", col2, diamondCY-diamondR),
		faceAt("y", "Y", col1, diamondCY),
	), shoulders("L", "R"), padBars("Select", "Start"))},
	{"3do", "3DO", padModel(dirSpots(), faces(
		faceAt("b", "A", col1+12, row2),
		faceAt("y", "B", col2, row2),
		faceAt("a", "C", col3-12, row2),
	), []padSpot{
		barAt("start", "Start", lbarX, barY, lbarW, barH),
	})},
	// An MVS panel: four buttons, and the two combinations they are played in.
	{"mvs", "MVS / NEOGEO", padModel(dirSpots(), arcadeFour(), padBars("投币", "Start"), []padSpot{
		macro("ab", "A+B", macroX1, "b", "y"),
		macro("cd", "C+D", macroX2, "a", "x"),
	})},
	// The board a game runs on names its own panel; this is the one for a board
	// the shelf cannot place, so no game is left without a model.
	{"arcade", "街机", arcadeSix()},
}

// arcadeFour is the MVS panel's buttons: A B along the bottom, C D over them.
func arcadeFour() []padSpot {
	return faces(
		faceAt("b", "A", col1, row2),
		faceAt("y", "B", col2, row2),
		faceAt("a", "C", col1, row1),
		faceAt("x", "D", col2, row1),
	)
}

// arcadeThree is a CPS1 panel: the three buttons a CPS1 board reads.
func arcadeThree() []padSpot {
	return padModel(dirSpots(), faces(
		faceAt("b", "1", col1, row2),
		faceAt("y", "2", col2, row2),
		faceAt("a", "3", col3, row2),
	), padBars("投币", "Start"))
}

// arcadeSix is a Capcom panel: six buttons, and the two three-button
// combinations a fighting game wants in a single press. Every board bigger than
// CPS1 reads these, so a game's panel is built from the board it runs on.
func arcadeSix() []padSpot {
	return padModel(dirSpots(), faces(
		faceAt("b", "1", col1, row2),
		faceAt("y", "2", col2, row2),
		faceAt("a", "3", col3, row2),
		faceAt("x", "4", col1, row1),
		faceAt("l", "5", col2, row1),
		faceAt("r", "6", col3, row1),
	), padBars("投币", "Start"), []padSpot{
		macro("abc", "ABC", macroX1, "b", "y", "a"),
		macro("def", "DEF", macroX2, "x", "l", "r"),
	})
}

// arcadeNames are the boards whose own name is not its id in capitals: CPS3 is
// the CP System III, and that is what the panel calls it.
var arcadeNames = map[string]string{
	"cps1": "CPS1", "cps2": "CPS2", "cps3": "CPSIII",
	"st-v": "ST-V", "naomi-gd": "NAOMI GD",
}

// arcadeName is a board's name, as the panel shows it.
func arcadeName(id string) string {
	if n, ok := arcadeNames[id]; ok {
		return n
	}
	return strings.ToUpper(strings.ReplaceAll(id, "-", " "))
}

// boardLayout builds the panel of a board the shelf names, at the moment a game
// of it starts: the board decides how many buttons it reads, so CPS1 gets three
// and everything above it the six of a Capcom cabinet.
func boardLayout(id string) *padLayout {
	spots := arcadeSix()
	if id == "cps1" {
		spots = arcadeThree()
	}
	return &padLayout{ID: id, Name: arcadeName(id), Spots: spots}
}

// layoutID is a board's name written as an id: lowercase, one dash where the
// shelf has a space, so a board files its bindings under one name however the
// shelf spells it (NAOMI GD, TAITO F2, ST-V).
func layoutID(board string) string {
	s := strings.ToLower(strings.TrimSpace(board))
	s = strings.NewReplacer(" ", "-", "_", "-", "/", "-").Replace(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// padLayoutFor finds a machine's model: one of the panels written down here, or
// the panel a board brings with it.
func padLayoutFor(id string) *padLayout {
	for i := range padLayouts {
		if padLayouts[i].ID == id {
			return &padLayouts[i]
		}
	}
	if id == "" {
		return nil
	}
	return boardLayout(id)
}

// coreOf is the core a game runs on, or "" when there is no shelf to ask —
// a panel test has no library behind it.
func (a *App) coreOf(g library.Game) string {
	if a.lib == nil {
		return ""
	}
	return a.lib.CoreOf(g)
}

// padLayoutOf is the model a game's pad is: the board of an arcade romset
// (CPS2, CPS3, MVS…), or the machine a console folder actually holds — SEGA
// keeps a Mega Drive, a Saturn and a Dreamcast side by side. The panel follows
// the game that is running, so there is nothing to pick by hand. "" means the
// machine is played on the keyboard, DOS and PC-98, and has no model.
func (a *App) padLayoutOf(g library.Game) string {
	rom := strings.ToLower(g.RomPath)
	switch g.PlatformKey {
	case "fc":
		return "fc"
	case "sfc":
		return "sfc"
	case "ps":
		// The folder holds a PS1, a PS2 or a PS3 side by side; the core says which.
		switch a.coreOf(g) {
		case "pcsx2", "pcsx_rearmed":
			return "ps2"
		case "rpcs3":
			return "ps3"
		}
		return "ps"
	case "3DO":
		return "3do"
	case "SEGA":
		// The folder says which SEGA machine it is, as the core choice does:
		// Model 2/3 and ST-V are arcade boards, DC is a Dreamcast, SS a Saturn,
		// and the rest is the Mega Drive.
		switch {
		case strings.Contains(rom, "/model 2/"), strings.Contains(rom, "/model 3/"),
			strings.Contains(rom, "/stv/"):
			return "arcade"
		case strings.Contains(rom, "/dc/"), strings.HasSuffix(rom, ".gdi"),
			strings.HasSuffix(rom, ".cdi"):
			return "dc"
		case strings.Contains(rom, "/ss/"):
			return "ss"
		}
		return "md"
	case "掌机":
		switch a.coreOf(g) {
		case "ppsspp":
			return "psp"
		case "vita3k":
			return "psv"
		case "mednafen_ngp":
			return "ngp"
		case "gearboy":
			return "gb"
		case "mgba":
			if strings.HasSuffix(rom, ".gb") || strings.HasSuffix(rom, ".gbc") {
				return "gb"
			}
			return "gba"
		}
		return "gba"
	case "任天堂":
		if a.coreOf(g) == "citra" || strings.Contains(rom, "/3ds/") ||
			strings.HasSuffix(rom, ".cia") || strings.HasSuffix(rom, ".cci") {
			return "3ds"
		}
		return "arcade"
	case "微软":
		// The folder holds an Xbox and an Xbox 360; the 360's core is the launcher.
		if a.coreOf(g) == "xenia_launcher" || strings.Contains(rom, "/360/") {
			return "xbox360"
		}
		return "xbox"
	case "arc", "majiang":
		// The board names its own panel, and its own profile: CPS1 reads three
		// buttons, CPS2 and CPS3 six, the MVS four. A romset filed under a name
		// that is not a board falls back to the cabinet every game is played on.
		if b := library.BoardOf(g.Board); library.BoardKnown(b) {
			return layoutID(b)
		}
		return "arcade"
	}
	return "" // dos, pc98: the keyboard is the pad
}

// --- what the buttons point at by default ---

// defaultKeys and defaultPads are what a button reads from until the player
// binds it themselves. Keys are VKs; pads are XInput indices + 1.
var defaultKeys = map[string]int{
	"a": 0x4A, "b": 0x4B, "x": 0x49, "y": 0x55,
	"l": 0x51, "r": 0x45, "start": 0x0D, "select": 0x09,
	"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
	// The stick has no keys of its own until the player gives it some: the gamepad
	// stick is its default, and WASD would only be a second set of arrows.
}

var defaultPads = map[string]int{
	"a": 11, "b": 12, "x": 13, "y": 14,
	"l": 9, "r": 10, "start": 5, "select": 6,
	"up": 1, "down": 2, "left": 3, "right": 4,
	"l2": 15, "r2": 16, "l3": 7, "r3": 8,
	// XInput indices 16..19 are the left stick's four directions.
	"lsup": 17, "lsdown": 18, "lsleft": 19, "lsright": 20,
}

// defaultPadBinds is a profile with nothing changed in it: every button the
// machine draws, but no macro — a combination is something the player adds.
func defaultPadBinds() map[string]padBind {
	m := make(map[string]padBind, len(padRetroIds))
	for id := range padRetroIds {
		b := padBind{Pad: defaultPads[id]}
		if vk, ok := defaultKeys[id]; ok {
			b.VK, b.Key = vk, vkName(vk)
		}
		m[id] = b
	}
	return m
}

// padProfile is the profile a machine actually runs with: its saved bindings
// laid over the defaults.
func (a *App) padProfile(layout string) map[string]padBind {
	m := defaultPadBinds()
	for id, b := range a.pads[layout] {
		m[id] = b
	}
	return m
}

// --- 数据/controls.json ---

type padFile struct {
	Version int                           `json:"version"`
	Layouts map[string]map[string]padBind `json:"layouts"`
}

func (a *App) controlsPath() string { return filepath.Join(a.root, "数据", "controls.json") }

// loadPads reads the bindings the player made. A missing file is the defaults.
func (a *App) loadPads() {
	a.pads = map[string]map[string]padBind{}
	data, err := os.ReadFile(a.controlsPath())
	if err != nil {
		return
	}
	var f padFile
	if err := json.Unmarshal(data, &f); err != nil || f.Layouts == nil {
		return
	}
	a.pads = f.Layouts
}

// savePads keeps them: one entry per machine, shared by every game of it.
func (a *App) savePads() {
	f := padFile{Version: 1, Layouts: a.pads}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(a.controlsPath()), 0o755)
	_ = os.WriteFile(a.controlsPath(), append(data, '\n'), 0o644)
}

// bindPad writes one button's source down. One source drives one button only,
// so the others give it up: two buttons on one key would fire together.
func (a *App) bindPad(layout, id, opt string, vk, pad int) {
	if a.pads[layout] == nil {
		a.pads[layout] = map[string]padBind{}
	}
	prof := a.padProfile(layout)
	for other, ob := range prof {
		if other == id {
			continue
		}
		gave := false
		if opt == "key" && ob.VK == vk {
			ob.VK, ob.Key, gave = 0, "", true
		}
		if opt == "pad" && ob.Pad == pad {
			ob.Pad, gave = 0, true
		}
		if gave {
			a.pads[layout][other] = ob
		}
	}
	if opt == "key" {
		a.pads[layout][id] = padBind{Key: vkName(vk), VK: vk}
	} else {
		a.pads[layout][id] = padBind{Pad: pad}
	}
	a.savePads()
}

// resetPads drops a machine's own bindings, leaving the defaults.
func (a *App) resetPads(layout string) {
	delete(a.pads, layout)
	a.savePads()
}

// --- reading the sources ---

// vkKey is one key the panel can read and write down: its VK code, and how a
// binding shows it.
type vkKey struct {
	VK   int
	Name string
}

// vkTable is the keyboard a pad button may be bound to.
var vkTable = buildVKTable()

func buildVKTable() []vkKey {
	list := make([]vkKey, 0, 96)
	for i := 0; i < 26; i++ {
		list = append(list, vkKey{0x41 + i, string(rune('A' + i))})
	}
	for i := 0; i < 10; i++ {
		list = append(list, vkKey{0x30 + i, strconv.Itoa(i)})
	}
	for i := 0; i < 12; i++ {
		list = append(list, vkKey{0x70 + i, "F" + strconv.Itoa(i+1)})
	}
	return append(list,
		vkKey{0x26, "↑"}, vkKey{0x28, "↓"}, vkKey{0x25, "←"}, vkKey{0x27, "→"},
		vkKey{0x0D, "Enter"}, vkKey{0x20, "空格"}, vkKey{0x09, "Tab"},
		vkKey{0x08, "Back"}, vkKey{0x1B, "Esc"},
		vkKey{0x2D, "-"}, vkKey{0xBB, "="}, vkKey{0x5B, "["}, vkKey{0x5D, "]"},
		vkKey{0x5C, `\`}, vkKey{0xBA, ";"}, vkKey{0xDE, "'"}, vkKey{0xBC, ","},
		vkKey{0xBE, "."}, vkKey{0xBF, "/"}, vkKey{0xC0, "`"},
		vkKey{0x10, "Shift"}, vkKey{0x11, "Ctrl"}, vkKey{0x12, "Alt"},
		vkKey{0x2E, "Del"}, vkKey{0x24, "Home"}, vkKey{0x23, "End"},
		vkKey{0x21, "PgUp"}, vkKey{0x22, "PgDn"}, vkKey{0x2C, "PrtSc"},
	)
}

// vkName is how a key reads on a bound button.
func vkName(vk int) string {
	for _, k := range vkTable {
		if k.VK == vk {
			return k.Name
		}
	}
	return ""
}

// padNames are the gamepad's buttons by XInput's index, as a binding shows
// them; a pad binding stores the index + 1.
var padNames = []string{
	"↑", "↓", "←", "→", "Start", "Back", "L3", "R3",
	"LB", "RB", "A", "B", "X", "Y", "LT", "RT",
	"摇杆↑", "摇杆↓", "摇杆←", "摇杆→",
}

// padFace is what a binding shows on the model: the key it reads, or the
// gamepad's button when no key is set — one short face that fits in a button,
// where both would not.
func padFace(b padBind) string {
	switch {
	case b.Key != "":
		return b.Key
	case b.Pad > 0 && b.Pad <= len(padNames):
		return padNames[b.Pad-1]
	}
	return "—"
}

// heldVKs is the keyboard as the panel reads it: which of the keys it knows are
// down right now. Read from the system rather than from the window's events, as
// the core does, so a key held when the panel opens is seen as held.
func heldVKs() map[int]bool {
	held := make(map[int]bool, 8)
	for _, k := range vkTable {
		if st, _, _ := procKeyState.Call(uintptr(k.VK)); st&0x8000 != 0 {
			held[k.VK] = true
		}
	}
	return held
}

var (
	xinputDLL   = syscall.NewLazyDLL("xinput1_4.dll")
	xinputGet   = xinputDLL.NewProc("XInputGetState")
	xinputReady = -1 // 1 when the library answered, 0 when there is none
	xinputAlt   = []string{"xinput9_1_0.dll", "xinput1_3.dll"}
)

// xinputState is XINPUT_STATE: the packet number and XINPUT_GAMEPAD.
type xinputState struct {
	Packet  uint32
	Buttons uint16
	LT, RT  byte
	LX, LY  int16
	RX, RY  int16
}

// padBits maps XInput's button mask onto the indices a binding stores.
var padBits = []struct {
	Bit uint16
	Idx int
}{
	{0x0001, 0}, {0x0002, 1}, {0x0004, 2}, {0x0008, 3},
	{0x0010, 4}, {0x0020, 5}, {0x0040, 6}, {0x0080, 7},
	{0x0100, 8}, {0x0200, 9}, {0x1000, 10}, {0x2000, 11},
	{0x4000, 12}, {0x8000, 13},
}

// heldPads is the gamepad as the panel reads it: the buttons down right now,
// by XInput's index. The first pad that answers is the one. Its stick counts as
// the d-pad, because most players hold the stick, not the cross.
func heldPads() map[int]bool {
	if xinputReady < 0 {
		if err := xinputGet.Find(); err == nil {
			xinputReady = 1
		} else {
			xinputReady = 0
			for _, name := range xinputAlt {
				dll := syscall.NewLazyDLL(name)
				proc := dll.NewProc("XInputGetState")
				if err := proc.Find(); err == nil {
					xinputGet, xinputReady = proc, 1
					break
				}
			}
		}
	}
	if xinputReady == 0 {
		return nil
	}
	for i := uint32(0); i < 4; i++ {
		var st xinputState
		r, _, _ := xinputGet.Call(uintptr(i), uintptr(unsafe.Pointer(&st)))
		if r != 0 {
			continue
		}
		held := make(map[int]bool, 8)
		for _, b := range padBits {
			if st.Buttons&b.Bit != 0 {
				held[b.Idx] = true
			}
		}
		if st.LT > 40 {
			held[14] = true
		}
		if st.RT > 40 {
			held[15] = true
		}
		// The stick is read as itself, not as the cross: a machine that has one
		// files its stick directions under ids of their own, so the two can be
		// bound apart, and a stick held in a direction keeps pressing it.
		if st.LY > 18000 {
			held[16] = true
		}
		if st.LY < -18000 {
			held[17] = true
		}
		if st.LX < -18000 {
			held[18] = true
		}
		if st.LX > 18000 {
			held[19] = true
		}
		return held
	}
	return nil
}

// maskOf is the RetroPad held this frame for a profile: a button counts when
// its key is down, or its gamepad button is. A macro counts as all the buttons
// it presses.
func maskOf(l *padLayout, prof map[string]padBind, keys, pads map[int]bool) map[int]bool {
	if l == nil {
		return nil
	}
	held := make(map[int]bool, 8)
	for _, s := range l.Spots {
		b := prof[s.ID]
		if (b.VK != 0 && keys[b.VK]) || (b.Pad > 0 && pads[b.Pad-1]) {
			for _, t := range targetsOf(s) {
				if id, ok := padRetroIds[t]; ok && id >= 0 {
					held[id] = true
				}
			}
		}
	}
	return held
}

// pushPad hands the pad to the running core, which reads it as its own pad:
// the machine's buttons are already the ids it knows.
func (a *App) pushPad() {
	if a.core == nil {
		return
	}
	a.core.SetPad(a.padMask(heldVKs(), heldPads()))
}

// padMask is the RetroPad the core reads this frame: the model's buttons, plus
// the direction the stick's hold key is keeping on.
func (a *App) padMask(keys, pads map[int]bool) map[int]bool {
	l := padLayoutFor(a.padLive)
	if l == nil {
		return nil
	}
	prof := a.padProfile(a.padLive)
	held := maskOf(l, prof, keys, pads)
	// The hold key is a button of its own: pressed while the stick is tilted it
	// keeps that direction on; pressed with nothing tilted it lets it go.
	hb := prof["lshold"]
	down := (hb.VK != 0 && keys[hb.VK]) || (hb.Pad > 0 && pads[hb.Pad-1])
	if down && !a.holdDown {
		a.holdDown = true
		a.holdOn = ""
		tilted := ""
		for _, s := range l.Spots {
			if s.Kind != "stick" {
				continue
			}
			b := prof[s.ID]
			if (b.VK != 0 && keys[b.VK]) || (b.Pad > 0 && pads[b.Pad-1]) {
				tilted = targetsOf(s)[0]
				break
			}
		}
		if tilted != "" {
			a.holdOn = tilted
		}
	}
	if !down {
		a.holdDown = false
	}
	if a.holdOn != "" {
		if id, ok := padRetroIds[a.holdOn]; ok && id >= 0 {
			held[id] = true
		}
	}
	return held
}

// --- opening the panel, and taking a button ---

// togglePads opens or closes the pad panel; opening it closes the filter's.
func (a *App) togglePads() {
	a.padsOpen = !a.padsOpen
	a.filtersOpen = false
	a.cap = ""
}

// startCapture puts a button of the model in wait: the next key pressed on the
// keyboard or button on the gamepad becomes its source.
func (a *App) startCapture(id string) {
	if a.cap == id {
		a.cap = ""
		return
	}
	a.cap = id
	a.capVK = heldVKs()
	a.capPad = heldPads()
}

// padTick closes the panel on Escape, and takes the button being waited for.
func (a *App) padTick() {
	if !a.padsOpen {
		a.escDown = false
		return
	}
	keys, pads := heldVKs(), heldPads()
	if a.cap == "" {
		if keys[0x1B] && !a.escDown {
			a.padsOpen = false
		}
		a.escDown = keys[0x1B]
		return
	}
	a.captureTick(keys, pads)
}

// captureTick is the other half of padTick: the first key or gamepad button
// pressed after the wait began becomes the one the button reads. Esc cancels
// it, and a key already held when the wait began is not a press.
func (a *App) captureTick(keys, pads map[int]bool) {
	if keys[0x1B] && !a.capVK[0x1B] {
		a.cap = ""
		return
	}
	for i := 0; i < len(padNames); i++ {
		if pads[i] && !a.capPad[i] {
			a.bindPad(a.padLive, a.cap, "pad", 0, i+1)
			a.cap = ""
			return
		}
	}
	for _, k := range vkTable {
		if k.VK == 0x1B {
			continue
		}
		if keys[k.VK] && !a.capVK[k.VK] {
			a.bindPad(a.padLive, a.cap, "key", k.VK, 0)
			a.cap = ""
			return
		}
	}
}
