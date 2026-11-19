package app

import (
	"fmt"
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
		a.searchField(c, t)
	})
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
	if a.axis == library.AxisMachine || a.axis == "" {
		if len(a.chips) == 0 {
			top := a.topPlatforms(5)
			if len(top) < 2 {
				return
			}
			for _, p := range top {
				p := p
				a.chip(c, t, p.Name, a.counts[p.Key], p.Key == a.platform, func() {
					a.choosePlatform(p.Key)
				})
			}
			if rest := a.restPlatforms(5); len(rest) > 0 {
				a.morePlatforms(c, t, rest)
			}
			return
		}
		a.chip(c, t, "全部", a.shelfTotal, a.menu == "", func() { a.chooseMenu("") })
		for _, m := range a.chips {
			m := m
			a.chip(c, t, m.Name, m.Count, a.menu == m.Name, func() { a.chooseMenu(m.Name) })
		}
		return
	}

	a.chip(c, t, "全部", a.shelfTotal, a.menu == "", func() { a.chooseMenu("") })
	for _, m := range a.chips {
		m := m
		a.chip(c, t, m.Name, m.Count, a.menu == m.Name, func() { a.chooseMenu(m.Name) })
	}
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

// restPlatforms are the ones 更多 holds.
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

// morePlatforms is the 更多 chip: the shelf's platforms that did not fit,
// with their counts.
func (a *App) morePlatforms(c *ui.Context, t *ui.Theme, rest []library.Platform) {
	anchor := a.chipBase(c, 32, false)
	if anchor.Hovered() {
		anchor.Border(1, ui.Hex("#ffffff4d"))
	}
	anchor.Children(func() {
		ui.Text(c, "更多").FontSize(12).FontWeight(500).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, fmt.Sprint(len(rest))).FontSize(10).TextColor(t.TextMuted.Alpha(0.7))
	})
	if anchor.Clicked() {
		a.moreOpen = !a.moreOpen
		a.shelfOpen = false
	}

	ui.PopoverBase(c, anchor, &a.moreOpen, func(panel ui.Element) {
		panel.Width(232).Padding(7).Radius(16).
			Background(ui.Hex("#17181ff5")).
			Border(1, ui.Hex("#ffffff1f")).
			Shadow(0, 22, 48, 0, ui.Hex("#000000b8"))
		panel.Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, p := range rest {
					p := p
					row := ui.ButtonBase(c).Height(36).PaddingX(11).Radius(10).
						AlignItems(ui.Center).Gap(8)
					if p.Key == a.platform {
						row.Background(ui.Hex("#e8763a1f"))
					} else if row.Hovered() {
						row.Background(ui.Hex("#ffffff12"))
					}
					row.Children(func() {
						color := t.TextMuted
						if p.Key == a.platform {
							color = t.Accent
						}
						ui.Text(c, p.Name).FontSize(12.5).FontWeight(600).
							TextColor(color).SingleLine().Grow(1)
						ui.Text(c, fmt.Sprint(a.counts[p.Key])).FontSize(10).
							TextColor(t.TextMuted.Alpha(0.7))
					})
					if row.Clicked() {
						a.moreOpen = false
						a.choosePlatform(p.Key)
					}
				}
			})
		})
	})
}

// searchField is the right pill: the same short pill as the chips, with the
// field sitting directly on its one plate of glass.
func (a *App) searchField(c *ui.Context, t *ui.Theme) {
	plate := a.chipBase(c, 32, false)
	plate.Width(248).PaddingX(11)
	plate.Children(func() {
		in := ui.TextInputBase(c.Key("search"), &a.query).Fill().FontSize(12).
			TextColor(t.Text).Placeholder("搜索游戏名").Label("搜索游戏名").
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
	cardH   = 172
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
// holds. Clicking it files the shelf by that value alone.
func (a *App) header(c *ui.Context, t *ui.Theme, ln *line) {
	s := a.sections[ln.head]
	r := ui.Row(c).Absolute().Left(4).Right(0).Top(ln.top).Height(headH).
		Gap(8).AlignItems(ui.Center)
	if s.name == "" {
		return
	}
	r.Children(func() {
		ui.Text(c, s.name).FontSize(13).FontWeight(700).
			TextColor(t.Text.Alpha(0.92)).SingleLine()
		ui.Text(c, fmt.Sprint(s.count)).FontSize(10.5).
			TextColor(t.TextMuted.Alpha(0.6))
	})
	if r.Clicked() {
		a.chooseMenu(s.name)
	}
	if r.Hovered() {
		r.Background(ui.Hex("#ffffff0a"))
	}
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
		ui.Column(c).Fill().Gap(6).Children(func() {
			cover := ui.Box(c).Height(128).Radius(10).
				Background(ui.Hex("#0b0b10")).Center().Clip()
			if chosen {
				cover.Border(2, t.Accent)
			}
			cover.Children(func() {
				if bmp := a.cover(a.coverAbs(g.Cover)); bmp != nil {
					// Contain, not Cover: a flyer keeps its own shape
					// instead of being cropped to the card.
					ui.Image(c, bmp).Fill().Fit(ui.Contain).Radius(10)
				} else {
					ui.Text(c, initial(g.Title)).FontSize(30).FontWeight(700).
						TextColor(t.TextMuted.Alpha(0.35))
				}
			})

			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, g.Title).FontSize(12).FontWeight(500).TextColor(t.Text).
					SingleLine().Grow(1)
				if g.Favorite {
					ui.Text(c, "★").FontSize(9).TextColor(t.Accent)
				}
			})
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
				ui.Text(c, "右键").FontSize(10).TextColor(t.TextMuted.Alpha(0.55))
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
	ui.Row(c).Height(26).PaddingX(9).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "单击选择 · 双击输入").FontSize(10).TextColor(t.TextMuted.Alpha(0.55))
	})
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
