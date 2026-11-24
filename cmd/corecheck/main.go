// Command corecheck loads one core on one rom the way the launcher does, steps
// it long enough to tell a picture from a black screen, asks it for a save state
// and puts that state back. One core per process, so a core that faults takes
// only this run down and the report still reaches whoever reads it.
//
//	go run ./cmd/corecheck -core mame -rom "E:\game\Sim ui_D\rom\arc\dino.zip"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/R19988088/GameRoom/internal/core"
)

type report struct {
	Core    string `json:"core"`
	Rom     string `json:"rom"`
	Status  string `json:"status"` // ok / no_frame / black / start_fail / panic / timeout
	Err     string `json:"err,omitempty"`
	Frames  int    `json:"frames"`
	FPS     float64 `json:"fps"`
	W       int    `json:"w"`
	H       int    `json:"h"`
	Colors  int    `json:"colors"`
	Mean    float64 `json:"mean"`
	Audio   int64  `json:"audio"`
	Format  int    `json:"format"`
	CanSave bool   `json:"can_save"`
	Declare int    `json:"declare"`
	Saved   int    `json:"saved"`
	LoadOK  bool   `json:"load_ok"`
	SaveMS  int64  `json:"save_ms"`
	LoadMS  int64  `json:"load_ms"`
	LoadS   int64  `json:"start_ms"`
	Note    string `json:"note,omitempty"`
}

func main() {
	ra := flag.String("ra", `E:\game\Sim ui_D\模拟器\RetroArch`, "RetroArch dir")
	coreName := flag.String("core", "", "core name, without _libretro.dll")
	rom := flag.String("rom", "", "rom path")
	secs := flag.Float64("secs", 3, "seconds to step")
	png := flag.String("png", "", "write the last frame here")
	flag.Parse()
	rep := report{Core: *coreName, Rom: *rom}

	fail := func(format string, a ...any) {
		rep.Status = "start_fail"
		rep.Err = fmt.Sprintf(format, a...)
		write(rep)
		os.Exit(1)
	}
	if *coreName == "" || *rom == "" {
		fail("usage: corecheck -core NAME -rom PATH")
	}
	corePath := filepath.Join(*ra, "cores", *coreName+"_libretro.dll")
	if _, err := os.Stat(corePath); err != nil {
		fail("no core %s", corePath)
	}
	if _, err := os.Stat(*rom); err != nil {
		fail("no rom %s", *rom)
	}
	sysDir := filepath.Join(*ra, "system")
	if p := filepath.Join(filepath.VolumeName(*ra)+`\`, "GAMEROOM", "system"); dirExists(p) {
		sysDir = p
	}
	if s := os.Getenv("CORE_CHECK_SYS"); s != "" {
		sysDir = s
	}

	// A fault inside a core comes back as a panic only when the frame loop is
	// reached, so the load is guarded here and the run is guarded below.
	// The core is started and stepped on one thread of its own, the way the app
	// does it: OpenGL lives on the thread that created the context, so a load on
	// one thread and a run on another would draw into a context nobody reads.
	var sess *core.Session
	var startErr error
	loadedCh := make(chan struct{}, 1)
	gate := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				rep.Status = "panic"
				rep.Err = fmt.Sprint(r)
				write(rep)
				os.Exit(2)
			}
		}()
		runtime.LockOSThread()
		t0 := time.Now()
		sess, startErr = core.Start(corePath, *rom, sysDir)
		rep.LoadS = time.Since(t0).Milliseconds()
		loadedCh <- struct{}{}
		if startErr != nil {
			return
		}
		<-gate
		sess.Run()
	}()
	<-loadedCh
	if startErr != nil {
		fail("%v", startErr)
	}
	// One key bound instead of the whole board: the default table is every key
	// the machine has, and asking the system about all of them costs more per
	// frame than the core does.
	sess.SetKeyBinds(0, map[int]int{0x41: 97})
	close(gate)

	deadline := time.Now().Add(time.Duration(*secs * float64(time.Second)))
	// The sound is drained and thrown away: a core that makes audio waits for
	// the output to take it, and this probe has no output to hand it to.
	go func() {
		buf := make([]float32, 4096)
		for {
			if _, err := sess.Audio().Read(buf); err != nil {
				return
			}
		}
	}()
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if sess.Frames() >= int(sess.FPS()*float64(*secs)) {
			break
		}
	}

	f := sess.Frame()
	if f != nil {
		rep.W, rep.H = f.Bounds().Dx(), f.Bounds().Dy()
		rep.Colors, rep.Mean = stats(f)
		if *png != "" {
			savePNG(*png, f)
		}
	}
	rep.Frames = sess.Frames()
	rep.FPS = sess.FPS()
	rep.Audio = sess.Audio().Pushed()
	rep.Format = sess.PixelFormat()
	rep.Status = "ok"
	if rep.Frames == 0 {
		rep.Status = "no_frame"
	} else if rep.Colors <= 2 {
		rep.Status = "black"
	}

	rep.CanSave = sess.CanState()
	if rep.CanSave {
		rep.Declare = sess.StateSize()
		start := time.Now()
		data, err := sess.SaveState()
		rep.SaveMS = time.Since(start).Milliseconds()
		if err != nil {
			rep.Note = "save: " + err.Error()
		} else {
			rep.Saved = len(data)
			t0 := time.Now()
			if err := sess.LoadState(data); err != nil {
				rep.Note = "load: " + err.Error()
			} else {
				rep.LoadOK = true
				sess.Step()
			}
			rep.LoadMS = time.Since(t0).Milliseconds()
		}
	} else {
		rep.Note = "core declares no state"
	}
	write(rep)
	sess.Close()
}

// write emits one line of JSON per run, so a batch can be read back whole.
func write(r report) {
	b, _ := json.Marshal(r)
	fmt.Println("CHECK " + string(b))
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// savePNG writes the frame as a picture, so a core that reports a picture can
// be looked at rather than trusted.
func savePNG(path string, img *image.RGBA) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

func stats(img *image.RGBA) (int, float64) {
	seen := map[uint32]struct{}{}
	var sum float64
	var n int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			off := img.PixOffset(x, y)
			c := img.Pix[off : off+4 : off+4]
			seen[uint32(c[0])<<16|uint32(c[1])<<8|uint32(c[2])] = struct{}{}
			sum += float64(int(c[0]) + int(c[1]) + int(c[2]))
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return len(seen), sum / float64(n*3)
}
