package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// oneJOJO is a card with one dump on it, the one the ips tests open the menu of.
func oneJOJO() *library.Library {
	return &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "JOJO的奇妙冒险", RomPath: "arc/jojobajr1.zip"},
		},
	}
}

// ipsTree writes 数据/ips/<stem> with a MAME Plus group, or leaves it out.
func ipsTree(t *testing.T, stem string, patches bool) string {
	t.Helper()
	root := t.TempDir()
	if !patches {
		return root
	}
	dir := filepath.Join(root, "数据", "ips", stem)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// One patch writing 'X' at 0x7A, as the 麻将 非作弊 patches do.
	patch := append([]byte("PATCH"), 0x00, 0x00, 0x7A, 0x00, 0x01, 'X')
	patch = append(patch, []byte("EOF")...)
	if err := os.WriteFile(filepath.Join(dir, "Noncheat.dat"),
		[]byte("ch-a-604.u17\tNoncheat-ch-a-604.u17\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Noncheat-ch-a-604.u17.ips"), patch, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestCardMenuShowsIPSAtItsBottom: the menu ends with the ips part — the label,
// then the patch groups with a switch each — when the dump has patches, and the
// part sits under the two folder buttons.
func TestCardMenuShowsIPSAtItsBottom(t *testing.T) {
	a := testApp(oneJOJO(), "arcade", "arc")
	a.root = ipsTree(t, "jojobajr1", true)
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	*a.menuOpen(1) = true
	a.stale = true
	tt.Frame()
	tt.Frame()

	if !tt.HasText("ips") {
		t.Fatalf("the menu has no ips section: %v", tt.Texts())
	}
	if !tt.HasText("Noncheat") {
		t.Fatalf("the ips section does not list the group: %v", tt.Texts())
	}
	rom, ok := tt.Find("rom 目录")
	if !ok {
		t.Fatalf("the menu has no rom 目录 button: %v", tt.Texts())
	}
	label, ok := tt.Find("ips")
	if !ok {
		t.Fatal("no ips label")
	}
	if label.Y < rom.Y {
		t.Errorf("ips sits at y=%v, the folder buttons at y=%v: it is not at the bottom",
			label.Y, rom.Y)
	}
}

// TestCardMenuHidesIPSWithoutPatches: no folder under 数据/ips for this rom means
// no ips row at all — the menu only offers what the dump has.
func TestCardMenuHidesIPSWithoutPatches(t *testing.T) {
	a := testApp(oneJOJO(), "arcade", "arc")
	a.root = ipsTree(t, "jojobajr1", false)
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	*a.menuOpen(1) = true
	a.stale = true
	tt.Frame()
	tt.Frame()

	for _, text := range []string{"ips", "Noncheat"} {
		if tt.HasText(text) {
			t.Errorf("%q is in the menu of a dump with no patches", text)
		}
	}
}

// TestIPSToggleIsKept: the switch writes the group into the library and takes it
// out again, and the dump reads it back.
func TestIPSToggleIsKept(t *testing.T) {
	a := testApp(oneJOJO(), "arcade", "arc")
	a.root = ipsTree(t, "jojobajr1", true)

	a.toggleIPS(*a.lib.Game(1), "Noncheat")
	g := a.lib.Game(1)
	if g == nil || !g.IPSOn("Noncheat") {
		t.Fatalf("game 1 = %+v after the switch, want Noncheat on", g)
	}
	if len(g.IPSGroups()) != 1 {
		t.Errorf("game 1 keeps %v, want one group", g.IPSGroups())
	}
	if len(a.ipsEnabled(*g)) != 1 {
		t.Errorf("the launch would run with %d groups, want 1", len(a.ipsEnabled(*g)))
	}

	a.toggleIPS(*a.lib.Game(1), "Noncheat")
	if g := a.lib.Game(1); g == nil || g.IPSOn("Noncheat") {
		t.Fatalf("game 1 = %+v after the second switch, want Noncheat off", g)
	}
	if len(a.ipsEnabled(*a.lib.Game(1))) != 0 {
		t.Error("a group that is off still counts for the launch")
	}
}

// TestIPSSwitchClickTurnsTheGroupOn: the switch at the right of a row is what
// turns a group on, one click, and the dump reads it as on.
func TestIPSSwitchClickTurnsTheGroupOn(t *testing.T) {
	a := testApp(oneJOJO(), "arcade", "arc")
	a.root = ipsTree(t, "jojobajr1", true)
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	*a.menuOpen(1) = true
	a.stale = true
	tt.Frame()
	tt.Frame()

	var row *ui.Rect
	for _, info := range tt.AccessAll() {
		if info.Name == "ips Noncheat" {
			r := info.Bounds
			row = &r
		}
	}
	if row == nil {
		for _, info := range tt.AccessAll() {
			t.Logf("%q %v", info.Name, info.Bounds)
		}
		t.Fatal("the ips row has no box to click")
	}
	// The switch sits at the row's right end.
	tt.ClickAt(row.X+row.W-20, row.Y+row.H/2)
	tt.Frame()
	if g := a.lib.Game(1); g == nil || !g.IPSOn("Noncheat") {
		t.Fatalf("game 1 = %+v after the switch, want Noncheat on", g)
	}
}

// TestIPSTitleSaysHowManyGroups: the caption of the playing game carries the
// count of groups it runs with, and only when there are any.
func TestIPSTitleSaysHowManyGroups(t *testing.T) {
	cases := []struct {
		groups int
		want   string
	}{
		{0, "JOJO的奇妙冒险"},
		{2, "JOJO的奇妙冒险    ips启用 2 组"},
	}
	for _, c := range cases {
		if got := ipsTitle("JOJO的奇妙冒险", c.groups); got != c.want {
			t.Errorf("ipsTitle(%d) = %q, want %q", c.groups, got, c.want)
		}
	}
}

// TestFBNeoIsToldWhereThePatchedSetIs: the copy goes to <system>/fbneo/patched
// under the set's own name, what was there before is replaced, and switching the
// patches off takes it away again — a leftover copy would otherwise keep running
// on FBNeo after the user turned the patches off.
func TestFBNeoIsToldWhereThePatchedSetIs(t *testing.T) {
	system := t.TempDir()
	rom := filepath.Join(t.TempDir(), "dino.zip")

	patched := filepath.Join(t.TempDir(), "dino.zip")
	if err := os.WriteFile(patched, []byte("patched set"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stagePatchedSet(system, rom, patched); err != nil {
		t.Fatalf("stagePatchedSet: %v", err)
	}
	dst := patchedSetPath(system, rom)
	if !strings.HasSuffix(filepath.ToSlash(dst), "/fbneo/patched/dino.zip") {
		t.Errorf("the copy went to %q, want fbneo/patched/dino.zip", dst)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "patched set" {
		t.Fatalf("the copy reads %q (%v)", data, err)
	}

	clearPatchedSet(system, rom)
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("the copy is still there after clearPatchedSet: %v", err)
	}

	for _, core := range []string{"fbneo", "fbneo_plus", "FBNeo"} {
		if !isFBNeoCore(core) {
			t.Errorf("%s should take the patched folder", core)
		}
	}
	for _, core := range []string{"mame", "fceumm", "snes9x"} {
		if isFBNeoCore(core) {
			t.Errorf("%s should be handed the copy itself", core)
		}
	}
}
