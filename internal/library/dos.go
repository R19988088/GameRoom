package library

import (
	"database/sql"
	"encoding/csv"
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

// The two machines of the DOS&PC98 shelf. A DOS game is the game's zip as its
// author packed it — DOSBox Pure mounts it and runs the play.BAT inside; a
// PC-98 game is the disk image (hdi / fdi / d88), never the archive it came in,
// because NP2kai mounts an image and knows nothing of zips.
const (
	DOSKey   = "dos"
	PC98Key  = "pc98"
	DOSGroup = "dos"
	dosSort  = 6
	pc98Sort = 7
	dosCore  = "dosbox_pure"
	pc98Core = "np2kai"
	emulator = "模拟器/RetroArch/retroarch.exe"
	dosExts  = "zip"
	pc98Exts = "hdi|fdi|d88|d98|hdm|fdd|img"
)

// dosRow is what list.csv knows about a game, in the collection's own words:
// the Chinese title, the year, who published it here, what kind of game it is.
type dosRow struct {
	title     string
	genre     string
	year      int
	publisher string
	region    string
}

// ImportDOS builds the DOS&PC98 shelf from what is already on disk: the games
// under rom/dos and rom/pc98, their covers, and list.csv — the Chinese titles
// and release data that shipped with the collection. Nothing else of the old
// frontend is read: its seven DOSBox builds, its loader.bat and its per-game
// command lines are gone, and every game here runs through one RetroArch.
//
// It rewrites only this shelf: the other platforms are left alone, so it can
// be run again after more games are dropped into rom/dos without touching the
// arcade or the consoles.
func ImportDOS(db *sql.DB, src, root string) (int, error) {
	rows, err := readListCSV(filepath.Join(src, "list.csv"))
	if err != nil {
		return 0, err
	}

	nextGame, err := maxID(db, "game")
	if err != nil {
		return 0, err
	}
	nextSim, err := maxID(db, "sim")
	if err != nil {
		return 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		`DELETE FROM game WHERE platform_key IN ('` + DOSKey + `','` + PC98Key + `')`,
		`DELETE FROM sim WHERE platform_key IN ('` + DOSKey + `','` + PC98Key + `')`,
		`DELETE FROM platform WHERE key IN ('` + DOSKey + `','` + PC98Key + `')`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return 0, err
		}
	}

	for _, p := range []Platform{
		{Key: DOSKey, Name: "DOS", Group: DOSGroup, Exts: splitExts(dosExts), Sort: dosSort},
		{Key: PC98Key, Name: "PC98", Group: DOSGroup, Exts: splitExts(pc98Exts), Sort: pc98Sort},
	} {
		if _, err := tx.Exec(`INSERT INTO platform (key, name, grp, exts, sort) VALUES (?, ?, ?, ?, ?)`,
			p.Key, p.Name, p.Group, joinList(p.Exts), p.Sort); err != nil {
			return 0, fmt.Errorf("insert platform %s: %w", p.Key, err)
		}
	}

	// One emulator per machine, both RetroArch, so the CRT preset, the window
	// takeover and the save folders are the same as every other shelf.
	for _, s := range []struct {
		platform string
		name     string
		core     string
	}{
		{DOSKey, "DOSBox Pure", dosCore},
		{PC98Key, "NP2kai", pc98Core},
	} {
		nextSim++
		if _, err := tx.Exec(`INSERT INTO sim (id, platform_key, name, exe, core, args, unzip, is_default)
			VALUES (?, ?, ?, ?, ?, ?, 0, 1)`,
			nextSim, s.platform, s.name, emulator, s.core, "{RomFullPath}"); err != nil {
			return 0, fmt.Errorf("insert sim %s: %w", s.name, err)
		}
	}

	games, err := dosGames(src, root, rows)
	if err != nil {
		return 0, err
	}
	for _, g := range games {
		nextGame++
		if _, err := tx.Exec(`INSERT INTO game (id, platform_key, menu, title, name, rom_path, size,
			cover, year, publisher, genre, region, board, favorite, core, sim_id)
			VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, '', 0)`,
			nextGame, g.PlatformKey, g.Title, g.Name, g.RomPath, g.Size, g.Cover,
			g.Year, g.Publisher, g.Genre, g.Region); err != nil {
			return 0, fmt.Errorf("insert game %s: %w", g.Title, err)
		}
	}
	if err := SetSetting(tx, "imported_dos_at", time.Now().Format("2006-01-02 15:04:05")); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(games), nil
}

// dosGames lists the games of both machines: one row per zip under rom/dos,
// one per disk image under rom/pc98.
func dosGames(src, root string, rows map[string]dosRow) ([]Game, error) {
	var out []Game

	dosDir := filepath.Join(root, "rom", DOSKey)
	entries, err := os.ReadDir(dosDir)
	if err != nil {
		return nil, fmt.Errorf("读 rom/dos：%w", err)
	}
	var stems []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
			continue
		}
		stems = append(stems, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
	}
	sort.Strings(stems)
	for _, stem := range stems {
		rel := DOSKey + "/" + stem + ".zip"
		out = append(out, dosGame(DOSKey, stem, rel, root, rows))
	}

	// PC-98: the image lives in a folder named after the game, so two games
	// that both shipped a "DISK1.hdi" keep their own file.
	pc98Dir := filepath.Join(root, "rom", PC98Key)
	dirs, err := os.ReadDir(pc98Dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("读 rom/pc98：%w", err)
	}
	var folders []string
	for _, d := range dirs {
		if d.IsDir() {
			folders = append(folders, d.Name())
		}
	}
	sort.Strings(folders)
	for _, name := range folders {
		image := imageIn(filepath.Join(pc98Dir, name))
		if image == "" {
			continue
		}
		rel := PC98Key + "/" + name + "/" + image
		out = append(out, dosGame(PC98Key, name, rel, root, rows))
	}
	return out, nil
}

