// Package ips applies the .ips patches a 数据/ips folder holds to the rom a
// game boots from.
//
// The folder is the one a MAME Plus build kept beside its roms: one directory
// per set, named after the set's rom file ("jojojr1"). A .dat lists which file
// inside the set a patch changes, and the patch beside it is a standard IPS
// (or IPS32) file. A folder without a .dat is read the other way round: every
// .ips is a group of its own, and the file it changes comes from its name.
//
// Nothing in rom/ is ever written: a patched copy is built under the emulator's
// cache folder and the core is handed that copy.
package ips

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Patch is one .ips file and the file inside the rom it changes.
type Patch struct {
	// Target is the file inside the zip a MAME Plus .dat names, "" when the
	// folder only had the .ips and the name has to tell.
	Target string
	// CRC is the checksum the .dat expects of the target, "" when it says none.
	CRC string
	// Path is the .ips on disk.
	Path string
}

// Group is one patch the menu offers as a row: a .dat and the patches it lists,
// or a loose .ips on its own.
type Group struct {
	Name    string
	Dir     string
	Patches []Patch
}

// Scan reads an ips folder: one group per .dat. A folder with no .dat at all is
// read the other way round — every .ips is a group of its own. A folder with a
// .dat keeps only what the .dat lists: the .ips files beside it are the pieces
// its groups are made of, not patches on their own.
func Scan(dir string) []Group {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var groups []Group
	hasDat := false
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".dat") {
			continue
		}
		g, ok := readDat(filepath.Join(dir, e.Name()))
		if !ok {
			continue // a broken .dat is skipped; the rest of the folder still counts
		}
		hasDat = true
		groups = append(groups, g)
	}
	if !hasDat {
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".ips") {
				continue
			}
			name := e.Name()
			groups = append(groups, Group{
				Name:    strings.TrimSuffix(name, filepath.Ext(name)),
				Dir:     dir,
				Patches: []Patch{{Path: filepath.Join(dir, name)}},
			})
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups
}

// readDat parses one MAME Plus patch list: a line per patch, the target's name,
// the patch's name without its .ips, and the checksum the target should have.
// Lines under a [language] heading are notes, not patches.
func readDat(path string) (Group, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Group{}, false
	}
	// Some of these lists were saved with a BOM: it would otherwise sit at the
	// front of the first target's name and make it miss its file.
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	g := Group{
		Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Dir:  filepath.Dir(path),
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") ||
			strings.HasPrefix(line, "[") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			fields = strings.Fields(line)
		}
		if len(fields) < 2 {
			continue
		}
		target := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(fields[1])
		if name == "" {
			continue
		}
		p := Patch{Target: target}
		if len(fields) > 2 {
			p.CRC = crcOf(fields[2])
		}
		p.Path = findPatch(g.Dir, name)
		if p.Path == "" {
			continue
		}
		g.Patches = append(g.Patches, p)
	}
	return g, len(g.Patches) > 0
}

