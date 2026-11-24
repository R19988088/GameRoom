package app

import (
	"fmt"
	"image"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/library"
)

// view draws the window each frame: covers fill the window and scroll under
// the floating bar, the platform's menus float below it, and the bar itself
// is a plate of glass the covers show through.
func (a *App) view(c *ui.Context) {
	// What the last frame's menu clicks queued runs first, so this frame's
	// lines are cut from the games as they are now.
	a.runQueue()
	c.SetTheme(&a.theme)
	t := &a.theme
	c.Root().Background(t.Background)

	// A game on screen owns the window: its picture and the two controls are
	// drawn here, in this page, with nothing of the library left showing.
	if a.core != nil || a.player != nil {
		a.playerView(c, t)
		return
	}

	ui.Box(c).Fill().Children(func() {
		if a.lib == nil {
			a.message(c, t, "没有数据", a.status)
			return
		}
		// Drawn first, so covers pass under the capsules floating above them.
		a.body(c, t)
		a.statusBar(c, t)
		// Drawn last: the capsules float over the covers, glassy enough to
		// see them behind.
		a.toolbar(c, t)
	})
}

// playerView draws the window over a running game: the game's picture fills it,
// and a way out and the filter picker float in its corners. Nothing else is
// drawn — no strip above the picture and no title inside it, because the window
// is the game's window and the title bar already carries its name.
func (a *App) playerView(c *ui.Context, t *ui.Theme) {
	a.pumpCore()
	a.pumpMouse()
	a.pushPad()
	a.padTick()
	if a.savesOpen {
		a.saveTick()
	}
	w, h := c.Size()
	ui.Box(c).Fill().Background(ui.Hex("#000000")).Children(func() {
		a.gamePicture(c, w, h)
		a.floatControls(c, t)
		if a.filtersOpen {
			a.filterPanel(c, t)
		}
		if a.padsOpen {
			a.padPanel(c, t)
		}
		if a.savesOpen {
			a.savePanel(c, t)
		}
	})
}

// pumpCore hands the newest picture the core drew to the bitmap the page shows.
// The frame is kept at its own size, the size the core drew it in: the tube's
// look is the GPU's job (crtEffect), which reads that bitmap as it is.
func (a *App) pumpCore() {
	if a.core == nil {
		return
	}
	f := a.core.Frame()
	if f == nil {
		return
	}
	n := a.core.Frames()
	if n == a.gameFrames && a.gameBmp != nil {
		return
	}
	b := f.Bounds()
	if a.gameImg == nil || a.gameImg.Bounds().Size() != b.Size() {
		a.gameImg = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	}
	draw.Draw(a.gameImg, a.gameImg.Bounds(), f, b.Min, draw.Src)
	if a.gameBmp == nil {
		a.gameBmp = ui.NewBitmap(a.gameImg).Pixelated()
	} else {
		a.gameBmp.Update(a.gameImg)
	}
	a.gameFrames = n
	a.frameW, a.frameH = b.Dx(), b.Dy()
}

// gamePicture draws the game at the shape its platform's glass was: the picture
// fills the largest box of that shape in the middle of the window, with black
// left around it. A CPS 384×224 board is drawn on a 4:3 glass, as it was made
// to be, rather than stretched to its pixels' shape — the pixels there are
// wider than they are tall, so the image is stretched into the box (FillBox),
// not kept at its own frame's shape.
func (a *App) gamePicture(c *ui.Context, w, h float32) {
	if a.gameBmp == nil {
		return
	}
	// The glass owns the whole window: the picture sits inside it at PIC_SIZE and
	// the bezel reflects the picture out to the window's own edges, in the same
	// page as the game. Without the shader nothing carries that reflection, so the
	// picture falls back to the shape of the platform's tube, centred.
	fx, ok := a.crtEffect()
	px, py, pw, ph := float32(0), float32(0), w, h
	if !ok {
		ratio := a.pictureRatio()
		if ratio <= 0 {
			return
		}
		pw, ph = w, w/ratio
		if ph > h {
			ph, pw = h, h*ratio
		}
		px, py = (w-pw)/2, (h-ph)/2
	}
	ui.Box(c).Absolute().Left(px).Top(py).Width(pw).Height(ph).Children(func() {
		pic := ui.Box(c).Absolute().Left(0).Top(0).Width(pw).Height(ph).Background(ui.Hex("#000000")).Children(func() {
			ui.Image(c, a.gameBmp).Absolute().Left(0).Top(0).Width(pw).Height(ph).Fit(ui.FillBox)
		})
		if ok {
			pic.Effect(fx)
		}
	})
}

// floatControls are the pills that hover over the game's corners, in the strip
// the player's overlay window covers.
func (a *App) floatControls(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(edgeInset).Right(edgeInset).Top(edgeInset).Height(backButtonSize).Gap(8).Children(func() {
		a.backButton(c, t)
		ui.Box(c).Grow(1)
		a.saveButton(c, t)
		a.controllerButton(c, t)
		a.filterButton(c, t)
	})
}

// The player's controls: a round way out, a pill for the pad the game reads,
// a pill for the look, and the strip their window is, tall enough for both and
// for the glass to breathe.
const (
	backButtonSize        = 34
	filterButtonWidth     = 104
	controllerButtonWidth = 118
	playerButtonsHeight   = 12 + backButtonSize + 12
)

