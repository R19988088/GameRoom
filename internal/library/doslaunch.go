package library

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A DOS game in this library is the zip its author packed, and DOSBox Pure
// mounts it as C: and shows its own start menu: a list of the exes inside, an
// entry for the command line, an exit. The player then picks an exe and presses
// enter, every single time, for a game that historically started with one
// command.
//
// Pure does have a way out of that menu, and it is a plain file: if the mounted
// content contains DOSBOX.BAT, the core runs that instead of drawing the menu
// (Libretro's core docs, "Auto-execution of DOSBOX.BAT"). So the way back to
// double-click-and-play is to put the game's own launch command into its zip as
// that batch file — and the launch command is already inside each zip, in the
// [autoexec] section of the dosbox.conf the collection's author shipped at the
// archive root:
//
//	[autoexec]
//	mount C .
//	C:
//	cd IF2
//	call PLAY.BAT
//	exit
//
// Everything the game needs to start is in those lines, tuned game by game by
// whoever built the collection (a CD to imgmount, a TSR to load, a specific
// exe). This file only drops the lines that belong to the old frontend rather
// than to the game — the mount of C: itself, and the exit that closed it — and
// writes the rest as DOSBOX.BAT.
const (
	launchBat  = "DOSBOX.BAT"
	launchConf = "dosbox.conf"
)

// LaunchReport says what PrepareDOSLaunch did with the shelf: how many zips it
// gave a start command, how many already had one, and, for the few it had to
// leave alone, why — those still open on Pure's menu, which is a game you can
// play, only not with a double click.
type LaunchReport struct {
	Prepared int
	Already  int
	Skipped  []LaunchSkip
}

// LaunchSkip is one game PrepareDOSLaunch could not give a start command.
type LaunchSkip struct {
	Zip    string
	Reason string
}

// PrepareDOSLaunch gives every zip under rom/dos a DOSBOX.BAT, so that a
// double click in the library lands in the game instead of in the core's menu.
// It is safe to run repeatedly: a zip that already carries DOSBOX.BAT is left
// untouched, so an import after adding games only touches the new ones.
func PrepareDOSLaunch(dir string) (LaunchReport, error) {
	var rep LaunchReport
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return rep, nil
		}
		return rep, fmt.Errorf("读 rom/dos：%w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(dir, name)
		switch res, err := prepareOne(path); {
		case err != nil:
			rep.Skipped = append(rep.Skipped, LaunchSkip{name, err.Error()})
		case res == "already":
			rep.Already++
		case res == "written":
			rep.Prepared++
		default:
			rep.Skipped = append(rep.Skipped, LaunchSkip{name, res})
		}
	}
	return rep, nil
}

// prepareOne writes DOSBOX.BAT into one zip. It answers "already" when the zip
// is done, "written" when it just added the batch, and any other string is the
// reason it left the zip alone.
//
// The archive is read once and closed before it is rebuilt: on Windows a zip
// still held open cannot be replaced by the rewritten copy.
func prepareOne(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("打不开 zip：%w", err)
	}
	names := make(map[string]bool, len(zr.File))
	for _, f := range zr.File {
		names[zipPath(f.Name)] = true
	}
	already := names[strings.ToLower(launchBat)]
	body, reason := "", ""
	if !already {
		body, reason = launchBody(zr)
	}
	zr.Close()

	if already {
		return "already", nil
	}
	if body == "" {
		return reason, nil
	}
	if err := addFileToZip(path, launchBat, body); err != nil {
		return "", err
	}
	return "written", nil
}