// findPatch resolves the name a .dat lists to the .ips beside it: the list
// leaves the extension off.
func findPatch(dir, name string) string {
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}
	for _, candidate := range []string{name, name + ".ips", name + ".IPS"} {
		p := filepath.Join(dir, candidate)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// crcOf reads the checksum out of a .dat's "CRC(D1E4836D)" field.
func crcOf(field string) string {
	field = strings.TrimSpace(field)
	up := strings.ToUpper(field)
	at := strings.Index(up, "CRC")
	if at < 0 {
		return ""
	}
	rest := field[at+3:]
	rest = strings.TrimLeft(rest, "(: ")
	end := strings.IndexAny(rest, ") ")
	if end >= 0 {
		rest = rest[:end]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	return strings.ToUpper(rest)
}

// Signature names a patched copy: which groups, and how big and how new each
// patch file is, so editing a patch builds a new copy instead of playing the
// old one.
func Signature(groups []Group) string {
	h := fnv.New32a()
	for _, g := range groups {
		io.WriteString(h, g.Name)
		io.WriteString(h, "\x00")
		for _, p := range g.Patches {
			io.WriteString(h, filepath.Base(p.Path))
			if st, err := os.Stat(p.Path); err == nil {
				fmt.Fprintf(h, "%d/%d", st.Size(), st.ModTime().UnixNano())
			}
			io.WriteString(h, "\x00")
		}
	}
	return fmt.Sprintf("%08x", h.Sum32())
}

// Prepare writes the rom with the groups' patches applied under cacheRoot and
// answers with the path of that copy. The same rom and the same groups land on
// the same path, so a game that keeps its patches pays the cost once.
func Prepare(romPath string, groups []Group, cacheRoot string) (string, error) {
	if len(groups) == 0 {
		return romPath, nil
	}
	if _, err := os.Stat(romPath); err != nil {
		return "", err
	}
	stem := strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath))
	dir := filepath.Join(cacheRoot, stem+"-"+Signature(groups))
	out := filepath.Join(dir, filepath.Base(romPath))
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := out + ".tmp"
	var err error
	if strings.EqualFold(filepath.Ext(romPath), ".zip") {
		err = patchZip(romPath, tmp, groups)
	} else {
		err = patchPlain(romPath, tmp, groups)
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	os.Remove(out)
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	// MAME reads a clone's parent set, and a driver its BIOS, from the folder it
	// was handed: the patched copy sits in a folder of its own, so the rom's
	// neighbours are linked in beside it.
	linkSiblings(filepath.Dir(romPath), dir, filepath.Base(romPath))
	return out, nil
}

// Check reports whether the groups fit this rom at all: every patch names a
// member the set holds, and the checksum is the one the .dat expects. It reads
// and applies nothing to disk, so a folder can be told apart from its dump
// before a launch is tried.
func Check(romPath string, groups []Group) error {
	if len(groups) == 0 {
		return nil
	}
	if strings.EqualFold(filepath.Ext(romPath), ".zip") {
		r, err := zip.OpenReader(romPath)
		if err != nil {
			return err
		}
		defer r.Close()
		members := make([]string, 0, len(r.File))
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				members = append(members, f.Name)
			}
		}
		byName := make(map[string]*zip.File, len(r.File))
		for _, f := range r.File {
			byName[f.Name] = f
		}
		for _, g := range groups {
			for _, p := range g.Patches {
				m := targetEntry(p, members)
				if m == "" {
					return fmt.Errorf("%s：补丁 %s 对不上 rom 里的文件", g.Name, filepath.Base(p.Path))
				}
				f := byName[m]
				if f == nil {
					return fmt.Errorf("%s：补丁 %s 对不上 rom 里的文件", g.Name, filepath.Base(p.Path))
				}
				data, err := readEntry(f)
				if err != nil {
					return err
				}
				if _, err := applyOne(data, p); err != nil {
					return fmt.Errorf("%s：%w", g.Name, err)
				}
			}
		}
		return nil
	}
	data, err := os.ReadFile(romPath)
	if err != nil {
		return err
	}
	for _, g := range groups {
		for _, p := range g.Patches {
			if p.Target != "" && !matchesEntry(p.Target, filepath.Base(romPath)) {
				continue
			}
			if _, err := applyOne(data, p); err != nil {
				return fmt.Errorf("%s：%w", g.Name, err)
			}
		}
	}
	return nil
}

// patchZip writes a copy of the zip with the patches applied to its members.
// Entries nothing patches keep their compressed bytes as they were.
func patchZip(romPath, out string, groups []Group) error {
	r, err := zip.OpenReader(romPath)
	if err != nil {
		return err
	}
	defer r.Close()

	members := make([]string, 0, len(r.File))
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			members = append(members, f.Name)
		}
	}
	// Every patch is aimed at one member before any of them is applied: a loose
	// .ips has only its file name to go by, and the name has to be measured
	// against the whole set.
	type aimed struct {
		patch  Patch
		member string
	}
	var aims []aimed
	for _, g := range groups {
		for _, p := range g.Patches {
			m := targetEntry(p, members)
			if m == "" {
				return fmt.Errorf("%s 的补丁 %s 对不上 rom 里的文件", g.Name, filepath.Base(p.Path))
			}
			aims = append(aims, aimed{patch: p, member: m})
		}
	}

	fw, err := os.Create(out)
	if err != nil {
		return err
	}
	defer fw.Close()
	zw := zip.NewWriter(fw)

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			fh := f.FileHeader
			if _, err := zw.CreateRaw(&fh); err != nil {
				return err
			}
			continue
		}
		var data []byte
		changed := false
		for _, aim := range aims {
			if aim.member != f.Name {
				continue
			}
			if data == nil {
				b, err := readEntry(f)
				if err != nil {
					return err
				}
				data = b
			}
			data, err = applyOne(data, aim.patch)
			if err != nil {
				return fmt.Errorf("%s：%w", filepath.Base(aim.patch.Path), err)
			}
			changed = true
		}
		if changed {
			hdr := &zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified}
			w, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
			continue
		}
		rc, err := f.OpenRaw()
		if err != nil {
			return err
		}
		fh := f.FileHeader
		w, err := zw.CreateRaw(&fh)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			return err
		}
	}
	return zw.Close()
}

// patchPlain writes a patched copy of a rom that is a plain file: nothing is
// inside it to name, so the patches apply to the file itself.
func patchPlain(romPath, out string, groups []Group) error {
	data, err := os.ReadFile(romPath)
	if err != nil {
		return err
	}
	for _, g := range groups {
		for _, p := range g.Patches {
			// A .dat written for a zip's member does not fit a plain rom.
			if p.Target != "" && !matchesEntry(p.Target, filepath.Base(romPath)) {
				continue
			}
			if data, err = applyOne(data, p); err != nil {
				return fmt.Errorf("%s：%w", filepath.Base(p.Path), err)
			}
		}
	}
	return os.WriteFile(out, data, 0o644)
}

