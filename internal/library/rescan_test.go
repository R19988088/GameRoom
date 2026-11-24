package library

import (
	"os"
	"path/filepath"
	"testing"
)

// mkfile writes a rom of the given size, creating the folders above it.
func mkfile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRescanAddsARomCopiedIn: a zip that appears under rom/<platform>/ after the
// import becomes a card, lands under the folder it was copied into, and is not
// added a second time by the next refresh.
func TestRescanAddsARomCopiedIn(t *testing.T) {
	root := t.TempDir()
	db, err := Open(filepath.Join(root, "launcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO platform (key, name, grp, exts, sort) VALUES ('arc','ARCADE','arcade','zip|7z',1)`); err != nil {
		t.Fatal(err)
	}

	mkfile(t, filepath.Join(root, "rom", "arc", "CAPCOM", "STREET FIGHTER", "hsf2.zip"), 4096)
	n, err := Rescan(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("added %d rows, want 1", n)
	}
	var title, menu, romPath string
	var size int64
	if err := db.QueryRow(`SELECT title, menu, rom_path, size FROM game`).Scan(&title, &menu, &romPath, &size); err != nil {
		t.Fatal(err)
	}
	if romPath != "arc/CAPCOM/STREET FIGHTER/hsf2.zip" {
		t.Errorf("rom path = %q", romPath)
	}
	if menu != "CAPCOM" {
		t.Errorf("menu = %q, want the folder it was copied into", menu)
	}
	if title != "hsf2" {
		t.Errorf("title = %q, want the file's name", title)
	}
	if size != 4096 {
		t.Errorf("size = %d, want the file's own", size)
	}

	// 再刷一次：同一个文件不会变成第二张卡。
	n, err = Rescan(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("the second rescan added %d rows, want none", n)
	}
}

// TestRescanFilesASecondDumpAsAVersion: hsf2.zip beside a shelf's hsf2.7z is the
// same game in another container — it joins the card as a version instead of
// becoming a second card with the same title.
func TestRescanFilesASecondDumpAsAVersion(t *testing.T) {
	root := t.TempDir()
	db, err := Open(filepath.Join(root, "launcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO platform (key, name, grp, exts, sort) VALUES ('arc','ARCADE','arcade','zip|7z',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO game (platform_key, menu, title, rom_path, publisher, year, board, sim_id)
		VALUES ('arc','CAPCOM','超级街头霸王Ⅱ 15周年纪念版','arc/CAPCOM/STREET FIGHTER/hsf2.7z','CAPCOM',1994,'capcom/cps2.cpp',32)`); err != nil {
		t.Fatal(err)
	}
	var base int
	if err := db.QueryRow(`SELECT id FROM game`).Scan(&base); err != nil {
		t.Fatal(err)
	}
	mkfile(t, filepath.Join(root, "rom", "arc", "CAPCOM", "STREET FIGHTER", "hsf2.7z"), 1024)
	mkfile(t, filepath.Join(root, "rom", "arc", "CAPCOM", "STREET FIGHTER", "hsf2.zip"), 2048)

	n, err := Rescan(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("added %d rows, want 1", n)
	}
	var title, board string
	var family, solo, simID int
	if err := db.QueryRow(`SELECT title, board, family, solo, sim_id FROM game WHERE rom_path LIKE '%.zip'`).
		Scan(&title, &board, &family, &solo, &simID); err != nil {
		t.Fatal(err)
	}
	if title != "超级街头霸王Ⅱ 15周年纪念版" {
		t.Errorf("the new dump is titled %q, not the game's name", title)
	}
	if family != base || solo != 0 {
		t.Errorf("family = %d solo = %d, want it hanging under base %d", family, solo, base)
	}
	if board != "capcom/cps2.cpp" || simID != 32 {
		t.Errorf("board = %q sim = %d, want the same as the base dump", board, simID)
	}
}
