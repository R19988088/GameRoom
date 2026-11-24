package library

import (
	"path/filepath"
	"testing"
)

func TestSplitCore(t *testing.T) {
	cases := []struct {
		args string
		core string
		rest string
	}{
		{`-L cores\geolith_libretro.dll {RomFullPath}`, "geolith", "{RomFullPath}"},
		{`-L cores/fbneo_libretro.so {RomFullPath}`, "fbneo", "{RomFullPath}"},
		{`-L "/opt/RetroArch/cores/mame_libretro.dylib" {RomFullPath}`, "mame", "{RomFullPath}"},
		{`-L cores\fbalpha2012_neogeo_libretro.dll`, "fbalpha2012_neogeo", ""},
		{`{RomFullPath}`, "", "{RomFullPath}"},
		{"", "", ""},
	}
	for _, c := range cases {
		core, rest := SplitCore(c.args)
		if core != c.core || rest != c.rest {
			t.Fatalf("SplitCore(%q) = %q, %q; want %q, %q", c.args, core, rest, c.core, c.rest)
		}
	}
}

// TestSetIPSRecordsTheGroups: the groups a dump plays with are written in the
// game's row and read back, so a restart keeps the switches as they were left.
func TestSetIPSRecordsTheGroups(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "launcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO platform (key, name, grp, exts, sort)
		VALUES ('arc', 'ARCADE', 'arcade', 'zip', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO game (platform_key, title, rom_path)
		VALUES ('arc', 'JOJO的奇妙冒险', 'arc/jojobajr1.zip')`); err != nil {
		t.Fatal(err)
	}
	if err := SetIPS(db, 1, []string{"Noncheat", "MyHack"}); err != nil {
		t.Fatal(err)
	}
	l, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Games) != 1 {
		t.Fatalf("the library holds %d games, want 1", len(l.Games))
	}
	g := l.Games[0]
	if !g.IPSOn("Noncheat") || !g.IPSOn("MyHack") {
		t.Errorf("game's ips = %q, want both groups", g.IPS)
	}
	if g.IPSOn("Other") {
		t.Error("a group that was never written reads as on")
	}

	if err := SetIPS(db, 1, nil); err != nil {
		t.Fatal(err)
	}
	l, err = Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.Games[0].IPSGroups(); len(got) != 0 {
		t.Errorf("after clearing, groups = %v", got)
	}
}
