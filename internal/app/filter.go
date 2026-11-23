package app

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// The picture is 脑浆油条's PhosphorLine chain (shaders/202603.slangp), run as
// one GPU shader over the framebuffer the core drew: CA-sharpen's edge and
// levels, PhosphorLine's beam blending, scanline profile and curve, diffusion's
// glow, PP-reflex's aperture mask and border reflection. Every parameter the
// preset exposes is a uniform of that shader, so all of them move from the
// filter panel while the game keeps running.

// crtParams are every number the preset sets, by the name it sets it under.
type crtParams map[string]float32

func (p crtParams) get(name string, def float32) float32 {
	if v, ok := p[name]; ok {
		return v
	}
	return def
}

// defaultCRT is the preset as it ships, used when the file cannot be read.
func defaultCRT() crtParams {
	return crtParams{
		"SAT": 0.99, "SOFTEN": 0.15, "SHARPEN": 0.05, "NTSCintensity": 0.65,
		"CORNER": 0.2, "PPOY": 0, "BLK": 0, "EXPOSURE": 1, "CT": 0,
		"LINEAR": 0.2, "CURVE_X": 0.07, "CURVE_Y": 0.07,
		"MASK_INTENSITY": 0.9, "MASK_TYPE": 0, "ss": 0.5,
		"DIFFUSION": 0.05, "DIFF_OVEREXP": 0, "REFLECTION": 0.25, "REFLECTION_SWITCH": 1,
		"E_OFFSET": 1.8, "REFLECT_GAP": 0.02,
		"LINE_PITCH_COEF": 1, "LINE_THICKNESS": 0.95, "LINE_OPACITY": 0.95,
		"PIC_SIZE": 0.5, "INT_TOL": 0.05, "INT_SCALE": 0, "GAMMA": 2.2, "PAD_Y": 0,
	}
}

// loadCRT reads the preset named in the settings, which sits beside the
// emulator's other shaders. A missing or broken preset leaves the shipped
// numbers in place.
func loadCRT(root, name string) crtParams {
	p := defaultCRT()
	if name == "" {
		return p
	}
	f, err := os.Open(filepath.Join(root, "模拟器", "RetroArch", "shaders", filepath.FromSlash(name)))
	if err != nil {
		return p
	}
	defer f.Close()

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
			p[k] = float32(n)
		}
	}
	return p
}

