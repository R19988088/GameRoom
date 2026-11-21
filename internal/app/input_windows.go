package app

import (
	"syscall"
	"unsafe"

	"github.com/R19988088/GameRoom/internal/core"
)

var (
	procGetCursorPos   = syscall.NewLazyDLL("user32.dll").NewProc("GetCursorPos")
	procGetClientRect  = syscall.NewLazyDLL("user32.dll").NewProc("GetClientRect")
	procClientToScreen = syscall.NewLazyDLL("user32.dll").NewProc("ClientToScreen")
	procKeyState       = syscall.NewLazyDLL("user32.dll").NewProc("GetAsyncKeyState")
)

type winPoint struct{ X, Y int32 }
type winRect struct{ Left, Top, Right, Bottom int32 }

// pumpMouse hands the pointer to the game: where it is on the picture, for a
// game that clicks a menu, and how far it moved since the last frame, for a
// game that steers with a mouse. The picture is the letterboxed one, so the
// pointer is mapped onto the picture, not onto the whole window.
func (a *App) pumpMouse() {
	if a.core == nil || a.gameBmp == nil {
		return
	}
	var cursor winPoint
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); r == 0 {
		return
	}
	var box winRect
	procGetClientRect.Call(a.win.NativeHandle(), uintptr(unsafe.Pointer(&box)))
	origin := winPoint{}
	procClientToScreen.Call(a.win.NativeHandle(), uintptr(unsafe.Pointer(&origin)))
	w := float64(box.Right - box.Left)
	h := float64(box.Bottom - box.Top)
	if w <= 0 || h <= 0 {
		return
	}
	cx := float64(cursor.X - origin.X) // physical pixels inside the client
	cy := float64(cursor.Y - origin.Y)

	fw, fh := float64(a.frameW), float64(a.frameH)
	if fw <= 0 || fh <= 0 {
		return
	}
	ratio := float64(a.pictureRatio())
	if ratio <= 0 {
		ratio = fw / fh
	}
	// The picture keeps its own shape inside the window; where it sits, and
	// how big it is, follows from the two shapes alone.
	var left, top, pw, ph float64
	if ratio > w/h {
		pw, ph = w, w/ratio
		top = (h - ph) / 2
	} else {
		pw, ph = h*ratio, h
		left = (w - pw) / 2
	}
	nx := (cx - left) / pw
	ny := (cy - top) / ph

	state := core.MouseState{}
	if nx >= 0 && nx <= 1 && ny >= 0 && ny <= 1 {
		state.X = uint16(nx * 0x7fff)
		state.Y = uint16(ny * 0x7fff)
		if pressed(0x01) {
			state.Buttons |= 1
		}
		if pressed(0x02) {
			state.Buttons |= 2
		}
		if pressed(0x04) {
			state.Buttons |= 4
		}
		if a.mouseSeen {
			state.DX = int16((cx - a.mouseX) * fw / pw)
			state.DY = int16((cy - a.mouseY) * fh / ph)
		}
	}
	a.mouseX, a.mouseY, a.mouseSeen = cx, cy, true
	a.core.SetMouse(state)
}

// pressed is a mouse button as the system reports it.
func pressed(vk int) bool {
	st, _, _ := procKeyState.Call(uintptr(vk))
	return st&0x8000 != 0
}
