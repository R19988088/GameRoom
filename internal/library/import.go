package library

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Import reads the library of the old frontend ("simUI", whose data is a
// SQLite file named data.dll) and writes it here, cleaned:
//
//   - a game's display name and its rom file are separate columns,
//   - rom paths are relative to rom/ and keep the platform's own
//     sub-folders ("arc/SNK/dino.zip"),
//   - menus that are junk ("_7b9") become 未分类,
//   - only covers are kept: each one is copied into 数据/covers/, so a
//     game's art is a relative path and the old install can go away,
//   - a rom's size is read from the file, not from the old text "3.93MB",
//   - a game's own emulator choice is kept as a core name, not a JSON blob,
//   - emulator paths point at the new layout, so RetroArch lives in
//     模拟器/RetroArch,
//   - rows that name the same rom twice are dropped.
//
// root is the folder the launcher runs from: covers are written under its
// 数据/ and sizes read from its rom/. oldRoot is the old install, the only
// place the art still lives.
func Import(db *sql.DB, oldPath, oldRoot, root string) error {
	tmp, err := copyForRead(oldPath)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	old, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	defer old.Close()

	platforms, romDirs, artDirs, err := readPlatforms(old, oldRoot)
	if err != nil {
		return fmt.Errorf("read platforms: %w", err)
	}
	menus, err := readMenus(old, platforms)
	if err != nil {
		return fmt.Errorf("read menus: %w", err)
	}
	sims, err := readSims(old, platforms, oldRoot)
	if err != nil {
		return fmt.Errorf("read simulators: %w", err)
	}
	games, err := readGames(old, platforms, romDirs, artDirs, root)
	if err != nil {
		return fmt.Errorf("read games: %w", err)
	}
	// The old library let every game pick its own emulator, so one console
	// held a dozen of them. Everything here runs through RetroArch with
	// the canonical core for its system instead, which is what keeps the
	// CRT shader the same from game to game.
	NormalizeConsoles(&sims, games)
	// The old library's extension lists were mostly RetroArch's whole
	// supported set copied in, "o", "xml" and "wav" among them. What a
	// platform really holds is what its roms say, so the lists come from
	// the games instead.
	seenExt := map[string]map[string]bool{}
	for _, g := range games {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(g.RomPath), "."))
		if ext == "" {
			continue
		}
		if seenExt[g.PlatformKey] == nil {
			seenExt[g.PlatformKey] = map[string]bool{}
		}
		seenExt[g.PlatformKey][ext] = true
	}
	for i := range platforms {
		set := seenExt[platforms[i].Key]
		if len(set) == 0 {
			continue
		}
		var exts []string
		for _, e := range platforms[i].Exts { // keep the old order for what exists
			if set[e] {
				exts = append(exts, e)
				delete(set, e)
			}
		}
		var rest []string
		for e := range set {
			rest = append(rest, e)
		}
		sort.Strings(rest)
		platforms[i].Exts = append(exts, rest...)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range []string{"DELETE FROM game", "DELETE FROM menu", "DELETE FROM sim", "DELETE FROM platform"} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	for _, p := range platforms {
		if _, err := tx.Exec(`INSERT INTO platform (id, key, name, grp, exts, sort)
			VALUES (?, ?, ?, ?, ?, ?)`,
			p.ID, p.Key, p.Name, p.Group, joinList(p.Exts), p.Sort); err != nil {
			return fmt.Errorf("insert platform %s: %w", p.Key, err)
		}
	}
	for _, m := range menus {
		// A menu is its name under a platform; nothing refers to its id,
		// so the ids are left to the database.
		if _, err := tx.Exec(`INSERT INTO menu (platform_key, name, sort) VALUES (?, ?, ?)`,
			m.PlatformKey, m.Name, m.Sort); err != nil {
			return fmt.Errorf("insert menu %s: %w", m.Name, err)
		}
	}
	for _, s := range sims {
		if _, err := tx.Exec(`INSERT INTO sim (id, platform_key, name, exe, core, args, unzip, is_default)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.PlatformKey, s.Name, s.Exe, s.Core, s.Args, s.Unzip, boolInt(s.IsDefault)); err != nil {
			return fmt.Errorf("insert sim %s: %w", s.Name, err)
		}
	}
	for _, g := range games {
		if _, err := tx.Exec(`INSERT INTO game (id, platform_key, menu, title, rom_path, size, cover,
			year, publisher, genre, region, favorite, core, sim_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			g.ID, g.PlatformKey, g.Menu, g.Title, g.RomPath, g.Size, g.Cover, g.Year,
			g.Publisher, g.Genre, g.Region, boolInt(g.Favorite), g.Core, g.SimID); err != nil {
			return fmt.Errorf("insert game %s: %w", g.Title, err)
		}
	}
	if err := SetSetting(tx, "imported", "1"); err != nil {
		return err
	}
	_ = SetSetting(tx, "imported_at", time.Now().Format("2006-01-02 15:04:05"))
	_ = SetSetting(tx, "imported_from", oldPath)
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// copyForRead copies a SQLite database, with its write-ahead log when it
// has one, so it can be read while the old frontend still runs.
func copyForRead(path string) (string, error) {
	f, err := os.CreateTemp("", "simui-import-*.db")
	if err != nil {
		return "", err
	}
	dest := f.Name()
	f.Close()

	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if data, err := os.ReadFile(path + suffix); err == nil {
			_ = os.WriteFile(dest+suffix, data, 0o600)
		}
	}
	return dest, nil
}

// row reads a table as maps, so columns a version lacks are just absent.
type row map[string]string

func readRows(db *sql.DB, query string) ([]row, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := make(row, len(cols))
		for i, c := range cols {
			r[strings.ToLower(c)] = valueString(vals[i])
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func valueString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case []byte:
		return strings.TrimSpace(string(x))
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// junkMenu matches menu names the old frontend wrote from folders that
// were not menus at all, as "_7b9".
var junkMenu = regexp.MustCompile(`^_[0-9a-zA-Z]+$`)

// cleanMenu turns a menu name into one the launcher shows, "" for a
// game that belongs to none.
func cleanMenu(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || junkMenu.MatchString(name) {
		return ""
	}
	return name
}

// readPlatforms reads the platforms and remembers where the old
// frontend kept each one's roms and art, so their paths can be cut down
// and their covers copied.
func readPlatforms(old *sql.DB, oldRoot string) ([]Platform, map[string]string, map[string][]string, error) {
	rows, err := readRows(old, `SELECT * FROM platform ORDER BY sort, id`)
	if err != nil {
		return nil, nil, nil, err
	}
	romDirs := map[string]string{}
	artDirs := map[string][]string{}
	var out []Platform
	for _, r := range rows {
		oldRomDir := trimSlash(normSlash(r["rom_path"]))
		key := filepath.Base(filepath.Dir("/" + oldRomDir))
		if key == "." || key == "" || key == "/" {
			key = r["pinyin"]
		}
		if key == "" || oldRomDir == "" {
			continue
		}
		sort, _ := strconv.Atoi(r["sort"])
		p := Platform{
			ID:    atoi(r["id"]),
			Key:   key,
			Name:  r["name"],
			Group: groupOf(key),
			Exts:  splitExts(r["rom_exts"]),
			Sort:  sort,
		}
		// Covers come from the art folders the launcher reads later.
		// Video, gif, doc and the rest are left behind on purpose.
		for _, name := range []string{"title_path", "poster_path", "snap_path", "thumb_path", "snaps_path", "thumbs_path", "background_path"} {
			if dir := r[name]; dir != "" {
				artDirs[key] = append(artDirs[key], oldRoot+"/"+trimSlash(normSlash(dir)))
			}
		}
		romDirs[key] = oldRomDir
		out = append(out, p)
	}
	return out, romDirs, artDirs, nil
}

func readMenus(old *sql.DB, platforms []Platform) ([]Menu, error) {
	rows, err := readRows(old, `SELECT * FROM menu`)
	if err != nil {
		return nil, err
	}
	byID := map[int]*Platform{}
	for i := range platforms {
		byID[platforms[i].ID] = &platforms[i]
	}
	seen := map[string]bool{}
	var out []Menu
	add := func(p *Platform, name string, id, sort int) {
		name = cleanMenu(name)
		if name == "" || seen[p.Key+"\x00"+name] {
			return
		}
		seen[p.Key+"\x00"+name] = true
		out = append(out, Menu{ID: id, PlatformKey: p.Key, Name: name, Sort: sort})
	}
	for _, r := range rows {
		p := byID[atoi(r["platform"])]
		if p == nil {
			continue
		}
		add(p, r["name"], atoi(r["id"]), atoi(r["sort"]))
	}
	// Menus the games name but the menu table lacks still get a row.
	gameMenus, err := readRows(old, `SELECT DISTINCT platform, menu FROM rom`)
	if err != nil {
		return nil, err
	}
	for _, r := range gameMenus {
		p := byID[atoi(r["platform"])]
		if p == nil {
			continue
		}
		add(p, r["menu"], 0, 100)
	}
	return out, nil
}

func readSims(old *sql.DB, platforms []Platform, oldRoot string) ([]Sim, error) {
	rows, err := readRows(old, `SELECT * FROM simulator`)
	if err != nil {
		return nil, err
	}
	byID := map[int]*Platform{}
	for i := range platforms {
		byID[platforms[i].ID] = &platforms[i]
	}
	var out []Sim
	for _, r := range rows {
		p := byID[atoi(r["platform"])]
		if p == nil || r["path"] == "" {
			continue
		}
		exe := normSlash(r["path"])
		base := filepath.Base(exe)
		var stored string
		if strings.EqualFold(base, "retroarch.exe") {
			// RetroArch has a place of its own now.
			stored = "模拟器/RetroArch/retroarch.exe"
		} else {
			stored = oldRoot + "/" + exe
		}
		default_, _ := strconv.Atoi(r["default"])
		// The core is kept by name, so the launcher can find its file on
		// whichever system it runs.
		core, rest := SplitCore(r["cmd"])
		out = append(out, Sim{
			ID:          atoi(r["id"]),
			PlatformKey: p.Key,
			Name:        first(r["name"]),
			Exe:         stored,
			Core:        core,
			Args:        rest,
			Unzip:       atoi(r["unzip"]),
			IsDefault:   default_ == 1,
		})
	}
	return out, nil
}

func readGames(old *sql.DB, platforms []Platform, romDirs map[string]string, artDirs map[string][]string, root string) ([]Game, error) {
	rows, err := readRows(old, `SELECT * FROM rom`)
	if err != nil {
		return nil, err
	}
	byID := map[int]*Platform{}
	for i := range platforms {
		byID[platforms[i].ID] = &platforms[i]
	}
	art := map[string]*artIndex{}
	var out []Game
	seen := map[string]bool{}
	for _, r := range rows {
		p := byID[atoi(r["platform"])]
		if p == nil || r["rom_path"] == "" {
			continue
		}
		rel := stripDir(r["rom_path"], romDirs[p.Key])
		if rel == "" {
			continue
		}
		romPath := p.Key + "/" + rel
		key := p.Key + "/" + strings.ToLower(romPath)
		if seen[key] {
			continue
		}
		seen[key] = true

		ix := art[p.Key]
		if ix == nil {
			ix = newArtIndex(artDirs[p.Key])
			art[p.Key] = ix
		}

		g := Game{
			ID:          atoi(r["id"]),
			PlatformKey: p.Key,
			Menu:        cleanMenu(r["menu"]),
			Title:       first(r["name"], strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))),
			RomPath:     romPath,
			Size:        sizeOf(root, romPath, r["size"]),
			Cover:       coverInto(root, p.Key, ix.find(stem(rel))),
			Year:        atoi(r["base_year"]),
			Publisher:   r["base_publisher"],
			Genre:       r["base_type"],
			Region:      r["base_country"],
			Favorite:    atoi(r["star"]) != 0,
			Core:        coreOverride(r["sim_conf"], atoi(r["sim_id"])),
			SimID:       atoi(r["sim_id"]),
		}
		out = append(out, g)
	}
	return out, nil
}

// sizeOf reads a rom's size from the file itself, falling back to the
// old frontend's text ("3.93MB") only when the file is gone.
func sizeOf(root, romPath, old string) int64 {
	if st, err := os.Stat(filepath.Join(root, "rom", filepath.FromSlash(romPath))); err == nil {
		return st.Size()
	}
	return parseSize(old)
}

var sizeText = regexp.MustCompile(`(?i)^([\d.]+)\s*(b|kb|mb|gb|tb)?$`)

// parseSize turns "3.93MB" into bytes, 0 when it cannot.
func parseSize(s string) int64 {
	m := sizeText.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	units := map[string]float64{"": 1, "b": 1, "kb": 1 << 10, "mb": 1 << 20, "gb": 1 << 30, "tb": 1 << 40}
	return int64(n * units[strings.ToLower(m[2])])
}

// coverInto copies a game's cover into 数据/covers/<platform>/, so the
// library points at its own copy and the old install can be deleted.
// It returns the path relative to 数据/, "" when the game has no cover.
func coverInto(root, key, src string) string {
	if src == "" {
		return ""
	}
	name := filepath.Base(src)
	dir := filepath.Join(root, "数据", "covers", key)
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ""
		}
		if err := copyFile(src, dest); err != nil {
			return ""
		}
	}
	return "covers/" + key + "/" + name
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	return out.Close()
}

