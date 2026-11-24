//go:build windows

package core

import (
	"image"
	"image/color"
	"testing"
)

// quad3x2 is a 3×2 picture whose colour says where each pixel came from:
// red is the column, green the row.
func quad3x2() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), A: 255})
		}
	}
	return img
}

// TestTurnRGBA180: a half turn keeps the shape, and puts the last pixel where
// the first one was — a turn, not a mirror.
func TestTurnRGBA180(t *testing.T) {
	img := turnRGBA(quad3x2(), 180)
	if b := img.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Fatalf("size = %dx%d, want 3x2", b.Dx(), b.Dy())
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			want := color.RGBA{R: uint8((2 - x) * 10), G: uint8((1 - y) * 10), A: 255}
			if got != want {
				t.Errorf("at %d,%d = %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestTurnRGBA90: a quarter turn clockwise swaps the sides: what was the top
// row becomes the right column.
func TestTurnRGBA90(t *testing.T) {
	img := turnRGBA(quad3x2(), 90)
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 3 {
		t.Fatalf("size = %dx%d, want 2x3", b.Dx(), b.Dy())
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			// new(x,y) = old(y, h-1-x): h is the old height, 2.
			want := color.RGBA{R: uint8(y * 10), G: uint8((1 - x) * 10), A: 255}
			if got != want {
				t.Errorf("at %d,%d = %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestTurnDegrees reads the count the way the cores actually send it: MAME
// logs "Screen rotation: 180deg" and hands over 2 for 电子基盘, a Psikyo
// vertical shooter hands over 1 for a frame that is upright only when turned
// anti-clockwise.
func TestTurnDegrees(t *testing.T) {
	cases := []struct {
		quarters int
		deg      int
	}{
		{0, 0},   // no spin asked for
		{1, 270}, // vertical board, monitor on its side
		{2, 180}, // mahjong table, tube mounted upside down
		{3, 90},
		{7, 0}, // nonsense is ignored rather than turned oddly
	}
	for _, c := range cases {
		if got := turnDegrees(c.quarters); got != c.deg {
			t.Errorf("turnDegrees(%d) = %d, want %d", c.quarters, got, c.deg)
		}
	}
}

func TestTurnRGBA270(t *testing.T) {
	img := turnRGBA(quad3x2(), 270)
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 3 {
		t.Fatalf("size = %dx%d, want 2x3", b.Dx(), b.Dy())
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			// new(x,y) = old(w-1-y, x): w is the old width, 3.
			want := color.RGBA{R: uint8((2 - y) * 10), G: uint8(x * 10), A: 255}
			if got != want {
				t.Errorf("at %d,%d = %v, want %v", x, y, got, want)
			}
		}
	}
}
