package app

import (
	"bufio"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// The look of the picture is 脑浆油条's PhosphorLine preset, the CRT chain the
// library has always run with (shaders/202603.slangp): its numbers are read
// from that preset, not made up here. The preset is a GPU shader chain, so what
// this app draws is the same idea worked on the pixels it already owns — the
// mask between lines, the softness, the curve, the glass — with the preset's
// own strengths.

type crtParams struct {
	mask       float32 // MASK_INTENSITY: how dark the mask is between lines
	soften     float32 // SOFTEN: how much a pixel takes its neighbour
	curve      float32 // CURVE_X/Y: how much the tube bends at the sides
	corner     float32 // CORNER: how dark the corners are
	reflection float32 // REFLECTION: the light the glass throws back
	linear     float32 // LINEAR: how much scanlines merge
	sat        float32 // SAT: how much colour is kept
}

// defaultCRT is the preset as it ships, used when the file cannot be read.
var defaultCRT = crtParams{
	mask: 0.9, soften: 0.15, curve: 0.07, corner: 0.2,
	reflection: 0.25, linear: 0.2, sat: 0.99,
}

// loadCRT reads the preset named in the settings, which sits beside the
// emulator's other shaders. A missing or broken preset leaves the shipped
// numbers in place.
func loadCRT(root, name string) crtParams {
	p := defaultCRT
	if name == "" {
		return p
	}
	f, err := os.Open(filepath.Join(root, "模拟器", "RetroArch", "shaders", filepath.FromSlash(name)))
	if err != nil {
		return p
	}
	defer f.Close()

	values := map[string]float32{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"`)
		if n, err := strconv.ParseFloat(v, 32); err == nil {
			values[k] = float32(n)
		}
	}
	set := func(dst *float32, key string) {
		if n, ok := values[key]; ok {
			*dst = n
		}
	}
	set(&p.mask, "MASK_INTENSITY")
	set(&p.soften, "SOFTEN")
	set(&p.curve, "CURVE_X")
	set(&p.corner, "CORNER")
	set(&p.reflection, "REFLECTION")
	set(&p.linear, "LINEAR")
	set(&p.sat, "SAT")
	if _, ok := values["CURVE_Y"]; ok {
		p.curve = (p.curve + values["CURVE_Y"]) / 2
	}
	return p
}

// aLook is the preset bent to the strength chosen in the player: the numbers
// are the preset's, and each choice scales the parts of it that make the tube
// read stronger or softer. Off draws none of it.
type aLook struct {
	scan     float32 // how much darker every other line is
	soft     float32 // how much a pixel takes on its neighbour
	vignette float32 // how much darker the sides and corners are
	glare    float32 // how much light the glass throws back on the top edge
}

func (p crtParams) forLevel(level string) aLook {
	switch level {
	case "off":
		return aLook{}
	case "soft":
		return aLook{
			scan:     p.mask * 0.28, // half the mask, the lines read as haze
			soft:     p.soften * 2.4,
			vignette: p.corner * p.curve * 4,
			glare:    p.reflection * 0.6,
		}
	case "strong":
		return aLook{
			scan:     p.mask * 0.55,
			soft:     p.soften * 0.8,
			vignette: p.corner * p.curve * 9,
			glare:    p.reflection * 1.2,
		}
	default: // 标准, the preset's own strengths
		return aLook{
			scan:     p.mask * 0.32,
			soft:     p.soften,
			vignette: p.corner * p.curve * 6,
			glare:    p.reflection,
		}
	}
}

// apply paints the look onto the picture, in place.
func (l aLook) apply(img *image.RGBA) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return
	}
	pix, stride := img.Pix, img.Stride

	if l.soft > 0 {
		// One pass along the rows, each pixel leaning towards the one before it:
		// the softness a tube's spot has, which takes the hard edge off the
		// pixels a game was drawn with.
		for y := 0; y < h; y++ {
			row := pix[y*stride : y*stride+w*4]
			for x := 4; x < w*4; x += 4 {
				for c := 0; c < 3; c++ {
					v := float32(row[x+c])
					p := float32(row[x-4+c])
					row[x+c] = byte(v*(1-l.soft) + p*l.soft + 0.5)
				}
			}
		}
	}

	if l.scan > 0 {
		// Every other row is darker, which is what the shadow mask leaves
		// behind between the lines the game drew.
		k := 1 - l.scan
		if k < 0.35 {
			k = 0.35
		}
		for y := 1; y < h; y += 2 {
			row := pix[y*stride : y*stride+w*4]
			for x := 0; x < w*4; x += 4 {
				row[x+0] = byte(float32(row[x+0]) * k)
				row[x+1] = byte(float32(row[x+1]) * k)
				row[x+2] = byte(float32(row[x+2]) * k)
			}
		}
	}

	if l.vignette > 0 {
		// The corners of the tube lose their light: the falloff the preset
		// curves its glass with, from the middle outwards.
		if l.vignette > 0.45 {
			l.vignette = 0.45
		}
		cx, cy := float32(w)/2, float32(h)/2
		for y := 0; y < h; y++ {
			row := pix[y*stride : y*stride+w*4]
			dy := (float32(y) - cy) / cy
			for x := 0; x < w; x++ {
				dx := (float32(x) - cx) / cx
				d := dx*dx + dy*dy
				if d <= 0.55 {
					continue
				}
				f := (d - 0.55) / 1.45
				if f > 1 {
					f = 1
				}
				k := 1 - l.vignette*f*f
				row[x*4+0] = byte(float32(row[x*4+0]) * k)
				row[x*4+1] = byte(float32(row[x*4+1]) * k)
				row[x*4+2] = byte(float32(row[x*4+2]) * k)
			}
		}
	}

	if l.glare > 0 {
		// The glass over the tube throws a little of its light back along the
		// top rows, which is what makes the picture read as a screen, not a
		// texture.
		k := 1 + l.glare
		rows := h / 12
		for y := 0; y < rows; y++ {
			f := 1 - float32(y)/float32(rows)
			row := pix[y*stride : y*stride+w*4]
			for x := 0; x < w*4; x += 4 {
				for c := 0; c < 3; c++ {
					v := float32(row[x+c]) * (1 + (k-1)*f*f)
					if v > 255 {
						v = 255
					}
					row[x+c] = byte(v)
				}
			}
		}
	}
}