// launchBody is the batch text for one game: the archive's own [autoexec]
// without the frontend's own lines, or, for the two archives that shipped
// without one, the single play.BAT sitting inside the game's folder.
func launchBody(zr *zip.ReadCloser) (body, reason string) {
	if lines, ok := autoexecLines(zr); ok {
		cmd := launchCommands(lines)
		if len(cmd) > 0 {
			if bad := checkCommands(cmd, zipNames(zr)); bad != "" {
				return "", bad
			}
			return "@echo off\r\n" + strings.Join(cmd, "\r\n") + "\r\n", ""
		}
	}

	// No dosbox.conf: fall back to the launcher scripts the game shipped, which
	// in practice means "the one play.BAT inside its folder".
	dir, bat, ok := shippedLauncher(zr)
	if !ok {
		return "", "没有 dosbox.conf，也找不到 play.BAT"
	}
	cmd := []string{"c:", "cd " + dir, "call " + bat}
	if bad := checkCommands(cmd, zipNames(zr)); bad != "" {
		return "", bad
	}
	return "@echo off\r\n" + strings.Join(cmd, "\r\n") + "\r\n", ""
}

// autoexecLines reads the [autoexec] section of the archive's root dosbox.conf.
func autoexecLines(zr *zip.ReadCloser) ([]string, bool) {
	var conf *zip.File
	for _, f := range zr.File {
		if zipPath(f.Name) == launchConf {
			conf = f
			break
		}
	}
	if conf == nil {
		return nil, false
	}
	rc, err := conf.Open()
	if err != nil {
		return nil, false
	}
	defer rc.Close()
	text, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		return nil, false
	}

	var out []string
	inAuto := false
	for _, line := range strings.Split(strings.ReplaceAll(string(text), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inAuto = strings.EqualFold(strings.Trim(line, "[]"), "autoexec")
			continue
		}
		if !inAuto || line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, true
}

// launchCommands keeps the lines that start the game and drops the ones that
// only made sense in the old standalone DOSBox: the mount of C: (Pure mounts the
// zip there itself) and the exit that ended the session.
func launchCommands(lines []string) []string {
	var out []string
	for _, line := range lines {
		low := strings.ToLower(strings.TrimSpace(line))
		switch {
		case low == "@echo off" || low == "echo off" || low == "cls" || low == "exit":
			continue
		case strings.HasPrefix(low, "mount c") || strings.HasPrefix(low, "mount d . "):
			continue
		}
		out = append(out, strings.TrimSpace(line))
	}
	return out
}

// shippedLauncher finds the folder + batch to run in an archive with no
// dosbox.conf: the deepest-free shape of these zips is one top folder holding
// the game and its play.BAT.
func shippedLauncher(zr *zip.ReadCloser) (dir, bat string, ok bool) {
	preferred := []string{"play.bat", "gameplay.bat", "game.bat", "start.bat"}
	type entry struct{ path, name string }
	var found []entry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if !strings.HasSuffix(strings.ToLower(name), ".bat") {
			continue
		}
		found = append(found, entry{strings.ToLower(name), name})
	}
	for _, want := range preferred {
		for _, e := range found {
			if strings.HasSuffix(e.path, "/"+want) {
				parts := strings.Split(e.name, "/")
				return strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1], true
			}
		}
	}
	return "", "", false
}

// checkCommands verifies statically that the batch points at files that are in
// the archive: a `cd IF2` with no IF2, or a `call PLAY.BAT` with no play.BAT, is
// a game that would open on a dead command line. Catching it at import time
// keeps the game on Pure's menu instead, where it can still be started by hand.
func checkCommands(cmd []string, files map[string]bool) string {
	dir := ""
	for _, line := range cmd {
		low := strings.ToLower(strings.TrimSpace(line))
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch {
		case low == "c:" || low == "c:\\":
			dir = ""
		case fields[0] == "cd" || fields[0] == "chdir" || strings.EqualFold(fields[0], "cd"):
			if len(fields) < 2 {
				continue
			}
			target := strings.TrimSpace(fields[1])
			if strings.HasPrefix(target, "..") || strings.Contains(target, "\\") && strings.HasPrefix(target, "\\") {
				dir = ""
				continue
			}
			next := joinZipPath(dir, target)
			if !hasDir(files, next) {
				return "批处理里的 cd " + target + " 在 zip 里不存在"
			}
			dir = next
		case strings.EqualFold(fields[0], "call"):
			if len(fields) < 2 {
				continue
			}
			// "call CM" runs CM.BAT, CM.COM or CM.EXE, whichever the disk has —
			// the collection relies on that for games with no play.BAT.
			if !resolves(files, dir, fields[1]) {
				return "批处理里的 call " + fields[1] + " 在 zip 里不存在"
			}
		}
	}
	return ""
}

