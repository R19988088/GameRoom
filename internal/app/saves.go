package app

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// A slot is one save of the game on screen: the core's whole state and the
// picture it was taken on, filed under 数据/存档/<game>/ as save-<n>.state with
// the picture and a small line of when beside it. The panel shows nine of them.
const (
	saveSlots  = 9
	saveThumbW = 320
)

// saveMeta is the line under a slot's picture: when it was taken and on which
// frame of the game, so two saves of the same moment can be told apart.
type saveMeta struct {
	At     string `json:"at"`
	Frames int    `json:"frames"`
}

// saveSlot is one save on the shelf, with its picture decoded.
type saveSlot struct {
	N      int
	At     time.Time
	Frames int
	Shot   *ui.Bitmap
}

// savable says whether a core that answers canState has slots to fill: one that
// declares no state has none, and neither has one whose first capture came back
// empty — the row remembers that as a plan with no step.
func savable(canState bool, r *rewind) bool {
	if !canState {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return !(r.planned && r.step <= 0)
}

// canSave is savable for the game on screen. With no game there is nothing to
// judge, and the click on an empty tile says so instead.
func (a *App) canSave() bool {
	if a.core == nil {
		return true
	}
	return savable(a.core.CanState(), &a.rew)
}

// savesDir is where the game on screen keeps its slots. Each game has its own
// folder, so a shelf full of games does not collide.
func (a *App) savesDir() string {
	if a.root == "" {
		return ""
	}
	return filepath.Join(a.root, "数据", "存档", a.saveKey())
}

// saveKey names this game's folder: the platform and the rom's own name, with
// what a file name cannot hold replaced. The rom name is the key the library
// itself uses, so a save survives a re-import that renumbers the games.
func (a *App) saveKey() string {
	g := a.playerGame
	name := strings.TrimSuffix(filepath.Base(g.RomPath), filepath.Ext(g.RomPath))
	key := fileSafe(g.PlatformKey) + "-" + fileSafe(name)
	if key == "-" || key == "" {
		key = "game"
	}
	return key
}

// fileSafe is a name a file can have: everything Windows reserves becomes an
// underscore, and a trailing dot or space goes away.
func fileSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			if r < 0x20 {
				b.WriteRune('_')
				continue
			}
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), " .")
}

// slotPaths are the three files of a slot: the state, its picture and its line.
func (a *App) slotPaths(n int) (state, shot, meta string) {
	dir := a.savesDir()
	base := fmt.Sprintf("save-%d", n)
	return filepath.Join(dir, base+".state"),
		filepath.Join(dir, base+".png"),
		filepath.Join(dir, base+".json")
}

// scanSaves reads the folder into the shelf the panel draws: which slots have a
// state, when it was taken, and its picture. It runs whenever the panel opens,
// so a save dropped in by hand shows up without a restart.
func (a *App) scanSaves() {
	a.slots = a.slots[:0]
	dir := a.savesDir()
	if dir == "" {
		return
	}
	for n := 1; n <= saveSlots; n++ {
		state, _, meta := a.slotPaths(n)
		if _, err := os.Stat(state); err != nil {
			continue
		}
		s := &saveSlot{N: n, Shot: a.slotShot(n)}
		if b, err := os.ReadFile(meta); err == nil {
			var m saveMeta
			if json.Unmarshal(b, &m) == nil {
				s.Frames = m.Frames
				if t, err := time.Parse(time.RFC3339, m.At); err == nil {
					s.At = t
				}
			}
		}
		if s.At.IsZero() {
			// A state without its line still has a time: the file's own.
			if info, err := os.Stat(state); err == nil {
				s.At = info.ModTime()
			}
		}
		a.slots = append(a.slots, s)
	}
}

// slotAt is the slot with that number, nil when the tile is empty.
func (a *App) slotAt(n int) *saveSlot {
	for _, s := range a.slots {
		if s.N == n {
			return s
		}
	}
	return nil
}

// slotShot decodes a slot's picture once and keeps it: nine small PNGs read
// every time a tile is drawn would be nine decodes a frame.
func (a *App) slotShot(n int) *ui.Bitmap {
	_, path, _ := a.slotPaths(n)
	if bmp, ok := a.shots[path]; ok {
		return bmp
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil
	}
	if a.shots == nil {
		a.shots = map[string]*ui.Bitmap{}
	}
	bmp := ui.NewBitmap(img)
	a.shots[path] = bmp
	return bmp
}

