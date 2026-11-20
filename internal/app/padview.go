package app

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// controllerButton is the pill beside the filter's: which machine's pad the
// game reads, and the way into the model of its buttons.
func (a *App) controllerButton(c *ui.Context, t *ui.Theme) {
	b := ui.ButtonBase(c).Width(controllerButtonWidth).Height(backButtonSize).PaddingX(14).
		Radius(backButtonSize/2).AlignItems(ui.Center).Gap(8).
		Material(glass.Glass{Clarity: 0.5, Tint: ui.Hex("#0d0f168f")}).
		Border(1, ui.Hex("#ffffff1f"))
	if a.padsOpen {
		b.Background(ui.Hex("#e8763a2e")).Border(1, ui.Hex("#e8763a66"))
	} else if b.Hovered() {
		b.Background(ui.Hex("#ffffff1f")).Border(1, ui.Hex("#ffffff4d"))
	}
	b.Children(func() {
		ui.Text(c, "控制器").FontSize(12).FontWeight(600).TextColor(t.Text.Alpha(0.9)).SingleLine()
		ui.Text(c, padShort(a.padLive)).FontSize(11).TextColor(t.TextMuted).SingleLine()
	})
	if b.Clicked() {
		a.win.Update(a.togglePads)
	}
}

// padShort is what the pill calls a machine.
func padShort(id string) string {
	switch id {
	case "fc":
		return "FC"
	case "sfc":
		return "SFC"
	case "gb":
		return "GB"
	case "gba":
		return "GBA"
	case "ngp":
		return "NGP"
	case "md":
		return "MD"
	case "ss":
		return "SS"
	case "ps":
		return "PS1"
	case "psp":
		return "PSP"
	case "3do":
		return "3DO"
	case "mvs":
		return "MVS"
	}
	// An arcade board is called by its own name: CPS2, CPSIII, ST-V…
	if l := padLayoutFor(id); l != nil {
		return l.Name
	}
	return "键盘"
}

// padPanel is the model of the pad, over the game it belongs to. It follows the
// game that is running — the machine is read from the game itself — so there is
// nothing to pick by hand: the model shown is the pad being played, and a click
// on one of its buttons waits for the key or the gamepad button to take its
// place. The bindings are filed per machine, so every game of that machine
// plays the same way.
func (a *App) padPanel(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Children(func() {
		col := ui.Column(c).Absolute().Right(edgeInset).Top(edgeInset).Width(508).
			Padding(12).Radius(18).Gap(10).
			Material(glass.Glass{Clarity: 0.62, Tint: ui.Hex("#14161ee6")}).
			Border(1, ui.Hex("#ffffff1f")).
			Shadow(0, 26, 60, 0, ui.Hex("#000000cc"))
		col.Children(func() {
			a.padHeader(c, t)
			if l := padLayoutFor(a.padLive); l != nil {
				a.padModel(c, t, l)
			} else {
				ui.Box(c).Width(padModelW).Height(96).Center().
					Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#ffffff0d")}).
					Border(1, ui.Hex("#ffffff14")).Radius(14).Children(func() {
					ui.Text(c, "这个机种用键盘，不需要手柄映射").FontSize(11).TextColor(t.TextMuted)
				})
			}
			a.padFooter(c, t)
		})
	})
}

// padHeader is the panel's title row: the machine the panel is showing, dead
// centre of the panel, and the way out. While a button waits for a source, the
// title turns into the wait itself.
func (a *App) padHeader(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(26).AlignItems(ui.Center).Children(func() {
		// A spacer as wide as the close button, so the title sits in the middle
		// of the panel rather than in the middle of what is left.
		ui.Box(c).Width(26)
		ui.Box(c).Grow(1).Center().Children(func() {
			if a.cap != "" {
				ui.Text(c, "按下键盘或手柄上的按键…").FontSize(10.5).
					TextColor(ui.Hex("#f3a06b")).SingleLine()
				return
			}
			name := "键盘"
			if l := padLayoutFor(a.padLive); l != nil {
				name = l.Name
			}
			ui.Text(c, name).FontSize(12).FontWeight(700).
				TextColor(t.Text.Alpha(0.92)).SingleLine()
		})
		close := ui.ButtonBase(c).Width(26).Height(26).Radius(13).Center().
			Background(ui.Hex("#ffffff14"))
		close.Children(func() {
			ui.Text(c, "✕").FontSize(12).TextColor(t.TextMuted)
		})
		if close.Clicked() {
			a.win.Update(a.togglePads)
		}
	})
}

