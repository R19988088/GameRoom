package library

import (
	"path/filepath"
	"testing"
)

// TestNameBecomesTheTitle: a row filed under its romset's file name takes the
// catalogue's name, and the shelf reads it; a row with a title of its own
// keeps that title.
func TestNameBecomesTheTitle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO platform (key, name, grp, exts, sort) VALUES ('arc','ARCADE','arcade','zip',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO game (platform_key, title, rom_path) VALUES
		('arc','avsp','arc/others/avsp.7z'),
		('arc','合金弹头I','arc/others/mslug.zip')`); err != nil {
		t.Fatal(err)
	}
	var avsp, mslug int
	if err := db.QueryRow(`SELECT id FROM game WHERE title = 'avsp'`).Scan(&avsp); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM game WHERE title = '合金弹头I'`).Scan(&mslug); err != nil {
		t.Fatal(err)
	}
	if err := SetName(db, avsp, "Alien vs. Predator"); err != nil {
		t.Fatal(err)
	}
	if err := SetName(db, mslug, ""); err != nil {
		t.Fatalf("an empty name is not an error: %v", err)
	}

	lib, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range lib.Games {
		switch g.ID {
		case avsp:
			if g.Title != "Alien vs. Predator" {
				t.Errorf("avsp reads %q, want the catalogue's title", g.Title)
			}
		case mslug:
			if g.Title != "合金弹头I" {
				t.Errorf("mslug reads %q, want its own title", g.Title)
			}
		}
	}
}
