package library

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// romFallbackExts are the containers a platform takes when it did not say which
// extensions it holds: a freshly made platform has an empty list until a game
// has been imported into it.
var romFallbackExts = []string{"zip", "7z", "rar"}

// rescanRow is what the shelf already knows about a dump, enough to file a new
// one beside it.
type rescanRow struct {
	id        int
	title     string
	menu      string
	publisher string
	year      int
	genre     string
	region    string
	board     string
	core      string
	simID     int
	family    int
	solo      bool
}

// Rescan walks rom/ and adds the dumps the library does not know yet: a rom
// copied in beside the others becomes a card, and one whose file name the shelf
// already has becomes another dump of that game — a version under the same
// card, not a second card with the same name.
//
// Nothing already filed is touched or removed: a star, a cover and a hand-typed
// name survive a folder being tidied, and a rom that went missing keeps its
// card until the user says otherwise.
//
// It returns how many rows it added.
func Rescan(db *sql.DB, root string) (int, error) {
	known := map[string]rescanRow{}  // platform|rom path (lower case)
	byName := map[string]rescanRow{} // platform|file stem without ext
	platforms := map[string][]string{}
	extsOf := map[string]map[string]bool{}

	rows, err := db.Query(`SELECT key, exts FROM platform`)
	if err != nil {
		return 0, fmt.Errorf("read platforms: %w", err)
	}
	for rows.Next() {
		var key, exts string
		if err := rows.Scan(&key, &exts); err != nil {
			rows.Close()
			return 0, err
		}
		list := splitList(exts)
		if len(list) == 0 {
			list = romFallbackExts
		}
		for i := range list {
			list[i] = strings.ToLower(strings.TrimPrefix(list[i], "."))
		}
		platforms[key] = list
		set := map[string]bool{}
		for _, e := range list {
			set[e] = true
		}
		extsOf[key] = set
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	grows, err := db.Query(`SELECT id, platform_key, rom_path, title, menu, publisher, year,
		genre, region, board, core, sim_id, family, solo FROM game`)
	if err != nil {
		return 0, fmt.Errorf("read games: %w", err)
	}
	for grows.Next() {
		var platform, romPath string
		var r rescanRow
		if err := grows.Scan(&r.id, &platform, &romPath, &r.title, &r.menu, &r.publisher,
			&r.year, &r.genre, &r.region, &r.board, &r.core, &r.simID, &r.family, &r.solo); err != nil {
			grows.Close()
			return 0, err
		}
		known[romKey(platform, romPath)] = r
		name := romKey(platform, romName(romPath))
		// Two dumps sharing a name: the first one read stays the one a new
		// dump is filed under, so the outcome does not depend on row order.
		if _, ok := byName[name]; !ok {
			byName[name] = r
		}
	}
	grows.Close()
	if err := grows.Err(); err != nil {
		return 0, err
	}

	added := 0
	keys := make([]string, 0, len(platforms))
	for k := range platforms {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// one file of a shelf: a rom the library knows, another dump of one it
	// knows, or a game nobody has seen.
	each := func(platform, dir, path string, size int64) error {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if !extsOf[platform][ext] || size == 0 {
			return nil
		}
		sub, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		romPath := platform + "/" + filepath.ToSlash(strings.TrimPrefix(sub, "./"))
		if _, ok := known[romKey(platform, romPath)]; ok {
			return nil
		}
		// Same name, another container: hsf2.zip beside hsf2.7z is the same
		// game dumped twice, so it hangs under the card that is already there
		// instead of becoming a second card.
		if match, ok := byName[romKey(platform, romName(romPath))]; ok {
			family, solo := match.family, match.solo
			if family == 0 && !solo {
				family = match.id
			}
			if solo {
				family = 0
			}
			_, err = db.Exec(`INSERT INTO game (platform_key, menu, title, rom_path, size,
				publisher, year, genre, region, board, core, sim_id, family, solo)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				platform, match.menu, match.title, romPath, size, match.publisher,
				match.year, match.genre, match.region, match.board, match.core,
				match.simID, family, boolInt(solo))
			if err != nil {
				return fmt.Errorf("add %s: %w", romPath, err)
			}
			added++
			return nil
		}
		// A dump nobody has seen: the file name is all there is, and the folder
		// it was copied into is the menu it lands under.
		menu := ""
		if parts := strings.Split(filepath.ToSlash(sub), "/"); len(parts) > 1 {
			menu = parts[0]
		}
		_, err = db.Exec(`INSERT INTO game (platform_key, menu, title, rom_path, size)
			VALUES (?, ?, ?, ?, ?)`,
			platform, menu, romName(romPath), romPath, size)
		if err != nil {
			return fmt.Errorf("add %s: %w", romPath, err)
		}
		added++
		return nil
	}

	// The walk follows links, which a plain walk of rom/ does not: rom/arc,
	// rom/SEGA and rom/3DO here are junctions onto an older install, so WalkDir
	// reports those three as files and never looks inside — which is how a zip
	// dropped into rom/arc stayed invisible.
	var walk func(platform, dir, at string) error
	seen := map[string]bool{}
	walk = func(platform, dir, at string) error {
		if real, err := filepath.EvalSymlinks(at); err == nil {
			if seen[real] {
				return nil
			}
			seen[real] = true
		}
		entries, err := os.ReadDir(at)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			path := filepath.Join(at, e.Name())
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.IsDir() {
				if err := walk(platform, dir, path); err != nil {
					return err
				}
				continue
			}
			if err := each(platform, dir, path, info.Size()); err != nil {
				return err
			}
		}
		return nil
	}

	for _, platform := range keys {
		dir := filepath.Join(root, "rom", filepath.FromSlash(platform))
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		if err := walk(platform, dir, dir); err != nil {
			return added, fmt.Errorf("walk %s: %w", dir, err)
		}
	}
	return added, nil
}

// romName is a rom path's file name without its extension, in the case it was
// written.
func romName(romPath string) string {
	base := filepath.Base(filepath.FromSlash(romPath))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// romKey is what two rows agree on when they name the same file: the platform
// and the path, neither of which the file system's case can change here.
func romKey(platform, rom string) string {
	return strings.ToLower(platform) + "|" + strings.ToLower(strings.TrimSpace(rom))
}
