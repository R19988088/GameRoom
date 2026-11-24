package app

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A decoder finishes on its own goroutine. When the picture is handed back it
// writes the cache map, and the frame being built reads it — two goroutines on
// one Go map is a fatal error, and a fatal error closes the window without
// leaving anything in the event log. The queue must therefore take its lock
// around the maps, the queue and the next pump.
func TestCoverQueueWaitsItsTurn(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 30; i++ {
		p := filepath.Join(dir, fmt.Sprintf("cover%d.png", i))
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				img.Set(x, y, color.RGBA{uint8(10 * i), 80, 160, 255})
			}
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
		paths = append(paths, p)
	}

	a := &App{
		covers:  map[string]*ui.Bitmap{},
		icons:   map[string]*ui.Bitmap{},
		pending: map[string]bool{},
	}
	// Ask the same way the grid does: every card wants its cover at once, and
	// keeps asking frame after frame while the queue chews through them.
	for round := 0; round < 200; round++ {
		for _, p := range paths {
			a.image(p, 320, false)
		}
		timeout := 0
		for {
			a.imgMu.Lock()
			busy := a.loading
			a.imgMu.Unlock()
			if !busy || timeout > 2000 {
				break
			}
			timeout++
		}
	}
	for _, p := range paths {
		a.imgMu.Lock()
		b := a.covers[p]
		a.imgMu.Unlock()
		if b == nil {
			t.Fatalf("%s never decoded", filepath.Base(p))
		}
	}
}