// resolves answers whether a name typed on a command line would find something
// to run: the name itself, or the same name with DOS's executable extensions.
func resolves(files map[string]bool, dir, name string) bool {
	name = strings.TrimSpace(name)
	if hasFile(files, joinZipPath(dir, name)) {
		return true
	}
	if strings.ContainsAny(name, `\/.:`) {
		return false
	}
	for _, ext := range []string{".bat", ".com", ".exe"} {
		if hasFile(files, joinZipPath(dir, name+ext)) {
			return true
		}
	}
	return false
}

// joinZipPath joins an archive path with a relative one, the way the game's own
// batch would have: "." is the archive root, ".." goes up one folder.
func joinZipPath(dir, rel string) string {
	rel = strings.ReplaceAll(rel, `\`, "/")
	base := dir
	for {
		up := strings.HasPrefix(rel, "../")
		if !up {
			break
		}
		rel = rel[3:]
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[:i]
		} else {
			base = ""
		}
	}
	if rel == "." || rel == "" {
		return base
	}
	if base == "" {
		return rel
	}
	return base + "/" + rel
}

func hasDir(files map[string]bool, dir string) bool {
	if dir == "" {
		return true
	}
	prefix := strings.ToLower(dir) + "/"
	for name := range files {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func hasFile(files map[string]bool, name string) bool {
	return files[strings.ToLower(strings.TrimPrefix(name, "./"))]
}

func zipNames(zr *zip.ReadCloser) map[string]bool {
	out := make(map[string]bool, len(zr.File))
	for _, f := range zr.File {
		out[zipPath(f.Name)] = true
	}
	return out
}

// zipPath normalises an archive entry name: lower case, forward slashes, so
// that 7-Zip's backslashes and DOS's upper case both compare the same way.
func zipPath(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, `\`, "/"))
}

// addFileToZip appends one file to an existing zip. The archive is rebuilt
// beside its original with every entry copied as raw compressed bytes
// (CreateRaw), so a 20 MB game is not recompressed just to gain a 60 byte
// batch — and each entry keeps its own CRC, so Pure's saves still match.
func addFileToZip(path, name, body string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("打不开 zip：%w", err)
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		zr.Close()
		return err
	}

	count := len(zr.File)

	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		hdr := f.FileHeader
		w, err := zw.CreateRaw(&hdr)
		if err != nil {
			zr.Close()
			out.Close()
			os.Remove(tmp)
			return fmt.Errorf("复制 %s：%w", f.Name, err)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.OpenRaw()
		if err != nil {
			zr.Close()
			out.Close()
			os.Remove(tmp)
			return fmt.Errorf("读 %s：%w", f.Name, err)
		}
		_, _ = io.Copy(w, rc)
	}
	if err := writeInto(zw, name, body); err != nil {
		zr.Close()
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("写入 %s：%w", name, err)
	}
	if err := zw.Close(); err != nil {
		zr.Close()
		out.Close()
		os.Remove(tmp)
		return err
	}
	zr.Close()
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	// Verify before replacing: the rewritten archive must still open and hold
	// exactly one more entry than the original.
	check, err := zip.OpenReader(tmp)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("新 zip 打不开：%w", err)
	}
	got := len(check.File)
	check.Close()
	if got != count+1 {
		os.Remove(tmp)
		return fmt.Errorf("新 zip 条目数 %d，原 %d", got, count)
	}

	return os.Rename(tmp, path)
}

// writeInto writes the batch body as an entry of the rebuilt archive.
func writeInto(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, body)
	return err
}
