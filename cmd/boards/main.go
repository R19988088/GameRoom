// Command boards prints how a shelf's games spread over boards and cores, and
// can list one line per game with its board, so a board that needs one core can
// be decided as a group instead of game by game.
//
//	go run ./cmd/boards -platform arc
//	go run ./cmd/boards -platform arc -games
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", `E:\game\Sim ui_D\数据\launcher.db`, "launcher.db")
	platform := flag.String("platform", "", "platform key, empty for all")
	games := flag.Bool("games", false, "print every game with its board and core")
	emptyOnly := flag.Bool("empty", false, "only games with no board")
	flag.Parse()
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	if *games {
		q := `SELECT platform_key, rom_path, COALESCE(board,''), COALESCE(core,'') FROM game`
		if *platform != "" {
			q += ` WHERE platform_key=?`
			if *emptyOnly {
				q += ` AND (board IS NULL OR board='')`
			}
		} else if *emptyOnly {
			q = `SELECT platform_key, rom_path, COALESCE(board,''), COALESCE(core,'') FROM game WHERE board IS NULL OR board=''`
		}
		q += ` ORDER BY platform_key, rom_path`
		var rows *sql.Rows
		if *platform != "" {
			rows, err = db.Query(q, *platform)
		} else {
			rows, err = db.Query(q)
		}
		if err != nil {
			fmt.Println("query:", err)
			os.Exit(1)
		}
		defer rows.Close()
		for rows.Next() {
			var p, rom, board, core string
			if err := rows.Scan(&p, &rom, &board, &core); err != nil {
				continue
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", p, rom, board, core)
		}
		return
	}

	q := `SELECT platform_key, COALESCE(board,''), COALESCE(core,''), COUNT(*) FROM game`
	if *platform != "" {
		q += ` WHERE platform_key=?`
	}
	q += ` GROUP BY platform_key, board, core ORDER BY platform_key, COUNT(*) DESC`
	var rows *sql.Rows
	if *platform != "" {
		rows, err = db.Query(q, *platform)
	} else {
		rows, err = db.Query(q)
	}
	if err != nil {
		fmt.Println("query:", err)
		os.Exit(1)
	}
	defer rows.Close()
	type key struct{ plat, board, core string }
	counts := map[key]int{}
	for rows.Next() {
		var k key
		var n int
		if err := rows.Scan(&k.plat, &k.board, &k.core, &n); err != nil {
			fmt.Println("scan:", err)
			continue
		}
		counts[k] += n
	}
	var list []key
	for k := range counts {
		list = append(list, k)
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.plat != b.plat {
			return a.plat < b.plat
		}
		return counts[a] > counts[b]
	})
	for _, k := range list {
		fmt.Printf("%-8s %-22s %-14s %4d\n", k.plat, k.board, k.core, counts[k])
	}
}
