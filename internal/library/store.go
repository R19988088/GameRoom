package library

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// schemaVersion is the shape of the tables below. A library of another
// version is rebuilt from the old frontend's data rather than patched.
const schemaVersion = "3"

// ErrOldSchema says the database was written by an earlier version of
// this launcher.
var ErrOldSchema = fmt.Errorf("库版本过旧（%s），需要重新导入", "v1")

// schema is the launcher's data, one table per kind of thing.
//
// It replaces the old frontend's tables, which mixed display names with
// file paths, kept thirty media folders on each platform, and let a game
// point at an emulator by a row id that could disappear. Here a platform
// is its key, a menu is a name under a platform, a game is a rom file,
// and a sim is an emulator.
const schema = `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS platform (
	id    INTEGER PRIMARY KEY,
	key   TEXT NOT NULL UNIQUE,
	name  TEXT NOT NULL,
	grp   TEXT NOT NULL DEFAULT '',
	exts  TEXT NOT NULL DEFAULT '',
	sort  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS menu (
	id           INTEGER PRIMARY KEY,
	platform_key TEXT NOT NULL REFERENCES platform(key) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	sort         INTEGER NOT NULL DEFAULT 0,
	UNIQUE (platform_key, name)
);

CREATE TABLE IF NOT EXISTS game (
	id           INTEGER PRIMARY KEY,
	platform_key TEXT NOT NULL REFERENCES platform(key) ON DELETE CASCADE,
	menu         TEXT NOT NULL DEFAULT '',
	title        TEXT NOT NULL,
	name         TEXT NOT NULL DEFAULT '',
	rom_path     TEXT NOT NULL,
	size         INTEGER NOT NULL DEFAULT 0,
	cover        TEXT NOT NULL DEFAULT '',
	year         INTEGER NOT NULL DEFAULT 0,
	publisher    TEXT NOT NULL DEFAULT '',
	genre        TEXT NOT NULL DEFAULT '',
	region       TEXT NOT NULL DEFAULT '',
	board        TEXT NOT NULL DEFAULT '',
	favorite     INTEGER NOT NULL DEFAULT 0,
	family       INTEGER NOT NULL DEFAULT 0,
	solo         INTEGER NOT NULL DEFAULT 0,
	ips          TEXT NOT NULL DEFAULT '',
	core         TEXT NOT NULL DEFAULT '',
	sim_id       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS game_platform ON game (platform_key, menu);
CREATE UNIQUE INDEX IF NOT EXISTS game_rom ON game (platform_key, rom_path);

CREATE TABLE IF NOT EXISTS sim (
	id           INTEGER PRIMARY KEY,
	platform_key TEXT NOT NULL REFERENCES platform(key) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	exe          TEXT NOT NULL,
	core         TEXT NOT NULL DEFAULT '',
	args         TEXT NOT NULL DEFAULT '',
	unzip        INTEGER NOT NULL DEFAULT 0,
	is_default   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS setting (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// Open opens the launcher's database, creating it when missing.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection: the launcher reads once and writes rarely.
	db.SetMaxOpenConns(1)

	// A file that already holds tables was written by an earlier version:
	// its columns are not this schema's, and the launcher says so instead
	// of half-reading it.
	if tables, err := tableCount(db); err == nil && tables > 0 {
		if !hasColumn(db, "game", "favorite") || !hasColumn(db, "platform", "grp") {
			db.Close()
			return nil, ErrOldSchema
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if old := Setting(db, "schema"); old != "" && old != schemaVersion {
		db.Close()
		return nil, ErrOldSchema
	}
	if err := SetSetting(db, "schema", schemaVersion); err != nil {
		db.Close()
		return nil, err
	}
	// The 基板 column arrived after the first libraries were written; adding
	// it in place keeps the covers and the metadata already fetched.
	if !hasColumn(db, "game", "board") {
		if _, err := db.Exec(`ALTER TABLE game ADD COLUMN board TEXT NOT NULL DEFAULT ''`); err != nil {
			db.Close()
			return nil, fmt.Errorf("add board: %w", err)
		}
	}
	// 游戏标题, likewise: the card used to show the romset's file name, which
	// is a key rather than a title ("avsp"), and a catalogue's name belongs in
	// a column of its own so the key is still there to match on.
	if !hasColumn(db, "game", "name") {
		if _, err := db.Exec(`ALTER TABLE game ADD COLUMN name TEXT NOT NULL DEFAULT ''`); err != nil {
			db.Close()
			return nil, fmt.Errorf("add name: %w", err)
		}
	}
	// 改版 by hand: the card a dump is filed under, and whether it keeps a card
	// of its own. Both came with the version picker's second round.
	if !hasColumn(db, "game", "family") {
		if _, err := db.Exec(`ALTER TABLE game ADD COLUMN family INTEGER NOT NULL DEFAULT 0`); err != nil {
			db.Close()
			return nil, fmt.Errorf("add family: %w", err)
		}
	}
	if !hasColumn(db, "game", "solo") {
		if _, err := db.Exec(`ALTER TABLE game ADD COLUMN solo INTEGER NOT NULL DEFAULT 0`); err != nil {
			db.Close()
			return nil, fmt.Errorf("add solo: %w", err)
		}
	}
	// IPS patches switched on for a dump arrived with the patch menu.
	if !hasColumn(db, "game", "ips") {
		if _, err := db.Exec(`ALTER TABLE game ADD COLUMN ips TEXT NOT NULL DEFAULT ''`); err != nil {
			db.Close()
			return nil, fmt.Errorf("add ips: %w", err)
		}
	}
	return db, nil
}

// tableCount counts the tables a database file already holds.
func tableCount(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&n)
	return n, err
}

// hasColumn reports whether a table has a column of that name.
func hasColumn(db *sql.DB, table, column string) bool {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}

// coreArg matches "-L <path>" in a RetroArch command line.
var coreArg = regexp.MustCompile(`(?i)-L\s+(?:"([^"]+)"|(\S+))`)

