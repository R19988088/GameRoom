// Package library holds the launcher's data: the platforms, the menus
// that split their games, the games themselves, and the emulators that
// run them.
//
// The schema keeps one thing in one place. A game knows its platform by
// key, not by the id of a table row; its rom is a path relative to
// rom/, so the folder can move; its emulator is a sim id, 0 meaning the
// platform's default. Platforms keep only the art folders the launcher
// reads (covers), never the video, gif or document folders the old
// frontend carried.
package library

import (
	"sort"
	"strings"
)

// Platform is a console or system: a folder of roms.
type Platform struct {
	ID    int
	Key   string   // the folder under rom/, as "arc"
	Name  string   // what the sidebar shows, as "ARCADE"
	Group string   // the shelf it belongs to: arcade, console, handheld, pc
	Exts  []string // rom extensions, lower case, no dots
	Sort  int
}

// Groups are the four shelves the top bar switches between.
var Groups = []struct{ Key, Name string }{
	{"arcade", "街机"},
	{"console", "家用机"},
	{"handheld", "掌机"},
	{"pc", "PC"},
}

// Menu is one split of a platform's games, as "SNK" or "格斗".
type Menu struct {
	ID          int
	PlatformKey string
	Name        string
	Sort        int
}

// Game is one rom.
type Game struct {
	ID          int
	PlatformKey string
	Menu        string // "" for games that belong to no menu
	Title       string
	RomPath     string // relative to rom/, as "arc/SNK/dino.zip"
	Size        int64  // bytes, 0 when unknown
	Cover       string // relative to 数据/, as "covers/arc/dino.png", "" when none
	Year        int
	Publisher   string
	Genre       string
	Region      string
	Favorite    bool
	// Core overrides the sim's core for this game alone, "" to use the
	// sim's. Old frontends kept this in a JSON blob per game.
	Core  string
	SimID int
}

// Sim is an emulator that runs a platform's games.
type Sim struct {
	ID          int
	PlatformKey string
	Name        string
	Exe         string // absolute path, or relative to the launcher's root
	Core        string // a RetroArch core's name, as "geolith", "" when none
	Args        string // the rest of the command line, with {RomFullPath} and the like
	Unzip       int
	IsDefault   bool
}

// Library is everything the launcher reads at start.
type Library struct {
	Platforms []Platform
	Menus     []Menu
	Games     []Game
	Sims      []Sim

	platforms map[string]*Platform
}

// index fills the lookup the helpers use.
func (l *Library) index() {
	l.platforms = make(map[string]*Platform, len(l.Platforms))
	for i := range l.Platforms {
		l.platforms[l.Platforms[i].Key] = &l.Platforms[i]
	}
}

// Platform returns the platform with key, nil when there is none.
func (l *Library) Platform(key string) *Platform {
	if l == nil {
		return nil
	}
	if l.platforms == nil {
		l.index()
	}
	return l.platforms[key]
}

// MenusOf returns the menus of platform key, in their order.
func (l *Library) MenusOf(key string) []Menu {
	var menus []Menu
	for _, m := range l.Menus {
		if m.PlatformKey == key {
			menus = append(menus, m)
		}
	}
	return menus
}

// PlatformsOf returns the platforms on a shelf: "arcade", "console",
// "handheld" or "pc", in their order.
func (l *Library) PlatformsOf(group string) []Platform {
	var out []Platform
	for _, p := range l.Platforms {
		if p.Group == group {
			out = append(out, p)
		}
	}
	return out
}

// GameCountOf returns how many games a shelf holds.
func (l *Library) GameCountOf(group string) int {
	counts := l.Counts()
	n := 0
	for _, p := range l.Platforms {
		if p.Group == group {
			n += counts[p.Key]
		}
	}
	return n
}

// Counts returns how many games each platform shows.
func (l *Library) Counts() map[string]int {
	counts := make(map[string]int, len(l.Platforms))
	for _, g := range l.Games {
		counts[g.PlatformKey]++
	}
	return counts
}

// SimFor returns the emulator of g: the one the game names, else the
// platform's default, else the platform's first, nil when it has none.
func (l *Library) SimFor(g Game) *Sim {
	var def, first *Sim
	for i := range l.Sims {
		s := &l.Sims[i]
		if s.PlatformKey != g.PlatformKey {
			continue
		}
		if first == nil {
			first = s
		}
		if s.IsDefault && def == nil {
			def = s
		}
	}
	if g.SimID != 0 {
		for i := range l.Sims {
			if l.Sims[i].ID == g.SimID && l.Sims[i].PlatformKey == g.PlatformKey {
				return &l.Sims[i]
			}
		}
	}
	if def != nil {
		return def
	}
	return first
}

// Sort orders platforms, menus and games as the launcher shows them.
func (l *Library) Sort() {
	sort.SliceStable(l.Platforms, func(i, j int) bool {
		if l.Platforms[i].Sort != l.Platforms[j].Sort {
			return l.Platforms[i].Sort < l.Platforms[j].Sort
		}
		return l.Platforms[i].Name < l.Platforms[j].Name
	})
	sort.SliceStable(l.Menus, func(i, j int) bool {
		a, b := l.Menus[i], l.Menus[j]
		if a.PlatformKey != b.PlatformKey {
			return a.PlatformKey < b.PlatformKey
		}
		if a.Sort != b.Sort {
			return a.Sort < b.Sort
		}
		return a.Name < b.Name
	})
	l.index()
}

// splitExts reads the extensions of a platform, as the old frontend
// wrote them (".zip,.7z"), as a list without dots.
func splitExts(s string) []string {
	var exts []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		e := strings.ToLower(strings.TrimSpace(part))
		e = strings.TrimPrefix(e, ".")
		if e == "" || strings.HasPrefix(e, "path") || seen[e] {
			continue
		}
		seen[e] = true
		exts = append(exts, e)
	}
	return exts
}
