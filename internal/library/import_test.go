package library

import "testing"

// TestGroupOf: 麻将 is a shelf of its own, not a corner of the arcade.
func TestGroupOf(t *testing.T) {
	cases := map[string]string{
		"arc":     "arcade",
		"majiang": "mahjong",
		"fc":      "console",
		"sfc":     "console",
		"3DO":     "console",
		"掌机":      "handheld",
		"微软":      "pc",
	}
	for key, want := range cases {
		if got := groupOf(key); got != want {
			t.Errorf("groupOf(%q) = %q, want %q", key, got, want)
		}
	}
}

// TestGroupsCoverEveryShelf: every group the bar offers is one groupOf can
// return, so no platform lands on a shelf that is not in the menu.
func TestGroupsCoverEveryShelf(t *testing.T) {
	shelves := map[string]bool{}
	for _, g := range Groups {
		shelves[g.Key] = true
	}
	for _, key := range []string{"arc", "majiang", "fc", "掌机", "微软"} {
		if !shelves[groupOf(key)] {
			t.Errorf("groupOf(%q) = %q is not a shelf in Groups", key, groupOf(key))
		}
	}
	if !shelves["mahjong"] {
		t.Error("Groups has no 麻将 shelf")
	}
}
