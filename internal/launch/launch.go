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
	// Shader is a preset beside RetroArch's shaders folder.
	// Empty leaves RetroArch's own shader settings alone.
	Shader string
}

// Resolve turns a sim and a rom into the command that runs them.
//
// root is the launcher's folder: a sim's exe is relative to it when it
// is not absolute, so the package can move as a whole.
func Resolve(root string, sim library.Sim, romAbs string) (Target, error) {
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
	return Target{Exe: exe, Args: args, Work: filepath.Dir(exe)}, nil
}

// InstallShader points RetroArch at a shader preset for every game.
//
// RetroArch 1.17 ignores video_shader passed on the command line, but it does
// load the per-content "global" preset in config/, so that is where the copy
// goes. A preset names its passes relative to the shaders folder, so those
// names gain the "../shaders/" prefix that the config folder needs. A missing
// preset is not an error: the game starts in RetroArch's own look.
func InstallShader(t Target, shader string) error {
	if shader == "" || !IsRetroArch(t.Exe) {
		return nil
	}
	exeDir := t.Work
	preset := ""
	for _, cand := range shaderCandidates(shader) {
		p := filepath.Join(exeDir, "shaders", filepath.FromSlash(cand))
		if _, err := os.Stat(p); err == nil {
			preset = p
			break
		}
	}
	if preset == "" {
		return nil
	}
	data, err := os.ReadFile(preset)
	if err != nil {
		return err
	}
	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if pass, ok := shaderPass(line); ok && !strings.HasPrefix(pass, "../") && !filepath.IsAbs(pass) {
			line = strings.Replace(line, `"`+pass+`"`, `"../shaders/`+pass+`"`, 1)
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	target := filepath.Join(exeDir, "config", "global.slangp")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(out.String()), 0o644)
}

// shaderPass returns the path from a "shaderN = "path"" line.
func shaderPass(line string) (string, bool) {
	if !strings.HasPrefix(line, "shader") {
		return "", false
	}
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", false
	}
	from := strings.Index(line[eq:], `"`)
	if from < 0 {
		return "", false
	}
	rest := line[eq+from+1:]
	to := strings.Index(rest, `"`)
	if to < 0 {
		return "", false
	}
	return rest[:to], true
}

// shaderCandidates lists the names a shader may live under, since the same
// preset is kept either as one file at the top of shaders or inside the pack
// it came from.
func shaderCandidates(shader string) []string {
	name := strings.TrimPrefix(filepath.ToSlash(shader), "/")
	out := []string{name}
	if !strings.HasPrefix(name, "shaders_") {
		out = append(out, "shaders_slang/"+name)
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		out = append(out, name[i+1:])
	}
	return out
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