// SplitCore takes the core out of a RetroArch command line, so the
// launcher can name it and find it on whichever system it runs:
//
//	"-L cores\geolith_libretro.dll {RomFullPath}" -> "geolith", "{RomFullPath}"
func SplitCore(args string) (core, rest string) {
	rest = strings.TrimSpace(args)
	m := coreArg.FindStringSubmatchIndex(args)
	if m == nil {
		return "", rest
	}
	path := ""
	if m[2] >= 0 {
		path = args[m[2]:m[3]]
	} else if m[4] >= 0 {
		path = args[m[4]:m[5]]
	}
	base := filepath.Base(strings.ReplaceAll(path, "\\", "/"))
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if strings.HasSuffix(strings.ToLower(name), "_libretro") {
		core = name[:len(name)-len("_libretro")]
	}
	rest = strings.TrimSpace(args[:m[0]] + " " + args[m[1]:])
	return core, rest
}

// Imported reports whether the launcher has imported a library already.
func Imported(db *sql.DB) bool {
	var v string
	err := db.QueryRow(`SELECT value FROM setting WHERE key = 'imported'`).Scan(&v)
	return err == nil && v == "1"
}

// SetCover records a game's cover, relative to 数据/.
func SetCover(db *sql.DB, gameID int, cover string) error {
	_, err := db.Exec(`UPDATE game SET cover = ? WHERE id = ?`, cover, gameID)
	return err
}

// SetFavorite stars a game, or takes the star away: the card's menu writes it
// here, and the bar's 收藏 page reads it back on the next start.
func SetFavorite(db *sql.DB, gameID int, on bool) error {
	_, err := db.Exec(`UPDATE game SET favorite = ? WHERE id = ?`, boolInt(on), gameID)
	return err
}

// FileDump says how a dump is shown beside its title: under the card of game
// base (0 to let the title decide), and soloed when it keeps a card of its own
// whatever its title says. Moving dumps by hand is a note in this table; the
// roms are not moved on disk.
func FileDump(db *sql.DB, gameID, base int, solo bool) error {
	_, err := db.Exec(`UPDATE game SET family = ?, solo = ? WHERE id = ?`,
		base, boolInt(solo), gameID)
	return err
}

// SetIPS records which patch groups of 数据/ips this dump plays with: the
// card's menu writes it here, and the launch reads it back on the next game.
func SetIPS(db *sql.DB, gameID int, groups []string) error {
	_, err := db.Exec(`UPDATE game SET ips = ? WHERE id = ?`, joinList(groups), gameID)
	return err
}

// SetPlatform files a game under another platform: a rom imported into the
// wrong machine is corrected by moving it, not by copying it.
func SetPlatform(db *sql.DB, gameID int, platformKey string) error {
	_, err := db.Exec(`UPDATE game SET platform_key = ? WHERE id = ?`, platformKey, gameID)
	return err
}

// Fields a game can be corrected in from the card's own menu, named as the
// menu names them. They are a whitelist: the statement below is fixed text, so
// a field name never reaches the SQL as a parameter.
const (
	FieldBoard     = "board"
	FieldPublisher = "publisher"
	FieldGenre     = "genre"
	FieldCore      = "core"
	FieldTitle     = "title"
)

// SetField writes one column of one game, so a wrong 厂商 or 基板 can be fixed
// without opening the database. An unknown field is refused rather than
// silently ignored.
func SetField(db *sql.DB, gameID int, field, value string) error {
	var q string
	switch field {
	case FieldBoard:
		q = `UPDATE game SET board = ? WHERE id = ?`
	case FieldPublisher:
		q = `UPDATE game SET publisher = ? WHERE id = ?`
	case FieldGenre:
		q = `UPDATE game SET genre = ? WHERE id = ?`
	case FieldCore:
		q = `UPDATE game SET core = ? WHERE id = ?`
	case FieldTitle:
		q = `UPDATE game SET title = ? WHERE id = ?`
	default:
		return fmt.Errorf("library: unknown field %q", field)
	}
	_, err := db.Exec(q, strings.TrimSpace(value), gameID)
	return err
}

