package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestRefreshSitsLeftOfTheSearch: the bar's refresh pill is drawn to the left
// of the search field, and pressing it rereads the library — the tested app has
// no database, so the reread is felt as the read failing rather than as new
// cards, which is enough to prove the click reached reload.
func TestRefreshSitsLeftOfTheSearch(t *testing.T) {
	a := testApp(twoHacks(), "arcade", "arc")
	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()

	refresh, ok := tt.Find("刷新")
	if !ok {
		t.Fatalf("the bar has no refresh button: %v", tt.Texts())
	}
	search, ok := tt.Find("搜索游戏名")
	if !ok {
		t.Fatalf("the bar has no search field: %v", tt.Texts())
	}
	if refresh.X >= search.X {
		t.Errorf("the refresh button sits at %v, the search at %v: it is not left of it",
			refresh.X, search.X)
	}

	tt.ClickAtWith(0, refresh.X+refresh.W/2, refresh.Y+refresh.H/2)
	tt.Frame()
	tt.Frame()
	if !strings.Contains(a.status, "读库") {
		t.Errorf("pressing the refresh button did not reread the library: status %q", a.status)
	}
}
