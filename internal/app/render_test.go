package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSearchFieldIsPainted keeps the bar's right end from silently losing
// its field: the glass lens under it has to differ from the page behind.
func TestSearchFieldIsPainted(t *testing.T) {
	a := newTestApp(manyPlatforms(), "fc")
	tt := ui.NewTester(a.view, 1500, 940)
	tt.Frame()
	img := tt.Image()

	field, _, _, _ := img.At(1300, 48).RGBA()
	page, _, _, _ := img.At(1300, 300).RGBA()
	d := int(field>>8) - int(page>>8)
	if d < 8 {
		t.Errorf("the search field is not painted: %d over a page of %d", field>>8, page>>8)
	}
}
