//go:build windows

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAsciiRomPathKeepsParentBesideTheSet: the mirror must hand MAME a path
// without spaces, and the set's parent has to be in the same folder, because
// MAME reads the parent from the directory it was given.
func TestAsciiRomPathKeepsParentBesideTheSet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Sim ui_D", "rom", "merge", "arc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rom := filepath.Join(dir, "ponpoko.zip")
	parent := filepath.Join(dir, "ponpoko_parent.zip")
	other := filepath.Join(dir, "bios.zip")
	for _, p := range []string{rom, parent, other} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mirror := asciiRomPath("mame", rom)
	if mirror == rom {
		t.Fatalf("a path with a space should be mirrored, got %q", mirror)
	}
	if !mameSafe(mirror) {
		t.Fatalf("mirror still unreadable by MAME: %q", mirror)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mirror), "ponpoko_parent.zip")); err != nil {
		t.Fatalf("parent not linked beside the set in %q", filepath.Dir(mirror))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(mirror), "bios.zip")); err != nil {
		t.Fatalf("bios not linked beside the set in %q", filepath.Dir(mirror))
	}
}

// TestAsciiRomPathSeparatesSetsThatShareAName: two patched sets of one driver
// both look like xymg.zip, and each must land in its own folder.
func TestAsciiRomPathSeparatesSetsThatShareAName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Sim ui_D", "rom")
	first := filepath.Join(root, "a", "xymg.zip")
	second := filepath.Join(root, "b", "xymg.zip")
	for _, p := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m1 := asciiRomPath("mame", first)
	m2 := asciiRomPath("mame", second)
	if m1 == m2 {
		t.Fatalf("two sets share one mirror file: %q", m1)
	}
	if !strings.HasSuffix(filepath.Base(m1), "xymg.zip") || !strings.HasSuffix(filepath.Base(m2), "xymg.zip") {
		t.Fatalf("the driver name must survive the mirror: %q %q", m1, m2)
	}
}
