//go:build windows

package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// A game plays inside the library window, as one panel of the app rather than
// as a window of its own.
//
// The frontend here is a build of RetroArch patched for this app: told the
// launcher's window through GAMEROOM_PARENT_HWND it creates its video window
// directly as a hidden child of it, never as a top-level window, never fullscreen,
// and never showing or activating anything. There is no window to chase, no
// off-screen position to keep it at, and nothing that can flash across the
// desktop for a frame: the panel appears where the app puts it and stays there.
//
// The pointer and the keyboard stay with the app until the player clicks inside
// the panel; RetroArch's own exit hotkey is off, because leaving is the back
// button's job.
var (
	procSetWindowPos    = user32.NewProc("SetWindowPos")
	procGetClientRect   = user32.NewProc("GetClientRect")
	procClientToScreen  = user32.NewProc("ClientToScreen")
	procIsWindow        = user32.NewProc("IsWindow")
	procGetDpiForWin    = user32.NewProc("GetDpiForWindow")
	procSetWindowText   = user32.NewProc("SetWindowTextW")
	procFindWindowEx    = user32.NewProc("FindWindowExW")
	procGetClassName    = user32.NewProc("GetClassNameW")
	procGetParent       = user32.NewProc("GetParent")
	procGetWindowThread = user32.NewProc("GetWindowThreadProcessId")
)

const (
	hwndTop = 0

	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	swHide = 0
	swShow = 5

	retroArchClass = "RetroArch"
)

// EmbedInto adds the settings that make RetroArch playable inside another
// window, and points it at that file on the command line. RetroArch's own
// config still says fullscreen; --appendconfig is a second config read on top
// of it, so the game keeps every other setting it had.
func EmbedInto(t Target) (Target, error) {
	if !IsRetroArch(t.Exe) {
		return t, nil
	}
	cfg := embedConfigPath(t)
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		return t, err
	}
	body := "" +
		// A window, not a fullscreen surface. The patched binary keeps to this
		// even if the core or the shader asks for something else.
		"video_fullscreen = \"false\"\n" +
		"video_windowed_fullscreen = \"false\"\n" +
		"ui_menubar_enable = \"false\"\n" +
		"video_threaded = \"false\"\n" +
		"video_window_save_positions = \"false\"\n" +
		// The pointer belongs to the app: the game never captures it.
		"input_auto_mouse_grab = \"false\"\n" +
		// No quit shortcut: Escape reaches the game, leaving is the back button.
		"input_exit_hotkey = \"nul\"\n" +
		// The network interface is how the app swaps the game's look while it
		// keeps playing; a preset change is one command, not a restart.
		"network_cmd_enable = \"true\"\n" +
		fmt.Sprintf("network_cmd_port = \"%d\"\n", CommandPort)
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		return t, err
	}
	if err := enableNetworkCommands(t); err != nil {
		return t, err
	}
	t.Args = append(t.Args, "--appendconfig=config\\embed.cfg")
	return t, nil
}

// enableNetworkCommands sets the one key RetroArch reads before it applies the
// append config, so the interface is listening when the app wants to switch a
// preset. It is a one-line edit of the frontend's own config, repeated only
// when the value is wrong.
func enableNetworkCommands(t Target) error {
	path := filepath.Join(t.Work, "retroarch.cfg")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(data)
	want := fmt.Sprintf("network_cmd_enable = \"true\"\nnetwork_cmd_port = \"%d\"", CommandPort)
	if strings.Contains(text, want) {
		return nil
	}
	text = strings.ReplaceAll(text, `network_cmd_enable = "false"`, `network_cmd_enable = "true"`)
	if !strings.Contains(text, `network_cmd_enable = "true"`) {
		text = strings.TrimRight(text, "\n") + "\nnetwork_cmd_enable = \"true\"\n"
	}
	port := fmt.Sprintf("network_cmd_port = \"%d\"", CommandPort)
	text = regexpPort(text, port)
	return os.WriteFile(path, []byte(text), 0o644)
}

// regexpPort rewrites the interface's port line.
func regexpPort(text, line string) string {
	re := regexp.MustCompile(`network_cmd_port = "\d+"`)
	if re.MatchString(text) {
		return re.ReplaceAllString(text, line)
	}
	return strings.TrimRight(text, "\n") + "\n" + line + "\n"
}

// Player is a game running inside the launcher's window: its process and the
// panel it draws into.
type Player struct {
	cmd    *exec.Cmd
	parent syscall.Handle

	mu     sync.Mutex
	hwnd   syscall.Handle
	hidden bool
}

