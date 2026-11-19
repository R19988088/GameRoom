package library

import "testing"

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
