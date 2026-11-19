package launch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
)

func TestSplitArgs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"-L cores\\geolith_libretro.dll {RomFullPath}", []string{"-L", "cores\\geolith_libretro.dll", "{RomFullPath}"}},
		{`-L "cores/a b.dll" rom.zip`, []string{"-L", "cores/a b.dll", "rom.zip"}},
		{"  spaced   out  ", []string{"spaced", "out"}},
	}
	for _, c := range cases {
		got := SplitArgs(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("SplitArgs(%q) = %#v, want %#v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("SplitArgs(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestExpand(t *testing.T) {
	rom := filepath.FromSlash("/games/arc/SNK/Foo.neo")
	got := Expand([]string{"{RomFullPath}", "{RomDir}", "{RomName}", "{RomBase}"}, rom)
	want := []string{rom, filepath.Dir(rom), "Foo.neo", "Foo"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Expand[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBundleExecutable(t *testing.T) {
	got := bundleExecutable(filepath.FromSlash("/Apps/RetroArch.app"))
	want := filepath.FromSlash("/Apps/RetroArch.app/Contents/MacOS/RetroArch")
	if got != want {
		t.Fatalf("bundleExecutable = %q, want %q", got, want)
	}
}

// TestResolve checks a sim's relative exe and its core against a rom,
// which is how a RetroArch game starts.
func TestResolve(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "模拟器", "RetroArch")
	coreDir := filepath.Join(exeDir, "cores")
	if err := os.MkdirAll(coreDir, 0o755); err != nil {
		t.Skip(err)
	}
	exe := filepath.Join(exeDir, "retroarch.exe")
	if err := os.WriteFile(exe, nil, 0o644); err != nil {
		t.Skip(err)
	}
	core := coreFile(exeDir, "geolith")
	if err := os.WriteFile(core, nil, 0o644); err != nil {
		t.Skip(err)
	}
	rom := filepath.Join(root, "rom", "arc", "SNK", "Foo.neo")
	if err := os.MkdirAll(filepath.Dir(rom), 0o755); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(rom, nil, 0o644); err != nil {
		t.Skip(err)
	}

	sim := library.Sim{
		Name: "snk家用机",
		Exe:  "模拟器/RetroArch/retroarch.exe",
		Core: "geolith",
		Args: "{RomFullPath}",
	}
	target, err := Resolve(root, sim, rom, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target.Exe != exe {
		t.Fatalf("Exe = %q, want %q", target.Exe, exe)
	}
	if len(target.Args) != 3 || target.Args[0] != "-L" || target.Args[1] != core || target.Args[2] != rom {
		t.Fatalf("Args = %#v", target.Args)
	}
	if target.Work != exeDir {
		t.Fatalf("Work = %q, want %q", target.Work, exeDir)
	}
}
