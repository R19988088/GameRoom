package library

import (
	"path/filepath"
	"testing"
)

// TestFileDumpSurvivesARestart: the note that a dump is filed under a card, or
// runs on its own, is a column pair, so the next start reads the same families
// back.
func TestFileDumpSurvivesARestart(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO platform (key, name, grp, exts, sort) VALUES ('arc','ARCADE','arcade','zip',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO game (platform_key, title, rom_path) VALUES ('arc','sf2','arc/sf2.zip'), ('arc','sf2ce','arc/sf2ce.zip')`); err != nil {
		t.Fatal(err)
	}
	var base, hack int
	if err := db.QueryRow(`SELECT id FROM game WHERE rom_path = 'arc/sf2.zip'`).Scan(&base); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM game WHERE rom_path = 'arc/sf2ce.zip'`).Scan(&hack); err != nil {
		t.Fatal(err)
	}
	if err := FileDump(db, hack, base, false); err != nil {
		t.Fatal(err)
	}
	lib, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	g := lib.Game(hack)
	if g == nil || g.Family != base || g.Solo {
		t.Fatalf("game %d = %+v after the drop", hack, g)
	}

	if err := FileDump(db, hack, 0, true); err != nil {
		t.Fatal(err)
	}
	lib, err = Load(db)
	if err != nil {
		t.Fatal(err)
	}
	g = lib.Game(hack)
	if g == nil || g.Family != 0 || !g.Solo {
		t.Fatalf("game %d = %+v after 独立运行", hack, g)
	}
}