// dosGame is one row: its title from list.csv, the archive's own name kept
// beside it so the rom can still be found by the name it shipped under.
func dosGame(key, stem, rel, root string, rows map[string]dosRow) Game {
	g := Game{PlatformKey: key, Title: stem, Name: titleFromRomName(stem), RomPath: rel}
	if st, err := os.Stat(filepath.Join(root, "rom", filepath.FromSlash(rel))); err == nil {
		g.Size = st.Size()
	}
	if r, ok := rows[strings.ToLower(stem)]; ok {
		if r.title != "" {
			g.Title, g.Name = r.title, r.title
		}
		g.Year, g.Publisher, g.Genre, g.Region = r.year, r.publisher, r.genre, r.region
	}
	// The catalogue is not complete — a few games it has no row for — and the
	// rom's own name carries the year and the maker inside it:
	// "Ash. (1994)(HimeyaSoft)(TW)[cr]". Reading them back out keeps those
	// games out of the 未标注 buckets, and what it finds never overrides what
	// the catalogue already said.
	if g.Year == 0 || g.Publisher == "" {
		if y, maker := fromRomName(stem); y > 0 || maker != "" {
			if g.Year == 0 {
				g.Year = y
			}
			if g.Publisher == "" {
				g.Publisher = maker
			}
		}
	}
	if a, ok := makerAlias[g.Publisher]; ok {
		g.Publisher = a
	}
	if _, err := os.Stat(filepath.Join(root, "数据", "covers", key, stem+".png")); err == nil {
		g.Cover = "covers/" + key + "/" + stem + ".png"
	}
	return g
}

// romNameParts is No-Intro's shape: the year, then the maker, in brackets.
var romNameParts = regexp.MustCompile(`\((\d{4})\)\(([^)]+)\)`)

// titleFromRomName is what the card shows for a game the catalogue has no
// Chinese title for: the name alone, without No-Intro's brackets —
// "Ash. (1994)(HimeyaSoft)(TW)[cr]" reads "Ash." on a shelf, its file name
// stays whole in the database.
func titleFromRomName(stem string) string {
	name := stem
	for _, cut := range []string{" (", "（", " ["} {
		if i := strings.Index(name, cut); i > 0 {
			name = name[:i]
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return stem
	}
	return name
}

// fromRomName reads the year and the maker out of a rom's file name.
func fromRomName(stem string) (year int, maker string) {
	m := romNameParts.FindStringSubmatch(stem)
	if m == nil {
		return 0, ""
	}
	year, _ = strconv.Atoi(m[1])
	maker = strings.TrimSpace(m[2])
	// "(TW)" and "(JP)" sit where a maker would in some datasets; a region is
	// not a publisher.
	if len(maker) == 2 && maker == strings.ToUpper(maker) && !strings.ContainsAny(maker, " ") {
		if _, ok := regions[maker]; ok {
			return year, ""
		}
	}
	return year, maker
}

// regions are the tags No-Intro puts in a rom's name.
var regions = map[string]bool{"TW": true, "JP": true, "US": true, "EU": true, "CN": true, "KR": true, "ASIA": true, "WORLD": true}

// makerAlias joins the two spellings of one house the catalogue uses, so the
// 厂商 axis does not show 天堂鳥 beside 天堂鸟.
var makerAlias = map[string]string{
	"天堂鳥": "天堂鸟",
	"歡樂盒": "欢乐盒",
	"華義":  "华义",
	"飛鯨":  "飞鲸",
	"精訊":  "精讯",
}

// imageIn picks the disk image of a PC-98 game's folder.
func imageIn(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best := ""
	for _, e := range entries {
		if e.IsDir() || !diskImage[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		if best == "" || e.Name() < best {
			best = e.Name()
		}
	}
	return best
}

// diskImage is what NP2kai will mount. An archive is not one: the shelf holds
// the image inside it, so this list never mentions zip.
var diskImage = map[string]bool{
	".hdi": true,
	".fdi": true,
	".d88": true,
	".d98": true,
	".hdm": true,
	".fdd": true,
	".img": true,
}

var yearLead = regexp.MustCompile(`^\d{4}`)

// readListCSV reads the collection's own catalogue: UTF-8 with a byte-order
// mark, comma separated, its header naming the columns, so a column may move
// without breaking the read.
func readListCSV(path string) (map[string]dosRow, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读 list.csv：%w", err)
	}
	text := strings.TrimPrefix(string(raw), "\ufeff")

	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("读 list.csv：%w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	cell := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	out := map[string]dosRow{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读 list.csv：%w", err)
		}
		name := cell(rec, "文件名")
		if name == "" {
			continue
		}
		// 文件名 is the rom's own name without its extension.
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		row := dosRow{
			title:     cell(rec, "游戏名称"),
			genre:     cell(rec, "类型"),
			publisher: first(cell(rec, "发行商"), cell(rec, "制作商")),
			region:    cell(rec, "地区"),
		}
		if y := yearLead.FindString(cell(rec, "发行日期")); y != "" {
			row.year, _ = strconv.Atoi(y)
		}
		out[strings.ToLower(stem)] = row
	}
	return out, nil
}

// maxID is the largest id a table holds, so new rows continue the sequence
// instead of colliding with an imported one.
func maxID(db *sql.DB, table string) (int, error) {
	var n sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(id) FROM ` + table).Scan(&n); err != nil {
		return 0, fmt.Errorf("读 %s 的最大 id：%w", table, err)
	}
	return int(n.Int64), nil
}
