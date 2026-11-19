package app

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/R19988088/GameRoom/internal/covers"
	"github.com/R19988088/GameRoom/internal/library"
)

// coverTarget is one game whose cover is missing, with what is needed to
// fetch and to store it.
type coverTarget struct {
	id       int
	platform string // the platform key, the folder the cover goes in
	rom      string // the romset name, what Arcade Italia indexes
	title    string
}

// missingCovers lists the games of the 街机 shelf that have no cover file. It
// is the arcade only, because that is the shelf Arcade Italia serves: its
// media tree is indexed by MAME romset, and the arcade roms are romsets.
func missingCovers(lib *library.Library, root string) []coverTarget {
	var out []coverTarget
	for _, g := range lib.Games {
		p := lib.Platform(g.PlatformKey)
		if p == nil || p.Group != "arcade" {
			continue
		}
		if g.Cover != "" {
			if _, err := os.Stat(filepath.Join(root, "数据", filepath.FromSlash(g.Cover))); err == nil {
				continue
			}
		}
		base := path.Base(filepath.ToSlash(g.RomPath))
		rom := strings.TrimSuffix(base, path.Ext(base))
		if rom == "" {
			continue
		}
		out = append(out, coverTarget{id: g.ID, platform: g.PlatformKey, rom: rom, title: g.Title})
	}
	return out
}

// downloadCovers fetches the covers of targets, one at a time, and writes the
// ones it gets into 数据/covers/<platform>/<rom>.png, recording them in the
// library. progress is called after each target, from the fetching goroutine.
func downloadCovers(db *sql.DB, root string, targets []coverTarget, progress func(done, total, ok int)) (ok, failed int) {
	adb := covers.NewADB()
	for i, t := range targets {
		data, err := adb.Cover(t.rom)
		if err == nil {
			rel := path.Join("covers", t.platform, t.rom+".png")
			abs := filepath.Join(root, "数据", filepath.FromSlash(rel))
			if mkErr := os.MkdirAll(filepath.Dir(abs), 0o755); mkErr != nil {
				err = mkErr
			} else if wErr := os.WriteFile(abs, data, 0o644); wErr != nil {
				err = wErr
			} else if sErr := library.SetCover(db, t.id, rel); sErr != nil {
				err = sErr
			}
		}
		if err != nil {
			failed++
		} else {
			ok++
		}
		if progress != nil {
			progress(i+1, len(targets), ok)
		}
	}
	return ok, failed
}

// infoTarget is one game whose page the launcher should read: the arcade and
// 麻将 roms, whose manufacturer and year decide the chips they are filed
// under, and whose romset file name is still standing in for a title.
type infoTarget struct {
	id    int
	rom   string
	title string
	// untitled says the row shows the romset's file name as its title, so the
	// catalogue's own name is an improvement; a row that already has a title
	// keeps it.
	untitled bool
}

// missingInfo lists the arcade and 麻将 games whose page has not been read:
// no manufacturer, no year, no 基板, or still no title of their own.
func missingInfo(lib *library.Library, root string) []infoTarget {
	var out []infoTarget
	for _, g := range lib.Games {
		p := lib.Platform(g.PlatformKey)
		if p == nil || p.Group != "arcade" && p.Group != "mahjong" {
			continue
		}
		base := path.Base(filepath.ToSlash(g.RomPath))
		rom := strings.TrimSuffix(base, path.Ext(base))
		if rom == "" {
			continue
		}
		complete := g.Publisher != "" && g.Year != 0 && g.Board != ""
		untitled := g.Name == "" && strings.EqualFold(strings.TrimSpace(g.Title), rom)
		if complete && !untitled {
			continue
		}
		out = append(out, infoTarget{id: g.ID, rom: rom, title: g.Title, untitled: untitled})
	}
	return out
}

// downloadInfo reads each target's page and writes what it says into the
// library: manufacturer, year, genre, and the driver source the 基板 comes
// from — and, for a row still named after its romset, the game's own title.
// progress is called per target.
func downloadInfo(db *sql.DB, targets []infoTarget, progress func(done, total, ok int)) (ok, failed int) {
	adb := covers.NewADB()
	for i, t := range targets {
		info, err := adb.Info(t.rom)
		if err == nil {
			if sErr := library.SetInfo(db, t.id, strings.ToUpper(info.Maker), info.Year, info.Genre, info.Board); sErr != nil {
				err = sErr
			} else if t.untitled {
				if nErr := library.SetName(db, t.id, info.Title); nErr != nil {
					err = nErr
				}
			}
		}
		if err != nil {
			failed++
		} else {
			ok++
		}
		if progress != nil {
			progress(i+1, len(targets), ok)
		}
	}
	return ok, failed
}

// FetchInfo is the no-window path: `GameRoom.exe -fetch-info` reads the
// missing manufacturers and years off Arcade Italia and exits.
func FetchInfo(limit int) error {
	root := findRoot()
	db, err := library.Open(filepath.Join(root, "数据", "launcher.db"))
	if err != nil {
		return fmt.Errorf("打不开数据库：%w", err)
	}
	defer db.Close()
	lib, err := library.Load(db)
	if err != nil {
		return fmt.Errorf("读库失败：%w", err)
	}
	targets := missingInfo(lib, root)
	if limit > 0 && len(targets) > limit {
		targets = targets[:limit]
	}
	fmt.Printf("缺厂商/年份 %d 个，开始读取…\n", len(targets))
	ok, failed := downloadInfo(db, targets, func(done, total, ok int) {
		if done == total || done%10 == 0 {
			fmt.Printf("\r%d/%d  写入 %d", done, total, ok)
		}
	})
	fmt.Printf("\n完成：写入 %d，失败 %d\n", ok, failed)
	return nil
}

// FetchCovers is the no-window path: `GameRoom.exe -fetch-covers` fills the
// library's missing arcade covers and exits, so the work can be run in a
// terminal and watched. limit stops early, 0 fetches all of them.
func FetchCovers(limit int) error {
	root := findRoot()
	db, err := library.Open(filepath.Join(root, "数据", "launcher.db"))
	if err != nil {
		return fmt.Errorf("打不开数据库：%w", err)
	}
	defer db.Close()

	lib, err := library.Load(db)
	if err != nil {
		return fmt.Errorf("读库失败：%w", err)
	}
	targets := missingCovers(lib, root)
	if limit > 0 && len(targets) > limit {
		targets = targets[:limit]
	}
	fmt.Printf("缺封面 %d 张，开始补全…\n", len(targets))
	ok, failed := downloadCovers(db, root, targets, func(done, total, ok int) {
		if done == total || done%10 == 0 {
			fmt.Printf("\r%d/%d  新增 %d", done, total, ok)
		}
	})
	fmt.Printf("\n完成：新增 %d，失败 %d\n", ok, failed)
	return nil
}
