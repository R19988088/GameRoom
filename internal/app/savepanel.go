package app

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

const (
	savePanelW   = 596
	savePanelPad = 24
	saveTileH    = 100
	saveCellH    = 118
	saveGap      = 10
	saveCols     = 3
	saveRows     = 3
	saveButtonW  = 88
)

// scrubDrag is the drag of the timeline: the entry it started on and how far the
// pointer has gone since, in DIPs. Dragging left goes back in time.
type scrubDrag struct {
	on     bool
	anchor int
	px     float32
}

// saveButton is the pill beside the look: the way into the save panel. It only
// appears for a game this process is stepping — a game handed to RetroArch's own
// window has no state here to save.
func (a *App) saveButton(c *ui.Context, t *ui.Theme) {
	if a.core == nil {
		return
	}
	b := ui.ButtonBase(c).Width(saveButtonW).Height(backButtonSize).Center().
		Radius(backButtonSize/2).
		Material(glass.Glass{Clarity: 0.5, Tint: ui.Hex("#0d0f168f")}).
		Border(1, ui.Hex("#ffffff1f"))
	if a.savesOpen {
		b.Background(ui.Hex("#e8763a2e")).Border(1, ui.Hex("#e8763a66"))
	} else if b.Hovered() {
		b.Background(ui.Hex("#ffffff1f")).Border(1, ui.Hex("#ffffff4d"))
	}
	b.Children(func() {
		ui.Text(c, "存档").FontSize(12).FontWeight(600).TextColor(t.Text.Alpha(0.9)).SingleLine()
	})
	if b.Clicked() {
		a.toggleSaves()
	}
}

// toggleSaves opens or closes the save panel. Opening stops the clock: the
// pictures in the tiles and the row of states behind them describe frames that
// must not move while they are looked at.
func (a *App) toggleSaves() {
	if a.core == nil {
		return
	}
	a.savesOpen = !a.savesOpen
	a.filtersOpen, a.padsOpen, a.cap = false, false, ""
	a.slotSel, a.slotNote = 0, ""
	a.scrub = scrubDrag{}
	if a.savesOpen {
		a.scanSaves()
		a.rew.freeze(true)
		a.core.Pause(true)
		return
	}
	a.closeSaves()
}

// closeSaves lets the game go on from wherever the panel left it, with a fresh
// row: what was rewound past is not offered again.
func (a *App) closeSaves() {
	a.savesOpen = false
	a.scrub = scrubDrag{}
	a.rew.reset()
	if a.core != nil {
		a.core.Pause(false)
	}
}

// saveTick closes the panel on Escape, the way the pad panel closes. It keeps
// its own record of the key, because the pad panel's ends with the pad panel.
func (a *App) saveTick() {
	keys := heldVKs()
	if !a.savesOpen {
		a.escSaves = keys[0x1B]
		return
	}
	if keys[0x1B] && !a.escSaves {
		a.toggleSaves()
	}
	a.escSaves = keys[0x1B]
}

// savePanel is the shelf of saves and the row of frames behind it: nine tiles,
// each a screenshot of a moment, and a timeline that steps the game back along
// the states the session kept.
func (a *App) savePanel(c *ui.Context, t *ui.Theme) {
	v := a.rew.view()
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Children(func() {
		col := ui.Column(c).Absolute().Right(edgeInset).Top(edgeInset).Width(savePanelW).
			Padding(16, savePanelPad, 16, savePanelPad).Radius(18).Gap(8).
			Material(glass.Glass{Clarity: 0.62, Tint: ui.Hex("#14161ee6")}).
			Border(1, ui.Hex("#ffffff1f")).
			Shadow(0, 26, 60, 0, ui.Hex("#000000cc"))
		col.Children(func() {
			a.saveHeader(c, t)
			for row := 0; row < saveRows; row++ {
				row := row
				ui.Row(c).Height(saveCellH).Gap(saveGap).Children(func() {
					for i := 0; i < saveCols; i++ {
						a.saveTile(c, t, row*saveCols+i+1)
					}
				})
			}
			a.rewindRow(c, t, v)
			a.saveFooter(c, t)
		})
	})
}