// bent is the preset with the strength the player picked: only the numbers that
// make the tube read harder or softer are scaled.
func (p crtParams) bent(level string) crtParams {
	q := crtParams{}
	for k, v := range p {
		q[k] = v
	}
	switch level {
	case "soft":
		q["MASK_INTENSITY"] = p.get("MASK_INTENSITY", 0.9) * 0.55
		q["LINE_OPACITY"] = p.get("LINE_OPACITY", 0.95) * 0.6
		q["CURVE_X"] = p.get("CURVE_X", 0.07) * 0.5
		q["CURVE_Y"] = p.get("CURVE_Y", 0.07) * 0.5
		q["LINE_THICKNESS"] = p.get("LINE_THICKNESS", 0.95) * 0.85
	case "strong":
		q["MASK_INTENSITY"] = min32(p.get("MASK_INTENSITY", 0.9)*1.05, 0.95)
		q["LINE_OPACITY"] = 0.95
		q["CURVE_X"] = p.get("CURVE_X", 0.07) * 1.6
		q["CURVE_Y"] = p.get("CURVE_Y", 0.07) * 1.6
		q["LINE_THICKNESS"] = min32(p.get("LINE_THICKNESS", 0.95)*1.1, 2)
	}
	return q
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// knob is one parameter of the panel: the name in the preset, what its author
// calls it, the range it moves in, and how the control reads it. kind 0 is the
// bar, kind 1 a switch; mul scales what the control shows against the number the
// shader runs with, so PIC_SIZE reads as a percentage.
type knob struct {
	key   string
	label string
	lo    float64
	hi    float64
	kind  int
	mul   float64
	step  float64
}

var knobs = []knob{
	{"GAMMA", "模拟伽马", 1, 4, 0, 1, 0.1},
	{"CURVE_X", "CRT曲率 X", 0, 1, 0, 1, 0.1},
	{"CURVE_Y", "CRT曲率 Y", 0, 1, 0, 1, 0.1},
	{"PIC_SIZE", "图像尺寸 (%)", 0.5, 1, 0, 10, 0.1},
	{"MASK_INTENSITY", "阴影蒙版强度", 0, 0.95, 0, 1, 0.1},
	{"MASK_TYPE", "蒙版类型", 0, 1, 0, 1, 0.1},
	{"ss", "蒙版尺寸", 0, 10, 0, 1, 0.1},
	{"LINE_THICKNESS", "扫描线粗细", 0.1, 2, 0, 1, 0.1},
	{"LINE_OPACITY", "扫描线不透明度", 0, 0.95, 0, 1, 0.1},
	{"LINE_PITCH_COEF", "扫描线间距", 0.1, 10, 0, 1, 0.1},
	{"LINEAR", "水平延伸", 0, 1, 0, 1, 0.1},
	{"SOFTEN", "柔化", 0, 1, 0, 1, 0.1},
	{"SHARPEN", "S-Video 锐化", 0, 2, 0, 1, 0.1},
	{"NTSCintensity", "NTSC 色彩强度", 0, 2, 0, 1, 0.1},
	{"SAT", "饱和度", 0, 1.5, 0, 1, 0.1},
	{"BLK", "黑电平 (%)", -17.5, 7.5, 0, 1, 0.1},
	{"EXPOSURE", "曝光", 0.1, 2, 0, 1, 0.1},
	{"CT", "色温", -2.8, 2.8, 0, 1, 0.1},
	{"CORNER", "圆角", 0, 1, 0, 1, 0.1},
	{"INT_TOL", "允许过扫描", 0, 0.1, 0, 1, 0.1},
	{"INT_SCALE", "强制整数缩放", 0, 1, 1, 1, 0.1},
	{"DIFFUSION", "光晕扩散", 0, 1, 0, 1, 0.1},
	{"DIFF_OVEREXP", "允许扩散过曝", 0, 1, 0, 1, 0.1},
	{"REFLECTION", "边框反射强度", 0, 1, 0, 1, 0.1},
	{"E_OFFSET", "边框反射偏移", 0, 2, 0, 1, 0.1},
	{"REFLECTION_SWITCH", "边框反射开关", 0, 1, 1, 1, 0.1},
	{"REFLECT_GAP", "反射间距 (%)", 0, 0.24, 0, 100, 1},
	{"PPOY", "图像位置 Y", -50, 50, 0, 1, 0.1},
	{"PAD_Y", "上下间距 (%)", 0, 10, 0, 1, 0.5},
}

// value is the number the shader runs with: what the player set, or the preset's
// own (bent by the preset picked).
func (a *App) value(key string) float32 {
	v, ok := a.cfg.CRTTune[key]
	if !ok {
		return a.crt.bent(a.cfg.FilterLevel).get(key, 0)
	}
	for _, k := range knobs {
		if k.key != key {
			continue
		}
		if float64(v) < k.lo {
			v = float32(k.lo)
		}
		if float64(v) > k.hi {
			v = float32(k.hi)
		}
		break
	}
	return v
}

// crtEffect builds the shader's uniforms for this frame. The shader is compiled
// once: parameters are uniforms, so moving one costs a value, not a recompile.
func (a *App) crtEffect() (shader.Effect, bool) {
	if a.cfg.FilterLevel == "off" {
		return shader.Effect{}, false
	}
	if a.fxShader == nil {
		sh, err := shader.Compile(crtShaderSrc)
		if err != nil {
			a.status = "滤镜无法编译：" + err.Error()
			return shader.Effect{}, false
		}
		a.fxShader = sh
	}
	u := map[string]any{}
	for _, k := range knobs {
		u[uniformName(k.key)] = float64(a.value(k.key))
	}
	// The shape the picture is drawn at, which the glass fits the frame into: a
	// 384×224 CPS board is 4:3, not its pixels' own shape.
	aspect := a.pictureRatio()
	if aspect <= 0 && a.gameBmp != nil {
		if bw, bh := ui.ImageSize(a.gameBmp); bw > 0 && bh > 0 {
			aspect = bw / bh
		}
	}
	u["GAME_ASPECT"] = float64(aspect)
	return a.fxShader.Effect(shader.Options{
		Uniforms:        u,
		Images:          [4]*ui.Bitmap{a.gameBmp},
		MaxSampleOffset: ui.Point{X: 1, Y: 1},
	}), true
}

// uniformName is the preset's key as the name of a uniform: Kage's uniforms are
// exported Go variables.
func uniformName(key string) string {
	switch key {
	case "ss":
		return "SS"
	}
	return key
}

// crtShaderSrc is PhosphorLine's chain as one effect over the frame the core
// drew (image0), in the order its passes run: colour, beam, scanlines, glow,
// mask, glass.
const crtShaderSrc = `package main

var GAMMA float
var CURVE_X float
var CURVE_Y float
var MASK_INTENSITY float
var MASK_TYPE float
var SS float
var LINE_THICKNESS float
var LINE_OPACITY float
var LINE_PITCH_COEF float
var LINEAR float
var SOFTEN float
var SHARPEN float
var NTSCintensity float
var SAT float
var BLK float
var EXPOSURE float
var CT float
var CORNER float
var PPOY float
var PAD_Y float
var PIC_SIZE float
var INT_TOL float
var INT_SCALE float
var DIFFUSION float
var DIFF_OVEREXP float
var REFLECTION float
var E_OFFSET float
var REFLECT_GAP float
var REFLECTION_SWITCH float
var GAME_ASPECT float

// warp is the tube's glass, taken on coordinates centred on the picture, so the
// picture bends around its own middle and stays centred.
func warp(c vec2) vec2 {
	if CURVE_X < 0.005 && CURVE_Y < 0.005 {
		return c
	}
	kx := CURVE_X * 2.0
	ky := CURVE_Y
	if kx < 0.01 {
		kx = 0.01
	}
	if kx > 1.0 {
		kx = 1.0
	}
	if ky < 0.01 {
		ky = 0.01
	}
	if ky > 1.0 {
		ky = 1.0
	}
	tx := (c.x - 0.00001) * 2.0
	ty := (c.y - 0.00001) * 2.0
	sx := sin(kx)
	sy := sin(ky)
	lx := sx * tx * tx / sin(tx*kx)
	ly := sy * ty * ty / sin(ty*ky)
	cx2 := (1.0 / kx) * (1.0 - cos(lx*kx))
	cy2 := (1.0 / ky) * (1.0 - cos(ly*ky))
	fx := 1.0 - cy2*0.3
	fy := 1.0 - cx2*0.3
	lx = lx / fx
	ly = ly / fy
	return vec2(lx*0.5+0.00001, ly*0.5+0.00001)
}

// beam is PhosphorLine's d(): the colour between the core's texels, mixed as the
// pass mixes a pair of lines and a pair of columns.
func beam(np vec2, ex float) vec3 {
	fp := np - vec2(0.5, 0.5)
	gx := fp.x - floor(fp.x)
	gy := fp.y - floor(fp.y)
	gx = clamp((gx-0.5)/ex+0.5, 0.0, 1.0)
	if SOFTEN > 0.001 {
		gy = clamp((gy-0.5)/SOFTEN+0.5, 0.0, 1.0)
	} else if gy >= 0.5 {
		gy = 1.0
	} else {
		gy = 0.0
	}
	p00 := floor(fp) + vec2(0.5, 0.5)
	c00 := image0At(p00).xyz
	c10 := image0At(p00 + vec2(1.0, 0.0)).xyz
	c01 := image0At(p00 + vec2(0.0, 1.0)).xyz
	c11 := image0At(p00 + vec2(1.0, 1.0)).xyz
	c00 = c00 * c00
	c10 = c10 * c10
	c01 = c01 * c01
	c11 = c11 * c11
	cx0 := mix(c00, c10, vec3(gx))
	cx1 := mix(c01, c11, vec3(gx))
	return sqrt(mix(cx0, cx1, vec3(gy)))
}

// prof is PhosphorLine's o(): the tube's own light, a bezier of the colour the
// scanline carries.
func prof(t vec3, d float) vec3 {
	c := clamp(t, vec3(0.0), vec3(1.0))
	u := vec3(1.0) - c
	tt := c * c
	uu := u * u
	ttt := tt * c
	y := uu*vec3(3.0*(0.25+d))*c + u*tt*vec3(2.25) + ttt
	return clamp(y, vec3(0.0), vec3(1.0))
}

// deltaOf is the preset's profile weight for a scanline pitch.
func deltaOf(i int) float {
	if i <= 0 {
		return 0.21
	}
	if i == 1 {
		return 0.15
	}
	if i == 2 {
		return 0.13
	}
	if i == 3 {
		return 0.12
	}
	return 0.1
}

// arcHit is PP-reflex's bb(): where the ray from p toward v crosses the tube's
// corner arc, the arc centred on o with radius r, in the corner's own quadrant.
func arcHit(p vec2, v vec2, o vec2, r float) vec2 {
	dx := v.x - p.x
	dy := v.y - p.y
	ix := p.x - o.x
	iy := p.y - o.y
	a1 := dx*dx + dy*dy
	b1 := 2.0 * (ix*dx + iy*dy)
	c1 := ix*ix + iy*iy - r*r
	d1 := b1*b1 - 4.0*a1*c1
	if a1 < 0.000001 {
		return vec2(0.0, 0.0)
	}
	if d1 < 0.0 {
		return vec2(0.0, 0.0)
	}
	root := sqrt(d1)
	t1 := (-b1 + root) / (2.0 * a1)
	if t1 >= 0.0 {
		if t1 <= 1.0 {
			px1 := p.x + t1*dx
			py1 := p.y + t1*dy
			if px1 >= o.x && py1 >= o.y {
				return vec2(px1, py1)
			}
		}
	}
	t2 := (-b1 - root) / (2.0 * a1)
	if t2 >= 0.0 {
		if t2 <= 1.0 {
			px2 := p.x + t2*dx
			py2 := p.y + t2*dy
			if px2 >= o.x && py2 >= o.y {
				return vec2(px2, py2)
			}
		}
	}
	return vec2(0.0, 0.0)
}

// fold mirrors a coordinate into the picture again and again, so the glass never
// runs out of picture however far from the frame it is read.
func fold(raw float) float {
	t := raw + 1.0
	t = t - 2.0*floor(t*0.5)
	return abs(t - 1.0)
}

// lin is one bilinear tap of the picture in linear light, where a halo adds the
// way light does.
func lin(p vec2, lo vec2, hi vec2) vec3 {
	return pow(clamp(image0Sample(clamp(p, lo, hi)).xyz, vec3(0.0), vec3(1.0)), vec3(GAMMA))
}

// gk is a gaussian blur of the picture in linear light, five taps per axis on a
// grid of step pixels: the weights fall off from its centre, so a wide radius
// reads as a haze and never as copies of what is under it, which a handful of
// samples spread far apart would leave as ghosts.
func gk(p vec2, isz vec2, sigma float, step float) vec3 {
	lo := vec2(0.0, 0.0)
	hi := isz - vec2(1.0, 1.0)
	q := clamp(p, lo, hi)
	if sigma < 0.35 {
		sigma = 0.35
	}
	acc := vec3(0.0, 0.0, 0.0)
	wsum := 0.0
	for j := -2; j <= 2; j++ {
		for i := -2; i <= 2; i++ {
			d2 := float(i*i+j*j) * step * step
			w := exp(-d2/(2.0*sigma*sigma))
			acc = acc + lin(q+vec2(float(i), float(j))*step, lo, hi)*w
			wsum = wsum + w
		}
	}
	if wsum < 0.0001 {
		wsum = 1.0
	}
	return acc / wsum
}

// tent is the small kernel the glow keeps close to the pixel: a [1 2 1] read as
// nine bilinear taps, at one pixel and two, so the two together come out as a
// gaussian of a couple of pixels and every edge is softened, not doubled.
func tent(p vec2, s float, isz vec2) vec3 {
	lo := vec2(0.0, 0.0)
	hi := isz - vec2(1.0, 1.0)
	q := clamp(p, lo, hi)
	dx := vec2(s, 0.0)
	dy := vec2(0.0, s)
	e := lin(q+dx, lo, hi) + lin(q-dx, lo, hi) + lin(q+dy, lo, hi) + lin(q-dy, lo, hi)
	c := lin(q+dx+dy, lo, hi) + lin(q+dx-dy, lo, hi) + lin(q-dx+dy, lo, hi) + lin(q-dx-dy, lo, hi)
	return (lin(q, lo, hi)*4.0 + e*2.0 + c) / 16.0
}

// glow is diffusion-h and diffusion-v as one: the picture blurred in linear
// light, which PP-reflex adds to the scanlines as the tube's own glow.
func glow(np vec2, isz vec2) vec3 {
	return tent(np, 1.0, isz)*0.62 + tent(np, 2.0, isz)*0.38
}

// sampled is the picture at a point of its own pixels, with CA-sharpen's edge,
// chroma and levels run on it.
func sampled(np vec2, isz vec2) vec3 {
	n := clamp(np, vec2(0.0, 0.0), isz-vec2(1.0, 1.0))
	col := beam(n, LINEAR)
	if SHARPEN > 0.001 {
		l := image0At(n - vec2(1.0, 0.0)).xyz
		r := image0At(n + vec2(1.0, 0.0)).xyz
		col.x = col.x*(SHARPEN*2.0+1.0) - (l.x+r.x)*SHARPEN
	}
	if NTSCintensity > 0.01 {
		nm := image0At(n+vec2(1.0, 0.0)).xyz
		nm = nm + image0At(n-vec2(1.0, 0.0)).xyz
		nm = nm + image0At(n+vec2(0.0, 1.0)).xyz
		nm = nm + image0At(n-vec2(0.0, 1.0)).xyz
		nm = nm * 0.25
		col = mix(col, nm, vec3(NTSCintensity*0.25))
	}
	if abs(BLK) > 0.01 {
		col = (col - vec3(BLK*0.01)) * (100.0 / (100.0 - BLK))
	}
	if abs(EXPOSURE-1.0) > 0.001 {
		col = col * EXPOSURE
	}
	if abs(CT) > 0.01 {
		col = col * vec3(1.0+CT*0.05, 1.0, 1.0-CT*0.05)
	}
	if SAT > 0.001 {
		y := dot(col, vec3(0.299, 0.587, 0.114))
		col = vec3(y, y, y) + (col-vec3(y, y, y))*SAT
	}
	return clamp(col, vec3(0.0), vec3(1.0))
}

// mirrorBlur is the glass of the bezel: the picture read around a point of
// itself through a gaussian whose radius grows with the distance from the frame,
// so the mirror goes soft the further out it is read and never shows a second,
// crisp copy of the frame.
func mirrorBlur(p vec2, isz vec2, r float) vec3 {
	st := 1.0
	if r > 2.6 {
		st = 2.0
	}
	if r > 5.0 {
		st = 3.0
	}
	if r > 9.0 {
		st = 4.0
	}
	return gk(p, isz, r*0.85, st)
}

func LayerEffect(pos vec2) vec4 {
	size := rectSize()
	ps := pixelScale()
	uv := pos / size
	c := warp(uv - vec2(0.5, 0.5))

	isz := image0Size()
	// The picture keeps its own shape inside the window; PIC_SIZE trims it, and
	// PAD_Y holds it away from the glass's top and bottom. Everything outside the
	// picture is the glass, so the reflection reaches the window's own edges.
	ar := isz.x / isz.y
	if GAME_ASPECT > 0.4 {
		if GAME_ASPECT < 5.0 {
			ar = GAME_ASPECT
		}
	}
	// PAD_Y is the spacing above and below the picture, in percent of the box on
	// each side. It sets the box the picture has to fit, not its scale: with the
	// integer lock on, the picture takes as many whole lines as the smaller box
	// allows, so its shape and its scanlines stay the picture's own and only the
	// black around it grows.
	padY := 1.0
	if PAD_Y > 0.0 {
		padY = 1.0 - PAD_Y*0.02
		if padY < 0.6 {
			padY = 0.6
		}
	}
	ratio := size.x / size.y
	rw := 1.0
	rh := 1.0
	if ratio > ar {
		rw = ar / ratio
		rh = 1.0
	} else {
		rw = 1.0
		rh = ratio / ar
	}
	rw = rw * PIC_SIZE * padY
	rh = rh * PIC_SIZE * padY

	// The picture is an even number of the core's own lines tall, so the bright
	// and dark rows of a line divide it the same way on every line: the scanline
	// pattern sits identically on line 1 and line 224 instead of sliding a
	// fraction of a pixel per line, which is what turns the mask into a moiré.
	// What the even step leaves unused stays black in the bezel.
	kRows := 0.0
	if INT_SCALE > 0.5 {
		boxH := rh * size.y * ps
		boxW := rw * size.x * ps
		k := floor(boxH/isz.y*(1.0+INT_TOL))
		if max := floor(size.y * ps / isz.y); k > max {
			k = max
		}
		if max := floor(boxW / (isz.y * ar)); k > max {
			k = max
		}
		if k < 2.0 {
			k = 1.0
		} else {
			// 双数: each line gets an even number of output rows, so a line pairs
			// up its bright and dark rows and no row is left over at its end.
			k = floor(k*0.5) * 2.0
		}
		kRows = k
		rh = k * isz.y / (size.y * ps)
		rw = k * isz.y * ar / (size.x * ps)
	}
	if rw < 0.03 {
		rw = 0.03
	}
	if rh < 0.03 {
		rh = 0.03
	}
	hx := rw * 0.5
	hy := rh * 0.5

	ax := abs(c.x)
	ay := abs(c.y)
	// The glass is measured in the picture's own pixels, so the gap, the corner
	// radius and the mirror read the same on both axes and stay off the frame.
	pw := hx * size.x * 2.0 * ps
	ph := hy * size.y * 2.0 * ps
	qx := ax * size.x * ps
	qy := ay * size.y * ps
	ex := pw * 0.5
	ey := ph * 0.5
	// PP-reflex, in the same numbers it uses: fff is the picture's half seen from
	// its middle, kkk the corner the picture is cut with, lll the centre of that
	// arc, mmm its radius, ggg the rim the mirror starts on. One unit is the
	// picture's own height, so both axes read alike.
	gp := REFLECT_GAP * ph
	off := E_OFFSET * 0.01 * ph
	kkk := CORNER * 0.2 * ph
	if kkk < 0.5 {
		kkk = 0.5
	}
	if kkk > ex {
		kkk = ex
	}
	if kkk > ey {
		kkk = ey
	}
	ccx := ex - kkk
	ccy := ey - kkk
	rimx := ex + gp + off
	rimy := ey + gp + off
	mmm := kkk + gp + off

	// d is how far outside the picture's cut frame the fragment lies.
	d := max(qx-ex, qy-ey)
	corner := false
	if qx > ccx && qy > ccy {
		corner = true
		d = sqrt((qx-ccx)*(qx-ccx)+(qy-ccy)*(qy-ccy)) - kkk
	}

	if d > 0.0 {
		rn := -1.0
		if corner {
			rn = sqrt((qx-ccx)*(qx-ccx)+(qy-ccy)*(qy-ccy)) - mmm
		}
		if qx <= rimx && qy <= rimy && rn <= 0.0 {
			// The black the tube keeps between its picture and the glass.
			return vec4(0.0, 0.0, 0.0, 1.0)
		}
		mx := qx
		my := qy
		wc := 1.0
		if rn > 0.0 {
			// The corner: the fragment is mirrored across the picture's own arc rim in
			// the round it sits in, at the same overshoot a flat side uses, so the
			// glass reads one continuous sheet around the frame.
			dx := qx - ccx
			dy := qy - ccy
			rr := sqrt(dx*dx+dy*dy)
			if rr < 0.0001 {
				rr = 0.0001
			}
			mr2 := mmm + (mmm-rr)*1.5
			mx = ccx + dx*(mr2/rr)
			my = ccy + dy*(mr2/rr)
		} else {
			// The flat sides: mirrored through the rim with the same 1.5 overshoot.
			if qx > rimx {
				mx = (qx-rimx)*(-1.5) + rimx
			}
			if qy > rimy {
				my = (qy-rimy)*(-1.5) + rimy
			}
		}
		// PP-reflex pulls the sample 5% toward the middle before it reads it.
		ux := 0.5 + mx*0.95/pw
		uy := 0.5 + my*0.95/ph
		if c.x < 0.0 {
			ux = 1.0 - ux
		}
		if c.y < 0.0 {
			uy = 1.0 - uy
		}
		// The haze thickens the further the glass runs from the frame: the kernel
		// grows with the distance, so the glass never reads as a second picture.
		dk := d/ph
		if dk > 1.0 {
			dk = 1.0
		}
		mr := 0.9 + dk*isz.y*0.055
		rcol := mirrorBlur(vec2(ux, uy)*isz, isz, mr)
		// PP-reflex fades nothing: the glass keeps its mirror all the way out, and
		// only the kernel grows with the distance.
		ii := (REFLECTION*1.7)*(REFLECTION*1.7)*wc*REFLECTION_SWITCH
		if ii < 0.0 {
			ii = 0.0
		}
		return vec4(pow(clamp(rcol*ii, vec3(0.0), vec3(1.0)), vec3(1.0/GAMMA)), 1.0)
	}

	// PAD_Y shrank the box above, before the picture's frame was measured, so the
	// picture, its scanlines and the glass all read the same shape.

	suv := vec2(c.x/(2.0*hx), c.y/(2.0*hy)) + vec2(0.5, 0.5)
	np := suv * isz
	np.y = np.y + PPOY*0.01*isz.y
	col := sampled(np, isz)

	// The scanlines: the core's own lines taken to the picture's own rows by the
	// integer scale its frame was cut to, so the mask lands on the rows the game
	// drew. dp is in the picture's pixels, not the window's: the pattern's pitch
	// has to count the picture's rows, or the tube's lines read smaller than the
	// picture is drawn and every line picks up a different part of them.
	dp := vec2(suv.x*pw, suv.y*ph)
	sc := kRows
	if sc < 1.0 {
		sc = ph / isz.y
	}
	if sc < 1.0 {
		sc = 1.0
	}
	// The spacing knob is scanlines per game line, snapped to a whole divisor of
	// the line (or a whole multiple of it): a pitch that does not divide the line
	// leaves a fraction over on every one of them and the mask slides across the
	// picture. Only when the lock is off can the pitch be any integer the knob
	// names, which is how it used to shimmer.
	pitch := sc
	if INT_SCALE > 0.5 {
		want := sc * LINE_PITCH_COEF
		if want > sc*1.25 {
			m := floor(want/sc + 0.5)
			if m < 2.0 {
				m = 2.0
			}
			pitch = sc * m
		} else {
			// Every divisor of the line, nearest to what the knob asks for; one
			// scanline per output row (1) is left out because that is a mask with
			// no bright row in it at all.
			pitch = 1.0
			if sc >= 2.0 {
				pitch = 2.0
			}
			best := abs(pitch - want)
			for i := 1; i <= 16; i++ {
				d := float(i)
				if mod(sc, d) < 0.001 {
					p := sc / d
					if p >= 1.5 && abs(p-want) < best {
						best = abs(p - want)
						pitch = p
					}
				}
			}
		}
	} else {
		pitch = floor(sc*LINE_PITCH_COEF + 0.5)
		if pitch < 3.0 {
			pitch = 3.0
		}
	}
	if pitch < 1.0 {
		pitch = 1.0
	}
	g := pow(col, vec3(GAMMA*0.5))
	z := prof(g, deltaOf(int(min(pitch, 7.0))-3))
	z = pow(z, vec3(GAMMA))
	aa := (z*0.7 + vec3(0.3)) * pitch
	aa = aa * LINE_THICKNESS
	// The phase is taken a hair inside the line: a row exactly on a line's edge
	// reads as that edge only from one side, and mod() of it either side lands the
	// bright/dark pair on a different colour of the mask, which is the one-row
	// stripe that shows up now and then. The hair keeps every edge on the same
	// side.
	bb := mod(dp.y+0.0001, pitch) - pitch*0.5
	cr := abs(bb*2.0 - 0.5)
	// The green line is flipped only when the pitch has a third row to flip it
	// against. At one or two rows per line the flip has nothing to pair with and
	// leaves every dark row green instead of dark, which tints the whole picture
	// on a display too small to carry three rows a line.
	cg := cr
	if pitch > 2.9 {
		cg = abs(-bb*2.0 - 0.5)
	}
	cc := vec3(cr, cg, cr)
	dd := clamp(aa-cc+vec3(0.5), vec3(0.0), vec3(1.0))
	dd = dd * z / (z*0.7 + vec3(0.3))
	dd = sqrt(dd)
	pl := mix(g, dd, vec3(LINE_OPACITY))

	// The glow: diffusion's blurred picture, squared back into linear light, added
	// as PP-reflex adds it and normalised by the same amount.
	bl := glow(np, isz)
	rr := DIFFUSION * DIFFUSION * LINE_THICKNESS
	pp := pl*pl + bl*rr
	pp = pp / (1.0 + rr*(1.0-DIFF_OVEREXP))

	// The aperture mask, locked to the picture's own columns: one triad of three
	// colour cells per game pixel. Its width is the column's, not the row scale's,
	// so a 4:3 frame made of wide pixels (384×224) still gets one triad per pixel
	// and the mask can neither beat against the columns nor turn the picture into
	// blocks.
	mcell := sc * isz.y * ar / isz.x * 0.3333
	mpx := floor(dp.x) + 0.5
	mpy := floor(dp.y) + 0.5
	if MASK_TYPE > 0.5 {
		// A slot mask steps every other row of triads by half a cell.
		mpx = mpx + mod(floor(mpy/sc), 2.0)*mcell*1.5
	}
	mfx := mpx / (mcell*3.0)
	mt := floor((mfx-floor(mfx))*3.0)
	mstr := MASK_INTENSITY*(0.35+SS*0.65)
	if mstr > 0.92 {
		mstr = 0.92
	}
	moff := 1.0 - mstr*0.55
	mcw := vec3(moff, moff, moff)
	if mt < 1.0 {
		mcw = vec3(1.0, moff, moff)
	} else if mt < 2.0 {
		mcw = vec3(moff, 1.0, moff)
	} else {
		mcw = vec3(moff, moff, 1.0)
	}
	mfin := pp * mcw

	// The chain ends in display space, as PP-reflex's does.
	return vec4(pow(clamp(mfin, vec3(0.0), vec3(1.0)), vec3(1.0/GAMMA)), 1.0)
}
`

// --- the picker, in the player ---

// filterButton is the pill at the right: what look the game is running with,
// and the way into the panel of its parameters.
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

// toggleFilters opens or closes the panel; closing keeps what was tuned.
func (a *App) toggleFilters() {
	a.filtersOpen = !a.filtersOpen
	a.padsOpen = false
	a.cap = ""
	a.dragKey = ""
	if a.filtersOpen {
		a.scanTunes()
		a.tuneNote = ""
		a.filterScroll.Y = 0
	} else {
		a.saveTune()
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

// filterPanelW is the panel's glass. The two parameter columns are the whole of
// its arithmetic: 720 wide, 48 of padding either side and a 120 gap between them
// (five times the 24 the columns used to sit apart) leaves 252 for each column —
// a 144 name and its 100 control, so the gap between a name and the number it
// moves is a third narrower than the 204 it had. The panel keeps its 720: the
// window it opens in is only that plus the two insets, so the padding and the
// gutter are paid for by the name's side, not by a wider sheet of glass.
// The top and bottom take 16, not 48: the title and its ✕ belong to the glass's
// own edge, and only the columns sit in from the sides.
// The 100 is what the arithmetic leaves a control; the control itself fills that,
// so a column that comes out a pixel narrow shrinks its own bar rather than
// pushing the right column's bar under the scroller's clip.
const (
	filterPanelW    = 720
	filterPanelPad  = 48
	filterPanelEdge = 16
	filterColumnGap = 120
	filterLabelW    = 144
	filterPillW     = 100
	filterRowGap    = 8
	filterRowH      = 26
	filterHeaderH   = 28
	filterSwitchH   = 24
	filterFooterH   = 26
	filterPanelGap  = 8
	filterMinBodyH  = 240
	filterMinPanelW = 520
)

// filterPanelWidth is the glass's width: the 720 whenever the window has room
// for it, and whatever is left between the two insets when it does not. The
// columns are what give way, so a window that is a little too narrow shortens
// the bars instead of cutting the panel's right side off at the window's edge.
func (a *App) filterPanelWidth(c *ui.Context) float32 {
	w := float32(filterPanelW)
	if pageW, _ := c.Size(); pageW > 0 {
		if room := pageW - 2*edgeInset; room < w {
			w = room
		}
	}
	if w < filterMinPanelW {
		w = filterMinPanelW
	}
	return w
}

// filterBodyContentH is the two columns at full height: the longer column's
// rows plus the gap between them, so the panel can hug the list instead of
// leaving an empty scroll area on a tall window.
func filterBodyContentH() float32 {
	rows := (len(knobs) + 1) / 2
	return float32(rows*filterRowH + (rows-1)*filterRowGap)
}

// filterBodyHeight is the part of the window the columns may use. On a 720p
// window the whole list fits; on a shorter one the same two columns scroll
// together rather than being clipped or split into separate scrollers.
func (a *App) filterBodyHeight(c *ui.Context) float32 {
	h := filterBodyContentH()
	if _, pageH := c.Size(); pageH > 0 {
		avail := pageH - 2*edgeInset - 2*filterPanelEdge -
			filterHeaderH - filterSwitchH - filterFooterH - 3*filterPanelGap
		if avail < h {
			h = avail
		}
	}
	if h < filterMinBodyH {
		h = filterMinBodyH
	}
	return h
}

// filterPanel is the panel itself: one group of the preset's parameters in two
// columns on one page, with a single on/off switch above it and the saved groups
// below. The columns are the whole of the layout standard: 48 in from the sides,
// 120 between them, 16 above the title and below the footer, and an 8px vertical
// rhythm. The footer is the group management: the cards on its left are the
// saved tune-<n>.json files, 保存 writes the numbers being heard into a new card,
// and the card under the hand can be 更新 / 另存 / 删除.
func (a *App) filterPanel(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Children(func() {
		col := ui.Column(c).Absolute().Right(edgeInset).Top(edgeInset).Width(a.filterPanelWidth(c)).
			Padding(filterPanelEdge, filterPanelPad, filterPanelEdge, filterPanelPad).Radius(18).Gap(filterPanelGap).
			Material(glass.Glass{Clarity: 0.62, Tint: ui.Hex("#14161ee6")}).
			Border(1, ui.Hex("#ffffff1f")).
			Shadow(0, 26, 60, 0, ui.Hex("#000000cc"))
		col.Children(func() {
			ui.Row(c).Height(filterHeaderH).AlignItems(ui.Center).Children(func() {
				// The title is the panel's own handle: a double click drops every
				// number the player moved and puts the preset back.
				title := ui.ButtonBase(c).Grow(1).Height(filterHeaderH).AlignItems(ui.Center)
				title.Children(func() {
					ui.Text(c, "脑浆油条 参数").FontSize(12).FontWeight(700).
						TextColor(t.Text.Alpha(0.92)).SingleLine()
				})
				if title.DoubleClicked() {
					a.cfg.CRTTune = map[string]float32{}
					a.saveTune()
				}
				close := ui.ButtonBase(c).Width(26).Height(26).Radius(13).Center().
					Background(ui.Hex("#ffffff14"))
				close.Children(func() {
					ui.Text(c, "✕").FontSize(12).TextColor(t.TextMuted)
				})
				if close.Clicked() {
					a.win.Update(a.toggleFilters)
				}
			})
			// One look on or off, not a row of preset groups: the switch is the
			// only thing above the parameters.
			on := a.cfg.FilterLevel != "off"
			ui.Row(c).Height(filterSwitchH).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "滤镜").FontSize(11).FontWeight(600).
					TextColor(t.Text.Alpha(0.9)).SingleLine()
				ui.Text(c, filterName(a.cfg.FilterLevel)).FontSize(10).
					TextColor(t.TextMuted).SingleLine()
				ui.Box(c).Grow(1)
				a.filterSwitch(c, t, on)
			})
			ui.Scroll(c).Height(a.filterBodyHeight(c)).TrackScroll(&a.filterScroll).Children(func() {
				half := (len(knobs) + 1) / 2
				ui.Row(c).Gap(filterColumnGap).AlignItems(ui.Start).Children(func() {
					for _, list := range [][]knob{knobs[:half], knobs[half:]} {
						list := list
						ui.Column(c).Grow(1).Gap(filterRowGap).Children(func() {
							for _, k := range list {
								k := k
								a.knobRow(c, t, k)
							}
						})
					}
				})
			})
			// The saved groups as cards on the left, and what the panel can do with
			// the lit one on the right: 保存 makes the first card out of the numbers
			// being heard, and once a card is lit 更新 writes back into it, 另存
			// puts another card beside it, 删除 takes it away.
			ui.Row(c).Height(filterFooterH).AlignItems(ui.Center).Gap(6).Children(func() {
				if note := a.tuneNote; note != "" {
					ui.Text(c, note).Width(150).FontSize(9.5).
						TextColor(t.TextMuted.Alpha(0.65)).SingleLine()
				}
				// A strip, not a box: with more groups saved than fit, the cards slide
				// instead of pushing the buttons off the glass.
				ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Height(filterFooterH).ClipX().Children(func() {
					ui.Row(c).Height(filterFooterH).AlignItems(ui.Center).Gap(6).Children(func() {
						for i := range a.tuneFiles {
							i := i
							a.tuneCard(c, t, i)
						}
					})
				})
				if a.tuneIdx >= 0 && a.tuneIdx < len(a.tuneFiles) {
					a.tuneButton(c, "更新", ui.Hex("#e8763a33"), ui.Hex("#e8763a66"), t.Text.Alpha(0.9), func() {
						a.tuneAction(a.updateTune)
					})
					a.tuneButton(c, "另存", ui.Hex("#ffffff14"), ui.Hex("#ffffff1f"), t.Text.Alpha(0.85), func() {
						a.tuneAction(a.newTune)
					})
					a.tuneButton(c, "删除", ui.Hex("#ff453a1a"), ui.Hex("#ff453a55"), t.Text.Alpha(0.85), func() {
						a.tuneAction(a.deleteTune)
					})
					return
				}
				a.tuneButton(c, "保存", ui.Hex("#e8763a33"), ui.Hex("#e8763a66"), t.Text.Alpha(0.9), func() {
					a.tuneAction(a.newTune)
				})
			})
		})
	})
}