// playAreaTop is the first DIP of the screen. The game owns the whole client
// area now, so nothing is kept clear above it.
const playAreaTop = 0

// edgeInset was the margin the player used to leave between the game and the
// window's sides; the game fills the window now, so the margin only remains for
// the controls that float over it.
const edgeInset = 16

// backButton leaves the game: the round arrow in the corner, glassy until the
// pointer touches it. It lives in the player's own overlay window, so its click
// is handed to the app's window before anything is touched.
func (a *App) backButton(c *ui.Context, t *ui.Theme) {
	b := ui.ButtonBase(c).Width(backButtonSize).Height(backButtonSize).Radius(backButtonSize/2).Center().
		Material(glass.Glass{Clarity: 0.5, Tint: ui.Hex("#0d0f168f")}).
		Border(1, ui.Hex("#ffffff1f"))
	if b.Hovered() {
		b.Background(ui.Hex("#ffffff1f")).Border(1, ui.Hex("#ffffff4d"))
	}
	b.Children(func() {
		ui.Text(c, "←").FontSize(15).FontWeight(600).TextColor(t.Text.Alpha(0.9))
	})
	if b.Clicked() {
		a.win.Update(func() { a.stopPlayer() })
	}
	// Escape leaves the game from the keyboard — but while a panel is open it
	// belongs to the panel, which closes on it, so the button stays quiet.
	if !a.filtersOpen && !a.padsOpen && !a.savesOpen {
		b.Shortcut(0, ui.KeyEscape)
	}
}

// toolbar is the top row: one shelf picker, the shelf's chips, and the
// search — three flat pills in a line. There is no plate of glass holding the
// chips: each pill keeps its own single layer, so nothing is nested, and the
// covers pass between them.
func (a *App) toolbar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(20).Top(14).Right(20).Height(46).Gap(10).
		AlignItems(ui.Center).Children(func() {
		a.shelfSwitch(c, t)
		// A scrolling strip, not a container: it has no glass of its own, so
		// the chips are the only thing on it.
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Height(46).ClipX().Children(func() {
			ui.Row(c).Height(46).Gap(8).AlignItems(ui.Center).Children(func() {
				a.contentRow(c, t)
			})
		})
		// The refresh pill and the search belong together: they are what asks
		// the shelf what it holds, so they sit as one pair at the bar's right
		// end instead of among the chips.
		ui.Row(c).Height(46).Gap(6).AlignItems(ui.Center).Children(func() {
			a.refreshPill(c, t)
			a.searchField(c, t)
		})
	})
}

// refreshPill rereads the library from the database: a rom copied in, a cover
// fetched or an edit made outside the shelf shows up without restarting it.
func (a *App) refreshPill(c *ui.Context, t *ui.Theme) {
	p := a.chipBase(c, 32, false).PaddingX(11)
	if p.Hovered() {
		p.Border(1, ui.Hex("#ffffff4d"))
	}
	p.Children(func() {
		ui.Text(c, "刷新").FontSize(12).FontWeight(500).TextColor(t.TextMuted).SingleLine()
	})
	p.Label("刷新列表")
	if p.Clicked() {
		// The rom folder is read again first, so a dump copied in beside the
		// others arrives as a card of its own, then the library is reread at the
		// top of the next frame, before the lines are cut from its games.
		a.later(func() {
			n, err := 0, error(nil)
			if a.db != nil {
				n, err = library.Rescan(a.db, a.root)
			}
			a.reload()
			switch {
			case err != nil:
				a.status = "扫描 rom 失败：" + err.Error()
			case n > 0:
				a.status = fmt.Sprintf("刷新完成：新增 %d 个", n)
			}
		})
	}
}

// glass is one pill of the top row: a plate of glass the covers show through,
// blurred once — the chips are not put inside another one.
func (a *App) glass(el ui.Element) ui.Element {
	return el.Radius(14).
		Material(glass.Glass{Clarity: 0.5, Tint: ui.Hex("#0d0f168f")}).
		Border(1, ui.Hex("#ffffff1f")).
		Shadow(0, 10, 26, 0, ui.Hex("#00000099"))
}