// saveInto writes the game as it stands into slot n: its state, the picture on
// the glass and the line that says when.
func (a *App) saveInto(n int) {
	a.slotNote = ""
	if a.core == nil {
		a.slotNote = "没有在跑的游戏"
		return
	}
	if !a.core.CanState() {
		a.slotNote = "这个核心不能存档"
		return
	}
	data, err := a.core.SaveState()
	if err != nil {
		a.slotNote = err.Error()
		return
	}
	dir := a.savesDir()
	if dir == "" {
		a.slotNote = "没有数据目录"
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.slotNote = err.Error()
		return
	}
	state, shot, meta := a.slotPaths(n)
	if err := writeStateFile(state, data); err != nil {
		a.slotNote = err.Error()
		return
	}
	if a.gameImg != nil {
		if err := writeShot(shot, a.gameImg, saveThumbW); err != nil {
			a.slotNote = err.Error()
		}
	}
	line, err := json.Marshal(saveMeta{At: time.Now().Format(time.RFC3339), Frames: a.core.Frames()})
	if err == nil {
		os.WriteFile(meta, append(line, '\n'), 0o644)
	}
	a.slotSel = n
	a.scanSaves()
}

// loadFrom puts slot n's state back and lets one frame through, so the glass
// shows the moment that was saved rather than the one left on it.
func (a *App) loadFrom(n int) {
	a.slotNote = ""
	if a.core == nil {
		a.slotNote = "没有在跑的游戏"
		return
	}
	state, _, _ := a.slotPaths(n)
	data, err := readStateFile(state)
	if err != nil {
		a.slotNote = err.Error()
		return
	}
	if err := a.core.LoadState(data); err != nil {
		a.slotNote = err.Error()
		return
	}
	a.core.Step()
	// Reading a slot is the game's own past, not the row's: the row starts again
	// from here instead of holding states of a game that no longer happened.
	a.rew.reset()
}

// deleteSlot takes a slot off the shelf: its state, its picture and its line.
func (a *App) deleteSlot(n int) {
	state, shot, meta := a.slotPaths(n)
	os.Remove(state)
	os.Remove(shot)
	os.Remove(meta)
	if bmp, ok := a.shots[shot]; ok {
		bmp.ReleasePixels()
		delete(a.shots, shot)
	}
	a.slotSel = 0
	a.slotNote = ""
	a.scanSaves()
}

// A slot's state file is the core's block, deflated. A PlayStation state is 3.6MB
// of mostly zeroes and repeated memory that weighs 380KB once deflated, and a launcher should
// not spend 16MB of disk on a moment of one game. Old files written before this are
// still read: without the marker they are the block itself.
const stateMark = "GRZ1"

// writeStateFile files a state deflated, behind a marker that says so.
func writeStateFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(stateMark); err != nil {
		return err
	}
	// BestSpeed: the block is written while the panel has the game paused, so a
	// tenth of a second is fine, and 10x smaller is the point.
	w, err := flate.NewWriter(f, flate.BestSpeed)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return w.Close()
}

// readStateFile answers the block a slot holds, deflated or not.
func readStateFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 4 || string(b[:4]) != stateMark {
		return b, nil
	}
	r := flate.NewReader(bytes.NewReader(b[4:]))
	defer r.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// writeShot keeps a picture of the glass at the width a tile can use, as a PNG.
func writeShot(path string, img *image.RGBA, maxW int) error {
	small := shrink(img, maxW)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, small)
}

// shrink makes a picture at most maxW wide by averaging blocks of pixels: a
// thumbnail has no business being a megabyte of screenshot.
func shrink(img *image.RGBA, maxW int) *image.RGBA {
	b := img.Bounds()
	factor := 1
	for b.Dx()/factor > maxW {
		factor++
	}
	if factor == 1 {
		return img
	}
	w, h := b.Dx()/factor, b.Dy()/factor
	if w < 1 || h < 1 {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl, al int
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					off := img.PixOffset(b.Min.X+x*factor+dx, b.Min.Y+y*factor+dy)
					r += int(img.Pix[off])
					g += int(img.Pix[off+1])
					bl += int(img.Pix[off+2])
					al += int(img.Pix[off+3])
				}
			}
			n := factor * factor
			off := out.PixOffset(x, y)
			out.Pix[off] = uint8(r / n)
			out.Pix[off+1] = uint8(g / n)
			out.Pix[off+2] = uint8(bl / n)
			out.Pix[off+3] = uint8(al / n)
		}
	}
	return out
}