// tuneCard is one saved group in the footer's strip: a small plate of glass with
// the group's number, in the accent once it is the one being heard. A click on it
// brings back that group's look and its numbers.
func (a *App) tuneCard(c *ui.Context, t *ui.Theme, i int) {
	on := i == a.tuneIdx
	card := ui.ButtonBase(c).Width(30).Height(22).Radius(11).Center().
		Material(glass.Glass{Clarity: 0.78, Tint: ui.Hex("#ffffff10")}).
		Border(1, ui.Hex("#ffffff24"))
	if on {
		card.Material(glass.Glass{Clarity: 0.78, Tint: ui.Hex("#e8763a2e")}).
			Border(1, ui.Hex("#e8763aa6"))
	} else if card.Hovered() {
		card.Border(1, ui.Hex("#ffffff4d"))
	}
	card.Children(func() {
		ui.Text(c, strconv.Itoa(tuneNumber(a.tuneFiles[i]))).FontSize(10).
			TextColor(t.Text.Alpha(0.9)).SingleLine()
	})
	if card.Clicked() {
		i := i
		a.tuneAction(func() error { return a.selectTune(i) })
	}
}

// tuneButton is one of the footer's buttons: the accent one is what the panel
// does with the numbers being heard, the plain ones what it does with the cards.
func (a *App) tuneButton(c *ui.Context, label string, tint, border, ink ui.Color, act func()) {
	b := ui.ButtonBase(c).Width(58).Height(24).Radius(12).Center().
		Material(glass.Glass{Clarity: 0.7, Tint: tint}).Border(1, border)
	b.Children(func() {
		ui.Text(c, label).FontSize(10.5).TextColor(ink).SingleLine()
	})
	if b.Clicked() {
		act()
	}
}