// shelfSwitch is the left pill: the shelf shown, with its menu listing all
// five. It is the same short pill as the bar's chips, so the row has one shape.
func (a *App) shelfSwitch(c *ui.Context, t *ui.Theme) {
	name, count := "街机", 0
	for _, g := range library.Groups {
		if g.Key == a.group {
			name = g.Name
		}
	}
	if a.lib != nil {
		count = a.lib.GameCountOf(a.group)
	}

	anchor := a.chipBase(c, 32, false)
	anchor.PaddingX(14)
	if anchor.Hovered() {
		anchor.Border(1, ui.Hex("#ffffff4d"))
	}
	anchor.Children(func() {
		ui.Text(c, name).FontSize(12).FontWeight(600).TextColor(t.Text).SingleLine()
		if count > 0 {
			ui.Text(c, fmt.Sprint(count)).FontSize(10).TextColor(t.TextMuted.Alpha(0.75))
		}
		ui.Text(c, "▾").FontSize(10).TextColor(t.TextMuted.Alpha(0.85))
	})
	if anchor.Clicked() {
		a.shelfOpen = !a.shelfOpen
		a.moreOpen = false
	}

	ui.PopoverBase(c, anchor, &a.shelfOpen, func(panel ui.Element) {
		panel.Width(228).Padding(7).Radius(16).
			Background(ui.Hex("#17181ff5")).
			Border(1, ui.Hex("#ffffff1f")).
			Shadow(0, 22, 48, 0, ui.Hex("#000000b8"))
		panel.Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, g := range library.Groups {
					g := g
					n := 0
					if a.lib != nil {
						n = a.lib.GameCountOf(g.Key)
					}
					chosen := g.Key == a.group
					row := ui.ButtonBase(c).Height(38).PaddingX(11).Radius(10).
						AlignItems(ui.Center).Gap(8)
					if chosen {
						row.Background(ui.Hex("#e8763a1f"))
					} else if row.Hovered() {
						row.Background(ui.Hex("#ffffff12"))
					}
					row.Children(func() {
						color := t.TextMuted
						if chosen {
							color = t.Accent
						}
						ui.Text(c, g.Name).FontSize(12.5).FontWeight(600).
							TextColor(color).SingleLine().Grow(1)
						ui.Text(c, fmt.Sprint(n)).FontSize(10).
							TextColor(t.TextMuted.Alpha(0.7))
					})
					if row.Clicked() {
						a.shelfOpen = false
						a.chooseGroup(g.Key)
					}
				}
			})
		})
	})
}

// contentRow fills the middle of the bar: first the switch that decides how
// the shelf files its games, then that standard's values — the platforms on
// the 机种 axis, the chips otherwise.
func (a *App) contentRow(c *ui.Context, t *ui.Theme) {
	if a.lib == nil {
		return
	}
	if axes := a.axes(); len(axes) > 1 {
		a.axisSwitch(c, t)
		ui.Box(c).Width(1).Height(16).Background(ui.Hex("#ffffff2e"))
	}

	// On the 机种 axis the machines are either the shelf's platforms (家用机,
	// PC: one platform per machine) or, when a platform runs several systems
	// (掌机: GBA / PSP / NGP), the chips built from the cores. The split says
	// which: splitGroup leaves no menus when the machine is the platform.
	machine := a.axis == library.AxisMachine || a.axis == ""
	platforms := machine && len(a.chips) == 0
	if platforms && len(a.topPlatforms(5)) < 2 {
		return
	}

	// 收藏 stands before the five, on this shelf only.
	if a.favTotal > 0 || a.special == specialFav {
		a.chip(c, t, "收藏", a.favTotal, a.special == specialFav, func() {
			a.chooseSpecial(specialFav)
		})
	}
	// 全部 is the way back from a value on the axis. A shelf whose machine is
	// its platform has no such value — the chips below are the machines — so
	// there it is only drawn while 收藏 or 其他 is showing, as the way back.
	if !platforms || a.special != "" {
		count := a.shelfTotal
		if platforms {
			count = a.platformTotal
		}
		a.chip(c, t, "全部", count, a.special == "" && a.menu == "", func() { a.chooseMenu("") })
	}

	// 其他's own page lists no single category on top: the body's headers say
	// which machine (or board) each run of cards belongs to.
	if a.special != specialOther {
		if platforms {
			for _, p := range a.topPlatforms(5) {
				p := p
				a.chip(c, t, p.Name, a.counts[p.Key], p.Key == a.platform, func() {
					a.choosePlatform(p.Key)
				})
			}
		} else {
			for _, m := range a.topChips() {
				m := m
				a.chip(c, t, m.Name, m.Count, a.menu == m.Name, func() { a.chooseMenu(m.Name) })
			}
		}
	}
	a.otherChip(c, t)
}

// otherChip is 其他: the categories the bar's five had no room for. It only
// appears when there is something in it, and on its own page it is the chip
// that is lit.
func (a *App) otherChip(c *ui.Context, t *ui.Theme) {
	if a.otherTotal <= 0 && a.special != specialOther {
		return
	}
	a.chip(c, t, "其他", a.otherTotal, a.special == specialOther, func() {
		a.chooseSpecial(specialOther)
	})
}

// axisSwitch is the little pill at the left of the middle section: it names
// the standard the shelf is filed by and, clicked, moves to the next one
// (街机 基板 ⇄ 厂商，家用机 机种 ⇄ 厂商).
func (a *App) axisSwitch(c *ui.Context, t *ui.Theme) {
	p := a.chipBase(c, 32, false)
	p.Children(func() {
		ui.Text(c, "⇄").FontSize(11).TextColor(t.TextMuted.Alpha(0.8))
		ui.Text(c, library.AxisName(a.axis)).FontSize(12).FontWeight(600).
			TextColor(t.Text).SingleLine()
	})
	if p.Hovered() {
		p.Border(1, ui.Hex("#ffffff4d"))
	}
	if p.Clicked() {
		a.nextAxis()
	}
}

