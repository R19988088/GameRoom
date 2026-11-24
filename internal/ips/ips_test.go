package ips

import (
	"archive/zip"
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

// patchAt builds a standard IPS file: one record writing data at off, then EOF.
func patchAt(off int, data []byte) []byte {
	var b []byte
	b = append(b, []byte("PATCH")...)
	b = append(b, byte(off>>16), byte(off>>8), byte(off))
	b = append(b, byte(len(data)>>8), byte(len(data)))
	b = append(b, data...)
	b = append(b, []byte("EOF")...)
	return b
}

// patchRun builds an IPS run record: one byte repeated run times at off.
func patchRun(off, run int, val byte) []byte {
	var b []byte
	b = append(b, []byte("PATCH")...)
	b = append(b, byte(off>>16), byte(off>>8), byte(off))
	b = append(b, 0, 0)
	b = append(b, byte(run>>8), byte(run))
	b = append(b, val)
	b = append(b, []byte("EOF")...)
	return b
}

func writeZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScanReadsADatFolder: a folder with a .dat is read by that list alone —
// the .ips beside it are its pieces, not groups of their own — and a folder with
// no .dat has every .ips as a group.
func TestScanReadsADatFolder(t *testing.T) {
	dir := t.TempDir()
	dat := "ch-a-604.u17\tNoncheat-ch-a-604.u17\tCRC(D1E4836D)\r\n\r\n[zh_CN]\r\n说明\r\nNoncheat\r\n"
	if err := os.WriteFile(filepath.Join(dir, "Noncheat.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Noncheat-ch-a-604.u17.ips"),
		patchAt(0x7A, []byte{'X'}), 0o644); err != nil {
		t.Fatal(err)
	}
	// A fragment the .dat never lists: it is not a group.
	if err := os.WriteFile(filepath.Join(dir, "fragment.ips"), patchAt(0x10, []byte{'Y'}), 0o644); err != nil {
		t.Fatal(err)
	}

	groups := Scan(dir)
	if len(groups) != 1 || groups[0].Name != "Noncheat" {
		t.Fatalf("groups = %v, want only the .dat's group", groups)
	}
	if len(groups[0].Patches) != 1 {
		t.Fatalf("the .dat lists %d patches, want 1", len(groups[0].Patches))
	}
	p := groups[0].Patches[0]
	if p.Target != "ch-a-604.u17" || p.CRC != "D1E4836D" {
		t.Errorf("patch = %+v, want the target and CRC the .dat names", p)
	}

	// A folder of loose .ips is one group each.
	loose := t.TempDir()
	for _, name := range []string{"hack-a.ips", "hack-b.ips"} {
		if err := os.WriteFile(filepath.Join(loose, name), patchAt(0x10, []byte{'Z'}), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := Scan(loose)
	if len(got) != 2 {
		t.Fatalf("loose folder has %d groups, want 2", len(got))
	}
	for _, g := range got {
		if len(g.Patches) != 1 || g.Patches[0].Target != "" {
			t.Errorf("loose group = %+v, want one patch with no target of its own", g)
		}
	}
}

// TestPreparePatchesAMemberOfTheZip: the copy the core is handed has the patch
// applied to the member the .dat names, the other members are as they were, and
// the library's own zip is not touched.
func TestPreparePatchesAMemberOfTheZip(t *testing.T) {
	romDir := t.TempDir()
	romPath := filepath.Join(romDir, "ch-a.zip")
	member := append(make([]byte, 128), 0)
	member[0x7A] = 'z'
	writeZip(t, romPath, map[string][]byte{"ch-a-604.u17": member, "other.rom": []byte("keep me")})

	patchDir := t.TempDir()
	dat := "ch-a-604.u17\tNoncheat-ch-a-604.u17\r\n"
	if err := os.WriteFile(filepath.Join(patchDir, "Noncheat.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchDir, "Noncheat-ch-a-604.u17.ips"),
		patchAt(0x7A, []byte{'X'}), 0o644); err != nil {
		t.Fatal(err)
	}

	groups := Scan(patchDir)
	out, err := Prepare(romPath, groups, filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if filepath.Base(out) != "ch-a.zip" {
		t.Errorf("copy = %q, want the set's own file name", out)
	}
	r, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got := map[string][]byte{}
	for _, f := range r.File {
		b, err := readEntry(f)
		if err != nil {
			t.Fatal(err)
		}
		got[f.Name] = b
	}
	if got["ch-a-604.u17"][0x7A] != 'X' {
		t.Errorf("the patched byte = %q, want 'X'", got["ch-a-604.u17"][0x7A])
	}
	if string(got["other.rom"]) != "keep me" {
		t.Errorf("other.rom = %q, want it untouched", got["other.rom"])
	}

	// The library's zip keeps the byte it had.
	orig, _ := zip.OpenReader(romPath)
	defer orig.Close()
	for _, f := range orig.File {
		if f.Name != "ch-a-604.u17" {
			continue
		}
		b, _ := readEntry(f)
		if b[0x7A] != 'z' {
			t.Errorf("rom/%s was written: byte = %q", romPath, b[0x7A])
		}
	}
}

// TestPrepareRefusesAPatchForAnotherDump: a .dat that names the checksum its
// target should have does not apply to a file with another one.
func TestPrepareRefusesAPatchForAnotherDump(t *testing.T) {
	romDir := t.TempDir()
	romPath := filepath.Join(romDir, "ch-a.zip")
	writeZip(t, romPath, map[string][]byte{"ch-a-604.u17": make([]byte, 64)})

	patchDir := t.TempDir()
	// The CRC of a file of zeros this size, not of the one above.
	right := crc32.ChecksumIEEE(make([]byte, 32))
	dat := "ch-a-604.u17\tNoncheat-ch-a-604.u17\tCRC(" + hex(right) + ")\r\n"
	if err := os.WriteFile(filepath.Join(patchDir, "Noncheat.dat"), []byte(dat), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchDir, "Noncheat-ch-a-604.u17.ips"),
		patchAt(0x10, []byte{'X'}), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(romPath, Scan(patchDir), filepath.Join(t.TempDir(), "cache")); err == nil {
		t.Fatal("a patch whose CRC does not match was applied")
	}
}

// TestPreparePatchesAPlainRom: a rom that is not a zip is patched as a whole, by
// the loose .ips beside its folder.
func TestPreparePatchesAPlainRom(t *testing.T) {
	romDir := t.TempDir()
	romPath := filepath.Join(romDir, "game.nes")
	data := append(make([]byte, 64), 0)
	data[5] = 'a'
	if err := os.WriteFile(romPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	patchDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(patchDir, "translate.ips"), patchAt(5, []byte{'b'}), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Prepare(romPath, Scan(patchDir), filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if got[5] != 'b' {
		t.Errorf("byte 5 = %q, want 'b'", got[5])
	}
	if b, _ := os.ReadFile(romPath); b[5] != 'a' {
		t.Error("the library's rom was written")
	}
}

// TestApplyIPS32AndRuns: an IPS32 header is read the same way, and a zero-length
// record is a run of one byte.
func TestApplyIPS32AndRuns(t *testing.T) {
	data := make([]byte, 16)
	ips32 := append([]byte("IPS32"), 0, 0, 0, 0x08, 0, 4, 'a', 'b', 'c', 'd', 0, 'E', 'O', 'F')
	got, err := applyIPS(data, ips32)
	if err != nil {
		t.Fatalf("applyIPS: %v", err)
	}
	if string(got[8:12]) != "abcd" {
		t.Errorf("got %q at 8, want abcd", got[8:12])
	}

	got, err = applyIPS(data, patchRun(2, 6, 'r'))
	if err != nil {
		t.Fatalf("applyIPS run: %v", err)
	}
	if string(got[2:8]) != "rrrrrr" {
		t.Errorf("got %q at 2, want the run", got[2:8])
	}
}

// TestApplyIPSKeepsTheSizeThePatchDoesNotAskFor: EOF is a marker, not a length —
// a patch ending in "EOF" must not read those bytes as the file's new size and
// blow the member up.
func TestApplyIPSKeepsTheSizeThePatchDoesNotAskFor(t *testing.T) {
	data := make([]byte, 64)
	patch := patchAt(0x10, []byte{'a', 'b'})
	got, err := applyIPS(data, patch)
	if err != nil {
		t.Fatalf("applyIPS: %v", err)
	}
	if len(got) != len(data) {
		t.Errorf("patched size = %d, want the file's own %d", len(got), len(data))
	}

	// A size after EOF is honoured.
	grown := append(patchAt(0x10, []byte{'a'}), 0x00, 0x00, 0x80)
	got, err = applyIPS(make([]byte, 32), grown)
	if err != nil {
		t.Fatalf("applyIPS with a size: %v", err)
	}
	if len(got) != 128 {
		t.Errorf("patched size = %d, want the 128 the patch asks for", len(got))
	}
}

// hex is the spelling a .dat uses for checksums.
func hex(v uint32) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 8)
	for i := 0; i < 8; i++ {
		out[7-i] = digits[v&0xF]
		v >>= 4
	}
	return string(out)
}
