//go:build windows

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// mameSystem makes an ASCII-path mirror of RetroArch's system folder and
// returns the config file that points MAME at it.
//
// MAME opens its files with the system's ANSI code page. Every path under the
// emulator folder contains Chinese, so the core cannot read its own plugins and
// data there and refuses to start a game. Folders are joined rather than copied
// (they hold gigabytes of BIOS and sample data) and single files are hard
// linked, so the mirror costs nothing and stays in step with the original.
func (a *App) mameSystem(work string) string {
	// On the drive of the launcher's folder, which is plain ASCII, and with no
	// space in it: MAME splits the system path it is given on whitespace as well
	// as on ';', and the launcher's own folder is called "Sim ui_D", so a mirror
	// beside it reads back as the game "Sim".
	dst := a.asciiMirror("mame_system", filepath.Join(work, "system"))
	if dst == "" {
		return ""
	}
	cfg := filepath.Join(dst, "system.cfg")
	os.WriteFile(cfg, []byte("system_directory = \""+dst+"\"\n"), 0o644)
	return cfg
}

// asciiSystem is the system folder to hand a core: the real one when its path
// is plain ASCII, an ASCII mirror when it is not.
//
// A core that opens its BIOS with the C library's fopen sees the ANSI code page,
// so "模拟器\RetroArch\system\panafz10.bin" is a file it cannot find and 3DO sits
// on a black screen with "no BIOS ROM found". RetroArch itself hands cores a
// wide path, which is why the same game runs there.
func (a *App) asciiSystem(dir string) string {
	real := filepath.Join(dir, "system")
	if isASCII(real) {
		return real
	}
	if dst := a.asciiMirror("system", real); dst != "" {
		return dst
	}
	return real
}

// asciiMirror joins every entry of src into a plain-ASCII folder on the drive,
// skipping the entries whose own names are not ASCII. Folders are junctions and
// files are hard links, so the mirror costs no space and never goes stale.
func (a *App) asciiMirror(name, src string) string {
	dst := filepath.Join(filepath.VolumeName(a.root)+`\`, "GAMEROOM", name)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return ""
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !isASCII(e.Name()) {
			continue
		}
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		if e.IsDir() {
			cmd := exec.Command("cmd", "/c", "mklink", "/J", to, from)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			cmd.Run()
			continue
		}
		if err := os.Link(from, to); err != nil {
			if data, err := os.ReadFile(from); err == nil {
				os.WriteFile(to, data, 0o644)
			}
		}
	}
	return dst
}

// isASCII reports whether a name is plain enough for MAME's ANSI file calls.
func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
