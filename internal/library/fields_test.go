package library

import (
	"path/filepath"
	"testing"
)

// TestSetFieldWritesARow: the card's menu writes real columns, and refuses a
// name that is not one of them.
func TestSetFieldWritesARow(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO platform (key, name, grp, exts, sort) VALUES ('arc','ARCADE','arcade','zip',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO game (platform_key, title, rom_path, publisher) VALUES ('arc','avsp','arc/avsp.zip','CAPCOM')`); err != nil {
		t.Fatal(err)
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM game WHERE title = 'avsp'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := SetField(db, id, FieldPublisher, "TAITO"); err != nil {
		t.Fatal(err)
	}
	if err := SetField(db, id, FieldBoard, "cps2"); err != nil {
		t.Fatal(err)
	}
	var publisher, board string
	if err := db.QueryRow(`SELECT publisher, board FROM game WHERE id = ?`, id).Scan(&publisher, &board); err != nil {
		t.Fatal(err)
	}
	if publisher != "TAITO" || board != "cps2" {
		t.Errorf("row = %q %q", publisher, board)
	}
	if err := SetField(db, id, "publisher = 'x' WHERE id = 1 --", "COLUMNS"); err == nil {
		t.Error("a field name that is not one of the columns was taken")
	}
}