// SetInfo records what the game's page says: manufacturer, year, genre, and
// the driver source the 基板 is read from. An empty value leaves the column as
// it was.
// SetInfo writes what a catalogue says about a game.
func SetInfo(db *sql.DB, gameID int, publisher, year, genre, board string) error {
	y := 0
	if year != "" {
		y, _ = strconv.Atoi(strings.TrimSpace(year))
	}
	_, err := db.Exec(`UPDATE game SET
		publisher = CASE WHEN ? <> '' THEN ? ELSE publisher END,
		year      = CASE WHEN ? > 0 THEN ? ELSE year END,
		genre     = CASE WHEN ? <> '' THEN ? ELSE genre END,
		board     = CASE WHEN ? <> '' THEN ? ELSE board END
		WHERE id = ?`, publisher, publisher, y, y, genre, genre, board, board, gameID)
	return err
}

// SetName gives a game the title a catalogue has for it. The caller only asks
// for it when the row still shows the romset's file name — "avsp" is a key,
// and a shelf should read the game's own name; a row that already has one
// ("合金弹头") keeps it, in the language the collection was built in.
func SetName(db *sql.DB, gameID int, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	_, err := db.Exec(`UPDATE game SET name = ? WHERE id = ?`, strings.TrimSpace(name), gameID)
	return err
}

// Setting reads a setting, "" when it is not set.
func Setting(db *sql.DB, key string) string {
	var v string
	if err := db.QueryRow(`SELECT value FROM setting WHERE key = ?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

// execer is a connection or a transaction: both write settings.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// SetSetting writes a setting.
func SetSetting(db execer, key, value string) error {
	_, err := db.Exec(`INSERT INTO setting (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Load reads the whole library.
func Load(db *sql.DB) (*Library, error) {
	l := &Library{}

	rows, err := db.Query(`SELECT id, key, name, grp, exts, sort FROM platform ORDER BY sort, name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p Platform
		var exts string
		if err := rows.Scan(&p.ID, &p.Key, &p.Name, &p.Group, &exts, &p.Sort); err != nil {
			rows.Close()
			return nil, err
		}
		p.Exts = splitList(exts)
		l.Platforms = append(l.Platforms, p)
	}
	rows.Close()

	// The menu table is not read any more: split.go derives each shelf's
	// menus from the games, so a shelf's chips follow one fixed standard.

	grows, err := db.Query(`SELECT id, platform_key, menu, title, name, rom_path, size, cover, year,
		publisher, genre, region, favorite, core, sim_id, board, family, solo, ips
		FROM game ORDER BY favorite DESC, title`)
	if err != nil {
		return nil, err
	}
	for grows.Next() {
		var g Game
		if err := grows.Scan(&g.ID, &g.PlatformKey, &g.Menu, &g.Title, &g.Name, &g.RomPath, &g.Size,
			&g.Cover, &g.Year, &g.Publisher, &g.Genre, &g.Region, &g.Favorite,
			&g.Core, &g.SimID, &g.Board, &g.Family, &g.Solo, &g.IPS); err != nil {
			grows.Close()
			return nil, err
		}
		// A known title is what the shelf shows and sorts by; the romset's
		// file name stays in the row for matching, not for reading.
		if g.Name != "" {
			g.Title = g.Name
		}
		l.Games = append(l.Games, g)
	}
	grows.Close()

	srows, err := db.Query(`SELECT id, platform_key, name, exe, core, args, unzip, is_default FROM sim ORDER BY platform_key, id`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var s Sim
		if err := srows.Scan(&s.ID, &s.PlatformKey, &s.Name, &s.Exe, &s.Core, &s.Args, &s.Unzip, &s.IsDefault); err != nil {
			srows.Close()
			return nil, err
		}
		l.Sims = append(l.Sims, s)
	}
	srows.Close()

	// The menus the shelf shows are derived from the games themselves
	// (see split.go); the old library's menu table mixed makers, hardware
	// and genres, and is ignored. Load leaves every shelf on its default
	// standard, and the bar switches from there.
	for _, group := range groups() {
		if axes := AxesOf(group); len(axes) > 0 {
			l.SplitGroup(group, axes[0])
		}
	}
	return l, nil
}

// groups lists the shelves the loaded platforms belong to, in bar order.
func groups() []string {
	out := make([]string, 0, len(Groups))
	for _, g := range Groups {
		out = append(out, g.Key)
	}
	return out
}

// splitList reads a "|"-separated list, dropping empty entries.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// joinList writes a "|"-separated list.
func joinList(values []string) string { return strings.Join(values, "|") }
