//go:build windows

package launch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procSetFocus                 = user32.NewProc("SetFocus")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procKeybdEvent               = user32.NewProc("keybd_event")

	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess               = kernel32.NewProc("OpenProcess")
	procCloseHandle               = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetCurrentThreadId        = kernel32.NewProc("GetCurrentThreadId")
)

const (
	swRestore  = 9
	swMinimize = 6

	vkShift = 0x10
	vkLWin  = 0x5B
	vkA     = 0x41
	keyUp   = 0x2

	processQueryLimited = 0x1000
)

type rect struct {
	left, top, right, bottom int32
}

// MagpieFilter decides whether an external program gets the CRT filter, and
// where the filter program lives.
type MagpieFilter struct {
	// Exe is Magpie's path; empty means "window management only".
	Exe string
}

// TakeOver fronts a started program's window and, when Magpie is available,
// puts Magpie's CRT over it. It runs off the UI thread: a game may take
// seconds to show a window.
//
// exeName is the program's file name. Some launchers hand the window to a
// second process (Windows' own Notepad does), so a window owned by a process
// with the same file name counts as the game's.
func TakeOver(pid int, exeName string, f MagpieFilter) {
	hwnd := waitWindow(pid, exeName, 30*time.Second)
	if hwnd == 0 {
		return
	}
	if f.Exe == "" {
		front(hwnd)
		return
	}
	if !magpieRunning() {
		configureMagpie()
		if !startMagpie(f.Exe) {
			front(hwnd)
			return
		}
	}
	front(hwnd)
	time.Sleep(300 * time.Millisecond)
	scaleHotkey()
}

// waitWindow polls until the program owns a visible, restored window.
func waitWindow(pid int, exeName string, d time.Duration) syscall.Handle {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if h := windowOf(pid, exeName); h != 0 {
			return h
		}
		time.Sleep(250 * time.Millisecond)
	}
	return 0
}

// windowOf returns the largest visible window owned by pid, or by any process
// with exeName when the launcher handed the window to a helper process.
func windowOf(pid int, exeName string) syscall.Handle {
	var best syscall.Handle
	var bestArea int32
	cb := syscall.NewCallback(func(h syscall.Handle, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadProcessID.Call(uintptr(h), uintptr(unsafe.Pointer(&owner)))
		vis, _, _ := procIsWindowVisible.Call(uintptr(h))
		if vis == 0 {
			return 1
		}
		iconic, _, _ := procIsIconic.Call(uintptr(h))
		if iconic != 0 {
			return 1
		}
		own := int(owner) == pid
		if !own && exeName != "" {
			own = strings.EqualFold(filepath.Base(processPath(int(owner))), exeName)
		}
		if !own {
			return 1
		}
		var r rect
		procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r)))
		area := (r.right - r.left) * (r.bottom - r.top)
		if area > bestArea {
			best, bestArea = h, area
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return best
}

// processPath is a process's executable path, or "" when it cannot be read.
func processPath(pid int) string {
	h, _, _ := procOpenProcess.Call(processQueryLimited, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	ok, _, _ := procQueryFullProcessImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// front puts the game's window in front of everything and gives it the
// keyboard.
//
// SetForegroundWindow alone is a request Windows is free to refuse: a process
// that is not already foreground may not hand focus to another one, and the
// call then quietly does nothing. The game window comes up behind the library,
// the player clicks it to get in, and keyboard or mouse that never reached the
// game looks like a game that cannot be played. Attaching our input queue to
// the game's while the call is made is what Windows accepts as "this is on
// purpose", and it is what makes the game playable without that first click.
func front(h syscall.Handle) {
	procShowWindow.Call(uintptr(h), swRestore)
	procBringWindowToTop.Call(uintptr(h))

	var tid uint32
	procGetWindowThreadProcessID.Call(uintptr(h), uintptr(unsafe.Pointer(&tid)))
	own, _, _ := procGetCurrentThreadId.Call()
	if tid != 0 && uintptr(tid) != own {
		procAttachThreadInput.Call(own, uintptr(tid), 1)
		defer procAttachThreadInput.Call(own, uintptr(tid), 0)
	}
	procSetForegroundWindow.Call(uintptr(h))
	procSetFocus.Call(uintptr(h))
}

// scaleHotkey presses Magpie's default "scale this window" shortcut,
// Win+Shift+A. It goes out after the game window is in front, so the keypress
// lands in the game and Magpie's global hook picks it up.
func scaleHotkey() {
	key(vkLWin, 0)
	key(vkShift, 0)
	key(vkA, 0)
	key(vkA, keyUp)
	key(vkShift, keyUp)
	key(vkLWin, keyUp)
}

func key(vk uintptr, flags uintptr) {
	procKeybdEvent.Call(vk, 0, flags, 0)
	time.Sleep(20 * time.Millisecond)
}

func magpieRunning() bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq Magpie.exe", "/NH").Output()
	return err == nil && strings.Contains(string(out), "Magpie.exe")
}

func startMagpie(exe string) bool {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return false
	}
	// Magpie opens its own window; get it out of the way so the game stays
	// in front and the hotkey lands there.
	time.Sleep(2500 * time.Millisecond)
	if h := windowOf(cmd.Process.Pid, "Magpie.exe"); h != 0 {
		procShowWindow.Call(uintptr(h), swMinimize)
	}
	return true
}

// configureMagpie makes CRT-Geom the default scaling mode, so the hotkey
// brings up the CRT look instead of plain Lanczos.
func configureMagpie() {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "Magpie", "config", "v5", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	modes, _ := cfg["scalingModes"].([]any)
	idx := -1
	for i, m := range modes {
		if mm, ok := m.(map[string]any); ok {
			if name, _ := mm["name"].(string); name == "CRT-Geom" {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return
	}
	profiles, _ := cfg["profiles"].([]any)
	if len(profiles) == 0 {
		return
	}
	p, _ := profiles[0].(map[string]any)
	if p == nil {
		return
	}
	if cur, _ := p["scalingMode"].(float64); int(cur) == idx {
		return // already set
	}
	p["scalingMode"] = idx
	out, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, out, 0o644)
}
