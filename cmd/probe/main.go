// Command probe loads one core on one rom the way the app does, steps it for a
// few seconds and reports what came out. It exists to answer "does this core
// really draw this game", which the app itself can only answer by eye.
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/R19988088/GameRoom/internal/core"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: probe <retroarch dir> <core name> <rom path> [seconds]")
		return
	}
	raDir, coreName, rom := os.Args[1], os.Args[2], os.Args[3]
	secs := 5.0
	if len(os.Args) > 4 {
		if v, err := strconv.ParseFloat(os.Args[4], 64); err == nil {
			secs = v
		}
	}
	corePath := filepath.Join(raDir, "cores", coreName+"_libretro.dll")
	if _, err := os.Stat(corePath); err != nil {
		fmt.Println("RESULT NO_CORE", corePath)
		return
	}
	if _, err := os.Stat(rom); err != nil {
		fmt.Println("RESULT NO_ROM", rom)
		return
	}
	s, err := core.Start(corePath, rom, filepath.Join(raDir, "system"))
	if err != nil {
		fmt.Println("RESULT START_FAIL", err)
		return
	}
	go s.Run()
	time.Sleep(time.Duration(secs * float64(time.Second)))

	f := s.Frame()
	w, h, colors, mean := 0, 0, 0, 0.0
	if f != nil {
		w, h = f.Bounds().Dx(), f.Bounds().Dy()
		colors, mean = stats(f)
		if out := os.Getenv("PROBE_PNG"); out != "" {
			if err := savePNG(out, f); err == nil {
				fmt.Println("wrote", out)
			}
		}
	}
	status := "OK"
	if s.Frames() == 0 {
		status = "NO_FRAME"
	} else if colors <= 2 {
		status = "BLACK"
	}
	fmt.Printf("RESULT %s frames=%d fps=%.3f size=%dx%d colors=%d mean=%.1f audio=%d fmt=%d\n",
		status, s.Frames(), s.FPS(), w, h, colors, mean, s.Audio().Len(), s.PixelFormat())
	s.Close()
}

// savePNG writes the frame as PNG so it can be looked at.
func savePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// stats counts distinct colours on a coarse grid: enough to tell a picture from
// a black screen without walking four million pixels.
func stats(img *image.RGBA) (int, float64) {
	seen := map[uint32]struct{}{}
	var sum float64
	var n int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			off := img.PixOffset(x, y)
			c := img.Pix[off : off+4 : off+4]
			seen[uint32(c[0])<<16|uint32(c[1])<<8|uint32(c[2])] = struct{}{}
			sum += float64(int(c[0]) + int(c[1]) + int(c[2]))
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return len(seen), sum / float64(n*3)
}