// topPlatforms is the shelf's platforms by game count, at most n of them.
// The chosen platform is always one of them, even when it is not busy:
// otherwise the bar would show no chip for the games below it.
func (a *App) topPlatforms(n int) []library.Platform {
	if a.lib == nil {
		return nil
	}
	ps := a.lib.PlatformsOf(a.group)
	sort.SliceStable(ps, func(i, j int) bool {
		if a.counts[ps[i].Key] != a.counts[ps[j].Key] {
			return a.counts[ps[i].Key] > a.counts[ps[j].Key]
		}
		return ps[i].Sort < ps[j].Sort
	})
	if len(ps) <= n {
		return ps
	}
	top := append([]library.Platform{}, ps[:n]...)
	inside := false
	for _, p := range top {
		if p.Key == a.platform {
			inside = true
		}
	}
	if !inside && a.platform != "" {
		for _, p := range ps {
			if p.Key == a.platform {
				top[n-1] = p
				break
			}
		}
	}
	return top
}

// restPlatforms are the ones 其他 holds: everything the bar's five chips left
// out.
func (a *App) restPlatforms(n int) []library.Platform {
	top := a.topPlatforms(n)
	shown := map[string]bool{}
	for _, p := range top {
		shown[p.Key] = true
	}
	var rest []library.Platform
	for _, p := range a.lib.PlatformsOf(a.group) {
		if !shown[p.Key] {
			rest = append(rest, p)
		}
	}
	return rest
}

// searchField is the right pill: the same short pill as the chips, with the
// field sitting directly on its one plate of glass.
func (a *App) searchField(c *ui.Context, t *ui.Theme) {
	plate := a.chipBase(c, 32, false)
	plate.Width(248).PaddingX(11)
	plate.Children(func() {
		in := ui.TextInputBase(c.Key("search"), &a.query).Fill().FontSize(12).
			TextColor(t.Text).Placeholder("搜索游戏名 / rom 名").Label("搜索游戏名").
			PlaceholderColor(t.TextMuted.Alpha(0.65))
		if in.Changed() {
			a.refilter()
		}
	})
}

// chip is one pill on the bar: a platform or a value of the axis. It carries
// its own flat background and its own shadow — it is never put inside another
// pill.
func (a *App) chip(c *ui.Context, t *ui.Theme, name string, count int, chosen bool, run func()) {
	p := a.chipBase(c, 32, chosen)
	if !chosen && p.Hovered() {
		p.Border(1, ui.Hex("#ffffff4d"))
	}
	weight := 500
	if chosen {
		weight = 600
	}
	text := t.TextMuted
	if chosen {
		text = t.Text
	}
	p.Children(func() {
		ui.Text(c, name).FontSize(12).FontWeight(weight).TextColor(text).SingleLine()
		if count > 0 {
			ui.Text(c, fmt.Sprint(count)).FontSize(10).TextColor(t.TextMuted.Alpha(0.7))
		}
	})
	if p.Clicked() {
		run()
	}
}

// chipBase is the one look the bar's small pills share: glass, so the covers
// passing behind them blur; a darker tint when the pill is chosen.
func (a *App) chipBase(c *ui.Context, height float32, chosen bool) ui.Element {
	bg, border := ui.Hex("#0d0f16a6"), ui.Hex("#ffffff24")
	if chosen {
		bg, border = ui.Hex("#e8763a3b"), ui.Hex("#e8763aa6")
	}
	return ui.ButtonBase(c).Height(height).PaddingX(13).Radius(16).
		AlignItems(ui.Center).Gap(7).
		Material(glass.Glass{Clarity: 0.45, Tint: bg}).
		Border(1, border).
		Shadow(0, 8, 20, 0, ui.Hex("#00000066"))
}

// The body's measurements, in DIPs. The gap at the top is the cards' first
// line: it scrolls away with them, so a shelf at rest starts under the bar
// and a scrolled one runs up behind it.
const (
	cardH   = 192
	cardMin = 200
	cardGap = 8
	headH   = 30
	secGap  = 18
	topGap  = 74
)

// body is the games that pass the platform, menu and search, filed in the
// sections the bar's standard makes them. It owns the window: the covers
// scroll up behind the floating bar and are clipped only at the window's own
// top, so nothing is cut in a band under the pills.
func (a *App) body(c *ui.Context, t *ui.Theme) {
	if len(a.shown) == 0 {
		a.message(c, t, "这里还没有游戏", "换个平台或分类，或检查 rom 目录")
		return
	}
	w, h := c.Size()
	if a.layout(w) {
		a.stale = false
	}

	// Only the lines the window can show (and a little beyond) are built: a
	// shelf of a thousand games keeps one screen of cards a frame.
	from, to := a.scroll.Y-300, a.scroll.Y+h+120
	view := ui.Scroll(c).Absolute().Left(26).Right(26).Top(0).Bottom(34).
		TrackScroll(&a.scroll)
	view.Children(func() {
		ui.Box(c).Height(a.contentH).Children(func() {
			for i := range a.lines {
				ln := &a.lines[i]
				if ln.top+cardH < from || ln.top > to {
					continue
				}
				if ln.head >= 0 {
					a.header(c, t, ln)
					continue
				}
				row := ui.Row(c).Absolute().Left(0).Right(0).Top(ln.top).
					Height(cardH).Gap(cardGap).AlignItems(ui.Stretch)
				row.Children(func() {
					for _, idx := range ln.games {
						a.card(c, t, idx)
					}
					// Empty places keep the last row's cards as wide as the
					// ones before them.
					for k := len(ln.games); k < a.cols; k++ {
						ui.Box(c).Grow(1).Basis(0)
					}
				})
			}
		})
	})
	if view.Shortcut(0, ui.KeyEnter) {
		a.launchChosen()
	}
}

