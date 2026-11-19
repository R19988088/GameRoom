package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/library"
)

// view draws the window each frame: a floating bar that switches shelves
// (街机, 家用机, 掌机, PC), a row of platform and menu pills, and a grid of
// landscape covers.
func (a *App) view(c *ui.Context) {
	c.SetTheme(&a.theme)
	t := &a.theme
	c.Root().Background(t.Background)

	ui.Box(c).Fill().Children(func() {
		// The content starts under the floating bar and ends above the
		// status line, which the bar's shadow needs room for.
		ui.Column(c).Fill().Padding(98, 26, 40, 26).Children(func() {
			if a.lib == nil {
				a.message(c, t, "没有数据", a.status)
				return
			}
			a.chips(c, t)
			a.body(c, t)
		})
		a.statusBar(c, t)
		// Drawn last: a floating bar sits over the covers.
		a.toolbar(c, t)
	})
}

// toolbar is the floating bar: the four shelves on the left, the count and
// the search field on the right.
func (a *App) toolbar(c *ui.Context, t *ui.Theme) {
	bar := ui.Row(c).Absolute().Left(24).Top(18).Right(24).Height(60).
		Padding(8, 12).Radius(18).AlignItems(ui.Center).Gap(12).
		Background(ui.Hex("#16171ce8")).
		Border(1, ui.Hex("#ffffff14")).
		Shadow(0, 16, 40, 0, ui.Hex("#000000a6"))
	bar.Children(func() {
		ui.Row(c).Padding(4).Radius(13).Gap(2).AlignItems(ui.Center).
			Background(ui.Hex("#ffffff0a")).Children(func() {
			for _, g := range library.Groups {
				g := g
				count := 0
				if a.lib != nil {
					count = a.lib.GameCountOf(g.Key)
				}
				a.shelfTab(c, t, g.Name, count, g.Key == a.group, func() { a.chooseGroup(g.Key) })
			}
		})

		ui.Spacer(c)

		ui.Text(c, fmt.Sprintf("%s · %d 个游戏", a.platformName(), len(a.shown))).
			FontSize(11).TextColor(t.TextMuted).SingleLine()

		field := ui.Box(c).Width(228).Height(34).
			PaddingX(12).Radius(10).AlignItems(ui.Center).
			Background(ui.Hex("#ffffff0d")).Border(1, ui.Hex("#ffffff17"))
		field.Children(func() {
			in := ui.TextInputBase(c.Key("search"), &a.query).Fill().FontSize(12.5).
				TextColor(t.Text).Placeholder("搜索游戏名").Label("搜索游戏名")
			if in.Changed() {
				a.refilter()
			}
		})
	})
}

// shelfTab is one shelf in the bar's segmented control.
func (a *App) shelfTab(c *ui.Context, t *ui.Theme, name string, count int, chosen bool, run func()) {
	text := t.TextMuted
	countColor := t.TextMuted.Alpha(0.55)
	bg := ui.Hex("#00000000")
	weight := 500
	if chosen {
		text = t.Text
		countColor = t.TextMuted
		bg = ui.Hex("#ffffff1f")
		weight = 700
	}
	tab := ui.ButtonBase(c).Height(34).PaddingX(13).Radius(10).
		AlignItems(ui.Center).Background(bg)
	if !chosen && tab.Hovered() {
		tab.Background(ui.Hex("#ffffff12"))
	}
	tab.Children(func() {
		ui.Text(c, name).FontSize(13).FontWeight(weight).TextColor(text).SingleLine()
		if count > 0 {
			ui.Text(c, fmt.Sprint(count)).FontSize(10).TextColor(countColor)
		}
	})
	if tab.Clicked() {
		run()
	}
}

// chips lists the chosen shelf's platforms, then the platform's menus.
func (a *App) chips(c *ui.Context, t *ui.Theme) {
	ui.ScrollHorizontal(c).Height(38).ClipX().Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			for _, p := range a.lib.PlatformsOf(a.group) {
				p := p
				a.pill(c, t, p.Name, a.counts[p.Key], p.Key == a.platform, func() {
					a.choosePlatform(p.Key)
				})
			}

			menus := a.lib.MenusOf(a.platform)
			if len(menus) == 0 {
				return
			}
			ui.Box(c).Width(1).Height(16).Background(t.Border)
			a.pill(c, t, "全部", a.platformTotal, a.menu == "", func() { a.chooseMenu("") })
			for _, m := range menus {
				m := m
				a.pill(c, t, m.Name, a.menuCounts[m.Name], a.menu == m.Name, func() {
					a.chooseMenu(m.Name)
				})
			}
		})
	})
}

// pill is a platform or menu chip, or the "all" of a platform's menus.
func (a *App) pill(c *ui.Context, t *ui.Theme, name string, count int, chosen bool, run func()) {
	bg := ui.Hex("#ffffff08")
	text := t.TextMuted
	if chosen {
		bg = ui.Hex("#ffffff1c")
		text = t.Text
	}
	p := ui.ButtonBase(c).Height(30).PaddingX(12).Radius(15).
		AlignItems(ui.Center).Gap(7).Background(bg)
	if !chosen && p.Hovered() {
		p.Background(ui.Hex("#ffffff12"))
		p.Border(1, ui.Hex("#ffffff1f"))
	}
	p.Children(func() {
		ui.Text(c, name).FontSize(12).FontWeight(500).TextColor(text).SingleLine()
		if count > 0 {
			ui.Text(c, fmt.Sprint(count)).FontSize(10).TextColor(t.TextMuted.Alpha(0.7))
		}
	})
	if p.Clicked() {
		run()
	}
}

// body is the grid of the games that pass the platform, menu and search.
func (a *App) body(c *ui.Context, t *ui.Theme) {
	if len(a.shown) == 0 {
		a.message(c, t, "这里还没有游戏", "换个平台或分类，或检查 rom 目录")
		return
	}

	a.grid.Key = func(i int) any { return a.shown[i].ID }
	a.grid.Label = func(i int) string { return a.shown[i].Title }
	a.grid.Selected = &a.chosen
	ui.Column(c).Grow(1).Padding(0, 8, 0, 0).Children(func() {
		// 200 wide keeps five cards on a 1000-DIP window, as a shelf of
		// landscape covers reads; the cover itself keeps its own shape.
		grid := ui.GridView(c, &a.grid, len(a.shown), 200, 172, func(i int) {
			a.cell(c, t, i)
		})
		grid.Grow(1)
		if grid.Submitted() {
			a.launchChosen()
		}
	})
}

// cell is one game: its cover at its own aspect ratio, and its title. The
// grid draws the tint and ring of the chosen cell itself.
func (a *App) cell(c *ui.Context, t *ui.Theme, i int) {
	g := a.shown[i]
	chosen := i == a.chosen

	ui.Column(c).Padding(4).Gap(6).Children(func() {
		card := ui.Box(c).Height(128).Radius(12).
			Background(ui.Hex("#0b0b10")).Center().Clip()
		if chosen {
			card.Border(2, t.Accent)
		}
		card.Children(func() {
			if b := a.cover(a.coverAbs(g.Cover)); b != nil {
				// Contain, not Cover: a flyer keeps its own shape instead
				// of being cropped to the cell.
				ui.Image(c, b).Fill().Fit(ui.Contain).Radius(12)
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
}

// statusBar is the line under the grid.
func (a *App) statusBar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(26).Right(26).Bottom(12).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1)
		ui.Text(c, "双击或 Enter 启动 · 本版只启动 RetroArch").FontSize(11).
			TextColor(t.TextMuted).Opacity(0.55)
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
