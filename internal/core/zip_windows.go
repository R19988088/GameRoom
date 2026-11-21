//go:build windows

package core

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// romExts are the files inside a zip that may be the rom itself. Anything else
// (a readme, a save, a screenshot) is skipped, so the biggest one of these is
// the picture the player asked for.
var romExts = map[string]bool{
	".nes": true, ".unf": true, ".unif": true, ".fds": true,
	".sfc": true, ".smc": true, ".swc": true, ".fig": true, ".bs": true,
	".gb": true, ".gbc": true, ".gba": true, ".nds": true,
	".md": true, ".gen": true, ".smd": true, ".sms": true, ".gg": true,
	".pce": true, ".sgx": true, ".ws": true, ".wsc": true, ".ngp": true, ".ngc": true,
	".a26": true, ".a78": true, ".col": true, ".int": true, ".vb": true,
	".n64": true, ".z64": true, ".v64": true, ".lnx": true,
	".bin": true, ".iso": true, ".cue": true, ".img": true,
}

func isZip(path string) bool { return strings.EqualFold(filepath.Ext(path), ".zip") }

// biggestRom is the entry a console rom zip is named after: the largest file
// whose extension is one a console reads. A zip with none of them is not a rom
// container at all and is left alone.
func biggestRom(files []*zip.File) *zip.File {
	var best *zip.File
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		if !romExts[strings.ToLower(filepath.Ext(f.Name))] {
			continue
		}
		if best == nil || f.UncompressedSize64 > best.UncompressedSize64 {
			best = f
		}
	}
	return best
}

func openZip(path string) ([]*zip.File, func(), bool) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, func() {}, false
	}
	return r.File, func() { r.Close() }, true
}

// romBytesFromZip is the rom in memory, for a core that can take its content
// as bytes: nothing is written anywhere, so nothing has to be cleaned up.
func romBytesFromZip(zipPath string) ([]byte, bool) {
	files, closeZip, ok := openZip(zipPath)
	if !ok {
		return nil, false
	}
	defer closeZip()
	f := biggestRom(files)
	if f == nil {
		return nil, false
	}
	rc, err := f.Open()
	if err != nil {
		return nil, false
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// cacheRoot is where a rom pulled out of a zip is kept: beside the system
// folder the cores were given, so it never lands in the user's rom library.
func cacheRoot(systemDir string) string {
	return filepath.Join(filepath.Dir(systemDir), "cache", "roms")
}

// romFromZip writes the rom a zip holds to the cache and answers with its
// path, for a core that only opens a plain file. The name comes from the zip
// and its folder, so the same game lands on the same path every time: the
// save the core writes beside it is found again on the next launch.
func romFromZip(zipPath, root string) (string, bool) {
	files, closeZip, ok := openZip(zipPath)
	if !ok {
		return "", false
	}
	defer closeZip()
	f := biggestRom(files)
	if f == nil {
		return "", false
	}
	dir := filepath.Join(root, filepath.Base(filepath.Dir(zipPath)), strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	dst := filepath.Join(dir, filepath.Base(f.Name))
	rc, err := f.Open()
	if err != nil {
		return "", false
	}
	defer rc.Close()
	written, err := os.Create(dst)
	if err != nil {
		return "", false
	}
	defer written.Close()
	if _, err := io.Copy(written, rc); err != nil {
		return "", false
	}
	return dst, true
}