// layout lays the body out for a window w DIPs wide, and reports whether it
// had to: a new width, or a new shelf, standard or search, rebuilds the lines.
func (a *App) layout(w float32) bool {
	cols := int((w - 52 + cardGap) / (cardMin + cardGap))
	cols = max(3, min(cols, 8))
	if !a.stale && cols == a.cols && len(a.lines) > 0 {
		return false
	}
	a.cols = cols
	a.lines = a.lines[:0]
	y := float32(topGap)
	for i := range a.sections {
		s := &a.sections[i]
		if s.name != "" {
			a.lines = append(a.lines, line{top: y, head: i})
			y += headH + 10
		}
		for k := s.from; k < s.from+s.count; k += cols {
			end := min(k+cols, s.from+s.count)
			games := make([]int, 0, end-k)
			for g := k; g < end; g++ {
				games = append(games, g)
			}
			a.lines = append(a.lines, line{top: y, head: -1, games: games})
			y += cardH + cardGap
		}
		if s.name != "" {
			y += secGap
		}
	}
	a.contentH = y + 24
	return true
}

// header labels a section: the value its chip names, and how many games it
// holds. It is a label, not a button: it used to file the shelf by that value
// when clicked, and a click on it — anywhere along the strip, or on the words
// themselves — collapsed the shelf to that one group, which reads as the shelf
// leaving. The chip above already does that job, and does it visibly.
func (a *App) header(c *ui.Context, t *ui.Theme, ln *line) {
	s := a.sections[ln.head]
	if s.name == "" {
		return
	}
	ui.Row(c).Absolute().Left(4).Top(ln.top).Height(headH).
		Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, s.name).FontSize(13).FontWeight(700).
			TextColor(t.Text.Alpha(0.92)).SingleLine()
		ui.Text(c, fmt.Sprint(s.count)).FontSize(10.5).
			TextColor(t.TextMuted.Alpha(0.6))
	})
}

// card is one game: its cover at its own aspect ratio, and its title. A click
// takes it, a double click plays it.
func (a *App) card(c *ui.Context, t *ui.Theme, idx int) {
	g := a.shown[idx]
	chosen := idx == a.chosen

	b := ui.ButtonBase(c).Grow(1).Basis(0).Height(cardH).Padding(4).Radius(12)
	if b.Hovered() {
		b.Background(ui.Hex("#ffffff0a"))
	}
	b.Children(func() {
		ui.Column(c).Fill().Gap(5).Children(func() {
			cover := ui.Box(c).Height(128).Radius(10).
				Background(ui.Hex("#0b0b10")).Center().Clip()
			if chosen {
				cover.Border(2, t.Accent)
			}
			cover.Children(func() {
				if bmp := a.cover(a.coverAbs(a.cardCover(g))); bmp != nil {
					// Contain, not Cover: a flyer keeps its own shape
					// instead of being cropped to the card.
					ui.Image(c, bmp).Fill().Fit(ui.Contain).Radius(10)
				} else {
					ui.Text(c, initial(g.Title)).FontSize(30).FontWeight(700).
						TextColor(t.TextMuted.Alpha(0.35))
				}
			})

			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				// 星在标题左边：没收藏也占同样宽的一个位子，收藏与否标题都对齐。
				star := ui.Box(c).Width(10).Height(14).AlignItems(ui.Center)
				star.Children(func() {
					if g.Favorite {
						ui.Text(c, "★").FontSize(9).TextColor(t.Accent)
					}
				})
				ui.Text(c, g.Title).FontSize(12).FontWeight(500).TextColor(t.Text).
					SingleLine().Grow(1)
			})

			// The versions of this game hang under it: which dump plays, and
			// the button that opens the list.
			a.versionBar(c, t, g)
		})
	})

	// A click chooses the card; a second one on the same card, soon enough,
	// starts it. Enter on the chosen card does the same — the key reaches the
	// element that has the focus, and the card takes it when it is pressed.
	if b.DoubleClicked() || b.Shortcut(0, ui.KeyEnter) {
		a.chosen = idx
		a.launchChosen()
	} else if b.Clicked() {
		now := time.Now()
		if a.lastClick == g.ID && now.Sub(a.lastClickAt) < 650*time.Millisecond {
			a.lastClick, a.lastClickAt = 0, now
			a.chosen = idx
			a.launchChosen()
		} else {
			a.lastClick, a.lastClickAt = g.ID, now
			a.chosen = idx
		}
	}

	// A dump dragged from Explorer onto this card is filed under it: the drop
	// writes a note in the library, and the rom stays in its folder.
	if files := b.DroppedFiles(); len(files) > 0 {
		a.mergeDumps(g, files)
	}

	// 按住 Ctrl 拖一张卡到另一张上，两张合成一张：被拖的那局成为这一卡的
	// 改版，名字不同也照并。只有按着 Ctrl 这张卡才是拖的来源，所以平时拖动
	// 卡片不会顺手抓走一局。
	if c.Modifiers()&ui.Ctrl != 0 {
		b.Drag(cardDrag{id: g.ID})
	}
	if d, ok := ui.DragOver[cardDrag](b); ok && d.id != g.ID {
		b.Border(2, t.Accent)
	}
	if d, ok := ui.Drop[cardDrag](b); ok && d.id != g.ID {
		a.mergeCards(d.id, g.ID)
	}

	// The card's own menu, on the right button: its fields can be corrected
	// from here without opening the database. Each card owns its open flag, so
	// the popup stays where it was opened.
	if b.Hovered() {
		a.hoverGame = g.ID
	}
	open := a.menuOpen(g.ID)
	m := ui.ContextMenuBase(c, b, open)
	if *open {
		a.menuFor(g.ID)
		m.Popup(func(panel ui.Element) { a.cardMenuPanel(c, t, panel, g) })
	} else if a.menuGame == g.ID {
		a.shutMenu()
	}
}

