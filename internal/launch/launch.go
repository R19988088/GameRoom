// Package launch turns a game and its emulator into a running process.
//
// This version launches RetroArch only: every platform the old frontend
// ran with a Windows-only emulator still lists its sim, so the launcher
// can say what would have run, but only RetroArch is started.
package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/R19988088/GameRoom/internal/library"
)

// Target is a resolved command: what runs, with what, and where.
type Target struct {
	Exe  string
	Args []string
	Work string
}

// IsRetroArch reports whether exe is RetroArch.
func IsRetroArch(exe string) bool {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(exe)))
	return base == "retroarch" || base == "retroarch.exe"
}

// Options are what the launcher adds to a sim's own command line.
type Options struct {
	// Shader is a preset beside RetroArch's shaders folder, applied through
	// a config override. Empty runs the game with RetroArch's own config.
	Shader string
}

// Resolve turns a sim and a rom into the command that runs them.
//
// root is the launcher's folder: a sim's exe is relative to it when it
// is not absolute, so the package can move as a whole.
func Resolve(root string, sim library.Sim, romAbs string, opt Options) (Target, error) {
	exe := filepath.FromSlash(sim.Exe)
	if !filepath.IsAbs(exe) {
		exe = filepath.Join(root, exe)
	}
	exe = bundleExecutable(exe)
	if _, err := os.Stat(exe); err != nil {
		return Target{}, fmt.Errorf("%s 不存在：%s", sim.Name, exe)
	}
	if _, err := os.Stat(romAbs); err != nil {
		return Target{}, fmt.Errorf("找不到 rom：%s", romAbs)
	}

	args := Expand(SplitArgs(sim.Args), romAbs)
	if sim.Core != "" {
		core := coreFile(filepath.Dir(exe), sim.Core)
		if _, err := os.Stat(core); err != nil {
			return Target{}, fmt.Errorf("找不到核心 %s：%s", sim.Core, core)
		}
		args = append([]string{"-L", core}, args...)
	}
	if len(args) == 0 && IsRetroArch(exe) {
		args = []string{romAbs}
	}
	if opt.Shader != "" && IsRetroArch(exe) {
		cfg, err := shaderConfig(root, filepath.Dir(exe), opt.Shader)
		if err != nil {
			return Target{}, err
		}
		args = append(args, "--appendconfig", cfg)
	}
	return Target{Exe: exe, Args: args, Work: filepath.Dir(exe)}, nil
}

// shaderConfig writes the two lines RetroArch reads to turn a shader preset
// on, and returns the file to append to its config.
//
// The preset is named the way RetroArch names it: relative to its shaders
// folder, as "shaders_slang/202603.slangp", because the passes inside a
// preset are named that way too.
func shaderConfig(root, exeDir, shader string) (string, error) {
	name := strings.TrimPrefix(filepath.ToSlash(shader), "/")
	preset := filepath.Join(exeDir, "shaders", filepath.FromSlash(name))
	if _, err := os.Stat(preset); err != nil {
		return "", fmt.Errorf("找不到滤镜 %s：%s", name, preset)
	}
	dir := filepath.Join(root, "数据")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cfg := filepath.Join(dir, "retroarch-shader.cfg")
	body := "video_shader_enable = \"true\"\nvideo_shader = \"" + name + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		return "", err
	}
	return cfg, nil
}

// coreFile is a core's file beside RetroArch: Windows loads a .dll,
// Linux a .so, macOS a .dylib, so a library imported on one runs on the
// others without changing its sims.
func coreFile(exeDir, core string) string {
	ext := ".so"
	switch runtime.GOOS {
	case "windows":
		ext = ".dll"
	case "darwin":
		ext = ".dylib"
	}
	return filepath.Join(exeDir, "cores", core+"_libretro"+ext)
}

// Expand fills the placeholders the old frontend's command lines use.
//
//	{RomFullPath} the rom's absolute path
//	{RomDir}      its folder
//	{RomName}     its file name
//	{RomBase}     its file name without the extension
func Expand(args []string, romAbs string) []string {
	dir := filepath.Dir(romAbs)
	name := filepath.Base(romAbs)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	replacer := strings.NewReplacer(
		"{RomFullPath}", romAbs,
		"{RomDir}", dir,
		"{RomName}", name,
		"{RomBase}", base,
	)
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = replacer.Replace(a)
	}
	return out
}

// SplitArgs splits a command line, keeping quoted parts whole.
func SplitArgs(s string) []string {
	var out []string
	var b strings.Builder
	quoted, started := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			started = true
		case (r == ' ' || r == '\t' || r == '\n') && !quoted:
			if started {
				out = append(out, b.String())
				b.Reset()
				started = false
			}
		default:
			b.WriteRune(r)
			started = true
		}
	}
	if started {
		out = append(out, b.String())
	}
	return out
}

// bundleExecutable turns a macOS .app bundle into the program inside it.
func bundleExecutable(exe string) string {
	if !strings.HasSuffix(strings.ToLower(exe), ".app") {
		return exe
	}
	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	return filepath.Join(exe, "Contents", "MacOS", name)
}

// Start runs the target in its own working directory, so RetroArch finds
// cores/ and system/ beside itself.
func Start(t Target) (*exec.Cmd, error) {
	cmd := exec.Command(t.Exe, t.Args...)
	cmd.Dir = t.Work
	applySysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
