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
		args = append([]string{"-L", coreArg(filepath.Dir(exe), core)}, args...)
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
// goes. A missing preset is not an error: the game starts in RetroArch's own
// look.
func InstallShader(t Target, shader string) error {
	text, ok, err := presetText(t, shader)
	if err != nil || !ok {
		return err
	}
	return writeFile(filepath.Join(t.Work, "config", "global.slangp"), text)
}

// presetText reads the preset named by shader and returns its text as it must
// sit in the config folder: a preset names its passes relative to the shaders
// folder, so those names gain the "../shaders/" prefix that folder needs. The
// second result says whether the preset was found at all.
func presetText(t Target, shader string) (string, bool, error) {
	if shader == "" || !IsRetroArch(t.Exe) {
		return "", false, nil
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
		return "", false, nil
	}
	data, err := os.ReadFile(preset)
	if err != nil {
		return "", false, err
	}
	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if pass, ok := shaderPass(line); ok && !strings.HasPrefix(pass, "../") && !filepath.IsAbs(pass) {
			line = strings.Replace(line, `"`+pass+`"`, `"../shaders/`+pass+`"`, 1)
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String(), true, nil
}

// writeFile makes the file and its folder, since the config folder may not
// exist on a first run.
func writeFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// InstallShaderLevel puts the preset in place and then bends its parameters to
// the look the player picked: "soft" and "strong" scale the scanline mask and
// the tube curvature of the PhosphorLine preset, "normal" leaves the preset as
// its author tuned it, and "off" removes the shader altogether so the game
// runs unfiltered.
func InstallShaderLevel(t Target, shader, level string) error {
	if !IsRetroArch(t.Exe) {
		return nil
	}
	target := filepath.Join(t.Work, "config", "global.slangp")
	if level == "off" || shader == "" {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	text, ok, err := presetText(t, shader)
	if err != nil || !ok {
		return err
	}
	if params := levelParams(level); len(params) > 0 {
		text = bendLines(text, params)
	}
	return writeFile(target, text)
}

// LivePresetPath is the preset a look has of its own, so switching one does not
// have to touch the global preset a game started with.
func LivePresetPath(t Target, level string) string {
	name := level
	if name == "" || name == "normal" {
		name = "standard"
	}
	return filepath.Join(t.Work, "config", "look-"+name+".slangp")
}

// WriteLivePreset writes the preset for a look and returns its path, so a
// running RetroArch can be pointed at it with one command. "off" has no preset
// of its own: RetroArch crashes on a pipeline with no passes, so turning the
// filter off is a toggle, not a load.
func WriteLivePreset(t Target, shader, level string) (string, error) {
	if !IsRetroArch(t.Exe) {
		return "", fmt.Errorf("不是 RetroArch，没有预置可换")
	}
	if level == "off" {
		return "", fmt.Errorf("关闭滤镜不靠换预置")
	}
	path := LivePresetPath(t, level)
	text, ok, err := presetText(t, shader)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("找不到着色器 %s", shader)
	}
	if params := levelParams(level); len(params) > 0 {
		text = bendLines(text, params)
	}
	return path, writeFile(path, text)
}

// levelParams is the parameter set each look asks for. The preset's own
// numbers stay untouched for "normal": they are the author's tuning, and the
// point of the other two is only to be noticeably lighter or heavier.
func levelParams(level string) map[string]string {
	switch level {
	case "soft":
		return map[string]string{
			"NTSCintensity":  "0.350000",
			"MASK_INTENSITY": "0.450000",
			"CORNER":         "0.080000",
			"SHARPEN":        "0.020000",
			"DIFFUSION":      "0.030000",
		}
	case "strong":
		return map[string]string{
			"NTSCintensity":  "0.950000",
			"MASK_INTENSITY": "1.000000",
			"CORNER":         "0.320000",
			"SHARPEN":        "0.100000",
			"DIFFUSION":      "0.080000",
		}
	}
	return nil
}

// bendLines rewrites the "KEY = "value"" lines of a preset's text. A key the
// preset does not define is left alone: the passes that read it would still
// take the value, but adding a line to someone else's preset is noise.
func bendLines(text string, params map[string]string) string {
	out := make([]string, 0, 256)
	for _, line := range strings.Split(text, "\n") {
		eq := strings.Index(line, " = ")
		if eq > 0 {
			key := strings.TrimSpace(line[:eq])
			if v, ok := params[key]; ok {
				line = key + ` = "` + v + `"`
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// shaderPass returns the path from a "shaderN = "path"" line.
//
// The preset also opens with `shaders = "8"`, the count of passes. That line
// looks like a pass to a careless prefix test, and rewriting it to a path
// makes RetroArch read the preset as eight passes that live in a folder called
// 8 — it then dies in the shader compiler. Only a numeric suffix counts as a
// pass.
func shaderPass(line string) (string, bool) {
	if !strings.HasPrefix(line, "shader") {
		return "", false
	}
	i := len("shader")
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == len("shader") || i >= len(line) || line[i] != ' ' {
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

// coreArg is the core as the frontend's command line should name it: relative
// to the folder it runs in, because a frontend started beside a folder whose
// name is not plain ASCII reads its own arguments through the system's legacy
// code page and loses the path, while a relative one is only ASCII.
func coreArg(exeDir, core string) string {
	rel, err := filepath.Rel(exeDir, core)
	if err != nil || strings.Contains(rel, "..") {
		return core
	}
	return rel
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
