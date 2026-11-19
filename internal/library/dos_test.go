package library

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRomNameGivesMakerAndYear: the catalogue is incomplete, so a rom's own
// name is the second source of a year and a maker — "(1994)(HimeyaSoft)".
func TestRomNameGivesMakerAndYear(t *testing.T) {
	year, maker := fromRomName("Ash. (1994)(HimeyaSoft)(TW)[cr]")
	if year != 1994 || maker != "HimeyaSoft" {
		t.Fatalf("got %d / %q, want 1994 / HimeyaSoft", year, maker)
	}
	// A region sits where the maker would in some datasets; it is not a maker.
	if _, m := fromRomName("Some Game (1991)(JP)[tr zh x]"); m != "" {
		t.Errorf("(JP) became a maker: %q", m)
	}
}

// TestDOSMakerIsTheGamesPublisher: the DOS&PC98 shelf files by the house that
// released the game here (松岗), not by the platform's maker.
func TestDOSMakerIsTheGamesPublisher(t *testing.T) {
	lib := &Library{Platforms: []Platform{{Key: DOSKey, Name: "DOS", Group: DOSGroup}}}
	lib.index()
	g := Game{PlatformKey: DOSKey, Publisher: "松岗", Menu: "厂商"}
	if got := lib.MakerOf(g); got != "松岗" {
		t.Errorf("maker = %q, want 松岗", got)
	}
	g.Publisher = ""
	if got := lib.MakerOf(g); got != "未标注厂商" {
		t.Errorf("an unnamed publisher became %q", got)
	}
}

// TestTitleFromRomName: a game the catalogue has no Chinese title for shows
// its name, not its whole file name.
func TestTitleFromRomName(t *testing.T) {
	cases := map[string]string{
		"Ash. (1994)(HimeyaSoft)(TW)[cr]":              "Ash.",
		"V.G. Variable Geo (1993)(Giga)(TW)[cr]":       "V.G. Variable Geo",
		"YES! HG":                                      "YES! HG",
		"(1995)(elf)(JP)":                              "(1995)(elf)(JP)",
		"Doukyuusei 2 (1995)(elf)(JP)[tr zh waxixixi]": "Doukyuusei 2",
	}
	for in, want := range cases {
		if got := titleFromRomName(in); got != want {
			t.Errorf("titleFromRomName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMakerAliasJoinsSpellings: 天堂鳥 and 天堂鸟 are one house.
func TestMakerAliasJoinsSpellings(t *testing.T) {
	if got := makerAlias["天堂鳥"]; got != "天堂鸟" {
		t.Errorf("alias = %q", got)
	}
}

// TestImageInPicksTheDiskImage: a PC-98 folder holds the image beside the
// leftovers of the archive; only the image is content.
func TestImageInPicksTheDiskImage(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"NANPA2.hdi", "dosbox.conf", "FONT.bmp", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := imageIn(dir); got != "NANPA2.hdi" {
		t.Errorf("imageIn = %q, want NANPA2.hdi", got)
	}
	if got := imageIn(t.TempDir()); got != "" {
		t.Errorf("an empty folder gave %q", got)
	}
}