// --- the picker, in the player ---

// filterButton is the pill at the right: what look the game is running with,
// and the way into the picker.
func (a *App) filterButton(c *ui.Context, t *ui.Theme) {
	b := ui.ButtonBase(c).Width(filterButtonWidth).Height(backButtonSize).PaddingX(14).
		Radius(backButtonSize/2).AlignItems(ui.Center).Gap(8).
		Material(glass.Glass{Clarity: 0.5, Tint: ui.Hex("#0d0f168f")}).
		Border(1, ui.Hex("#ffffff1f"))
	if a.filtersOpen {
		b.Background(ui.Hex("#e8763a2e")).Border(1, ui.Hex("#e8763a66"))
	} else if b.Hovered() {
		b.Background(ui.Hex("#ffffff1f")).Border(1, ui.Hex("#ffffff4d"))
	}
	b.Children(func() {
		ui.Text(c, "滤镜").FontSize(12).FontWeight(600).TextColor(t.Text.Alpha(0.9)).SingleLine()
		ui.Text(c, filterName(a.cfg.FilterLevel)).FontSize(11).TextColor(t.TextMuted).SingleLine()
	})
	if b.Clicked() {
		a.win.Update(a.toggleFilters)
	}
}

// toggleFilters opens or closes the picker, taking the game aside for it: the
// picker is drawn by the page, which the game's window covers.
func (a *App) toggleFilters() {
	a.filtersOpen = !a.filtersOpen
	if a.player != nil {
		if a.filtersOpen {
			a.player.Hide()
		} else {
			a.player.Show()
		}
	}
}

// filterName is what the bar calls a look.
func filterName(level string) string {
	switch level {
	case "off":
		return "关闭"
	case "soft":
		return "柔和"
	case "strong":
		return "强烈"
	}
	return "标准"
}

// filterOptions are the looks the picker offers, in the order it shows them.
var filterOptions = []struct {
	Level string
	Name  string
	Note  string
}{
	{"", "标准", "脑浆油条原参数"},
	{"soft", "柔和", "扫描线更淡、弧面更浅"},
	{"strong", "强烈", "扫描线更重、弧面更弯"},
	{"off", "关闭", "不做任何处理"},
}

// filterPanel is the picker itself: a plate of glass over the play area with
// one row per look. Choosing one closes the picker and restarts the game with
// the new preset, because RetroArch reads its shader only when it loads.
func (a *App) filterPanel(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
		Background(ui.Hex("#05060ac9")).Center().Children(func() {
		ui.Column(c).Width(420).Padding(10).Radius(18).Gap(4).
			Background(ui.Hex("#17181ffa")).
			Border(1, ui.Hex("#ffffff1f")).
			Shadow(0, 26, 60, 0, ui.Hex("#000000cc")).
			Children(func() {
				ui.Text(c, "试试别的滤镜").FontSize(11).FontWeight(700).
					TextColor(t.TextMuted).PaddingX(8).PaddingY(4)
				for _, opt := range filterOptions {
					opt := opt
					row := ui.ButtonBase(c).Height(48).PaddingX(10).Radius(12).
						AlignItems(ui.Center).Gap(12)
					if opt.Level == a.cfg.FilterLevel ||
						(opt.Level == "" && (a.cfg.FilterLevel == "" || a.cfg.FilterLevel == "normal")) {
						row.Background(ui.Hex("#e8763a1f"))
					} else if row.Hovered() {
						row.Background(ui.Hex("#ffffff12"))
					}
					row.Children(func() {
						ui.Column(c).Gap(3).Grow(1).Children(func() {
							ui.Text(c, opt.Name).FontSize(13).FontWeight(600).
								TextColor(t.Text.Alpha(0.92)).SingleLine()
							ui.Text(c, opt.Note).FontSize(10.5).
								TextColor(t.TextMuted.Alpha(0.75)).SingleLine()
						})
						ui.Text(c, "↵").FontSize(11).TextColor(t.TextMuted.Alpha(0.4))
					})
					if row.Clicked() {
						a.setFilter(opt.Level)
					}
				}
			})
	})
}
