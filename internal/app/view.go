package app

import (
	"fmt"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"sort"
	"strings"
	"unicode/utf8"

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

// toolbar is the floating bar in three parts: the shelf picker on the left,
// the chosen shelf's platforms in the middle (five of them, the rest behind
// 更多), and the search field on the right.
func (a *App) toolbar(c *ui.Context, t *ui.Theme) {
	bar := ui.Row(c).Absolute().Left(24).Top(18).Right(24).Height(60).
		Padding(9, 10, 9, 10).Radius(18).AlignItems(ui.Center).Gap(10).
		Background(ui.Hex("#141519d9")).
		Border(1, ui.Hex("#ffffff14")).
		Shadow(0, 16, 40, 0, ui.Hex("#000000a6"))
	bar.Children(func() {
		a.shelfSwitch(c, t)
		ui.Box(c).Width(1).Height(24).Background(ui.Hex("#ffffff10"))
		a.platformRow(c, t)
		a.searchField(c, t)
	})
}

// shelfSwitch is one button naming the shelf shown; its menu lists all four,
// so the bar no longer carries a row of shelf tabs.
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

	anchor := ui.ButtonBase(c).Height(38).PaddingX(14).Radius(12).
		AlignItems(ui.Center).Gap(9).Background(ui.Hex("#ffffff14"))
	if anchor.Hovered() {
		anchor.Background(ui.Hex("#ffffff1f"))
	}
	anchor.Children(func() {
		ui.Text(c, name).FontSize(13).FontWeight(700).TextColor(t.Text).SingleLine()
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

// platformRow is the middle: the shelf's five busiest platforms, and a
// 更多 button for the rest.
func (a *App) platformRow(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Grow(1).Gap(6).AlignItems(ui.Center).Children(func() {
		for _, p := range a.topPlatforms(5) {
			p := p
			a.pill(c, t, p.Name, a.counts[p.Key], p.Key == a.platform, func() {
				a.choosePlatform(p.Key)
			})
		}
		rest := a.restPlatforms(5)
		if len(rest) > 0 {
			a.morePlatforms(c, t, rest)
		}
	})
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

// morePlatforms is the 更多 button: the shelf's platforms that did not fit,
// with their counts.
func (a *App) morePlatforms(c *ui.Context, t *ui.Theme, rest []library.Platform) {
	anchor := ui.ButtonBase(c).Height(30).PaddingX(12).Radius(15).
		AlignItems(ui.Center).Gap(7).Background(ui.Hex("#ffffff08"))
	if anchor.Hovered() {
		anchor.Background(ui.Hex("#ffffff16"))
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

// searchField is the right end: a lens of liquid glass over the covers.
func (a *App) searchField(c *ui.Context, t *ui.Theme) {
	field := ui.Box(c).Width(236).Height(38).PaddingX(13).Radius(12).
		AlignItems(ui.Center).
		Material(glass.Glass{Clarity: 0.6, Tint: ui.Hex("#ffffff1a")}).
		Border(1, ui.Hex("#ffffff20"))
	field.Children(func() {
		in := ui.TextInputBase(c.Key("search"), &a.query).Fill().FontSize(12.5).
			TextColor(t.Text).Placeholder("搜索游戏名").Label("搜索游戏名").
			PlaceholderColor(t.TextMuted.Alpha(0.65))
		if in.Changed() {
			a.refilter()
		}
	})
}

// chips lists the chosen platform's menus, the row under the bar.
func (a *App) chips(c *ui.Context, t *ui.Theme) {
	if a.lib == nil {
		return
	}
	menus := a.lib.MenusOf(a.platform)
	if len(menus) == 0 {
		return
	}
	ui.ScrollHorizontal(c).Height(38).ClipX().Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
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

// statusBar is the line under the grid: what happened, and what the bar's
// chips add up to.
func (a *App) statusBar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(26).Right(26).Bottom(12).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1)
		ui.Text(c, fmt.Sprintf("%s · %d 个游戏 · 双击或 Enter 启动",
			a.platformName(), len(a.shown))).FontSize(11).
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
