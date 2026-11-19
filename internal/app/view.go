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

// view draws the window each frame: covers fill the window and scroll under
// the floating bar, the platform's menus float below it, and the bar itself
// is a plate of glass the covers show through.
func (a *App) view(c *ui.Context) {
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

// toolbar is the top row: three separate capsules of glass rather than one
// long bar — the shelf picker, the shelf's platforms and menus, and the
// search. The cover art scrolls behind all three.
func (a *App) toolbar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Absolute().Left(24).Top(18).Right(24).Height(60).Gap(14).
		AlignItems(ui.Stretch).Children(func() {
		// The picker is a capsule of its own, not content inside one.
		a.shelfSwitch(c, t)

		mid := a.capsule(ui.Row(c).Grow(1).MinWidth(0).Padding(6, 10, 6, 10).AlignItems(ui.Center))
		mid.Children(func() { a.contentRow(c, t) })

		find := a.capsule(ui.Row(c).Width(320).PaddingX(12).PaddingY(11).AlignItems(ui.Center))
		find.Children(func() { a.searchField(c, t) })
	})
}

// capsule is one of the three pills on the top row: a plate of glass the
// covers show through, blurred.
func (a *App) capsule(el ui.Element) ui.Element {
	return el.Radius(18).
		Material(glass.Glass{Clarity: 0.55, Tint: ui.Hex("#0d0f168f")}).
		Border(1, ui.Hex("#ffffff1f")).
		Shadow(0, 16, 40, 0, ui.Hex("#000000a6"))
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

	anchor := a.capsule(ui.ButtonBase(c).Height(60).PaddingX(18).AlignItems(ui.Center).Gap(9))
	if anchor.Hovered() {
		anchor.Border(1, ui.Hex("#ffffff3d"))
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

// contentRow is what the middle capsule holds: the shelf's five busiest
// platforms when it has more than one (the rest behind 更多), then a rule,
// then the chosen platform's menus. Both scroll sideways inside the capsule.
func (a *App) contentRow(c *ui.Context, t *ui.Theme) {
	ui.ScrollHorizontal(c).Fill().ClipX().Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			top := a.topPlatforms(5)
			if len(top) > 1 {
				for _, p := range top {
					p := p
					a.pill(c, t, p.Name, a.counts[p.Key], p.Key == a.platform, func() {
						a.choosePlatform(p.Key)
					})
				}
				if rest := a.restPlatforms(5); len(rest) > 0 {
					a.morePlatforms(c, t, rest)
				}
				ui.Box(c).Width(1).Height(20).Background(ui.Hex("#ffffff17"))
			}
			if a.lib == nil {
				return
			}
			a.pill(c, t, "全部", a.platformTotal, a.menu == "", func() { a.chooseMenu("") })
			for _, m := range a.lib.MenusOf(a.platform) {
				m := m
				a.pill(c, t, m.Name, a.menuCounts[m.Name], a.menu == m.Name, func() {
					a.chooseMenu(m.Name)
				})
			}
		})
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

// searchField is the right capsule: a lens of liquid glass inside the
// capsule, so the covers behind it blur twice.
func (a *App) searchField(c *ui.Context, t *ui.Theme) {
	field := ui.Box(c).Height(38).Fill().PaddingX(13).Radius(12).
		AlignItems(ui.Center).
		Material(glass.Glass{Clarity: 0.6, Tint: ui.Hex("#ffffff14")}).
		Border(1, ui.Hex("#ffffff24"))
	field.Children(func() {
		in := ui.TextInputBase(c.Key("search"), &a.query).Fill().FontSize(12.5).
			TextColor(t.Text).Placeholder("搜索游戏名").Label("搜索游戏名").
			PlaceholderColor(t.TextMuted.Alpha(0.65))
		if in.Changed() {
			a.refilter()
		}
	})
}

// pill is a chip in the middle capsule: a platform, or one of the menus.
func (a *App) pill(c *ui.Context, t *ui.Theme, name string, count int, chosen bool, run func()) {
	a.chip(c, t, name, count, chosen, ui.Hex("#ffffff0d"), ui.Hex("#ffffff1a"), ui.Hex("#ffffff26"), run)
}

func (a *App) chip(c *ui.Context, t *ui.Theme, name string, count int, chosen bool, bg, hover, hoverBorder ui.Color, run func()) {
	text := t.TextMuted
	if chosen {
		bg = ui.Hex("#ffffff26")
		text = t.Text
	}
	p := ui.ButtonBase(c).Height(30).PaddingX(12).Radius(15).
		AlignItems(ui.Center).Gap(7).Background(bg)
	if chosen {
		p.Border(1, ui.Hex("#ffffff33"))
	}
	if !chosen && p.Hovered() {
		p.Background(hover)
		p.Border(1, hoverBorder)
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

// body is the grid of the games that pass the platform, menu and search. It
// owns the window: its first row sits under the floating bar and the menus,
// and scrolling carries the covers up behind them.
func (a *App) body(c *ui.Context, t *ui.Theme) {
	if len(a.shown) == 0 {
		a.message(c, t, "这里还没有游戏", "换个平台或分类，或检查 rom 目录")
		return
	}

	a.grid.Key = func(i int) any { return a.shown[i].ID }
	a.grid.Label = func(i int) string { return a.shown[i].Title }
	a.grid.Selected = &a.chosen
	ui.Column(c).Absolute().Left(26).Top(26).Right(26).Bottom(34).Children(func() {
		// 200 wide keeps five cards on a 1000-DIP window, as a shelf of
		// landscape covers reads; the cover itself keeps its own shape.
		grid := ui.GridView(c, &a.grid, len(a.shown), 200, 172, func(i int) {
			a.cell(c, t, i)
		})
		// The bars float over the top 96 DIPs, so the grid's content starts
		// below them and scrolls up underneath: padding here scrolls with
		// the content, unlike the space around it.
		grid.Fill().Padding(64, 0, 0, 0)
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
			btn := ui.ButtonBase(c).Height(24).PaddingX(11).Radius(12).
				AlignItems(ui.Center).Background(ui.Hex("#ffffff10")).
				Border(1, ui.Hex("#ffffff1a"))
			if btn.Hovered() {
				btn.Background(ui.Hex("#ffffff1f"))
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