// saveHeader is the panel's own line: what it is, which game, and the way out.
func (a *App) saveHeader(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(28).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "存档").FontSize(12).FontWeight(700).
			TextColor(t.Text.Alpha(0.92)).SingleLine()
		ui.Text(c, a.playerGame.Title).FontSize(10).TextColor(t.TextMuted).
			Grow(1).MinWidth(0).SingleLine()
		close := ui.ButtonBase(c).Width(26).Height(26).Radius(13).Center().
			Background(ui.Hex("#ffffff14"))
		close.Children(func() {
			ui.Text(c, "✕").FontSize(12).TextColor(t.TextMuted)
		})
		if close.Clicked() {
			a.toggleSaves()
		}
	})
}

// saveTile is one slot: the picture of the moment, and under it the slot's number
// and when it was taken. An empty tile is the way to make a save — clicking it
// writes the game as it stands now — and a full one is only selected, so reading
// it is a second, deliberate press. A core that cannot give its state has no tiles
// to press: every slot shows a cross, because there is nothing to write into it.
func (a *App) saveTile(c *ui.Context, t *ui.Theme, n int) {
	slot := a.slotAt(n)
	savable := a.canSave()
	sel := a.slotSel == n && savable
	cell := ui.ButtonBase(c).Grow(1).Basis(0).Radius(10)
	if cell.Hovered() && savable {
		cell.Background(ui.Hex("#ffffff0a"))
	}
	cell.Children(func() {
		ui.Column(c).Fill().Gap(4).Children(func() {
			shot := ui.Box(c).Height(saveTileH).Radius(8).
				Background(ui.Hex("#0b0b10")).Clip()
			if sel {
				shot.Border(2, t.Accent)
			} else if slot != nil {
				shot.Border(1, ui.Hex("#ffffff24"))
			}
			shot.Children(func() {
				switch {
				case slot != nil && slot.Shot != nil:
					ui.Image(c, slot.Shot).Fill().Fit(ui.FillBox).Radius(8)
				case !savable:
					// The core has no state: the slot is not a way in, it is a
					// statement that there is nothing to put there.
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, "✕").FontSize(22).TextColor(ui.Hex("#ff453a80"))
					})
				default:
					ui.Column(c).Fill().Center().Gap(2).Children(func() {
						ui.Text(c, "＋").FontSize(15).TextColor(t.TextMuted.Alpha(0.5))
						ui.Text(c, "建档").FontSize(9).TextColor(t.TextMuted.Alpha(0.45))
					})
				}
			})
			ui.Row(c).Height(14).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, fmt.Sprint(n)).FontSize(9.5).
					TextColor(t.TextMuted.Alpha(0.7)).SingleLine()
				switch {
				case slot != nil:
					ui.Text(c, slot.At.Format("01-02 15:04")).FontSize(9.5).
						TextColor(t.TextMuted.Alpha(0.55)).SingleLine()
				case !savable:
					ui.Text(c, "不支持").FontSize(9.5).
						TextColor(ui.Hex("#ff453a73")).SingleLine()
				default:
					ui.Text(c, "空位").FontSize(9.5).
						TextColor(t.TextMuted.Alpha(0.45)).SingleLine()
				}
			})
		})
	})
	if !savable {
		if cell.Clicked() {
			a.slotNote = "这个核心不能存档"
		}
		return
	}
	if cell.Clicked() {
		if slot == nil {
			a.saveInto(n)
			return
		}
		a.slotSel = n
		a.slotNote = ""
	}
}

// rewindRow is the timeline's own line: how much of the game the row covers, how
// far back the picture is, and the way out of the past.
func (a *App) rewindRow(c *ui.Context, t *ui.Theme, v rewindView) {
	ui.Row(c).Height(24).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "时间轴").FontSize(10).FontWeight(600).
			TextColor(t.Text.Alpha(0.85)).SingleLine()
		if v.Count > 1 {
			ui.Text(c, fmt.Sprintf("覆盖 %d 帧", v.Span)).FontSize(9.5).
				TextColor(t.TextMuted.Alpha(0.55)).SingleLine()
		} else if v.Note != "" {
			ui.Text(c, v.Note).FontSize(9.5).
				TextColor(t.TextMuted.Alpha(0.65)).SingleLine()
		} else {
			ui.Text(c, "还没有可回退的帧").FontSize(9.5).
				TextColor(t.TextMuted.Alpha(0.55)).SingleLine()
		}
		ui.Box(c).Grow(1)
		if v.Back > 0 {
			ui.Text(c, fmt.Sprintf("已回退 %d 帧", v.Back)).FontSize(10.5).
				TextColor(t.Accent).SingleLine()
		}
		cont := ui.ButtonBase(c).Width(58).Height(24).Radius(12).Center().
			Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#e8763a33")}).
			Border(1, ui.Hex("#e8763a66"))
		cont.Children(func() {
			ui.Text(c, "继续").FontSize(10.5).TextColor(t.Text.Alpha(0.9)).SingleLine()
		})
		if cont.Clicked() {
			a.closeSaves()
		}
	})
	a.rewindTrack(c, t, v)
}