// tuneAction runs one of the footer's buttons: a failure is written in the strip
// beside the cards, a clean one clears that line, so the cards are the answer
// unless something went wrong.
func (a *App) tuneAction(f func() error) {
	if err := f(); err != nil {
		a.tuneNote = err.Error()
		return
	}
	a.tuneNote = ""
}

// filterSwitch is the one on/off control above the group: the look on keeps the
// numbers and the preset, off takes the picture back to the core's own.
func (a *App) filterSwitch(c *ui.Context, t *ui.Theme, on bool) {
	track := ui.ButtonBase(c).Width(44).Height(26).Background(ui.Hex("#00000000"))
	track.Draw(func(p *ui.Painter, r ui.Rect) {
		trackColor := ui.Hex("#ffffff2b")
		knobX := r.X + 2
		if on {
			trackColor = ui.Hex("#34c759")
			knobX = r.X + 24
		}
		p.Fill(ui.Rect{X: r.X, Y: r.Y + 2, W: 44, H: 22}, trackColor, 11)
		p.Fill(ui.Rect{X: knobX, Y: r.Y + 4, W: 18, H: 18}, ui.Hex("#f7f8fa"), 9)
	})
	if track.Clicked() {
		a.setFilterOn(!on)
	}
}

// knobRow is one parameter: its name and its control. The name is a fixed 144 so
// every control starts at the same x, and the control takes whatever the column
// has left — the 100 the arithmetic leaves it — so it ends on the column's right
// edge instead of running past it when the scroller's inner width comes out a
// hair narrower than the panel's own sums.
func (a *App) knobRow(c *ui.Context, t *ui.Theme, k knob) {
	ui.Row(c).Height(filterRowH).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, k.label).Width(filterLabelW).FontSize(10).
			TextColor(t.TextMuted).SingleLine()
		if k.kind == 1 {
			a.knobSwitch(c, t, k)
			return
		}
		a.knobStepper(c, t, k)
	})
}

