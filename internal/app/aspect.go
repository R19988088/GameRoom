package app

// tubeGlass marks the platforms whose picture was made for a 4:3 tube: their
// pixels are not square, so the shape of the frame is not the shape of the
// picture. A CPS 384×224 board belongs on the same glass as a 320×240 one,
// even though its frame is 1.71 across.
//
// The cores do not help here: fbneo, fbalpha, opera, np2kai and the rest report
// no display aspect at all (av_info.geometry.aspect 0), so a frontend that
// falls back to the frame's own ratio draws every one of these wide. The
// platforms left out — 掌机, 任天堂 — are the ones with square pixels (GBA 240×160,
// GB 160×144, PSP 480×272, 3DS 400×240), where the frame ratio is already right.
var tubeGlass = map[string]bool{
	"arc":     true, // 街机：CPS1/2/3、NeoGeo、System16、MAME 各板
	"majiang": true, // 麻将：同样是街机板
	"fc":      true, // 256×240 的 8:7 像素
	"sfc":     true, // 256×224
	"SEGA":    true, // MD/SS 320×224，DC 640×480
	"3DO":     true,
	"ps":      true,
	"dos":     true, // 320×200 的 8:10 像素
	"pc98":    true, // 640×400
}

// pictureRatio is the shape the running game's picture is drawn at: the
// platform's tube when it has one, else the core's own display aspect, else the
// frame's pixel shape. Everything that places the picture reads this one
// number — the letterboxed fallback, the CRT glass, and the pointer mapping —
// so the three cannot drift apart.
func (a *App) pictureRatio() float32 {
	if r := a.glassRatio(); r > 0 {
		return r
	}
	if a.gameAspect > 0.4 && a.gameAspect < 5 {
		return a.gameAspect
	}
	if a.frameW > 0 && a.frameH > 0 {
		return float32(a.frameW) / float32(a.frameH)
	}
	return 0
}

// glassRatio is the shape of the tube the platform's games were made for, 0 when
// the frame's own shape is the answer.
//
// A frame taller than it is wide is a board mounted sideways — a vertical
// shooter — and its glass is 3:4, the same tube turned on its side. A frame
// past three to one is a triple screen or another odd board whose own shape is
// the only honest one, so it is left alone.
func (a *App) glassRatio() float32 {
	if !tubeGlass[a.playerGame.PlatformKey] {
		return 0
	}
	fw, fh := float64(a.frameW), float64(a.frameH)
	if fw <= 0 || fh <= 0 {
		return 0
	}
	if px := fw / fh; px > 3 || px < 1.0/3.0 {
		return 0
	}
	if fh > fw {
		return 3.0 / 4.0
	}
	return 4.0 / 3.0
}
