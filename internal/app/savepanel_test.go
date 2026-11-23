package app

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R19988088/GameRoom/internal/core"
	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// TestSavePanelShowsNineSlotsAndTheTimeline: the panel is a shelf of nine tiles
// and a timeline under it. A save on the shelf takes the tile its number names and
// shows when it was taken; the empty ones are the way to make a save.
func TestSavePanelShowsNineSlotsAndTheTimeline(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root, theme: buildTheme(), crt: defaultCRT()}
	a.playerGame = library.Game{Title: "电精", RomPath: `rom\sfc\电精.sfc`, PlatformKey: "sfc"}
	dir := a.savesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, filepath.Join("数据", "存档", "sfc-电精")) {
		t.Fatalf("the game's saves are filed under %s, want 数据/存档/sfc-电精", dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "save-2.state"), []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "save-2.json"),
		[]byte(`{"at":"2026-11-23T16:20:00+08:00","frames":900}`), 0o644); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rect(0, 0, 64, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 64; x++ {
			shot.Set(x, y, color.RGBA{R: 232, G: 118, B: 58, A: 255})
		}
	}
	if err := writeShot(filepath.Join(dir, "save-2.png"), shot, saveThumbW); err != nil {
		t.Fatal(err)
	}
	a.savesOpen = true
	a.scanSaves()
	if len(a.slots) != 1 || a.slots[0].N != 2 {
		t.Fatalf("the shelf holds %d slots, want the one at 2", len(a.slots))
	}
	if a.slots[0].Shot == nil {
		t.Fatalf("slot 2 has no picture on the shelf")
	}
	if got := a.slots[0].At.Format("01-02 15:04"); got != "11-23 16:20" {
		t.Fatalf("slot 2 reads %s, want the time it was saved at", got)
	}

	tt := ui.NewTester(func(c *ui.Context) { a.savePanel(c, &a.theme) }, 1280, 800)
	tt.Frame()

	title, ok := tt.Find("存档")
	if !ok {
		t.Fatalf("the panel's title is missing")
	}
	// The panel's own right edge is the ✕ inside its padding: nothing may sit
	// wider than that, or the glass has run past the window's inset.
	close, ok := tt.Find("✕")
	if !ok {
		t.Fatalf("the panel has no way out")
	}
	if edge := float32(1280 - edgeInset - savePanelPad); close.X+close.W > edge {
		t.Fatalf("the ✕ ends at %.0f, past the panel's own edge at %.0f", close.X+close.W, edge)
	}
	if nine, ok := tt.Find("9"); ok && nine.X+nine.W > float32(1280-edgeInset-savePanelPad) {
		t.Fatalf("the last tile ends at %.0f, outside the glass", nine.X+nine.W)
	}
	if title.X < float32(1280-edgeInset-savePanelW) {
		t.Fatalf("the title starts at %.0f, left of the panel at %d", title.X, 1280-edgeInset-savePanelW)
	}
	if _, ok := tt.Find("电精"); !ok {
		t.Fatalf("the header does not name the game")
	}
	empties := 0
	for _, s := range tt.Texts() {
		switch s {
		case "建档":
			empties++
		}
	}
	if empties != saveSlots-1 {
		t.Fatalf("%d tiles are empty, want %d (nine slots with one saved)", empties, saveSlots-1)
	}
	if _, ok := tt.Find("11-23 16:20"); !ok {
		t.Fatalf("slot 2 does not show when it was taken")
	}
	for _, label := range []string{"时间轴", "继续", "读取", "删除"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("%s is missing from the panel", label)
		}
	}

	// Reading is only offered for a selected tile: before a click it is quiet, and
	// a click on the saved tile is what selects it.
	if a.slotSel != 0 {
		t.Fatalf("an opened panel has %d selected, want nothing", a.slotSel)
	}
	at, ok := tt.Find("2")
	if !ok {
		t.Fatalf("slot 2's number is missing")
	}
	tt.ClickAt(at.X+40, at.Y-30)
	tt.Frame()
	if a.slotSel != 2 {
		t.Fatalf("a click on slot 2 left %d selected, want 2", a.slotSel)
	}
}

