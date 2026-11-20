//go:build !windows

package launch

import "os/exec"

// EmbedInto is a no-op off Windows: without the platform's window manager
// there is no window to adopt, so games start the way they always did.
func EmbedInto(t Target) (Target, error) { return t, nil }

// Player is a running game that the app cannot put inside its window here.
type Player struct{ cmd *exec.Cmd }

// Play starts the game as its own process.
func Play(t Target, parent uintptr, stripDP int, onExit func(error)) (*Player, error) {
	cmd := exec.Command(t.Exe, t.Args...)
	cmd.Dir = t.Work
	applySysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		err := cmd.Wait()
		if onExit != nil {
			onExit(err)
		}
	}()
	return &Player{cmd: cmd}, nil
}

// Hide, Show and Kill keep the same shape as the Windows player.
func (p *Player) Hide() {}
func (p *Player) Show() {}
func (p *Player) Kill() { _ = p.cmd.Process.Kill() }

// EmbedStatus is the line the app shows while a game is running.
func EmbedStatus(name string) string { return "正在玩：" + name }

// SetWindowTitle is a no-op off Windows: the caption is the window manager's.
func SetWindowTitle(hwnd uintptr, title string) {}
