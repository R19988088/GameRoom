// Command fixsims rebuilds a platform's sim table around the cores its games
// really run: one sim per (platform, core), named after the core, pointing at
// the bundled RetroArch, and every game filed under the sim it needs.
//
// The library that came from simUI had sims whose names and cores did not agree
// ("MAME 2010" naming a mame core, "fbneo_plus" naming mame2016), and games
// pointing at a mame2016 that crashes the process. This rewrites that in place.
//
//	go run ./cmd/fixsims -plan plan.tsv -defaults arc=fbneo,majiang=mame -apply
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

var coreNames = map[string]string{
	"fbneo":           "fbneo",
	"fbneo_plus":      "fbneo_plus",
	"mame":            "MAME",
	"mame2010":        "MAME 2010",
	"mame2003_plus":   "MAME 2003 Plus",
	"mame2016":        "MAME 2016",
	"mamearcade":      "MAME Arcade",
	"flycast":         "Flycast",
	"geolith":         "snk家用机",
	"mednafen_saturn": "Mednafen Saturn",
	"opera":           "Opera",
	"fbalpha":         "FBA",
	"neocd":           "NEOGEO CD",
}

func main() {
	dbPath := flag.String("db", `E:\game\Sim ui_D\数据\launcher.db`, "launcher.db")
	plan := flag.String("plan", "", "tsv: rom_path <tab> core")
	plats := flag.String("platforms", "arc,majiang", "platforms to rebuild")
	defaults := flag.String("defaults", "", "platform=core pairs for is_default")
	apply := flag.Bool("apply", false, "write; without it only report")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	want := map[string]string{}
	if *plan != "" {
		f, err := os.Open(*plan)
		if err != nil {
			fmt.Println("plan:", err)
			os.Exit(1)
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			want[strings.ReplaceAll(parts[0], `\`, "/")] = strings.TrimSpace(parts[1])
		}
		f.Close()
	}

	// Current state: which core each game effectively asks for.
	type game struct {
		id     int
		plat   string
		rom    string
		core   string
		simID  int
	}
	var games []game
	rows, err := db.Query(`SELECT g.id, g.platform_key, g.rom_path, COALESCE(g.core,''),
		COALESCE(s.core,''), COALESCE(g.sim_id,0) FROM game g LEFT JOIN sim s ON s.id=g.sim_id`)
	if err != nil {
		fmt.Println("query:", err)
		os.Exit(1)
	}
	for rows.Next() {
		var g game
		var simCore string
		if err := rows.Scan(&g.id, &g.plat, &g.rom, &g.core, &simCore, &g.simID); err != nil {
			continue
		}
		if g.core == "" {
			g.core = simCore
		}
		games = append(games, g)
	}
	rows.Close()

	platSet := map[string]bool{}
	for _, p := range strings.Split(*plats, ",") {
		platSet[strings.TrimSpace(p)] = true
	}

	// Desired core per game, and the cores each platform needs.
	desired := map[int]string{}
	need := map[string]map[string]bool{}
	stats := map[string]int{}
	for _, g := range games {
		if !platSet[g.plat] {
			continue
		}
		c, ok := want[g.rom]
		if !ok {
			c = g.core
		}
		if c == "" {
			continue
		}
		desired[g.id] = c
		if need[g.plat] == nil {
			need[g.plat] = map[string]bool{}
		}
		need[g.plat][c] = true
		stats[g.plat+" "+c]++
	}

	var order []string
	for k := range stats {
		order = append(order, k)
	}
	sort.Strings(order)
	for _, k := range order {
		fmt.Printf("%-18s %4d\n", k, stats[k])
	}
	if !*apply {
		fmt.Println("(report only)")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		fmt.Println("begin:", err)
		os.Exit(1)
	}
	defer tx.Rollback()

	var maxID int
	tx.QueryRow(`SELECT COALESCE(MAX(id),0) FROM sim`).Scan(&maxID)

	for plat, cores := range need {
		ids := map[string]int{}
		for core := range cores {
			var id int
			err := tx.QueryRow(`SELECT id FROM sim WHERE platform_key=? AND core=?`, plat, core).Scan(&id)
			if err == sql.ErrNoRows {
				maxID++
				name := coreNames[core]
				if name == "" {
					name = core
				}
				if _, err := tx.Exec(`INSERT INTO sim (id, platform_key, name, exe, core, args, is_default)
					VALUES (?, ?, ?, ?, ?, ?, 0)`, maxID, plat, name, "模拟器/RetroArch/retroarch.exe", core, "{RomFullPath}"); err != nil {
					fmt.Println("insert sim:", plat, core, err)
					continue
				}
				id = maxID
			}
			ids[core] = id
			// The name follows the core, and the exe is the bundled RetroArch.
			tx.Exec(`UPDATE sim SET name=?, exe=?, args=? WHERE id=?`,
				coreNames[core], "模拟器/RetroArch/retroarch.exe", "{RomFullPath}", id)
		}
		for gid, core := range desired {
			var platOf string
			tx.QueryRow(`SELECT platform_key FROM game WHERE id=?`, gid).Scan(&platOf)
			if platOf != plat {
				continue
			}
			if id, ok := ids[core]; ok {
				tx.Exec(`UPDATE game SET sim_id=?, core='' WHERE id=?`, id, gid)
			}
		}
	}

	// Every sim of these platforms that no game uses any more is dropped (the
	// stale "MAME Arcade"/"MAME 2016" rows that named the wrong core).
	for plat := range need {
		rows, err := tx.Query(`SELECT id FROM sim WHERE platform_key=?`, plat)
		if err != nil {
			continue
		}
		var all []int
		for rows.Next() {
			var id int
			rows.Scan(&id)
			all = append(all, id)
		}
		rows.Close()
		for _, id := range all {
			var used int
			tx.QueryRow(`SELECT COUNT(*) FROM game WHERE sim_id=?`, id).Scan(&used)
			if used == 0 {
				tx.Exec(`DELETE FROM sim WHERE id=?`, id)
			}
		}
	}

	if *defaults != "" {
		for _, pair := range strings.Split(*defaults, ",") {
			p, c, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if !ok {
				continue
			}
			var id int
			if err := tx.QueryRow(`SELECT id FROM sim WHERE platform_key=? AND core=?`, p, c).Scan(&id); err == nil {
				tx.Exec(`UPDATE sim SET is_default = CASE WHEN id=? THEN 1 ELSE 0 END WHERE platform_key=?`, id, p)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Println("commit:", err)
		os.Exit(1)
	}
	fmt.Println("done")
}