// cardField is one row of the card's menu: which column it writes, what it is
// called on this shelf, what the game has there now, and how it is corrected —
// editable, or 游戏名 which is typed in rather than picked from a list (there
// is no list of every name in the shelf worth showing).
type cardField struct {
	key, label, value string
	editable          bool
	typeIn            bool
}

// fields are the rows a card's menu offers, in the order the menu lists them:
// the hardware first, then who made it, what it is, which core runs it, and
// its name.
func (a *App) fields(g library.Game) []cardField {
	hardware := "机种"
	p := a.lib.Platform(g.PlatformKey)
	if p != nil && p.Group == "arcade" {
		hardware = "基板"
	}
	maker := a.lib.MakerOf(g)
	// 厂商 能改的只有街机与 DOS 货架：那里的厂商是游戏自己的发行商，写错了
	// 就该能改。家用机的厂商是机器本身的制造商（NINTENDO、SONY），改它等于
	// 说这台 FC 不是任天堂做的，没有意义。
	editableMaker := p != nil && (p.Group == "arcade" || p.Group == library.DOSGroup)
	genre := g.Genre
	if genre == "" {
		genre = "未标注类型"
	}
	core := a.lib.CoreOf(g)
	if core == "" {
		core = "未指定"
	}
	return []cardField{
		{"machine", hardware, a.lib.MachineNameOf(g), true, false},
		{"publisher", "厂商", maker, editableMaker, false},
		{"genre", "游戏类型", genre, true, false},
		{"core", "核心", core, true, false},
		// 名字能改：双击（或单击）直接在输入框里改。它不是一列，所以没有
		// 可选的值列表。
		{"title", "游戏名", g.Title, true, true},
	}
}

// cardMenuPanel fills the panel of a card's menu: the game and its fields,
// or — after a click on one — the values it could take, or — after a double
// click — a field to type the value in.
func (a *App) cardMenuPanel(c *ui.Context, t *ui.Theme, panel ui.Element, g library.Game) {
	panel.Width(288).Padding(8).Radius(16).
		Background(ui.Hex("#1b1d26f7")).
		Border(1, ui.Hex("#ffffff2e")).
		Shadow(0, 26, 56, 0, ui.Hex("#000000e6"))
	panel.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Row(c).Height(30).PaddingX(8).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, g.Title).FontSize(12).FontWeight(600).TextColor(t.Text).
					SingleLine().Grow(1)
				ui.Text(c, romNameOf(g)).FontSize(10).TextColor(t.TextMuted.Alpha(0.55)).
					SingleLine()
			})
			ui.Box(c).Height(1).Background(ui.Hex("#ffffff1f"))

			switch {
			case a.cardEdit:
				a.fieldInput(c, t, g)
			case a.cardField != "":
				a.fieldValues(c, t, g)
			default:
				a.fieldList(c, t, g)
			}
		})
	})
}

// fieldList is the menu at rest: one row per field, its value on the right.
// 游戏名那一行能改名，但点了不筛：一筛整个货架只剩一张卡，那不是看名字的
// 意思。所以它单击直接进输入框，不弹值列表。
func (a *App) fieldList(c *ui.Context, t *ui.Theme, g library.Game) {
	for _, f := range a.fields(g) {
		f := f
		row := ui.ButtonBase(c).Height(32).PaddingX(9).Radius(9).Gap(10).
			AlignItems(ui.Center)
		if row.Hovered() {
			row.Background(ui.Hex("#ffffff10"))
		}
		row.Children(func() {
			ui.Text(c, f.label).FontSize(11).TextColor(t.TextMuted.Alpha(0.8)).Width(58)
			ui.Text(c, f.value).FontSize(12).FontWeight(500).
				TextColor(t.Text.Alpha(0.9)).SingleLine().Grow(1)
		})
		// 游戏名没有可选的值列表，所以单击就直接进输入框；别的字段单击先看
		// 能改成什么，双击才自己打字。
		switch {
		case f.typeIn && (row.Clicked() || row.DoubleClicked()):
			a.cardField, a.cardEdit, a.cardText = f.key, true, f.value
		case row.DoubleClicked():
			a.cardField, a.cardEdit, a.cardText = f.key, true, f.value
		case row.Clicked():
			a.cardField, a.cardEdit = f.key, false
		}
	}
	ui.Box(c).Height(1).Background(ui.Hex("#ffffff1f"))
	a.favoriteRow(c, t, g)
	// A dump that shares its title with others can be sent out on its own: it
	// keeps a card of its own from then on (取消合并).
	if len(a.verMembers[a.verFam[g.ID]]) > 1 {
		a.splitRow(c, t, g)
	}
	a.folderRow(c, t, g)
	// 最下面一段是这台机器上的 ips 补丁：只有 数据/ips 里真有这个 rom 的补丁才画，
	// 不弹第二个窗口，行尾的开关当场开关。
	a.ipsSection(c, t, g)
}

