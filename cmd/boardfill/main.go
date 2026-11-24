// Command boardfill writes each arcade romset's 基板 into the library, taken
// from MAME's own machine list: every romset MAME knows answers with the driver
// source it runs on (neogeo.cpp, cps1.cpp, pgm.cpp …), which is what the shelf
// files games by and what the panel counts buttons from. Romsets MAME does not
// know — hacks, bootlegs, translations — can be asked of Arcade Italia instead.
//
//	# once, from a MAME that can dump its list:
//	mame64 -listxml > E:\game\tmp\mame-listxml.xml
//	go run ./cmd/boardfill -xml E:\game\tmp\mame-listxml.xml -dry
//	go run ./cmd/boardfill -xml E:\game\tmp\mame-listxml.xml
//	go run ./cmd/boardfill -xml E:\game\tmp\mame-listxml.xml -online
package main

import (
	"database/sql"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/R19988088/GameRoom/internal/covers"
	"github.com/R19988088/GameRoom/internal/library"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", `E:\game\Sim ui_D\数据\launcher.db`, "launcher.db")
	xmlPath := flag.String("xml", `E:\game\tmp\mame-listxml.xml`, "MAME -listxml dump")
	platform := flag.String("platform", "arc", "platform key to fill")
	force := flag.Bool("force", false, "overwrite a board that is already written")
	dry := flag.Bool("dry", false, "print what would change, write nothing")
	online := flag.Bool("online", false, "ask Arcade Italia for romsets MAME does not know")
	family := flag.Bool("family", true, "guess a hack's board from the romset it is built on (wofh → wof)")
	limit := flag.Int("limit", 0, "stop after this many writes, 0 for all")
	flag.Parse()

	drivers, err := readDrivers(*xmlPath)
	if err != nil {
		fmt.Println("xml:", err)
		os.Exit(1)
	}
	fmt.Printf("MAME 认得 %d 个 romset\n", len(drivers))

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()
	lib, err := library.Load(db)
	if err != nil {
		fmt.Println("load:", err)
		os.Exit(1)
	}

	type row struct {
		id    int
		rom   string
		board string
		stem  string
		ext   string
	}
	var rows []row
	for _, g := range lib.Games {
		if g.PlatformKey != *platform {
			continue
		}
		slash := filepath.ToSlash(g.RomPath)
		base := path.Base(slash)
		ext := strings.ToLower(path.Ext(base))
		stem := strings.ToLower(strings.TrimSuffix(base, path.Ext(base)))
		if stem == "" {
			continue
		}
		if g.Board != "" && !*force {
			continue
		}
		rows = append(rows, row{id: g.ID, rom: g.RomPath, board: g.Board, stem: stem, ext: ext})
	}

	var adb *covers.ADB
	if *online {
		adb = covers.NewADB()
	}
	wrote, missed, failed, guessed := 0, 0, 0, 0
	for _, r := range rows {
		if *limit > 0 && wrote >= *limit {
			break
		}
		drive, ok := drivers[r.stem]
		from := ""
		if !ok && adb != nil {
			if info, err := adb.Info(r.stem); err == nil && strings.TrimSpace(info.Board) != "" {
				drive, ok = strings.TrimSpace(info.Board), true
			}
		}
		if !ok && r.ext == ".neo" {
			drive, ok = "neogeo.cpp", true // a NeoGeo CD image runs on the same hardware
		}
		if !ok {
			if a, has := aliases[r.stem]; has {
				drive, ok, from = a, true, r.stem
			}
		}
		if !ok && *family && r.ext != ".cue" && r.ext != ".gdi" && r.ext != ".chd" {
			// A hack or a translation of a romset MAME knows: the romset it was built
			// on says which hardware it runs on. The longest prefix that MAME knows
			// wins, and it is printed, so a wrong guess is visible.
			if base, found := parentOf(drivers, r.stem); found {
				drive, ok, from = drivers[base], true, base
			}
		}
		if !ok {
			missed++
			fmt.Printf("没查到  %s\n", r.rom)
			continue
		}
		if drive == r.board {
			continue
		}
		if *dry {
			fmt.Printf("%-46s %-22s → %s%s\n", r.rom, orNone(r.board), drive, hint(from))
			wrote++
			continue
		}
		if err := library.SetField(db, r.id, library.FieldBoard, drive); err != nil {
			failed++
			fmt.Printf("写入失败 %s: %v\n", r.rom, err)
			continue
		}
		if from != "" {
			guessed++
		}
		wrote++
		fmt.Printf("%-46s %-22s → %s%s\n", r.rom, orNone(r.board), drive, hint(from))
	}
	fmt.Printf("\n基板写入 %d（其中按家族推断 %d），MAME/ADB 都没有 %d，写入失败 %d\n",
		wrote, guessed, missed, failed)
	if missed > 0 && !*online {
		fmt.Println("（没查到的可以加 -online 让 Arcade Italia 查一遍）")
	}
}

// aliases are the few改版 whose file name matches no romset MAME lists, while
// the hardware they run on is known: 侍魂 II 的改版 samsh2sp is a NeoGeo cartridge,
// 少年街霸 3 的改版 sfz3mix a CPS2 one.
var aliases = map[string]string{
	"samsh2sp": "neogeo.cpp",
	"sfz3mix":  "cps2.cpp",
}

func orNone(s string) string {
	if s == "" {
		return "（空）"
	}
	return s
}

// hint says which romset a board was guessed from, "" when it was read straight
// off MAME's list.
func hint(from string) string {
	if from == "" {
		return ""
	}
	return "   （按 " + from + " 推断）"
}

// parentOf is the romset a hack is built on: the longest prefix of its name that
// MAME knows, with at most four letters left over (wofh → wof, kof97pla → kof97,
// doubledrbh → doubledr). A prefix that leaves most of the name behind belongs to
// another game — "Samurai Shodown II X" is no "samurai" — so it is refused.
func parentOf(drivers map[string]string, stem string) (string, bool) {
	for left := 1; left <= 4 && len(stem)-left >= 3; left++ {
		if _, ok := drivers[stem[:len(stem)-left]]; ok {
			return stem[:len(stem)-left], true
		}
	}
	return "", false
}

// readDrivers reads MAME's machine list into romset → driver source. Devices and
// non-runnable machines are skipped: they are chips and cartridges, not boards a
// game is filed under.
func readDrivers(file string) (map[string]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make(map[string]string, 40000)
	dec := xml.NewDecoder(f)
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		el, ok := tok.(xml.StartElement)
		if !ok || el.Name.Local != "machine" {
			continue
		}
		var name, source, device, runnable string
		for _, a := range el.Attr {
			switch a.Name.Local {
			case "name":
				name = a.Value
			case "sourcefile":
				source = a.Value
			case "isdevice":
				device = a.Value
			case "runnable":
				runnable = a.Value
			}
		}
		if name == "" || source == "" || device == "yes" || runnable == "no" {
			continue
		}
		out[strings.ToLower(name)] = source
	}
	return out, nil
}
