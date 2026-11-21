package app

import (
	"math"
	"testing"

	"github.com/R19988088/GameRoom/internal/library"
)

// A CPS 384×224 frame is 1.71 across as pixels, but the board's glass is 4:3:
// drawing it at its pixel shape is what made the arcade games look wide.
func TestACPSPictureIsDrawnOnA4By3Glass(t *testing.T) {
	a := &App{}
	a.playerGame = library.Game{PlatformKey: "arc"}
	a.frameW, a.frameH = 384, 224
	if got := a.pictureRatio(); math.Abs(float64(got)-4.0/3.0) > 0.001 {
		t.Errorf("384×224 arcade ratio = %v, want 4:3", got)
	}

	// NeoGeo is 304×224: all but the same glass.
	a.frameW, a.frameH = 304, 224
	if got := a.pictureRatio(); math.Abs(float64(got)-4.0/3.0) > 0.001 {
		t.Errorf("304×224 arcade ratio = %v, want 4:3", got)
	}
}

// A vertical shooter is the same tube on its side.
func TestAVerticalBoardIsDrawnOnA3By4Glass(t *testing.T) {
	a := &App{}
	a.playerGame = library.Game{PlatformKey: "arc"}
	a.frameW, a.frameH = 224, 384
	if got := a.pictureRatio(); math.Abs(float64(got)-3.0/4.0) > 0.001 {
		t.Errorf("224×384 arcade ratio = %v, want 3:4", got)
	}
}

// Square pixels are their own answer: a GBA's 240×160 is 3:2, a GB's 160×144 is
// 10:9, and neither sits on a 4:3 glass.
func TestHandheldsKeepTheirPixelShape(t *testing.T) {
	a := &App{}
	a.playerGame = library.Game{PlatformKey: "掌机"}
	a.frameW, a.frameH = 240, 160
	if got := a.pictureRatio(); math.Abs(float64(got)-1.5) > 0.001 {
		t.Errorf("240×160 GBA ratio = %v, want 1.5", got)
	}
}

// A board past three to one is a triple screen: its own shape is the honest one.
func TestATripleScreenKeepsItsOwnShape(t *testing.T) {
	a := &App{}
	a.playerGame = library.Game{PlatformKey: "arc"}
	a.frameW, a.frameH = 1024, 256
	if got := a.pictureRatio(); math.Abs(float64(got)-4.0) > 0.001 {
		t.Errorf("1024×256 arcade ratio = %v, want 4", got)
	}
}

// A core that does report a display aspect is taken at its word on a platform
// without a tube of its own.
func TestTheCoreAspectIsUsedWhenThePlatformHasNoTube(t *testing.T) {
	a := &App{}
	a.playerGame = library.Game{PlatformKey: "任天堂"}
	a.gameAspect = 1.25
	a.frameW, a.frameH = 400, 240
	if got := a.pictureRatio(); math.Abs(float64(got)-1.25) > 0.001 {
		t.Errorf("ratio = %v, want the core's 1.25", got)
	}
}