// romNameOf is the dump's file name without its extension, which the card's
// menu shows at its top right: several dumps can hang under one title, and the
// name tells which one this is.
func romNameOf(g library.Game) string { return romStem(g.RomPath) }

// cardCover is the picture a card shows: the cover of the dump that plays for
// it, and the card's own when that dump has none — so switching the default
// version switches the cover too.
func (a *App) cardCover(g library.Game) string {
	if c := a.versionTarget(g).Cover; c != "" {
		return c
	}
	return g.Cover
}

// splitRow is the menu's 取消合并: this dump stops being a version of the title
// it is filed under and gets a card of its own. The dumps beside it stay as they
// were — only this one comes apart.
func (a *App) splitRow(c *ui.Context, t *ui.Theme, g library.Game) {
	row := ui.ButtonBase(c).Height(32).PaddingX(9).Radius(9).
		AlignItems(ui.Center).Gap(8)
	if row.Hovered() {
		row.Background(ui.Hex("#ffffff10"))
	}
	row.Children(func() {
		ui.Text(c, "⇱").FontSize(12).TextColor(t.TextMuted.Alpha(0.85))
		ui.Text(c, "取消合并").FontSize(12).FontWeight(500).
			TextColor(t.Text.Alpha(0.9)).SingleLine().Grow(1)
		ui.Text(c, "只取消这一个").FontSize(10).TextColor(t.TextMuted.Alpha(0.6))
	})
	if row.Clicked() {
		a.splitDump(g.ID)
	}
}

// favoriteRow is the menu's own button: it stars the card, which puts it on
// the bar's 收藏 page, or takes the star away.
func (a *App) favoriteRow(c *ui.Context, t *ui.Theme, g library.Game) {
	row := ui.ButtonBase(c).Height(32).PaddingX(9).Radius(9).
		AlignItems(ui.Center).Gap(8)
	if row.Hovered() {
		row.Background(ui.Hex("#ffffff10"))
	}
	row.Children(func() {
		star, label := "☆", "添加收藏"
		if g.Favorite {
			star, label = "★", "取消收藏"
		}
		ui.Text(c, star).FontSize(13).TextColor(t.Accent)
		ui.Text(c, label).FontSize(12).FontWeight(500).
			TextColor(t.Text.Alpha(0.9)).SingleLine().Grow(1)
	})
	if row.Clicked() {
		a.toggleFavorite(g.ID)
	}
}

// folderRow is the menu's last line: the two folders this dump lives in, side
// by side. 拖动、替换 rom 都在资源管理器里做，所以只把目录打开，不改任何东西。
func (a *App) folderRow(c *ui.Context, t *ui.Theme, g library.Game) {
	ui.Row(c).Height(30).Gap(6).Children(func() {
		a.folderButton(c, t, "rom 目录", filepath.Dir(filepath.Join(a.root, "rom", filepath.FromSlash(g.RomPath))))
		a.folderButton(c, t, "封面目录", filepath.Join(a.root, "数据", "covers"))
	})
}

func (a *App) folderButton(c *ui.Context, t *ui.Theme, label, dir string) {
	b := ui.ButtonBase(c).Height(30).Radius(9).Grow(1).PaddingX(9).
		AlignItems(ui.Center).Justify(ui.Center).
		Border(1, ui.Hex("#ffffff24"))
	if b.Hovered() {
		b.Background(ui.Hex("#ffffff10")).Border(1, ui.Hex("#ffffff4d"))
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(t.Text.Alpha(0.85)).SingleLine().
			Grow(1).TextAlign(ui.Center)
	})
	b.Label("打开 " + label)
	if b.Clicked() {
		a.openFolder(dir)
	}
}

// openFolder shows a folder in Explorer; a folder that is not there says so in
// the status line instead of opening some other place.
func (a *App) openFolder(dir string) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		a.status = "目录不存在：" + dir
		return
	}
	cmd := exec.Command("explorer.exe", dir)
	if err := cmd.Start(); err != nil {
		a.status = "打不开目录：" + err.Error()
	}
}