// knobSwitch is a parameter that is on or off, in the same glass as the bars.
func (a *App) knobSwitch(c *ui.Context, t *ui.Theme, k knob) {
	on := a.value(k.key) > 0.5
	// A switch is narrower than a bar, so it takes the right edge the bars end on
	// rather than starting where they do.
	ui.Box(c).Grow(1)
	// The switch of a phone's settings: a short track, a knob that sits at the
	// end it is not, green once it is on.
	track := ui.ButtonBase(c).Width(44).Height(26).Background(ui.Hex("#00000000"))
	track.Draw(func(p *ui.Painter, r ui.Rect) {
		trackColor := ui.Hex("#ffffff2b")
		knobX := r.X + 2
		if on {
			trackColor = ui.Hex("#34c759")
			knobX = r.X + 24
		}
		p.Fill(ui.Rect{X: r.X, Y: r.Y + 2, W: 44, H: 22}, trackColor, 11)
		p.Fill(ui.Rect{X: knobX, Y: r.Y + 4, W: 18, H: 18}, ui.Hex("#f7f8fa"), 9)
	})
	if track.Clicked() {
		if on {
			a.setKnob(k, 0.0)
		} else {
			a.setKnob(k, 1.0)
		}
	}
}

// knobStepper is the glass control of a parameter: the arrows at its ends take a
// tenth of a step, dragging its middle moves it faster. It shows one decimal of
// the number the shader runs with, scaled by k.mul.
func (a *App) knobStepper(c *ui.Context, t *ui.Theme, k knob) {
	v := float64(a.value(k.key)) * k.mul
	pill := ui.Row(c).Grow(1).MinWidth(60).Height(22).Radius(11).AlignItems(ui.Center).
		Material(glass.Glass{Clarity: 0.78, Tint: ui.Hex("#ffffff10")}).
		Border(1, ui.Hex("#ffffff24"))
	pill.Children(func() {
		back := ui.ButtonBase(c).Width(22).Height(22).Center().Radius(11).
			Background(ui.Hex("#ffffff0f"))
		back.Children(func() {
			ui.Text(c, "‹").FontSize(11).TextColor(t.Text.Alpha(0.65))
		})
		if back.Clicked() {
			a.nudge(k, -1.0)
		}
		mid := ui.ButtonBase(c).Grow(1).Height(22).Center().Radius(8).
			Background(ui.Hex("#ffffff05"))
		mid.Children(func() {
			ui.Text(c, fmt1(float32(v))).FontSize(10.5).TextColor(t.Text.Alpha(0.9))
		})
		dx, _, held := mid.Dragged()
		if held {
			a.dragKnob(k, float64(dx))
		} else if a.dragKey == k.key {
			a.dragKey = ""
		}
		fwd := ui.ButtonBase(c).Width(22).Height(22).Center().Radius(11).
			Background(ui.Hex("#ffffff0f"))
		fwd.Children(func() {
			ui.Text(c, "›").FontSize(11).TextColor(t.Text.Alpha(0.65))
		})
		if fwd.Clicked() {
			a.nudge(k, 1.0)
		}
	})
}