// padModel draws the machine's pad: a body, the d-pad on the left, and its own
// buttons where they sit on the real thing. The body is as wide as the panel's
// contents, so its frame lines up with the rows above and below it.
func (a *App) padModel(c *ui.Context, t *ui.Theme, l *padLayout) {
	ui.Box(c).Width(padModelW).Height(padModelH).Children(func() {
		ui.Box(c).Fill().Radius(20).Background(ui.Hex("#0a0c12d9")).
			Border(1, ui.Hex("#ffffff12"))
		ui.Box(c).Absolute().Left(6).Top(6).Right(6).Bottom(6).Radius(16).
			Background(ui.Hex("#1b1f29f2")).Border(1, ui.Hex("#ffffff22"))
		// The cross of the d-pad: two arms in one flat tone, so the middle is a
		// join and not a square where two bars pile up, with a triangle on each
		// arm.
		ui.Box(c).Absolute().Left(dpadX - 15).Top(dpadY - 45).Width(30).Height(90).Radius(6).
			Background(ui.Hex("#2b303c"))
		ui.Box(c).Absolute().Left(dpadX - 39).Top(dpadY - 15).Width(78).Height(30).Radius(6).
			Background(ui.Hex("#2b303c"))
		// A machine with a stick gets its well beside the cross: the stick's four
		// directions are read apart from the cross's, so they get a well of their
		// own rather than sitting on the same arms.
		for _, s := range l.Spots {
			if s.Kind == "stick" {
				ui.Box(c).Absolute().Left(stickX-42).Top(stickY-42).Width(84).Height(84).
					Radius(42).Background(ui.Hex("#00000030")).Border(1, ui.Hex("#ffffff22"))
				ui.Box(c).Absolute().Left(stickX - 34).Top(stickY - 34).Width(68).Height(68).
					Radius(34).Background(ui.Hex("#ffffff0f"))
				break
			}
		}
		for _, s := range l.Spots {
			a.padSpot(c, t, l, s)
		}
	})
}