// fieldValues is the menu after a click: what that field could be set to.
func (a *App) fieldValues(c *ui.Context, t *ui.Theme, g library.Game) {
	var label string
	for _, f := range a.fields(g) {
		if f.key == a.cardField {
			label = f.label
		}
	}
	ui.Row(c).Height(28).PaddingX(6).Gap(6).AlignItems(ui.Center).Children(func() {
		back := ui.ButtonBase(c).Height(22).PaddingX(8).Radius(7)
		if back.Hovered() {
			back.Background(ui.Hex("#ffffff12"))
		}
		back.Children(func() {
			ui.Text(c, "返回").FontSize(10.5).TextColor(t.TextMuted)
		})
		if back.Clicked() {
			a.cardField, a.cardEdit = "", false
		}
		ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(t.Text.Alpha(0.9))
		ui.Text(c, fmt.Sprint(len(a.values(a.cardField, g)))).FontSize(10).
			TextColor(t.TextMuted.Alpha(0.6))
	})

	vals := a.values(a.cardField, g)
	ui.Scroll(c).Height(min(float32(30*len(vals)+8), 216)).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			for _, v := range vals {
				v := v
				row := ui.ButtonBase(c).Height(28).PaddingX(9).Radius(8).
					AlignItems(ui.Center).Gap(8)
				if v == currentValue(a.fields(g), a.cardField) {
					row.Background(ui.Hex("#e8763a24"))
				} else if row.Hovered() {
					row.Background(ui.Hex("#ffffff10"))
				}
				row.Children(func() {
					ui.Text(c, v).FontSize(12).TextColor(t.Text).SingleLine().Grow(1)
					if v == currentValue(a.fields(g), a.cardField) {
						ui.Text(c, "✓").FontSize(10).TextColor(t.Accent)
					}
				})
				if row.DoubleClicked() {
					a.cardEdit, a.cardText = true, v
				} else if row.Clicked() {
					a.assign(a.cardField, v)
				}
			}
		})
	})
}

// fieldInput is the menu after a double click: the value typed in.
func (a *App) fieldInput(c *ui.Context, t *ui.Theme, g library.Game) {
	var label string
	for _, f := range a.fields(g) {
		if f.key == a.cardField {
			label = f.label
		}
	}
	ui.Row(c).Height(28).PaddingX(9).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).FontWeight(600).TextColor(t.Text.Alpha(0.9))
	})
	box := ui.Box(c).Height(32).PaddingX(10).Radius(9).
		Background(ui.Hex("#0d0f16a6")).Border(1, ui.Hex("#ffffff33")).
		AlignItems(ui.Center)
	box.Children(func() {
		in := ui.TextInputBase(c.Key("cardedit"), &a.cardText).Fill().FontSize(12).
			TextColor(t.Text).Placeholder("输入" + label).Label(label)
		if in.Submitted() {
			a.assign(a.cardField, a.cardText)
		}
	})
	ui.Row(c).Height(30).PaddingX(9).Gap(8).AlignItems(ui.Center).Children(func() {
		save := a.chipBase(c, 24, true)
		save.Children(func() {
			ui.Text(c, "保存").FontSize(11).FontWeight(600).TextColor(t.Text)
		})
		if save.Clicked() {
			a.assign(a.cardField, a.cardText)
		}
		cancel := a.chipBase(c, 24, false)
		cancel.Children(func() {
			ui.Text(c, "取消").FontSize(11).TextColor(t.TextMuted)
		})
		if cancel.Clicked() {
			a.cardField, a.cardEdit = "", false
		}
		ui.Text(c, "回车也行").FontSize(10).TextColor(t.TextMuted.Alpha(0.55))
	})
}

// currentValue is the value a field holds now, the one the list ticks.
func currentValue(fs []cardField, key string) string {
	for _, f := range fs {
		if f.key == key {
			return f.value
		}
	}
	return ""
}

// statusBar is the line under the grid: what happened, the offer to fill in
// missing covers, and what the bar's chips add up to.
func (a *App) statusBar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(26).Right(26).Bottom(12).Gap(12).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1)
		if a.fetching {
			ui.Text(c, fmt.Sprintf("补全中 %d/%d", a.fetched, a.fetchAll)).
				FontSize(11).FontWeight(500).TextColor(t.Accent)
		} else if a.missing > 0 {
			btn := a.chipBase(c, 26, false)
			if btn.Hovered() {
				btn.Border(1, ui.Hex("#ffffff4d"))
			}
			btn.Children(func() {
				ui.Text(c, fmt.Sprintf("补全封面 %d", a.missing)).FontSize(11).
					FontWeight(500).TextColor(t.Text)
			})
			if btn.Clicked() {
				a.fetchCovers()
			}
		}
		ui.Text(c, fmt.Sprintf("%s · %d 个游戏 · 双击或 Enter 启动",
			a.scopeName(), len(a.shown))).FontSize(11).
			TextColor(t.TextMuted).Opacity(0.6).SingleLine()
	})
}

func (a *App) message(c *ui.Context, t *ui.Theme, title, detail string) {
	ui.Column(c).Fill().Center().Gap(8).Children(func() {
		ui.Text(c, title).FontSize(16).FontWeight(600).TextColor(t.Text)
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted).SingleLine()
		}
	})
}

// initial is the letter or character a cover-less game shows instead of
// its cover.
func initial(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "?"
	}
	r, _ := utf8.DecodeRuneInString(title)
	return strings.ToUpper(string(r))
}