// rewindTrack is the timeline itself: a bar as long as the row of states, a
// handle at the one the picture is at, and the live edge at its right end.
// Dragging it left steps the game back, one capture per its share of the bar.
func (a *App) rewindTrack(c *ui.Context, t *ui.Theme, v rewindView) {
	track := ui.ButtonBase(c).Height(44).Grow(1)
	track.Draw(func(p *ui.Painter, r ui.Rect) {
		a.trackW = r.W
		bar := ui.Rect{X: r.X, Y: r.Y + 17, W: r.W, H: 10}
		p.Fill(bar, ui.Hex("#ffffff14"), 5)
		if v.Count <= 1 {
			return
		}
		pos := func(i int) float32 {
			return r.X + r.W*float32(i)/float32(v.Count-1)
		}
		if v.Idx >= 0 {
			mine := pos(v.Idx)
			p.Fill(ui.Rect{X: r.X, Y: r.Y + 17, W: mine - r.X, H: 10}, ui.Hex("#e8763a66"), 5)
			p.Fill(ui.Rect{X: mine - 3, Y: r.Y + 8, W: 6, H: 28}, t.Accent, 3)
		}
		// The live edge: where the game would be if it were let go.
		p.Fill(ui.Rect{X: r.X + r.W - 2, Y: r.Y + 12, W: 2, H: 20}, ui.Hex("#ffffff3d"), 1)
	})
	if v.Count <= 1 {
		return
	}
	dx, _, held := track.Dragged()
	if !held {
		a.scrub.on = false
		return
	}
	if !a.scrub.on {
		a.scrub.on = true
		a.scrub.anchor = v.Idx
		if a.scrub.anchor < 0 {
			a.scrub.anchor = v.Count - 1
		}
		a.scrub.px = 0
	}
	a.scrub.px += dx
	per := a.trackW / float32(v.Count-1)
	if per < 1 {
		per = 1
	}
	target := a.scrub.anchor + int(math.Round(float64(a.scrub.px/per)))
	if target < 0 {
		target = 0
	}
	if target > v.Count-1 {
		target = v.Count - 1
	}
	if target != v.Idx {
		a.rewindTo(target)
	}
}

// saveFooter is what can be done with a tile that is selected: read it back, or
// take it off the shelf.
func (a *App) saveFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(24).AlignItems(ui.Center).Gap(8).Children(func() {
		note := a.slotNote
		if note == "" && !a.canSave() {
			note = "这个核心不能存档"
		}
		if note != "" {
			ui.Text(c, note).Grow(1).MinWidth(0).FontSize(9.5).
				TextColor(t.TextMuted.Alpha(0.65)).SingleLine()
		} else {
			ui.Box(c).Grow(1)
		}
		read := ui.ButtonBase(c).Width(58).Height(24).Radius(12).Center().
			Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#e8763a33")}).
			Border(1, ui.Hex("#e8763a66"))
		read.Children(func() {
			ui.Text(c, "读取").FontSize(10.5).TextColor(t.Text.Alpha(0.9)).SingleLine()
		})
		if a.slotSel == 0 {
			read.Disabled(true)
		} else if read.Clicked() {
			a.loadFrom(a.slotSel)
		}
		del := ui.ButtonBase(c).Width(58).Height(24).Radius(12).Center().
			Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#ff453a1a")}).
			Border(1, ui.Hex("#ff453a55"))
		del.Children(func() {
			ui.Text(c, "删除").FontSize(10.5).TextColor(t.Text.Alpha(0.85)).SingleLine()
		})
		if a.slotSel == 0 {
			del.Disabled(true)
		} else if del.Clicked() {
			a.deleteSlot(a.slotSel)
		}
	})
}