// stripDir cuts the platform's rom folder off a rom path, so
// "games/arc/roms/SNK/dino.zip" becomes "SNK/dino.zip".
func stripDir(path, dir string) string {
	p := trimSlash(normSlash(path))
	d := trimSlash(normSlash(dir))
	if d == "" {
		return p
	}
	lower := strings.ToLower(p)
	if strings.HasPrefix(lower, strings.ToLower(d)+"/") {
		return trimSlash(p[len(d)+1:])
	}
	if i := strings.Index(lower, "/"+strings.ToLower(d)+"/"); i >= 0 {
		return trimSlash(p[i+len(d)+2:])
	}
	return p
}

// artIndex finds covers by the rom's file name, which is how the old
// frontend named them: dino.zip -> title/dino.png.
type artIndex struct {
	files map[string]string
}

var artExts = map[string]int{".png": 0, ".webp": 1, ".jpg": 2, ".jpeg": 3}

func newArtIndex(dirs []string) *artIndex {
	ix := &artIndex{files: map[string]string{}}
	for _, dir := range dirs {
		filepath.WalkDir(filepath.FromSlash(dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			rank, ok := artExts[ext]
			if !ok {
				return nil
			}
			base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
			if cur, ok := ix.files[base]; ok {
				if rank >= rankOf(cur) {
					return nil
				}
			}
			ix.files[base] = path
			return nil
		})
	}
	return ix
}