// targetEntry is the member a patch changes: the one the .dat names, or — for a
// loose .ips — the one its file name ends with ("MyHack-ch-a-604.u17.ips").
func targetEntry(p Patch, members []string) string {
	if p.Target != "" {
		for _, m := range members {
			if matchesEntry(p.Target, m) {
				return m
			}
		}
		return ""
	}
	stem := strings.TrimSuffix(filepath.Base(p.Path), filepath.Ext(p.Path))
	for _, m := range members {
		if tailMatches(stem, m) {
			return m
		}
	}
	if len(members) == 1 {
		return members[0]
	}
	return ""
}

// matchesEntry reports whether a name a .dat lists is this member: the whole
// name, its file name, or the tail of a folder.
func matchesEntry(target, member string) bool {
	t, m := normName(target), normName(member)
	if t == "" || m == "" {
		return false
	}
	return m == t || baseName(m) == baseName(t) || strings.HasSuffix(m, "/"+t)
}

// tailMatches reports whether a patch file's name ends with a member's: MAME
// Plus names its patches "组名-目标文件.ips".
func tailMatches(stem, member string) bool {
	s, m := normName(stem), normName(member)
	if s == "" || m == "" {
		return false
	}
	if s == m || s == baseName(m) || strings.HasSuffix(s, "/"+m) {
		return true
	}
	if !strings.HasSuffix(s, baseName(m)) {
		return false
	}
	// The characters before the match must separate the group's name from the
	// target's, so "xch-a-604.u17" does not pass for "ch-a-604.u17".
	at := len(s) - len(baseName(m)) - 1
	if at < 0 {
		return true
	}
	switch s[at] {
	case '-', '_', '.', '/', '+', ' ':
		return true
	}
	return false
}

func normName(s string) string { return strings.ToLower(strings.ReplaceAll(s, "\\", "/")) }

func baseName(s string) string {
	if at := strings.LastIndex(s, "/"); at >= 0 {
		return s[at+1:]
	}
	return s
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// applyOne runs one patch over a file's bytes, checking the checksum the .dat
// expects first: a patch aimed at another dump must not be applied to this one.
func applyOne(data []byte, p Patch) ([]byte, error) {
	if p.CRC != "" {
		var want uint32
		if _, err := fmt.Sscanf(p.CRC, "%X", &want); err == nil {
			if got := crc32.ChecksumIEEE(data); got != want {
				return nil, fmt.Errorf("CRC 不符：补丁要 %s，这里是 %X", p.CRC, got)
			}
		}
	}
	raw, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, err
	}
	return applyIPS(data, raw)
}

// applyIPS is the standard IPS format, and the IPS32 one some hacks use: the
// header, then offset/length records until EOF. A length of zero is a run of
// one repeated byte instead.
func applyIPS(data, patch []byte) ([]byte, error) {
	out := append([]byte(nil), data...)
	pos, offLen := 0, 3
	switch {
	case bytes.HasPrefix(patch, []byte("PATCH")):
		pos = 5
	case bytes.HasPrefix(patch, []byte("IPS32")):
		pos, offLen = 5, 4
	default:
		return nil, fmt.Errorf("不是 IPS 补丁")
	}
	grow := func(to int) {
		for len(out) < to {
			out = append(out, 0)
		}
	}
	eof := false
	for pos+offLen+2 <= len(patch) {
		off := readBig(patch[pos:], offLen)
		pos += offLen
		if off == 0x454F46 { // "EOF"
			// The marker is eaten here, so only what comes after it is read as
			// the file's new length.
			eof = true
			break
		}
		size := int(binary.BigEndian.Uint16(patch[pos:]))
		pos += 2
		switch {
		case size == 0:
			if pos+3 > len(patch) {
				return nil, fmt.Errorf("补丁在 RLE 段中间断了")
			}
			run := int(binary.BigEndian.Uint16(patch[pos:]))
			pos += 2
			b := patch[pos]
			pos++
			grow(off + run)
			for i := 0; i < run; i++ {
				out[off+i] = b
			}
		default:
			if pos+size > len(patch) {
				return nil, fmt.Errorf("补丁在数据段中间断了")
			}
			grow(off + size)
			copy(out[off:off+size], patch[pos:pos+size])
			pos += size
		}
	}
	// An EOF may be followed by the file's new size, which grows or cuts it.
	if eof && pos+3 <= len(patch) {
		if to := int(readBig(patch[pos:], 3)); to > 0 {
			grow(to)
			out = out[:to]
		}
	}
	return out, nil
}

func readBig(b []byte, n int) int {
	v := 0
	for i := 0; i < n && i < len(b); i++ {
		v = v<<8 | int(b[i])
	}
	return v
}

// linkSiblings links the other files of the rom's folder into the patched copy's
// folder, which is where MAME looks for a clone's parent and a driver's BIOS.
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
			if b, rerr := os.ReadFile(from); rerr == nil {
				os.WriteFile(to, b, 0o644)
			}
		}
	}
}