// nudge moves a parameter by one step of what its control shows.
func (a *App) nudge(k knob, sign float64) {
	cur := float64(a.value(k.key)) * k.mul
	v := roundStep(cur+sign*k.step, k.step)
	lo := k.lo * k.mul
	hi := k.hi * k.mul
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	a.setKnob(k, v/k.mul)
}

// dragKnob moves a parameter with the drag of its middle: the first frame starts
// from what it holds, then every pixel of the drag is part of the range.
func (a *App) dragKnob(k knob, dx float64) {
	if a.dragKey != k.key {
		a.dragKey = k.key
		a.dragVal = float64(a.value(k.key)) * k.mul
	}
	lo := k.lo * k.mul
	hi := k.hi * k.mul
	a.dragVal += dx * (hi - lo) / 220
	a.dragVal = math.Max(lo, math.Min(hi, a.dragVal))
	a.setKnob(k, roundStep(a.dragVal, k.step)/k.mul)
}

// setKnob records a parameter as the player moved it.
func (a *App) setKnob(k knob, v float64) {
	if a.cfg.CRTTune == nil {
		a.cfg.CRTTune = map[string]float32{}
	}
	a.cfg.CRTTune[k.key] = float32(v)
}

// roundStep keeps a control at the step it moves in.
func roundStep(v float64, step float64) float64 {
	if step <= 0 {
		step = 0.1
	}
	return math.Round(v/step) * step
}

// round1 keeps a parameter at a tenth of a unit.
func round1(v float64) float32 {
	return float32(math.Round(v*10) / 10)
}

// saveTune keeps the parameters as the player left them.
func (a *App) saveTune() { a.saveConfig() }

func fmt1(v float32) string { return strconv.FormatFloat(float64(v), 'f', 1, 32) }