// padSpot is one button of the model. It shows what it reads from — the key's
// face, the gamepad's button, or both — and the machine's own name for it.
func (a *App) padSpot(c *ui.Context, t *ui.Theme, l *padLayout, s padSpot) {
	waiting := a.cap == s.ID
	p := a.padProfile(l.ID)[s.ID]
	face := padFace(p)
	combo := len(targetsOf(s)) > 1

	b := ui.ButtonBase(c).Absolute().Left(s.X).Top(s.Y).Width(s.W).Height(s.H)
	switch s.Kind {
	case "dir":
		// An arm of the cross: the triangle says which way. A key the player gave
		// it reads under it; the arrow it already carries is not written twice.
		b.Radius(6).Center()
		if waiting {
			b.Background(ui.Hex("#e8763a3b")).Border(1, ui.Hex("#e8763aa6"))
		} else if b.Hovered() {
			b.Background(ui.Hex("#ffffff1f"))
		}
		b.Children(func() {
			ui.Column(c).Center().Gap(0).Children(func() {
				ui.Text(c, s.Label).FontSize(10).TextColor(t.Text.Alpha(0.8)).SingleLine()
				if !isDefaultFace(p, s.ID) {
					ui.Text(c, faceText(face, waiting)).FontSize(7).
						FontWeight(700).TextColor(textColorFor(t, waiting)).SingleLine()
				}
			})
		})
	case "stick":
		// A direction of the stick: the same triangle, in the well, with the
		// stick's own binding under it.
		b.Radius(5).Center()
		if waiting {
			b.Background(ui.Hex("#e8763a3b")).Border(1, ui.Hex("#e8763aa6"))
		} else if b.Hovered() {
			b.Background(ui.Hex("#ffffff1f"))
		}
		b.Children(func() {
			ui.Column(c).Center().Gap(0).Children(func() {
				ui.Text(c, s.Label).FontSize(9).TextColor(t.Text.Alpha(0.72)).SingleLine()
				if !isDefaultFace(p, s.ID) {
					ui.Text(c, faceText(face, waiting)).FontSize(6.5).
						FontWeight(700).TextColor(textColorFor(t, waiting)).SingleLine()
				}
			})
		})
	case "hold":
		// The stick's hold key, in the middle of the well: a key like the others,
		// and it reads orange while it is keeping a direction on.
		b.Radius(5).Center()
		switch {
		case waiting:
			b.Background(ui.Hex("#e8763a3b")).Border(1, ui.Hex("#e8763aa6"))
		case a.holdOn != "":
			b.Background(ui.Hex("#e8763a2e")).Border(1, ui.Hex("#e8763aa6"))
		case b.Hovered():
			b.Background(ui.Hex("#ffffff1f")).Border(1, ui.Hex("#ffffff40"))
		}
		b.Children(func() {
			ui.Column(c).Center().Gap(0).Children(func() {
				ui.Text(c, s.Label).FontSize(7.5).TextColor(t.Text.Alpha(0.7)).SingleLine()
				ui.Text(c, faceText(face, waiting)).FontSize(6.5).
					FontWeight(700).TextColor(textColorFor(t, waiting)).SingleLine()
			})
		})
	case "pill":
		b.Radius(s.H / 2).AlignItems(ui.Center).PaddingX(8)
		if waiting {
			b.Background(ui.Hex("#e8763a3b")).Border(1, ui.Hex("#e8763aa6"))
		} else {
			// A macro is a combination, not a button of the machine: it reads
			// softer until the player gives it a source of its own.
			bg, bd := "#ffffff12", "#ffffff26"
			if combo {
				bg, bd = "#ffffff08", "#ffffff14"
			}
			b.Background(ui.Hex(bg)).Border(1, ui.Hex(bd))
			if b.Hovered() {
				b.Background(ui.Hex("#ffffff1f")).Border(1, ui.Hex("#ffffff40"))
			}
		}
		b.Children(func() {
			ui.Text(c, s.Label).FontSize(9).TextColor(t.TextMuted).SingleLine()
			ui.Box(c).Grow(1)
			ui.Text(c, faceText(face, waiting)).FontSize(9.5).
				FontWeight(600).TextColor(textColorFor(t, waiting)).SingleLine()
		})
	default: // a round button of the machine's own
		b.Radius(s.W / 2).Center()
		if waiting {
			b.Background(ui.Hex("#e8763a3b")).Border(1, ui.Hex("#e8763aa6"))
		} else {
			b.Background(ui.Hex("#ffffff12")).Border(1, ui.Hex("#ffffff30"))
			if b.Hovered() {
				b.Background(ui.Hex("#ffffff1f")).Border(1, ui.Hex("#ffffff4d"))
			}
		}
		b.Children(func() {
			// The machine's own name reads first: it says which button this is;
			// what it is bound to reads under it.
			ui.Column(c).Center().Gap(1).Children(func() {
				ui.Text(c, s.Label).FontSize(10.5).
					FontWeight(600).TextColor(t.Text.Alpha(0.92)).SingleLine()
				ui.Text(c, faceText(face, waiting)).FontSize(7.5).
					TextColor(textColorFor(t, waiting)).SingleLine()
			})
		})
	}
	if b.Clicked() {
		a.startCapture(s.ID)
	}
}

// faceText is what a button shows: its source, or a mark while it waits.
func faceText(face string, waiting bool) string {
	if waiting {
		return "…"
	}
	return face
}

// isDefaultFace says whether a button only reads what it already draws: an arm
// of the cross that presses the arrow it carries, or a stick direction that
// presses the gamepad's own stick. Such a face is not written a second time.
func isDefaultFace(b padBind, id string) bool {
	if b.Key != "" {
		return isDirectionFace(b.Key)
	}
	return b.Pad != 0 && b.Pad == defaultPads[id]
}

// isDirectionFace is a source whose name is just a direction.
func isDirectionFace(name string) bool {
	switch name {
	case "↑", "↓", "←", "→", "摇杆↑", "摇杆↓", "摇杆←", "摇杆→":
		return true
	}
	return false
}

func textColorFor(t *ui.Theme, waiting bool) ui.Color {
	if waiting {
		return t.Text
	}
	return t.Text.Alpha(0.9)
}

// padFooter is the last row: the way back to the defaults, at the panel's edge.
func (a *App) padFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(24).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Grow(1)
		reset := ui.ButtonBase(c).Height(22).PaddingX(11).Radius(11).Center().
			Material(glass.Glass{Clarity: 0.7, Tint: ui.Hex("#ffffff14")}).
			Border(1, ui.Hex("#ffffff1f"))
		reset.Children(func() {
			ui.Text(c, "恢复默认").FontSize(10).TextColor(t.Text.Alpha(0.85))
		})
		if reset.Clicked() {
			a.resetPads(a.padLive)
			a.holdOn, a.holdDown = "", false
			a.cap = ""
		}
	})
}
