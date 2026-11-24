// Command audit prints how the library is spread over platforms and cores, so
// the core that each (platform, core) pair really runs can be checked.
//
//	go run ./cmd/audit -db "E:\game\Sim ui_D\数据\launcher.db" -ra "E:\game\Sim ui_D\模拟器\RetroArch"
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type simRow struct {
	platform string
	name     string
	exe      string
	core     string
	def      bool
}

type group struct {
	platform string
	core     string
	simName  string
	exe      string
	games    int
	rom      string
}

type gameRow struct {
	platform, rom, core string
	simID               int
}

func uniqueGames(rows []gameRow) int {
	seen := map[string]bool{}
	for _, g := range rows {
		seen[g.platform+"\x00"+g.rom] = true
	}
	return len(seen)
}

func main() {
	dbPath := flag.String("db", `E:\game\Sim ui_D\数据\launcher.db`, "launcher.db")
	raDir := flag.String("ra", `E:\game\Sim ui_D\模拟器\RetroArch`, "RetroArch dir")
	out := flag.String("out", "", "write tsv here")
	list := flag.String("list", "", "print every rom of this core")
	detail := flag.String("detail", "", "print every game of this platform with its sim and core")
	romRoot := flag.String("rom", `E:\game\Sim ui_D\rom`, "rom root, for the file check")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	sims := map[int]simRow{}
	rows, err := db.Query(`SELECT id, platform_key, name, exe, core, is_default FROM sim ORDER BY platform_key, core`)
	if err != nil {
		fmt.Println("sim:", err)
		os.Exit(1)
	}
	for rows.Next() {
		var id int
		var r simRow
		var isDefault int
		rows.Scan(&id, &r.platform, &r.name, &r.exe, &r.core, &isDefault)
		r.def = isDefault != 0
		sims[id] = r
	}
	rows.Close()

	// SimFor: the sim the game names, else the platform's default, else its first.
	byPlatform := map[string][]int{}
	for id, s := range sims {
		byPlatform[s.platform] = append(byPlatform[s.platform], id)
	}
	simFor := func(platform string, id int) (simRow, bool) {
		if s, ok := sims[id]; ok && s.platform == platform {
			return s, true
		}
		var first *simRow
		for _, sid := range byPlatform[platform] {
			s := sims[sid]
			if s.def {
				return s, true
			}
			if first == nil {
				cp := s
				first = &cp
			}
		}
		if first != nil {
			return *first, true
		}
		return simRow{}, false
	}

	all := []gameRow{}
	platformCount := map[string]int{}
	gr, err := db.Query(`SELECT platform_key, rom_path, core, sim_id FROM game`)
	if err != nil {
		fmt.Println("game:", err)
		os.Exit(1)
	}
	for gr.Next() {
		var g gameRow
		if err := gr.Scan(&g.platform, &g.rom, &g.core, &g.simID); err != nil {
			fmt.Println("scan:", err)
			continue
		}
		all = append(all, g)
		platformCount[g.platform]++
		if _, err := os.Stat(filepath.Join(*romRoot, filepath.FromSlash(g.rom))); err != nil {
			fmt.Printf("MISSING\t%s\t%s\n", g.platform, g.rom)
		}
	}
	gr.Close()
	fmt.Printf("games=%d unique=%d\n", len(all), uniqueGames(all))

	byKey := map[string]*group{}
	var order []string
	for _, g := range all {
		s, found := simFor(g.platform, g.simID)
		core := ""
		if found {
			core = s.core
		}
		if g.core != "" {
			core = g.core
		}
		k := g.platform + "\x00" + core
		r, ok2 := byKey[k]
		if !ok2 {
			r = &group{platform: g.platform, core: core, exe: s.exe}
			if s.name != "" {
				r.simName = s.name
			}
			byKey[k] = r
			order = append(order, k)
		}
		r.games++
		if r.rom == "" {
			r.rom = g.rom
		}
		if *list != "" && strings.EqualFold(core, *list) {
			fmt.Printf("ROM\t%s\t%s\n", g.platform, g.rom)
		}
		if *detail != "" && strings.EqualFold(g.platform, *detail) {
			fmt.Printf("GAME\t%s\t%s\tsim=%d(%s/%s)\tover=%s\truns=%s\n",
				g.platform, g.rom, g.simID, s.platform, s.core, g.core, core)
		}
	}

	coresDir := filepath.Join(*raDir, "cores")
	missing := map[string]bool{}
	for _, k := range order {
		r := byKey[k]
		if r.core == "" {
			missing[k] = true
			continue
		}
		if _, err := os.Stat(filepath.Join(coresDir, r.core+"_libretro.dll")); err != nil {
			missing[k] = true
		}
	}

	sort.Slice(order, func(i, j int) bool {
		a, b := byKey[order[i]], byKey[order[j]]
		if a.platform != b.platform {
			return a.platform < b.platform
		}
		return a.core < b.core
	})

	var b strings.Builder
	fmt.Println("platform                 games")
	var plats []string
	for p := range platformCount {
		plats = append(plats, p)
	}
	sort.Strings(plats)
	for _, p := range plats {
		fmt.Printf("%-24s %5d\n", p, platformCount[p])
		b.WriteString(fmt.Sprintf("PLATFORM\t%s\t%d\n", p, platformCount[p]))
	}
	fmt.Println()
	fmt.Printf("%-10s %-26s %-18s %5s  %s\n", "platform", "core", "sim", "games", "dll")
	for _, k := range order {
		r := byKey[k]
		mark := "ok"
		if missing[k] {
			mark = "MISSING/NO-CORE"
		}
		fmt.Printf("%-10s %-26s %-18s %5d  %s\n", r.platform, r.core, r.simName, r.games, mark)
		b.WriteString(fmt.Sprintf("ROW\t%s\t%s\t%s\t%s\t%d\t%s\t%t\n",
			r.platform, r.core, r.simName, r.exe, r.games, r.rom, missing[k]))
	}
	if *out != "" {
		os.WriteFile(*out, []byte(b.String()), 0o644)
		fmt.Println("wrote", *out)
	}
}
