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
		"E_OFFSET": 1.8, "REFLECT_GAP": 0.02, "FILLET_D": -0.5, "B_CORNER": 0.8, "BORDER_SIZE": 0,
		"LINE_PITCH_COEF": 1, "LINE_THICKNESS": 0.95, "LINE_OPACITY": 0.95,
		"PIC_SIZE": 0.5, "INT_TOL": 0.05, "INT_SCALE": 0, "GAMMA": 2.2,
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
	{"PIC_SIZE", "图像尺寸 (%)", 5, 10, 0, 10, 0.1},
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
	{"BORDER_SIZE", "边框尺寸", 0, 0.5, 0, 1, 0.1},
	{"INT_TOL", "允许过扫描", 0, 0.1, 0, 1, 0.1},
	{"INT_SCALE", "强制整数缩放", 0, 1, 1, 1, 0.1},
	{"DIFFUSION", "光晕扩散", 0, 1, 0, 1, 0.1},
	{"DIFF_OVEREXP", "允许扩散过曝", 0, 1, 0, 1, 0.1},
	{"REFLECTION", "边框反射强度", 0, 1, 0, 1, 0.1},
	{"E_OFFSET", "边框反射偏移", 0, 2, 0, 1, 0.1},
	{"REFLECTION_SWITCH", "边框反射开关", 0, 1, 1, 1, 0.1},
	{"REFLECT_GAP", "反射间距 (%)", 0, 0.24, 0, 100, 1},
	{"B_CORNER", "边框圆角大小", 0, 1, 0, 1, 0.1},
	{"FILLET_D", "边框圆角曲率", -0.5, 0.5, 0, 1, 0.1},
	{"PPOY", "图像位置 Y", -50, 50, 0, 1, 0.1},
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
	// The shape the core asks its picture to be drawn at, which the glass fits the
	// frame into: a 320×200 DOS picture is 4:3, not its pixels' own shape.
	aspect := a.gameAspect
	if aspect < 0.4 || aspect > 5 {
		if a.gameBmp != nil {
			if bw, bh := ui.ImageSize(a.gameBmp); bw > 0 && bh > 0 {
				aspect = bw / bh
			}
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
var PIC_SIZE float
var BORDER_SIZE float
var INT_TOL float
var INT_SCALE float
var DIFFUSION float
var DIFF_OVEREXP float
var REFLECTION float
var E_OFFSET float
var REFLECT_GAP float
var REFLECTION_SWITCH float
var GAME_ASPECT float
var B_CORNER float
var FILLET_D float

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

// tap is one sample of diffusion's glow, in linear light.
func tap(np vec2, o vec2, k float) vec3 {
	d2 := dot(o, o)
	wt := exp(-d2 * 0.35)
	return pow(clamp(image0Sample(np+o*k).xyz, vec3(0.0), vec3(1.0)), vec3(GAMMA)) * wt
}

// glow is diffusion-h and diffusion-v as one: the picture blurred in linear
// light with a gaussian, which PP-reflex adds to the scanlines as the tube's glow.
func glow(np vec2, k float) vec3 {
	s := tap(np, vec2(0.0, 0.0), k)
	s = s + tap(np, vec2(1.0, 0.0), k) + tap(np, vec2(-1.0, 0.0), k)
	s = s + tap(np, vec2(0.0, 1.0), k) + tap(np, vec2(0.0, -1.0), k)
	s = s + tap(np, vec2(1.0, 1.0), k) + tap(np, vec2(-1.0, 1.0), k)
	s = s + tap(np, vec2(1.0, -1.0), k) + tap(np, vec2(-1.0, -1.0), k)
	s = s + tap(np, vec2(2.0, 0.0), k) + tap(np, vec2(-2.0, 0.0), k)
	s = s + tap(np, vec2(0.0, 2.0), k) + tap(np, vec2(0.0, -2.0), k)
	s = s + tap(np, vec2(2.0, 1.0), k) + tap(np, vec2(-2.0, 1.0), k)
	s = s + tap(np, vec2(2.0, -1.0), k) + tap(np, vec2(-2.0, -1.0), k)
	s = s + tap(np, vec2(1.0, 2.0), k) + tap(np, vec2(-1.0, 2.0), k)
	s = s + tap(np, vec2(1.0, -2.0), k) + tap(np, vec2(-1.0, -2.0), k)
	return s / 8.19
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

// mirrorBlur is the glass of the bezel: the picture sampled around a point of
// itself with a wide, soft kernel, so the reflection reads as haze on glass and
// not as a second, crisp copy of the frame.
func mirrorBlur(p vec2, isz vec2, r float) vec3 {
	// Bilinear taps on three rings, so the kernel covers the glass evenly and the
	// reflection reads as haze instead of a handful of copies of the frame.
	lo := vec2(0.0, 0.0)
	hi := isz - vec2(1.0, 1.0)
	// A gaussian read as six rings of eight bilinear taps: dense enough that the
	// glass reads as haze, and each ring turned differently so the taps never line
	// up into a lattice over the reflection.
	s := image0Sample(clamp(p, lo, hi)).xyz * 2.0
	s = s + blurRing(p, lo, hi, r*0.28, 0.35) * 1.90
	s = s + blurRing(p, lo, hi, r*0.50, 0.05) * 1.60
	s = s + blurRing(p, lo, hi, r*0.72, 0.55) * 1.30
	s = s + blurRing(p, lo, hi, r*0.95, 0.20) * 1.00
	s = s + blurRing(p, lo, hi, r*1.25, 0.85) * 0.70
	s = s + blurRing(p, lo, hi, r*1.60, 0.50) * 0.45
	return s / 57.6
}

// blurRing is a ring of eight bilinear taps around p at radius r, turned by a.
func blurRing(p vec2, lo vec2, hi vec2, r float, a float) vec3 {
	// The ring's angle is nudged by where the fragment sits, so neighbouring
	// fragments read the glass at different angles and no lattice shows up.
	a = a + (p.x*0.717 + p.y*1.237)
	s := image0Sample(clamp(p+vec2(r*cos(a), r*sin(a)), lo, hi)).xyz
	s = s + image0Sample(clamp(p+vec2(r*cos(a+0.785), r*sin(a+0.785)), lo, hi)).xyz
	s = s + image0Sample(clamp(p+vec2(r*cos(a+1.571), r*sin(a+1.571)), lo, hi)).xyz
	s = s + image0Sample(clamp(p+vec2(r*cos(a+2.356), r*sin(a+2.356)), lo, hi)).xyz
	s = s + image0Sample(clamp(p+vec2(r*cos(a+3.142), r*sin(a+3.142)), lo, hi)).xyz
	s = s + image0Sample(clamp(p+vec2(r*cos(a+3.927), r*sin(a+3.927)), lo, hi)).xyz
	s = s + image0Sample(clamp(p+vec2(r*cos(a+4.712), r*sin(a+4.712)), lo, hi)).xyz
	s = s + image0Sample(clamp(p+vec2(r*cos(a+5.498), r*sin(a+5.498)), lo, hi)).xyz
	return s
}

func LayerEffect(pos vec2) vec4 {
	size := rectSize()
	ps := pixelScale()
	uv := pos / size
	c := warp(uv - vec2(0.5, 0.5))

	isz := image0Size()
	// The picture keeps its own shape inside the window; PIC_SIZE trims it and
	// BORDER_SIZE takes the glass's rim off what is left. Everything outside the
	// picture is the glass, so the reflection reaches the window's own edges.
	ar := isz.x / isz.y
	if GAME_ASPECT > 0.4 {
		if GAME_ASPECT < 5.0 {
			ar = GAME_ASPECT
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
	rw = rw * PIC_SIZE * (1.0 - BORDER_SIZE)
	rh = rh * PIC_SIZE * (1.0 - BORDER_SIZE)
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

	// eee is the fragment seen from the middle with its sign thrown away, and d
	// how far outside the picture's cut frame it lies.
	eee := vec2(qx, qy)
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
			// PP-reflex's corner: a ray is cast from the fragment at a start point
			// turned by FILLET_D, the point where it crosses the arc is found, and
			// the fragment is mirrored through it by ttt, the arc's own factor.
			o := mmm * 5.0 * (1.0 - B_CORNER)
			th := FILLET_D * 1.5707963
			cvx := ccx - (o*cos(th) - o*sin(th))
			cvy := ccy - (o*sin(th) + o*cos(th))
			q := arcHit(eee, vec2(cvx, cvy), vec2(ccx, ccy), mmm)
			if q.x > 0.0001 || q.y > 0.0001 {
				rx := (q.x-ccx) / mmm
				ry := (q.y-ccy) / mmm
				sx := q.x - cvx
				sy := q.y - cvy
				tx := (rx-0.5) * -2.0
				ty := (ry-0.5) * -2.0
				l1 := sqrt(sx*sx+sy*sy)
				l2 := sqrt((sx*tx)*(sx*tx)+(sy*ty)*(sy*ty))
				if l2 > 0.000001 {
					tx = tx * (l1/l2)
					ty = ty * (l1/l2)
				}
				tx = tx * (1.0 + rx*0.5)
				ty = ty * (1.0 + ry*0.5)
				mx = (qx-q.x)*tx + q.x
				my = (qy-q.y)*ty + q.y
				wc = 1.0 + (rx*ry)*(rx*ry)
			}
			if mx > rimx {
				mx = (mx-rimx)*(-1.5) + rimx
				}
			if my > rimy {
				my = (my-rimy)*(-1.5) + rimy
			}
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
		mr := 0.8 + dk*isz.y*0.30
		rcol := mirrorBlur(vec2(ux, uy)*isz, isz, mr)
		// PP-reflex fades nothing: the glass keeps its mirror all the way out, and
		// only the kernel grows with the distance.
		ii := (REFLECTION*1.7)*(REFLECTION*1.7)*wc*REFLECTION_SWITCH
		if ii < 0.0 {
			ii = 0.0
		}
		return vec4(rcol*pow(ii, 1.0/GAMMA), 1.0)
	}

	suv := vec2(c.x/(2.0*hx), c.y/(2.0*hy)) + vec2(0.5, 0.5)
	np := suv * isz
	np.y = np.y + PPOY*0.01*isz.y
	col := sampled(np, isz)

	// The scanlines: the core's own lines taken to the output's rows by an
	// integer scale, so the mask lands on the rows the game drew.
	dp := vec2(suv.x*size.x, suv.y*size.y) * ps
	rows := size.y * ps * rh
	sc := floor(rows/isz.y*(1.0+INT_TOL))
	if sc < 1.0 {
		sc = 1.0
	}
	if INT_SCALE > 0.5 {
		sc = floor(sc + 0.5)
	}
	pitch := floor(sc*LINE_PITCH_COEF + 0.5)
	if pitch < 3.0 {
		pitch = 3.0
	}
	g := pow(col, vec3(GAMMA*0.5))
	z := prof(g, deltaOf(int(min(pitch, 7.0))-3))
	z = pow(z, vec3(GAMMA))
	aa := (z*0.7 + vec3(0.3)) * pitch
	aa = aa * LINE_THICKNESS
	bb := mod(dp.y, pitch) - pitch*0.5
	cr := abs(bb*2.0 - 0.5)
	cg := abs(-bb*2.0 - 0.5)
	cc := vec3(cr, cg, cr)
	dd := clamp(aa-cc+vec3(0.5), vec3(0.0), vec3(1.0))
	dd = dd * z / (z*0.7 + vec3(0.3))
	dd = sqrt(dd)
	pl := mix(g, dd, vec3(LINE_OPACITY))

	// The glow: diffusion's blurred picture, squared back into linear light, added
	// as PP-reflex adds it and normalised by the same amount.
	k := 1.6 + isz.y/240.0
	bl := glow(np, k)
	rr := DIFFUSION * DIFFUSION * LINE_THICKNESS
	pp := pl*pl + bl*rr
	pp = pp / (1.0 + rr*(1.0-DIFF_OVEREXP))

	// PP-reflex's aperture mask between the phosphor columns.
	period := floor(sc*0.6*SS + 0.5)
	if period < 2.0 {
		period = 2.0
	}
	if period > sc {
		// A mask cell wider than the game row turns the picture into flat bands of
		// one channel, so it is held to the row's own width.
		period = sc
	}
	tx := floor(dp.x) + 0.5
	ty := floor(dp.y) + 0.5
	uu := floor(period*0.5 + 0.1)
	tx = tx + floor(ty/uu)*uu*MASK_TYPE
	ww := floor(period*0.5 + 0.5)
	vx := abs((mod(tx, period)-period*0.5)*2.0 - 0.5)
	vy := abs((mod(tx+ww, period)-period*0.5)*2.0 - 0.5)
	vv := vec3(vx, vy, vx)
	yy := (pp*MASK_INTENSITY + vec3(1.0-MASK_INTENSITY)) * period
	zz := clamp(yy-vv+vec3(0.5), vec3(0.0), vec3(1.0))
	zz = zz * pp / (pp*MASK_INTENSITY + vec3(1.0-MASK_INTENSITY))

	// The chain ends in display space, as PP-reflex's does.
	return vec4(pow(clamp(zz, vec3(0.0), vec3(1.0)), vec3(1.0/GAMMA)), 1.0)
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
	a.dragKey = ""
	if !a.filtersOpen {
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

// filterOptions are the presets the panel offers at its top.
var filterOptions = []struct {
	Level string
	Name  string
}{
	{"", "标准"},
	{"soft", "柔和"},
	{"strong", "强烈"},
	{"off", "关闭"},
}

// filterPanel is the panel itself: the preset's own parameters, each on a glass
// control, over the game it belongs to. Two columns keep the whole list on one
// page, so nothing scrolls while a control is dragged.
func (a *App) filterPanel(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Children(func() {
		col := ui.Column(c).Absolute().Right(edgeInset).Top(edgeInset).Width(472).
			Padding(12).Radius(18).Gap(8).
			Material(glass.Glass{Clarity: 0.62, Tint: ui.Hex("#14161ee6")}).
			Border(1, ui.Hex("#ffffff1f")).
			Shadow(0, 26, 60, 0, ui.Hex("#000000cc"))
		col.Children(func() {
			ui.Row(c).Height(28).AlignItems(ui.Center).Children(func() {
				// The title is the panel's own handle: a double click drops every
				// number the player moved and puts the preset back.
				title := ui.ButtonBase(c).Grow(1).Height(28).AlignItems(ui.Center)
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
			ui.Row(c).Height(28).Gap(6).AlignItems(ui.Center).Children(func() {
				for _, opt := range filterOptions {
					opt := opt
					pill := ui.ButtonBase(c).Height(24).PaddingX(11).Radius(12).Center()
					if opt.Level == a.cfg.FilterLevel ||
						(opt.Level == "" && (a.cfg.FilterLevel == "" || a.cfg.FilterLevel == "normal")) {
						pill.Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#e8763a33")}).
							Border(1, ui.Hex("#e8763a66"))
					} else {
						pill.Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#ffffff14")}).
							Border(1, ui.Hex("#ffffff1f"))
					}
					pill.Children(func() {
						ui.Text(c, opt.Name).FontSize(11).TextColor(t.Text.Alpha(0.85))
					})
					if pill.Clicked() {
						a.pickPreset(opt.Level)
					}
				}
				ui.Box(c).Grow(1)
				reset := ui.ButtonBase(c).Height(24).PaddingX(11).Radius(12).Center().
					Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#ffffff14")}).
					Border(1, ui.Hex("#ffffff1f"))
				reset.Children(func() {
					ui.Text(c, "恢复原参数").FontSize(11).TextColor(t.Text.Alpha(0.85))
				})
				if reset.Clicked() {
					a.cfg.CRTTune = map[string]float32{}
					a.saveTune()
				}
			})
			half := (len(knobs) + 1) / 2
			ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
				for _, list := range [][]knob{knobs[:half], knobs[half:]} {
					list := list
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						for _, k := range list {
							k := k
							a.knobRow(c, t, k)
						}
					})
				}
			})
		})
	})
}

// knobRow is one parameter: its name and its control.
func (a *App) knobRow(c *ui.Context, t *ui.Theme, k knob) {
	ui.Row(c).Height(26).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, k.label).Grow(1).MinWidth(0).FontSize(10).
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
	pill := ui.Row(c).Width(120).Height(22).Radius(11).AlignItems(ui.Center).
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

// pickPreset sets a look and drops what was tuned on top of the last one.
func (a *App) pickPreset(level string) {
	a.cfg.FilterLevel = level
	a.cfg.CRTTune = map[string]float32{}
	a.dragKey = ""
	a.saveTune()
}

// saveTune keeps the parameters as the player left them.
func (a *App) saveTune() { a.saveConfig() }

func fmt1(v float32) string { return strconv.FormatFloat(float64(v), 'f', 1, 32) }