func rankOf(path string) int {
	if r, ok := artExts[strings.ToLower(filepath.Ext(path))]; ok {
		return r
	}
	return 99
}

func (ix *artIndex) find(base string) string {
	if base == "" {
		return ""
	}
	return ix.files[strings.ToLower(base)]
}

// simConf is the old frontend's per-game emulator note: one entry under
// each sim's id, naming the command line it would run.
type simConf struct {
	Cmd   string `json:"Cmd"`
	Unzip int    `json:"Unzip"`
	File  string `json:"File"`
	Lua   string `json:"Lua"`
}

// coreOverride reads the core a game asked for, preferring the entry of
// the sim the game names. Entries that disagree are dropped: picking one
// would start an emulator the game never asked for.
func coreOverride(conf string, simID int) string {
	conf = strings.TrimSpace(conf)
	if conf == "" || conf == "{}" || conf == "null" {
		return ""
	}
	var confs map[string]simConf
	if err := json.Unmarshal([]byte(conf), &confs); err != nil {
		return ""
	}
	cmdOf := func(c simConf) string {
		if c.Cmd == "" {
			return c.Lua
		}
		return c.Cmd
	}
	if c, ok := confs[strconv.Itoa(simID)]; ok {
		if core, _ := SplitCore(cmdOf(c)); core != "" {
			return core
		}
	}
	distinct := map[string]bool{}
	for _, c := range confs {
		if core, _ := SplitCore(cmdOf(c)); core != "" {
			distinct[core] = true
		}
	}
	if len(distinct) == 1 {
		for core := range distinct {
			return core
		}
	}
	return ""
}

// groupOf puts a platform on one of the shelves the top bar shows. The old
// library has no such notion: a platform is a folder, and what ran on it is
// what decides. 麻将 is its own shelf rather than a corner of the arcade.
func groupOf(key string) string {
	switch key {
	case "arc":
		return "arcade"
	case "majiang":
		return "mahjong"
	case "掌机":
		return "handheld"
	case "微软":
		return "pc"
	default:
		return "console"
	}
}

func normSlash(p string) string { return strings.ReplaceAll(strings.TrimSpace(p), "\\", "/") }
func trimSlash(p string) string { return strings.Trim(p, "/") }
func stem(p string) string      { return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) }

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