// TestSavePanelSaysWhatItCannotDo: without a game the panel still opens — it is
// the shelf — and a click on an empty tile says why nothing was written rather
// than making a broken file.
func TestSavePanelSaysWhatItCannotDo(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root, theme: buildTheme(), crt: defaultCRT()}
	a.playerGame = library.Game{Title: "电精", RomPath: `rom\sfc\电精.sfc`, PlatformKey: "sfc"}
	a.savesOpen = true
	a.scanSaves()
	tt := ui.NewTester(func(c *ui.Context) { a.savePanel(c, &a.theme) }, 1280, 800)
	tt.Frame()
	plus, ok := tt.Find("＋")
	if !ok {
		t.Fatalf("an empty tile has no way to make a save")
	}
	tt.ClickAt(plus.X, plus.Y)
	tt.Frame()
	if !tt.HasText("没有在跑的游戏") {
		t.Fatalf("a click on an empty tile without a game said nothing")
	}
	if entries, err := os.ReadDir(filepath.Join(root, "数据", "存档")); err == nil && len(entries) > 0 {
		t.Fatalf("a failed save still made %d files", len(entries))
	}
}

// TestSavePanelCrossesOffACoreWithoutState: a core that declares no state offers
// no slot to press — all nine show a cross and say so, instead of pretending a
// click would write something.
func TestSavePanelCrossesOffACoreWithoutState(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root, theme: buildTheme(), crt: defaultCRT(), core: &core.Session{}}
	a.playerGame = library.Game{Title: "电精", RomPath: `rom\sfc\电精.sfc`, PlatformKey: "sfc"}
	a.savesOpen = true
	a.scanSaves()
	tt := ui.NewTester(func(c *ui.Context) { a.savePanel(c, &a.theme) }, 1280, 800)
	tt.Frame()

	if a.canSave() {
		t.Fatalf("a core with no state is still offered as savable")
	}
	crosses := 0
	for _, s := range tt.Texts() {
		switch s {
		case "✕":
			crosses++
		}
	}
	// Eight tiles plus the panel's own way out, which is also a ✕.
	if crosses != saveSlots+1 {
		t.Fatalf("%d crosses, want one per slot and one for the panel (%d)", crosses, saveSlots+1)
	}
	for _, s := range []string{"＋", "建档"} {
		if tt.HasText(s) {
			t.Fatalf("a slot still offers %s on a core with no state", s)
		}
	}
	if !tt.HasText("不支持") {
		t.Fatalf("the slots do not say the core has no state")
	}
	label, ok := tt.Find("不支持")
	if !ok {
		t.Fatalf("no crossed slot to press")
	}
	tt.ClickAt(label.X+10, label.Y-40)
	tt.Frame()
	if a.slotNote != "这个核心不能存档" {
		t.Fatalf("a click on a crossed slot said %q, want why it did nothing", a.slotNote)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "数据", "存档")); err == nil && len(entries) > 0 {
		t.Fatalf("a crossed slot wrote %d files", len(entries))
	}
}

// TestSavable: the two ways a core can have no slots are a core that declares no
// state and one whose first capture came back empty.
func TestSavable(t *testing.T) {
	var fresh rewind
	if !savable(true, &fresh) {
		t.Fatalf("a core that declares state and has not been asked yet is not savable")
	}
	if savable(false, &fresh) {
		t.Fatalf("a core that declares no state is savable")
	}
	var broken rewind
	broken.mu.Lock()
	broken.planned, broken.step = true, 0
	broken.mu.Unlock()
	if savable(true, &broken) {
		t.Fatalf("a core whose first capture came back empty is savable")
	}
	var good rewind
	good.mu.Lock()
	good.planned, good.step = true, 5
	good.mu.Unlock()
	if !savable(true, &good) {
		t.Fatalf("a core whose capture worked is not savable")
	}
}

// TestFileSafeAndShrink: a folder name survives everything Windows reserves, and a
// thumbnail is a few blocks of averages rather than a megabyte of screenshot.
func TestFileSafeAndShrink(t *testing.T) {
	if got := fileSafe(`FF7?/disc:1*"x"`); got != "FF7__disc_1__x_" {
		t.Fatalf("fileSafe gave %q", got)
	}
	if got := fileSafe("name. "); got != "name" {
		t.Fatalf("fileSafe kept a trailing dot and space: %q", got)
	}

	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
			img.Set(x+4, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	small := shrink(img, 4)
	if b := small.Bounds(); b.Dx() != 4 || b.Dy() != 2 {
		t.Fatalf("an 8x4 picture shrunk to 4 wide came out %dx%d, want 4x2", b.Dx(), b.Dy())
	}
	if r, _, _, _ := small.At(0, 0).RGBA(); r != 0 {
		t.Fatalf("the left block averaged to %d, want the black it was", r>>8)
	}
	if r, _, _, _ := small.At(3, 0).RGBA(); r != 0xffff {
		t.Fatalf("the right block averaged to %d, want the white it was", r>>8)
	}
}