// Play starts the game, which draws into parent's client area from its first
// frame. onExit runs once the process is gone, from the player's own goroutine.
func Play(t Target, parent uintptr, _ int, onExit func(error)) (*Player, error) {
	cmd := exec.Command(t.Exe, t.Args...)
	cmd.Dir = t.Work
	applySysProcAttr(cmd)
	// The contract with the patched frontend: this is the window the game
	// belongs to, so it creates no window of its own that anyone could see.
	cmd.Env = append(os.Environ(), fmt.Sprintf("GAMEROOM_PARENT_HWND=%d", parent))
	if err := cmd.Start(); err != nil {
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "play: start failed: %v\nargs=%q\n", err, t.Args)
		}
		return nil, err
	}
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "play: started %s %q\n", t.Exe, t.Args)
	}
	p := &Player{cmd: cmd, parent: syscall.Handle(parent)}
	go p.run(onExit)
	return p, nil
}

// run adopts the panel once it exists and keeps it fitted until the process
// ends. RetroArch takes a few seconds to load a core before it creates one.
func (p *Player) run(onExit func(error)) {
	exit := make(chan error, 1)
	go func() { exit <- p.cmd.Wait() }()

	if h := p.findPanel(30 * time.Second); h != 0 {
		p.adopt(h)
	}
	for {
		select {
		case err := <-exit:
			if os.Getenv("GAMEROOM_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "play: exited err=%v\n", err)
			}
			if onExit != nil {
				onExit(err)
			}
			return
		case <-time.After(300 * time.Millisecond):
			p.fit()
		}
	}
}

// findPanel waits for the game's video window to exist under the launcher's
// window. It is a child from the moment it is created, so it is looked for there
// and never among the desktop's windows: nothing else can be mistaken for it.
func (p *Player) findPanel(timeout time.Duration) syscall.Handle {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h, _, _ := procFindWindowEx.Call(uintptr(p.parent), 0,
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(retroArchClass))), 0); h != 0 {
			return syscall.Handle(h)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0
}

// adopt shows the panel where the app wants it and hands it the size it is
// allowed to fill.
func (p *Player) adopt(h syscall.Handle) {
	p.mu.Lock()
	p.hwnd = h
	p.mu.Unlock()
	p.fit()
	procShowWindow.Call(uintptr(h), swShow)
}

// fit sizes the panel to the whole client area: the game is the window, apart
// from the buttons that float on top of it. It compares against where the window
// really is, so the panel is put back if the game moved it.
func (p *Player) fit() {
	p.mu.Lock()
	h, hidden := p.hwnd, p.hidden
	p.mu.Unlock()
	if h == 0 || hidden {
		return
	}
	var cr rect
	if ok, _, _ := procGetClientRect.Call(uintptr(p.parent), uintptr(unsafe.Pointer(&cr))); ok == 0 {
		return
	}
	if cr.right < 1 || cr.bottom < 1 {
		return
	}
	w, hh := int(cr.right), int(cr.bottom)

	var wr rect
	if ok, _, _ := procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&wr))); ok == 0 {
		return
	}
	origin := rect{}
	procClientToScreen.Call(uintptr(p.parent), uintptr(unsafe.Pointer(&origin)))
	cur := rect{
		left:   wr.left - origin.left,
		top:    wr.top - origin.top,
		right:  wr.right - origin.left,
		bottom: wr.bottom - origin.top,
	}
	if cur == (rect{0, 0, int32(w), int32(hh)}) {
		return
	}
	procSetWindowPos.Call(uintptr(h), hwndTop, 0, 0,
		uintptr(w), uintptr(hh), swpNoActivate|swpShowWindow)
}

// Hide takes the game off screen for a moment where the app needs the space
// for itself — the filter picker does.
func (p *Player) Hide() {
	p.mu.Lock()
	p.hidden = true
	h := p.hwnd
	p.mu.Unlock()
	if h != 0 {
		procShowWindow.Call(uintptr(h), swHide)
	}
}

// Show puts the game back where the play area is.
func (p *Player) Show() {
	p.mu.Lock()
	p.hidden = false
	h := p.hwnd
	p.mu.Unlock()
	if h != 0 {
		procShowWindow.Call(uintptr(h), swShow)
	}
	p.fit()
}

// Kill ends the game; onExit reports it, as with any other quit.
func (p *Player) Kill() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

// dpiOf is the parent window's scale, 1 when the system does not say.
func dpiOf(h syscall.Handle) float64 {
	dpi, _, _ := procGetDpiForWin.Call(uintptr(h))
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// EmbedStatus is the line the app shows while a game owns the play area.
func EmbedStatus(name string) string { return fmt.Sprintf("正在玩：%s", name) }

// SetWindowTitle renames the app's own caption. While a game runs the title bar
// carries its name, the way a player application does, and goes back to the
// library's when the game ends.
func SetWindowTitle(hwnd uintptr, title string) {
	if hwnd == 0 || title == "" {
		return
	}
	p, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(p)))
}

// embedConfigPath is where EmbedInto puts its file.
func embedConfigPath(t Target) string { return filepath.Join(t.Work, "config", "embed.cfg") }
