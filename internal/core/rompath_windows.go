//go:build windows

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// mameCores are the cores that read a rom path as if it were command line text:
// MAME's libretro build splits what it is given on spaces and passes the pieces
// to its option parser, and opens files through the ANSI code page. A rom under
// "rom\majiang\80年\7toitsu.zip" therefore reads back as the game "80年" and a
// stray option "ui_D\rom\majiang\80…", and the game never starts.
var mameCores = []string{"mame", "mame2010", "mame2016", "mamearcade", "mame2003_plus", "mame2003_midway"}

// asciiRomPath hands a MAME-family core a path it can parse: when the real one
// carries a space or a non-ASCII byte, the same file is hard linked under an
// ASCII folder on the same drive first. A hard link is the same bytes on disk,
// so the core still reads the real zip; only the name it sees changes, and the
// file name (the driver MAME looks for) is kept.
//
// The file goes into a folder of its own, named after the rom plus a hash of
// the original path, and the rest of the rom's own folder is linked in beside
// it. MAME reads the set's parent — and the BIOS zip a driver asks for — from
// the directory it was handed, so a flat mirror with one name would leave
// clones without their parent and two sets called xymg.zip, one per patch,
// fighting over the same file. A folder per game gives each set its own
// directory with its own parent beside it.
//
// The mirrors live in <drive>\GAMEROOM\roms, beside the system mirror
// mamesys_windows.go builds, because the launcher's own folder is "Sim ui_D" and
// a mirror inside it would be mangled the same way.
func asciiRomPath(coreName, romPath string) string {
	if !isMameCore(coreName) || mameSafe(romPath) {
		return romPath
	}
	drive := filepath.VolumeName(romPath)
	if drive == "" {
		return romPath
	}
	base := filepath.Base(romPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	id := stem + "-" + pathHash(romPath)
	if !mameSafe(id) {
		id = pathHash(romPath)
	}
	dst := filepath.Join(drive+`\`, "GAMEROOM", "roms", id)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return romPath
	}
	to := filepath.Join(dst, base)
	if _, err := os.Stat(to); err != nil {
		if err := os.Link(romPath, to); err != nil {
			// Another volume or a filesystem without hard links: copy the bytes
			// once. Arcade zips are tens of megabytes, which is affordable here.
			if data, rerr := os.ReadFile(romPath); rerr == nil {
				if werr := os.WriteFile(to, data, 0o644); werr != nil {
					return romPath
				}
			} else {
				return romPath
			}
		}
	}
	linkSiblings(filepath.Dir(romPath), dst, base)
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "rom mirror: %q -> %q\n", romPath, to)
	}
	return to
}

// linkSiblings links the other files of the rom's folder in beside it, which
// is where a clone finds its parent set and a driver its BIOS. A folder with
// hundreds of roms is left alone: those sets carry their own files, and the
// cost of listing thousands of links per launch is not worth it.
func linkSiblings(srcDir, dstDir, skip string) {
	entries, err := os.ReadDir(srcDir)
	if err != nil || len(entries) > 64 {
		return
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == skip {
			continue
		}
		from := filepath.Join(srcDir, e.Name())
		to := filepath.Join(dstDir, e.Name())
		if _, err := os.Stat(to); err == nil {
			continue
		}
		if err := os.Link(from, to); err != nil {
			if data, rerr := os.ReadFile(from); rerr == nil {
				os.WriteFile(to, data, 0o644)
			}
		}
	}
}

// pathHash names a mirror folder: six hex digits of the path are enough to
// keep two sets that share a file name apart.
func pathHash(p string) string {
	var h uint32 = 2166136261
	for _, r := range p {
		h ^= uint32(r)
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

// mameSafe reports whether MAME can read the path as it stands: plain ASCII and
// no whitespace.
func mameSafe(p string) bool {
	for _, r := range p {
		if r > 127 || r == ' ' {
			return false
		}
	}
	return true
}

func isMameCore(name string) bool {
	for _, c := range mameCores {
		if strings.EqualFold(name, c) {
			return true
		}
	}
	return false
}
