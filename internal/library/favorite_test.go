package library

import (
	"path/filepath"
	"testing"
)

// TestSetFavoriteSurvivesARestart: the star the card's menu writes is a column,
// so the next start reads the same 收藏 page back.
func TestSetFavoriteSurvivesARestart(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO platform (key, name, grp, exts, sort) VALUES ('arc','ARCADE','arcade','zip',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO game (platform_key, title, rom_path) VALUES ('arc','dino','arc/dino.zip')`); err != nil {
		t.Fatal(err)
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM game WHERE title = 'dino'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := SetFavorite(db, id, true); err != nil {
		t.Fatal(err)
	}
	lib, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	g := lib.Game(id)
	if g == nil || !g.Favorite {
		t.Fatalf("game %d = %+v after the star", id, g)
	}
	if err := SetFavorite(db, id, false); err != nil {
		t.Fatal(err)
	}
	var star int
	if err := db.QueryRow(`SELECT favorite FROM game WHERE id = ?`, id).Scan(&star); err != nil {
		t.Fatal(err)
	}
	if star != 0 {
		t.Errorf("favorite column = %d after the star came off, want 0", star)
	}
}
