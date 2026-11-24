package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
	"github.com/egoist/mygo/ui"
)

// TestFavoriteStarSitsLeftOfTheTitle: the star of a starred card is drawn on the
// left of its title, not on the right.
func TestFavoriteStarSitsLeftOfTheTitle(t *testing.T) {
	a := testApp(twoHacks(), "arcade", "arc")
	a.lib.Games[0].Favorite = true
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	star, ok := tt.Find("★")
	if !ok {
		t.Fatalf("the starred card has no star: %v", tt.Texts())
	}
	title, ok := tt.Find("街头霸王2")
	if !ok {
		t.Fatalf("the card has no title: %v", tt.Texts())
	}
	if star.X >= title.X {
		t.Errorf("the star sits at %v, the title at %v: the star is not left of it",
			star.X, title.X)
	}
}

// TestCardMenuShowsTheRomName: the card's menu shows the dump's file name at its
// top right, without the extension, and no hint lines under it; the star row
// says 添加收藏 or 取消收藏 by whether the card is starred.
func TestCardMenuShowsTheRomName(t *testing.T) {
	l := &library.Library{
		Platforms: []library.Platform{{Key: "arc", Name: "ARCADE", Group: "arcade"}},
		Games: []library.Game{
			{ID: 1, PlatformKey: "arc", Title: "JOJO的奇妙冒险：未来遗产", RomPath: "arc/jojobajr.zip"},
		},
	}
	a := testApp(l, "arcade", "arc")
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	*a.menuOpen(1) = true
	a.stale = true
	tt.Frame()

	if !tt.HasText("jojobajr") {
		t.Errorf("the menu does not show the rom name: %v", tt.Texts())
	}
	if tt.HasText("jojobajr.zip") {
		t.Error("the rom name keeps its extension")
	}
	for _, gone := range []string{"右键", "单击选择", "拖入改版"} {
		if tt.HasText(gone) {
			t.Errorf("%q is still in the menu", gone)
		}
	}
	if !tt.HasText("添加收藏") {
		t.Error("an unstarred card does not offer 添加收藏")
	}
	if tt.HasText("取消收藏") {
		t.Error("an unstarred card already offers 取消收藏")
	}

	// 收藏之后同一行换成 取消收藏（动作本身会关菜单，再开一次）。
	a.toggleFavorite(1)
	a.runQueue()
	*a.menuOpen(1) = true
	a.stale = true
	tt.Frame()
	if !tt.HasText("取消收藏") {
		t.Error("a starred card does not offer 取消收藏")
	}
}

// TestCardMenuOpensBothFolders: the menu ends with the dump's own rom folder and
// the cover folder, side by side — two buttons on one line, one beside the other.
func TestCardMenuOpensBothFolders(t *testing.T) {
	a := testApp(twoHacks(), "arcade", "arc")
	a.buildVersions()
	a.refilter()
	a.stale = true

	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	*a.menuOpen(1) = true
	a.stale = true
	tt.Frame()
	tt.Frame()

	romBtn, ok := tt.Find("rom 目录")
	if !ok {
		t.Fatalf("the menu has no rom 目录 button: %v", tt.Texts())
	}
	coverBtn, ok := tt.Find("封面目录")
	if !ok {
		t.Fatalf("the menu has no 封面目录 button: %v", tt.Texts())
	}
	if romBtn.Y != coverBtn.Y {
		t.Errorf("the two buttons are not on one line: y=%v and y=%v", romBtn.Y, coverBtn.Y)
	}
	if romBtn.X >= coverBtn.X {
		t.Errorf("rom 目录 sits at %v, 封面目录 at %v: they are not side by side", romBtn.X, coverBtn.X)
	}

	// 尺寸不对的目录只在状态行里说，不弹资源管理器、也不改任何数据。
	a.status = ""
	a.openFolder(filepath.Join(t.TempDir(), "没有这个目录"))
	if !strings.Contains(a.status, "目录不存在") {
		t.Errorf("status = %q for a missing folder", a.status)
	}
}
