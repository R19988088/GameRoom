package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDoubleClickLaunches(t *testing.T) {
	a := testApp(fakeLibrary(), "arcade", "arc")
	tt := ui.NewTester(a.view, 1486, 908)
	tt.Frame()
	tt.ClickAt(146, 200)
	tt.Frame()
	t.Logf("first click: chosen=%d status=%q", a.chosen, a.status)
	tt.ClickAt(146, 200)
	tt.Frame()
	t.Logf("second click: chosen=%d status=%q", a.chosen, a.status)
	if a.status == "" {
		t.Error("a double click did nothing")
	}
}
