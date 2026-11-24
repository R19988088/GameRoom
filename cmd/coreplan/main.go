// Command coreplan rewrites the core a game points at, and the core a sim
// names, so the stored library says what the launcher really runs. It is a
// one-off maintenance tool: games keep an empty core when the platform default
// is right, and only the ones measured onto another core get an override.
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", `E:\game\Sim ui_D\数据\launcher.db`, "launcher.db")
	plan := flag.String("plan", "", "tsv: relative rom path <tab> core (empty clears)")
	psToMednafen := flag.Bool("ps", false, "name mednafen_psx as the PS1 core")
	defaults := flag.String("defaults", "", "platform=core pairs: ensure a sim and make it the platform default")
	paths := flag.String("paths", "", "tsv: old rom path <tab> new rom path (rewrite rom_path only)")
	list := flag.Bool("list", false, "dump id, platform, core, menu, rom_path")
	apply := flag.Bool("apply", false, "write; without it only report")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	if *defaults != "" {
		names := map[string]string{
			"fbneo": "fbneo", "fbneo_plus": "fbneo_plus", "mame": "MAME",
			"mame2010": "MAME 2010", "mame2003_plus": "MAME 2003 Plus",
			"flycast": "Flycast", "geolith": "snk家用机", "neocd": "NEOGEO CD",
		}
		for _, pair := range strings.Split(*defaults, ",") {
			p, c, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if !ok || p == "" || c == "" {
				fmt.Println("bad default:", pair)
				continue
			}
			var id int
			err := db.QueryRow(`SELECT id FROM sim WHERE platform_key=? AND core=?`, p, c).Scan(&id)
			if err != nil {
				name := names[c]
				if name == "" {
					name = c
				}
				_, insErr := db.Exec(`INSERT INTO sim (platform_key, name, exe, core, args, is_default)
					VALUES (?, ?, ?, ?, ?, 0)`, p, name, "模拟器/RetroArch/retroarch.exe", c, "{RomFullPath}")
				if insErr != nil {
					fmt.Println("insert:", p, c, insErr)
					continue
				}
				db.QueryRow(`SELECT id FROM sim WHERE platform_key=? AND core=?`, p, c).Scan(&id)
				fmt.Printf("default: created sim %d for %s=%s\n", id, p, c)
			}
			if *apply {
				if _, err := db.Exec(`UPDATE sim SET is_default = CASE WHEN id=? THEN 1 ELSE 0 END
					WHERE platform_key=?`, id, p); err != nil {
					fmt.Println("default:", p, err)
				} else {
					fmt.Printf("default: %s now runs %s (sim %d)\n", p, c, id)
				}
			}
		}
	}

	if *psToMednafen {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM sim WHERE platform_key='ps' AND core='duckstation'`).Scan(&n)
		fmt.Printf("ps sims on duckstation: %d\n", n)
		if *apply && n > 0 {
			_, err := db.Exec(`UPDATE sim SET core='mednafen_psx', name='Mednafen PSX'
				WHERE platform_key='ps' AND core='duckstation'`)
			if err != nil {
				fmt.Println("ps:", err)
				os.Exit(1)
			}
			fmt.Println("ps sims moved to mednafen_psx")
		}
		// Games that carry an explicit duckstation override are moved too.
		var m int
		db.QueryRow(`SELECT COUNT(*) FROM game WHERE core='duckstation'`).Scan(&m)
		if m > 0 {
			fmt.Printf("games overriding duckstation: %d\n", m)
			if *apply {
				db.Exec(`UPDATE game SET core='mednafen_psx' WHERE core='duckstation'`)
			}
		}
	}

	if *list {
		rows, err := db.Query(`SELECT id, platform_key, core, menu, title, rom_path FROM game
			WHERE platform_key IN ('arc','majiang') ORDER BY platform_key, rom_path`)
		if err != nil {
			fmt.Println("list:", err)
			os.Exit(1)
		}
		for rows.Next() {
			var id int
			var pk, core, menu, title, rom string
			if err := rows.Scan(&id, &pk, &core, &menu, &title, &rom); err != nil {
				continue
			}
			fmt.Printf("%d\t%s\t%s\t%s\t%s\t%s\n", id, pk, core, menu, title, rom)
		}
		rows.Close()
	}

	if *paths != "" {
		f, err := os.Open(*paths)
		if err != nil {
			fmt.Println("paths:", err)
			os.Exit(1)
		}
		defer f.Close()
		var done, missed int
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			from := strings.ReplaceAll(strings.TrimSpace(parts[0]), `\`, "/")
			to := strings.ReplaceAll(strings.TrimSpace(parts[1]), `\`, "/")
			var id int
			err := db.QueryRow(`SELECT id FROM game WHERE rom_path=?`, from).Scan(&id)
			if err != nil {
				missed++
				fmt.Println("no game for", from)
				continue
			}
			done++
			if !*apply {
				continue
			}
			if _, err := db.Exec(`UPDATE game SET rom_path=? WHERE id=?`, to, id); err != nil {
				fmt.Println("path update:", from, err)
				os.Exit(1)
			}
		}
		fmt.Printf("paths: %d matched, %d not found\n", done, missed)
	}

	if *plan == "" {
		return
	}
	f, err := os.Open(*plan)
	if err != nil {
		fmt.Println("plan:", err)
		os.Exit(1)
	}
	defer f.Close()

	var changed, missing int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		rom := strings.ReplaceAll(parts[0], `\`, "/")
		core := strings.TrimSpace(parts[1])
		var id int
		var platform string
		err := db.QueryRow(`SELECT id, platform_key FROM game WHERE rom_path=?`, rom).Scan(&id, &platform)
		if err != nil {
			missing++
			continue
		}
		changed++
		if !*apply {
			continue
		}
		if _, err := db.Exec(`UPDATE game SET core=? WHERE id=?`, core, id); err != nil {
			fmt.Println("update:", rom, err)
			os.Exit(1)
		}
	}
	fmt.Printf("plan: %d matched, %d not found\n", changed, missing)
	if *apply {
		fmt.Println("applied")
	}
}
