package app

import (
	"strings"
	"testing"
)

// TestScanlineIsLockedToThePicture keeps the four steps that took the moiré out
// of the scanlines (2026-11-23) from being dropped as cosmetic edits:
//
//  1. dp is in the picture's pixels (suv*pw, suv*ph), not the window's
//     (suv*size*ps). Reading the pitch off the window draws a picture that is
//     half the window twice as fine as it is, so every game line reads a
//     different part of the pattern and the mask turns into blocks.
//  2. the line scale is a whole number of screen pixels per game pixel, 1 to 8,
//     never a fraction and never only the even steps (2026-11-23: the even-only
//     rule left a 27" 4K display with just 4x, a patch in the middle, and 8x,
//     nearly the whole screen, and nothing between them).
//  3. the pitch is snapped to a divisor of that line (or a whole multiple), so
//     the scanline and the line never drift against each other.
//  4. the phase is taken a hair inside the line, so a row exactly on an edge
//     does not read as a different colour of the mask.
func TestScanlineIsLockedToThePicture(t *testing.T) {
	for _, want := range []struct{ src, why string }{
		{"dp := vec2(suv.x*pw, suv.y*ph)",
			"the scanline phase must be measured in the picture's own pixels"},
		{"if k > 8.0 {",
			"the picture's scale must be a whole 1..8 screen pixels per game pixel"},
		{"pitch = sc\n",
			"the pitch must start on the whole line, which divides even when k is odd"},
		{"if mod(sc, d) < 0.001",
			"the scanline pitch must be a divisor of the line it sits on"},
		{"mod(dp.y+0.0001, pitch)",
			"the phase must be nudged off the line edge"},
		{"mcell := sc * isz.y * ar / isz.x * 0.3333",
			"the mask cell must be a game column, not half a row"},
	} {
		if !strings.Contains(crtShaderSrc, want.src) {
			t.Errorf("%s: %q is gone from the shader", want.why, want.src)
		}
	}
}
