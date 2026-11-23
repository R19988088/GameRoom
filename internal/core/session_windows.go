//go:build windows

// Package core runs a libretro core inside this process: the game's emulation
// is a library call here, not a program with a window of its own. The frontend
// — this app — owns the window, the picture and the input; the core only
// produces frames and consumes input.
package core

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Pixel formats a core may ask for, as retro_pixel_format.
const (
	pix0RGB1555 = 0
	pixXRGB8888 = 1
	pixRGB565   = 2
)

// Devices, as RETRO_DEVICE_*.
const (
	DeviceJoypad   = 1
	DeviceMouse    = 2
	DeviceKeyboard = 3
	DevicePointer  = 6
)

// Session is one running game.
type Session struct {
	dll uintptr

	setEnv, setVideo, setAudio, setAudioBatch, setInputPoll, setInputState uintptr
	init, loadGame, reset, run, unload, deinit, getAVInfo, getSystemInfo   uintptr
	// setPort plugs a controller into a port: a core that reads the device
	// type leaves its input uninitialised until one is announced.
	setPort uintptr

	keys  atomic.Value // map[int]bool, RETROK codes held down
	pads  atomic.Value // map[int]bool, RETRO_DEVICE_ID_JOYPAD ids held down
	mouse atomic.Value // MouseState, the pointer as a core reads it

	mu     sync.RWMutex
	frame  *image.RGBA
	frames int
	w, h   int
	format int

	// A core that draws with OpenGL fills a framebuffer of ours instead of the
	// software one; the picture is read back from there every frame.
	hw     hwRenderCallback
	hwOn   bool
	hwBL   bool
	vkOn   bool
	// A core such as PCSX2 asks for a GL context for its window handling but
	// renders the picture itself and hands the finished frame to the video
	// callback. Once that happens the frontend has no business touching GL in
	// the run loop: those calls land on a thread without a context and fault.
	softFrames atomic.Bool
	fbAsked    atomic.Bool
	cbNanos    atomic.Int64
	hwFail     error

	// A core that presents after the frame callback has the picture ready only
	// once Run() returns, so the readback can be taken there instead.
	pendOn    bool
	pendW     int
	pendH     int
	pendPitch int
	pendBL    bool

	closed int32
	quit   chan struct{}
	done   chan struct{}

	onFrame atomic.Value // func()
	fps     float64
	sample  float64 // the rate the core really plays at, as it reports it
	aspect  float32
	audio   *AudioPipe

	sndOnce sync.Once
	sndDone chan struct{}
}

// Aspect is how wide the picture is to be shown against its height, which on
// an old screen is not the shape of the frame: a 320×200 DOS picture belongs on
// a 4:3 glass.
func (s *Session) Aspect() float32 { return s.aspect }

// FPS is the rate the core says it runs at, so the window can pace it.
func (s *Session) FPS() float64 { return s.fps }

// Audio is the sound the running core makes, for the app's audio output.
func (s *Session) Audio() *AudioPipe { return s.audio }

// MouseState is where the pointer is and how it moved, as a core asks for it:
// movement in pixels since the last frame for a game that reads a mouse, and a
// place on the picture, 0..0x7fff across each side, for a game that reads a
// pointer. Buttons are pressed bits: left, right, middle, wheel.
type MouseState struct {
	DX, DY  int16
	X, Y    uint16
	Buttons uint8
}

// SetMouse hands the pointer to the core.
func (s *Session) SetMouse(m MouseState) { s.mouse.Store(m) }

var (
	workDir string
	curCore string
	keepStr [][]byte
)

// Start loads a core and its content and leaves it ready to be stepped.
func Start(corePath, romPath, systemDir string) (*Session, error) {
	installExceptionFilter()
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "start: core=%q rom=%q sys=%q\n", corePath, romPath, systemDir)
	}
	workDir = systemDir
	h, err := loadLibrary(corePath)
	if err != nil {
		return nil, err
	}
	s := &Session{dll: h, quit: make(chan struct{}), done: make(chan struct{}), sndDone: make(chan struct{}), fps: 60}
	coreName = strings.TrimSuffix(filepath.Base(corePath), "_libretro.dll")
	// The options saved beside the core are what RetroArch would have answered;
	// a core that reads them expects the same answers here, or it runs on values
	// nobody chose — for Geolith that is a palette the game never asks for and a
	// black screen. The config folder sits beside the cores folder, not in the
	// system folder a caller may have swapped for an ASCII mirror.
	loadCoreOptions(coreName, filepath.Dir(filepath.Dir(corePath)))
	// Geolith boots from a BIOS zip it finds in the system folder. A home
	// (AES) cartridge needs aes.zip; without one, every cartridge here runs
	// on the MVS board the library ships.
	if coreName == "geolith" {
		for _, bios := range []string{"aes.zip", filepath.Join("geolith", "aes.zip")} {
			if _, err := os.Stat(filepath.Join(systemDir, bios)); err == nil {
				coreVars["geolith"] = map[string]string{"geolith_system_type": "aes"}
				break
			}
		}
	}
	// A 3DO arcade disc (American Laser Games, the "duck" encrypted CD-ROMs)
	// boots only on the arcade board, so the core has to be told which BIOS to
	// use. Opera's own default picks a console BIOS and the disc then runs to a
	// black screen.
	if coreName == "opera" && isArcadeDisc(romPath, systemDir) {
		coreVars["opera"] = map[string]string{"opera_bios": "3do_arcade_saot.bin"}
	}
	// Flycast's emulator thread does not wait for its own picture unless it is
	// told to: with vblank pacing off it runs the Dreamcast as fast as the host
	// allows, and the frontend merely samples whatever frame is ready — the
	// game's clock, its music and its notes all run fast. The core is asked to
	// keep time itself, and the frontend's own wait then falls away.
	if coreName == "flycast" {
		if coreVars[coreName] == nil {
			coreVars[coreName] = map[string]string{}
		}
		coreVars[coreName]["reicast_vblank_pacing"] = "enabled"
	}
	// The library is a shelf of loose romsets (arc/dino.zip), not a software
	// list. MAME reads the folder above the zip as a *system* name and, with the
	// software lists on, tries to boot that system instead of the driver named
	// after the file — which is a running but black screen. Its own defaults are
	// for a curated software-list library; here the file name is the driver.
	if strings.HasPrefix(coreName, "mame") {
		if coreVars[coreName] == nil {
			coreVars[coreName] = map[string]string{}
		}
		for k, v := range map[string]string{
			"mame_softlists_enable":     "disabled",
			"mame_softlists_auto_media": "disabled",
			"mame_media_type":           "rom",
			"mame_boot_to_bios":         "disabled",
			"mame_boot_to_osd":          "disabled",
		} {
			coreVars[coreName][k] = v
		}
	}
	s.keys.Store(map[int]bool{})
	s.pads.Store(map[int]bool{})
	s.mouse.Store(MouseState{})
	s.onFrame.Store(func() {})

	proc := func(name string) uintptr {
		a, err := syscall.BytePtrFromString(name)
		if err != nil {
			return 0
		}
		r, _, _ := procGetProc.Call(h, uintptr(unsafe.Pointer(a)))
		return r
	}
	s.setEnv = proc("retro_set_environment")
	s.setVideo = proc("retro_set_video_refresh")
	s.setAudio = proc("retro_set_audio_sample")
	s.setAudioBatch = proc("retro_set_audio_sample_batch")
	s.setInputPoll = proc("retro_set_input_poll")
	s.setInputState = proc("retro_set_input_state")
	s.setPort = proc("retro_set_controller_port_device")
	s.init = proc("retro_init")
	s.loadGame = proc("retro_load_game")
	s.reset = proc("retro_reset")
	s.run = proc("retro_run")
	s.unload = proc("retro_unload_game")
	s.deinit = proc("retro_deinit")
	s.getAVInfo = proc("retro_get_system_av_info")
	s.getSystemInfo = proc("retro_get_system_info")
	if s.init == 0 || s.loadGame == 0 || s.run == 0 {
		return nil, fmt.Errorf("不是可用的模拟核心：%s", corePath)
	}

	active = s
	s.call(s.setEnv, syscall.NewCallback(envCallback))
	s.call(s.setVideo, syscall.NewCallback(videoCallback))
	s.call(s.setAudio, syscall.NewCallback(audioSampleCallback))
	s.call(s.setAudioBatch, syscall.NewCallback(audioBatchCallback))
	s.call(s.setInputPoll, syscall.NewCallback(func() uintptr { return 0 }))
	s.call(s.setInputState, syscall.NewCallback(inputCallback))
	s.call(s.init)

	// A core does not open a rom with fopen of its own: it goes through
	// libretro-common, which takes the path as UTF-8 and turns it into a wide
	// open, so a Chinese name reaches the file. The older cores still call
	// fopen and want the system's ANSI spelling of the same path; those are
	// tried second, and only when the first says no.
	var gi struct {
		path *byte
		data unsafe.Pointer
		size uint64
		meta *byte
	}

	var si struct {
		name, version, validExts *byte
		needFullPath             bool
		blockExtract             bool
	}
	if s.getSystemInfo != 0 {
		s.call(s.getSystemInfo, uintptr(unsafe.Pointer(&si)))
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "core info name=%q exts=%q needFullPath=%v blockExtract=%v\n",
				cString(si.name), cString(si.validExts), si.needFullPath, si.blockExtract)
		}
	}

	// A zip is a container, not a rom: a core that reads its own archives
	// (FBNeo, MAME and the arcade boards) sets blockExtract and wants the zip
	// itself, while a console core wants the rom inside it — the file when it
	// insists on a path, the bytes when it can take them in memory. Doing this
	// here is what lets a library of zipped console roms run at all.
	loadPath := romPath
	var romData []byte
	if isZip(romPath) && !si.blockExtract {
		if si.needFullPath {
			if p, ok := romFromZip(romPath, cacheRoot(workDir)); ok {
				loadPath = p
			}
		} else if b, ok := romBytesFromZip(romPath); ok {
			romData = b
		}
	} else if !si.needFullPath {
		// A core that does not ask for the path expects the frontend to hand it
		// the rom's bytes, the way RetroArch does; one that only gets a path may
		// come up with an empty board.
		if b, err := os.ReadFile(romPath); err == nil && len(b) < 256<<20 {
			romData = b
		}
	}

	attempt := func(path string, data []byte) bool {
		gi.path, gi.data, gi.size, gi.meta = nil, nil, 0, nil
		b := append([]byte(path), 0)
		keepStr = append(keepStr, b)
		gi.path = &b[0]
		if len(data) > 0 {
			keepStr = append(keepStr, data)
			gi.data = unsafe.Pointer(&data[0])
			gi.size = uint64(len(data))
		}
		ok, _, _ := syscall.SyscallN(s.loadGame, uintptr(unsafe.Pointer(&gi)))
		return ok != 0
	}

	loaded := attempt(loadPath, romData)
	if !loaded {
		// Same path, the system's spelling of it: the cores that open with
		// fopen never see the UTF-8 one as a file.
		loaded = attempt(string(ansiBytes(loadPath)), romData)
	}
	if !loaded {
		return nil, fmt.Errorf("这个核心打不开这个游戏：%s", romPath)
	}
	// The core has its framebuffer ready and asks the frontend to build its own
	// OpenGL or Vulkan objects now; until that call it draws nothing.
	if (s.hwOn || s.vkOn) && s.hw.ContextReset != 0 {
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "ctx: reset begin\n")
		}
		glUseCurrent()
		syscall.SyscallN(s.hw.ContextReset)
		// The reset is over; the context goes back so a core that starts its
		// shader threads now can share it (Dolphin does exactly that, and
		// without a share its shader compiler fails and takes the core down).
		procWglMakeCurrent.Call(0, 0)
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "ctx: reset done\n")
		}
	}
	// A core expects the frontend to reset it once the rom is in, the way
	// RetroArch does; without it some cores sit on an uninitialised board.
	// RetroArch does not reset a core it has just loaded: retro_load_game has
	// already put it at the start of the game, and a second reset makes the
	// heavier cores (PCSX2 above all) start their machine threads twice and
	// deadlock in the second one. Reset stays on the user's reset button.
	if s.reset != 0 && os.Getenv("GAMEROOM_RESET_ON_LOAD") != "" {
		s.call(s.reset)
	}
	// RetroArch announces a joypad on both ports before it steps a core;
	// without that some cores keep their input, and their front end, unset.
	if s.setPort != 0 {
		for port := uintptr(0); port < 2; port++ {
			s.call(s.setPort, port, DeviceJoypad)
		}
	}

	// retro_system_av_info is one retro_game_geometry — five words, the last of
	// them the aspect ratio — followed by retro_system_timing, two doubles. The
	// geometry must be laid out exactly that way: a struct of three floats is
	// only 12 bytes, so the timing that follows lands on the wrong offsets and
	// every core reads back as "60 fps, 44100 Hz" no matter what it really
	// plays. That is what made arcade sound drift: a 48 kHz core was drained at
	// 44.1 kHz, so a little more sound queued up every second.
	var av struct {
		geometry struct {
			baseW, baseH, maxW, maxH uint32
			aspect                   float32
		}
		timing struct{ fps, sample float64 }
	}
	if s.getAVInfo != 0 {
		s.call(s.getAVInfo, uintptr(unsafe.Pointer(&av)))
		if av.timing.fps > 1 && av.timing.fps < 240 {
			s.fps = av.timing.fps
		}
		if av.timing.sample > 8000 && av.timing.sample < 192000 {
			s.sample = av.timing.sample
		}
		if av.geometry.aspect > 0.4 && av.geometry.aspect < 5 {
			s.aspect = av.geometry.aspect
		}
	}
	s.audio = NewAudioPipe(int(s.sample))
	// Loading is done, and the context is handed back before the first frame.
	// A core that draws through threads of its own (Dolphin) shares our objects
	// while loading; it can only do that while the context is current nowhere,
	// and holding it on this thread for the whole run would lock those threads
	// out of their own picture.
	if cur, _, _ := procWglGetCurrentContext.Call(); cur != 0 {
		procWglMakeCurrent.Call(0, 0)
	}
	return s, nil
}

var active *Session

// A frame has to land within a hundredth of a second of its slot, and the
// system timer wakes a sleeper late by up to a whole tick. Asking for the
// finest tick and spinning out the last millisecond keeps a 60 Hz game at 60.
var procTimeBeginPeriod = syscall.NewLazyDLL("winmm.dll").NewProc("timeBeginPeriod")

func waitUntil(t time.Time) {
	// Occupied until the slot: time.Sleep on this machine wakes late by more
	// than the frame budget allows, so the last stretch is held here.
	for time.Now().Before(t) {
	}
}

// Run steps the core until Close. Frames are published as they come.
func (s *Session) Run() {
	defer close(s.done)
	procTimeBeginPeriod.Call(1)
	// OpenGL is bound to a thread, and the cores call it straight from here.
	// A goroutine that migrates to another thread leaves its context behind,
	// so this one stays where it is for the whole game.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	glUseCurrent()
	period := time.Duration(float64(time.Second) / s.fps)
	// Wake-up jitter is trimmed frame by frame: the overshoot measured on the
	// last frames is taken off the next slot, so the rate settles onto the
	// core's own rate instead of staying a fixed few percent slow.
	adj := time.Duration(0)
	// The speed is measured rather than assumed: a core that waits for its own
	// vblank and a frontend that also waits add up to half speed, and only a
	// measurement tells the two apart.
	coreSum, keysSum, prepSum, waitSum, uiSum, frameCount := time.Duration(0), time.Duration(0), time.Duration(0), time.Duration(0), time.Duration(0), 0
	lastT, lastN := time.Now(), 0
	for {
		select {
		case <-s.quit:
			return
		default:
		}
		frameStart := time.Now()
		if os.Getenv("GAMEROOM_DEBUG") != "" && s.frames < 40 {
			fmt.Fprintf(os.Stderr, "run: enter frame %d fps=%v\n", s.frames, s.fps)
		}
		t0 := time.Now()
		prep := time.Duration(0)
		if s.hwOn && s.fbAsked.Load() && !s.softFrames.Load() {
			// A core that asked for our framebuffer draws into it, so it is bound
			// again for every frame.
			prep = time.Since(t0)
			glBindDrawFBO()
			prep = time.Since(t0) - prep
		}
		// The keyboard is read right before the core steps, not a frame earlier:
		// one press then reaches one emulated frame, not two.
		tk := time.Now()
		s.SetKeys(pollKeys())
		keysDur := time.Since(tk)
		s.call(s.run)
		runDur := time.Since(t0)
		if s.pendOn {
			// The core finished its whole frame, present included, by now.
			if buf := glReadFrame(s.pendW, s.pendH, s.pendPitch, s.pendBL); buf != nil {
				s.mu.Lock()
				if s.frame == nil || s.w != s.pendW || s.h != s.pendH {
					s.frame = image.NewRGBA(image.Rect(0, 0, s.pendW, s.pendH))
					s.w, s.h = s.pendW, s.pendH
				}
				s.frames++
				copy(s.frame.Pix, buf)
				s.mu.Unlock()
			}
		}
		if os.Getenv("GAMEROOM_DEBUG") != "" && s.frames < 40 {
			fmt.Fprintf(os.Stderr, "run: frame %d done after %v\n", s.frames, time.Since(t0))
		}
		coreSum += runDur
		keysSum += keysDur
		prepSum += prep
		frameCount++
		if os.Getenv("GAMEROOM_DEBUG") != "" && frameCount%120 == 0 && frameCount >= 120 {
			winN := frameCount - lastN
			winT := time.Since(lastT)
			fmt.Fprintf(os.Stderr, "speed: fps=%.2f target=%.2f win=%.2fms prep=%.2fms core=%.2fms cb=%.2fms key=%.2fms wait=%.2fms ui=%.2fms frames=%d\n",
				float64(winN)/winT.Seconds(), s.fps, float64(winT.Microseconds())/float64(winN)/1000,
				float64(prepSum.Microseconds())/float64(frameCount)/1000,
				float64(coreSum.Microseconds())/float64(frameCount)/1000,
				float64(s.cbNanos.Load())/float64(frameCount)/1e6,
				float64(keysSum.Microseconds())/float64(frameCount)/1000,
				float64(waitSum.Microseconds())/float64(frameCount)/1000,
				float64(uiSum.Microseconds())/float64(frameCount)/1000, frameCount)
			lastT, lastN = time.Now(), frameCount
		}
		tu := time.Now()
		if f, ok := s.onFrame.Load().(func()); ok && f != nil {
			f()
		}
		uiSum += time.Since(tu)
		// Each frame gets one period counted from its own start: work that fits
		// waits out the rest, work that does not simply runs on without an
		// added wait. That makes the long-run rate exactly the core's rate.
		if target := frameStart.Add(period - adj); time.Until(target) > 0 {
			tw := time.Now()
			waitUntil(target)
			waitSum += time.Since(tw)
			over := time.Since(frameStart) - period
			if over > 2*time.Millisecond {
				over = 2 * time.Millisecond
			} else if over < -2*time.Millisecond {
				over = -2 * time.Millisecond
			}
			adj += (over - adj) / 8
		}
	}
}

// SetKeys takes the keyboard keys held down, as retrok codes. A change of
// state is logged so a menu that moves twice can be told apart: two presses
// here, or one press the core read on two frames.
func (s *Session) SetKeys(keys map[int]bool) {
	if os.Getenv("GAMEROOM_INPUT_LOG") != "" {
		old, _ := s.keys.Load().(map[int]bool)
		for k, v := range keys {
			if v && !old[k] {
				fmt.Fprintf(os.Stderr, "input: frame=%d down=%d\n", s.Frames(), k)
			}
		}
	}
	s.keys.Store(keys)
}

// SetPad takes the gamepad's buttons held down, as RETRO_DEVICE_ID_JOYPAD ids:
// which physical button drives which id is the frontend's business, the core
// only reads the ids it expects.
func (s *Session) SetPad(pads map[int]bool) { s.pads.Store(pads) }

// Frame is the last picture the core drew, owned by the session.
func (s *Session) Frame() *image.RGBA {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.frame
}

// Frames is how many pictures the core has drawn, so the app can tell a new
// frame from the one it already shows.
func (s *Session) Frames() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.frames
}

// PixelFormat is the buffer format the core asked for, for diagnostics.
func (s *Session) PixelFormat() int { return s.format }

// OnFrame sets what runs after each stepped frame, on the core's own clock.
func (s *Session) OnFrame(f func()) { s.onFrame.Store(f) }

// Close ends the game and lets the core free what it held.
func (s *Session) Close() {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return
	}
	close(s.quit)
	<-s.done
	if s.audio != nil {
		s.audio.close()
	}
	s.call(s.unload)
	s.call(s.deinit)
	// The library stays loaded: a core that is unloaded and loaded again in the
	// same process does not always come back, and a game may well be followed
	// by another one on the same core.
	active = nil
}

func (s *Session) call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

var (
	procGetAsyncKey = syscall.NewLazyDLL("user32.dll").NewProc("GetAsyncKeyState")
)

// keyMap is the keyboard a DOS game expects: letters, digits, the punctuation
// on a US layout, the arrows, the function keys, and the modifiers a game may
// read for shortcuts. Everything else is left out on purpose — a launcher does
// not need to hand the core keys no game uses.
var keyMap = map[int]int{}

func init() {
	for i := 0; i < 26; i++ { // VK 'A'..'Z' to retrok 'a'..'z'
		keyMap[0x41+i] = 97 + i
	}
	for i := 0; i < 10; i++ { // VK '0'..'9'
		keyMap[0x30+i] = 48 + i
	}
	for vk, rk := range map[int]int{
		0x08: 8, 0x09: 9, 0x0D: 13, 0x1B: 27, 0x20: 32,
		0x25: 276, 0x26: 273, 0x27: 275, 0x28: 274,
		0x2E: 127, 0x24: 278, 0x23: 279, 0x21: 280, 0x22: 281, 0x2D: 277,
		0xBA: 59, 0xBB: 61, 0xBC: 44, 0xBD: 45, 0xBE: 46, 0xBF: 47,
		0xC0: 96, 0xDB: 91, 0xDC: 92, 0xDD: 93, 0xDE: 39,
		0x10: 304, 0x11: 306, 0x12: 308, // shift, ctrl, alt
	} {
		keyMap[vk] = rk
	}
	for i := 0; i < 12; i++ { // VK_F1..F12
		keyMap[0x70+i] = 282 + i
	}
}

// pollKeys is the keyboard as the core sees it this frame. It is read from the
// system rather than from the app's own events on purpose: a game window has
// the keyboard whether or not the page is looking at it.
func pollKeys() map[int]bool {
	held := make(map[int]bool, 8)
	for vk, rk := range keyMap {
		if st, _, _ := procGetAsyncKey.Call(uintptr(vk)); st&0x8000 != 0 {
			held[rk] = true
		}
	}
	return held
}

var (
	procLoadLibrary         = syscall.NewLazyDLL("kernel32.dll").NewProc("LoadLibraryW")
	procGetProc             = syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcAddress")
	procSetDllDir           = syscall.NewLazyDLL("kernel32.dll").NewProc("SetDllDirectoryW")
	procWideCharToMultiByte = syscall.NewLazyDLL("kernel32.dll").NewProc("WideCharToMultiByte")
	dllDirSet               bool
)

func loadLibrary(path string) (uintptr, error) {
	// A core is a library with libraries of its own, and those sit beside the
	// emulator, not in the cores folder: without this the DLL search would not
	// find them and the core would refuse to load at all.
	if !dllDirSet {
		if p, err := syscall.UTF16PtrFromString(filepath.Dir(filepath.Dir(path))); err == nil {
			procSetDllDir.Call(uintptr(unsafe.Pointer(p)))
		}
		dllDirSet = true
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, _, _ := procLoadLibrary.Call(uintptr(unsafe.Pointer(p)))
	if h == 0 {
		return 0, fmt.Errorf("加载核心失败：%s", path)
	}
	return h, nil
}

// cstrp reads a C string for the debug log only.
func cstrp(p *byte) string {
	if p == nil {
		return ""
	}
	var b []byte
	for i := 0; i < 256; i++ {
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func envCallback(cmd, data uintptr) uintptr {
	if active == nil {
		return 0
	}
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "env: cmd=%d(0x%x) data=0x%x\n", cmd, cmd, data)
	}
	// GET_HW_RENDER_INTERFACE. It arrives as 41|0x10000, which the masked
	// switch below cannot tell apart from plain 41, so it is answered first.
	if cmd == 0x10029 {
		if active.vkOn && data != 0 {
			*(*uintptr)(ptr(data)) = uintptr(unsafe.Pointer(&vkIface))
			return 1
		}
		return 0
	}
	switch int(cmd) & 0xffff {
	case 3: // GET_CAN_DUPE
		if data != 0 {
			*(*bool)(ptr(data)) = true
		}
		return 1
	case 10: // SET_PIXEL_FORMAT
		if data != 0 {
			active.format = int(*(*uint32)(ptr(data)))
		}
		return 1
	case 9, 30, 31: // system, core assets, save directory
		if data != 0 {
			putStr(ptr(data), workDir)
		}
		return 1
	case 14: // SET_HW_RENDER: the core wants to draw with OpenGL of its own
		if data == 0 {
			return 0
		}
		cb := (*hwRenderCallback)(ptr(data))
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "hwrender: ask type=%d v=%d.%d\n", cb.ContextType, cb.VersionMajor, cb.VersionMinor)
		}
		if cb.ContextType == hwContextVulkan && os.Getenv("GAMEROOM_VK") != "" {
			// Vulkan cores do not draw into a framebuffer of ours: they take a
			// device and queue from the frontend and hand finished images back.
			if err := vkPrepare(); err != nil {
				if os.Getenv("GAMEROOM_DEBUG") != "" {
					fmt.Fprintf(os.Stderr, "vk: %v\n", err)
				}
				active.hwFail = err
				return 0
			}
			active.vkOn = true
			active.hwOn = false
			cb.GetFramebuffer = 0
			cb.GetProcAddress = 0
			// Kept so the context reset can be called once the rom is in, the
			// order RetroArch uses: a Vulkan core picks its renderer while
			// loading, and only then builds its device objects.
			active.hw = *cb
			active.hwBL = cb.BottomLeftOrigin != 0
			return 1
		}
		switch cb.ContextType {
		case hwContextOpenGL, hwContextOpenGLCore:
		default:
			return 0 // Direct3D: this app has only OpenGL and Vulkan to give
		}
		active.hw = *cb
		active.hwBL = cb.BottomLeftOrigin != 0
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "hwrender: type=%d v=%d.%d depth=%v stencil=%v bl=%v\n",
				cb.ContextType, cb.VersionMajor, cb.VersionMinor, cb.Depth != 0, cb.Stencil != 0, cb.BottomLeftOrigin != 0)
		}
		cb.GetFramebuffer = syscall.NewCallback(glCurrentFramebuffer)
		cb.GetProcAddress = syscall.NewCallback(glProcName)
		if err := glPrepare(cb.ContextType, cb.VersionMajor, cb.VersionMinor, cb.Depth != 0, cb.Stencil != 0); err != nil {
			active.hwFail = err
			return 0
		}
		active.hwOn = true
		return 1
	case 15: // GET_VARIABLE: the few options a core must be told about
		if data != 0 {
			opt := (*retroVariable)(ptr(data))
			v, ok := coreVar(opt.key)
			if os.Getenv("GAMEROOM_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "var: %q -> %q (%v)\n", cstrp(opt.key), v, ok)
			}
			if ok {
				opt.value = &keepStrAdd(v)[0]
				return 1
			}
		}
		return 0
	case 17: // GET_VARIABLE_UPDATE
		// No options are offered, so there is never an update. The pointer some
		// cores hand over here is not always a bool of theirs, so it is not
		// written through: answering "nothing changed" is all a core needs.
		return 0
	case 27: // GET_LOG_INTERFACE
		// A core asks for a way to say something. It is given one that keeps
		// quiet, because the app has no log of its own to write to; what matters
		// is that the pointer is not null, which some cores would call blind.
		if data != 0 {
			*(*uintptr)(ptr(data)) = logCallback()
		}
		return 1
	case 19: // GET_LIBRETRO_PATH
		if data != 0 {
			putStr(ptr(data), curCore)
		}
		return 1
	case 45: // GET_VFS_INTERFACE (experimental, so 45|0x10000 on the wire)
		// A core that reads its own files asks for a file system instead of
		// opening them with fopen. Refusing leaves it with nothing to read: it
		// either refuses the rom or boots to a black screen. The version is the
		// one the core asked for, or the newest this frontend speaks when the
		// core asks for a probe.
		// Only Geolith's cartridge reader is the format below. Every other core
		// asks for its own version of this interface, and answering with v1's
		// layout makes PlayStation cores die on the first read: they are told
		// "no" and keep reading their discs and roms with ordinary files.
		if coreName != "geolith" || os.Getenv("GAMEROOM_NOVFS") != "" {
			return 0
		}
		if data != 0 {
			info := (*vfsInfo)(ptr(data))
			asked := info.version
			want := asked
			if want == 0 || want > vfsVersion {
				want = vfsVersion
			}
			info.version = want
			info.iface = initVFS()
			if os.Getenv("GAMEROOM_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "vfs: offered version %d (core asked %d)\n", info.version, asked)
			}
			return 1
		}
		return 0
	case 16, 52, 53, 82, 83: // SET_VARIABLES, SET_CORE_OPTIONS*, all keeping defaults
		// The core is describing what it offers. Its defaults are kept: a core
		// builds its own switches from the answers to GET_VARIABLE, and a
		// "not found" for every key leaves MAME with empty switches of its own
		// making ("unknown option: -<UNADORNED2>") and the game unloadable.
		if data != 0 {
			keepOptions(int(cmd)&0xffff, data)
		}
		return 1
	case 35, 65: // SET_CONTROLLER_INFO, SET_CONTENT_INFO_OVERRIDE
		// Nothing is asked for here, and turning it down is what some cores
		// crash on, because they keep walking the array they just handed over.
		return 1
	}
	return 0
}

// retroCoreOptionValue and retroCoreOptionDefinition are libretro's option
// entries: a key with the values it takes, and the one it starts on.
type retroCoreOptionValue struct {
	Value, Label *byte
}

type retroCoreOptionDefinition struct {
	Key, Desc, Info *byte
	Values          [128]retroCoreOptionValue
	Default         *byte
}

type retroCoreOptionsIntl struct {
	US    *retroCoreOptionDefinition
	Local *retroCoreOptionDefinition
}

// keepOptions reads the list a core hands over and remembers each key's
// default, so GET_VARIABLE can hand the core a usable answer.
func keepOptions(cmd int, data uintptr) {
	// Only the MAME cores build their command line out of these answers and
	// cannot start a game without them. Other cores were already running on
	// "not found" and their saved option files; handing them defaults that
	// nobody chose changes what they load, and FBNeo crashed on it.
	if !strings.HasPrefix(coreName, "mame") {
		return
	}
	if coreVars[coreName] == nil {
		coreVars[coreName] = map[string]string{}
	}
	vars := coreVars[coreName]
	keep := func(key, value string) {
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "option: %s = %q\n", key, value)
		}
		if key == "" || value == "" {
			return
		}
		if _, ok := vars[key]; ok {
			return // the saved option file and the launcher's own choices win
		}
		vars[key] = value
	}
	switch cmd {
	case 16: // retro_variable list: value is "Label|value;default"
		p := (*byte)(ptr(data))
		for p != nil && readable(uintptr(unsafe.Pointer(p))) {
			v := (*retroVariable)(unsafe.Pointer(p))
			if v.key == nil || !readable(uintptr(unsafe.Pointer(v.key))) {
				break
			}
			keep(cString(v.key), optionDefault(cString(v.value)))
			p = (*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(v)) + unsafe.Sizeof(retroVariable{})))
		}
	case 52: // SET_CORE_OPTIONS_INTL: { us list, localised list }
		intl := (*retroCoreOptionsIntl)(ptr(data))
		first := intl.US
		if first == nil {
			first = intl.Local
		}
		base := uintptr(unsafe.Pointer(first))
		size := unsafe.Sizeof(retroCoreOptionDefinition{})
		for i := 0; i < 64 && base != 0; i++ {
			at := base + uintptr(i)*size
			if !readable(at) {
				break
			}
			def := (*retroCoreOptionDefinition)(unsafe.Pointer(at))
			if def.Key == nil || !readable(uintptr(unsafe.Pointer(def.Key))) {
				break
			}
			keep(cString(def.Key), cString(def.Default))
		}
	}
}

// optionDefault digs the starting value out of a legacy variable's
// "Label|value1;Label2|value2" style description.
func optionDefault(desc string) string {
	s := desc
	if i := strings.LastIndexByte(desc, ';'); i >= 0 {
		s = desc[i+1:]
	}
	if i := strings.IndexByte(s, '|'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func putStr(dst unsafe.Pointer, s string) {
	// The folders a core is handed go through the same libretro-common code as
	// the rom path, so they are spelled the same way: UTF-8, which that code
	// turns into a wide open. The bytes are kept because the core holds on to
	// the pointer rather than copying the string.
	b := append([]byte(s), 0)
	keepStr = append(keepStr, b)
	*(*unsafe.Pointer)(dst) = unsafe.Pointer(&b[0])
}

// retroVariable is the question a core asks in GET_VARIABLE: which option, and
// where to put its value.
type retroVariable struct {
	key   *byte
	value *byte
}

// coreVars are the options a core must be told about, by core and key. A core
// only asks for what it has a default for; answering the key we care about and
// staying quiet about the rest is how the frontend gets to choose the machine
// instead of the core's own guess.
var coreVars = map[string]map[string]string{
	// Geolith boots from the BIOS zip it finds: the library ships the MVS one
	// (neogeo.zip) and has no AES zip, so every Neo Geo cartridge here is run
	// as an MVS board. Without this the core asks for AES and refuses to load.
	"geolith": {"geolith_system_type": "mvs"},
}

var coreName string

// loadCoreOptions reads the core's saved options — config\<Core>\<Core>.opt —
// into coreVars, so the values a core asks for are the ones written in the
// file rather than whatever default the core would have used.
func loadCoreOptions(name, raDir string) {
	if name == "" || raDir == "" {
		return
	}
	path := filepath.Join(raDir, "config", name, name+".opt")
	b, err := os.ReadFile(path)
	if err != nil {
		// The folder is not always named after the core: PlayStation 2 keeps
		// its settings under LRPS2, and the keys inside still start with
		// "pcsx2_". Whichever folder speaks the core's own key prefix is the
		// one that belongs to it.
		b = findOptionsByKeys(raDir, name)
		if b == nil {
			return
		}
	}
	vars := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"`)
		if k != "" {
			vars[k] = v
		}
	}
	// The options the launcher decides itself stay on top of the file.
	for k, v := range coreVars[name] {
		vars[k] = v
	}
	// PCSX2's hardware renderer wants the GS to draw OpenGL on its own thread,
	// which this frontend cannot hand to it yet: the shaders fail to compile
	// and the core then faults. Its software renderer runs every PS2 game and
	// hands back finished frames, so it is the default until the GL path is
	// settled; GAMEROOM_PCSX2_GL=1 switches the experiment back on.
	if strings.HasPrefix(name, "pcsx2") && os.Getenv("GAMEROOM_PCSX2_GL") == "" {
		vars["pcsx2_renderer"] = "Software"
	}
	coreVars[name] = vars
}

// findOptionsByKeys looks in config\* for the .opt file that speaks this core's
// option keys, which is how a core keeps its settings under a folder that has
// nothing to do with its library name.
func findOptionsByKeys(raDir, name string) []byte {
	root := filepath.Join(raDir, "config")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var best []byte
	bestScore := 2
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name(), e.Name()+".opt")
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		score := 0
		for _, line := range strings.Split(string(data), "\n") {
			k, _, ok := strings.Cut(line, " = ")
			if ok && strings.HasPrefix(strings.TrimSpace(k), name+"_") {
				score++
			}
		}
		if score > bestScore {
			bestScore, best = score, data
		}
	}
	return best
}

// isArcadeDisc reports whether the image in romPath is an American Laser Games
// arcade CD: its header names the disc as CD-ROM and its file table as "duck".
// Those discs need the arcade BIOS, and one is only used when it is present.
func isArcadeDisc(romPath, systemDir string) bool {
	if _, err := os.Stat(filepath.Join(systemDir, "3do_arcade_saot.bin")); err != nil {
		return false
	}
	f, err := os.Open(romPath)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 1024)
	n, _ := f.Read(head)
	return strings.Contains(string(head[:n]), "CD-ROM") && strings.Contains(string(head[:n]), "duck")
}

func coreVar(key *byte) (string, bool) {
	if key == nil || coreName == "" {
		return "", false
	}
	name := cString(key)
	// Overrides from the environment make it possible to try a core option
	// without rebuilding the launcher: "key=value;key=value".
	if env := os.Getenv("GAMEROOM_CORE_VAR"); env != "" {
		for _, pair := range strings.Split(env, ";") {
			if k, v, ok := strings.Cut(pair, "="); ok && strings.EqualFold(strings.TrimSpace(k), name) {
				return strings.TrimSpace(v), true
			}
		}
	}
	vars, ok := coreVars[coreName]
	if !ok {
		return "", false
	}
	v, ok := vars[name]
	return v, ok
}

// cString reads a zero-ended C string, with a cap so a bad pointer cannot hang
// the frame it was asked in, and a guard so a pointer the core never meant as a
// string (a bare number in a log format) cannot take the whole app down.
func cString(p *byte) string {
	if p == nil || !readable(uintptr(unsafe.Pointer(p))) {
		return ""
	}
	var b []byte
	for i := 0; i < 256; i++ {
		if !readable(uintptr(unsafe.Pointer(p)) + uintptr(i)) {
			break
		}
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

// memoryBasicInformation is what VirtualQuery fills in. The layout is Windows'
// own, which has a two-byte partition id and the region size ahead of the
// state: getting it wrong reads the protection of the wrong word.
type memoryBasicInformation struct {
	base       uintptr
	allocation uintptr
	protect    uint32
	partition  uint16
	_          uint16
	regionSize uintptr
	state      uint32
	page       uint32
	type_      uint32
	_          uint32
}

var procVirtualQuery = syscall.NewLazyDLL("kernel32.dll").NewProc("VirtualQuery")

// readable reports whether an address lies in memory this process may read. A
// core that logs "%s" with something that is not a string would otherwise be
// followed into unmapped memory.
func readable(addr uintptr) bool {
	var mbi memoryBasicInformation
	r, _, _ := procVirtualQuery.Call(addr, uintptr(unsafe.Pointer(&mbi)), unsafe.Sizeof(mbi))
	if r == 0 || mbi.state != 0x1000 { // MEM_COMMIT
		return false
	}
	const (
		pageReadOnly    = 0x02
		pageReadWrite   = 0x04
		pageWriteCopy   = 0x08
		pageExecuteRead = 0x20
		pageExecuteRw   = 0x40
		pageExecuteCopy = 0x80
	)
	return mbi.page&(pageReadOnly|pageReadWrite|pageWriteCopy|pageExecuteRead|pageExecuteRw|pageExecuteCopy) != 0
}

// coreLogText is the one-line log a core asked to write: its format with the
// few arguments a message carries filled in, so a refusal says which path or
// which file it could not open instead of only "failed".
func coreLogText(msg uintptr, args ...uintptr) string {
	s := cString((*byte)(ptr(msg)))
	var out []byte
	arg := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+1 < len(s) {
			switch s[i+1] {
			case '%':
				out = append(out, '%')
				i++
				continue
			case 's', 'd', 'u', 'x', 'X':
				if arg < len(args) {
					a := args[arg]
					arg++
					if s[i+1] == 's' {
						out = append(out, cString((*byte)(ptr(a)))...)
					} else {
						out = append(out, fmt.Sprint(a)...)
					}
				}
				i++
				continue
			}
		}
		out = append(out, s[i])
	}
	return string(out)
}

// keepStrAdd keeps a string alive for the core that was handed its pointer.
func keepStrAdd(s string) []byte {
	b := append([]byte(s), 0)
	keepStr = append(keepStr, b)
	return b
}

// ansiBytes is a path as the system's ANSI code page spells it, for the older
// cores that still open with fopen: UTF-8 to UTF-16 to ACP.
func ansiBytes(s string) []byte {
	u16, err := syscall.UTF16FromString(s)
	if err != nil || len(u16) == 0 {
		return append([]byte(s), 0)
	}
	n, _, _ := procWideCharToMultiByte.Call(0 /* CP_ACP */, 0,
		uintptr(unsafe.Pointer(&u16[0])), uintptr(len(u16)-1), 0, 0, 0, 0)
	if int32(n) <= 0 {
		return append([]byte(s), 0)
	}
	out := make([]byte, int32(n)+1)
	procWideCharToMultiByte.Call(0, 0,
		uintptr(unsafe.Pointer(&u16[0])), uintptr(len(u16)-1),
		uintptr(unsafe.Pointer(&out[0])), uintptr(int32(n)), 0, 0)
	return out
}

// logCallback is the one function pointer a core may call to say something.
// Its message may come with arguments, so the arguments are left alone and the
// text is not formatted: a core that logs is not a core that must be read.
var logFn uintptr

func logCallback() uintptr {
	if logFn == 0 {
		logFn = syscall.NewCallback(func(level, message, a1, a2, a3 uintptr) uintptr {
			// A core logs from its own threads and may pass anything at all in
			// the argument slots; a log line is never worth losing the app over.
			defer func() { recover() }()
			if os.Getenv("GAMEROOM_DEBUG") != "" && message != 0 {
				head := ""
				if readable(message) {
					for i := 0; i < 24; i++ {
						if !readable(message + uintptr(i)) {
							break
						}
						head += fmt.Sprintf("%02x ", *(*byte)(ptr(message + uintptr(i))))
					}
				}
				fmt.Fprintf(os.Stderr, "core log level=%d msg=0x%x bytes=%s text=%q\n",
					level, message, head, coreLogText(message, a1, a2, a3))
			}
			return 0
		})
	}
	return logFn
}

func ptr(p uintptr) unsafe.Pointer { return unsafe.Pointer(p) }

func inputCallback(port, device, index, id uintptr) uintptr {
	s := active
	if s == nil {
		return 0
	}
	switch int(device) {
	case DeviceJoypad:
		pads, _ := s.pads.Load().(map[int]bool)
		if pads != nil && pads[int(id)] {
			return 1
		}
		return 0
	case DeviceMouse:
		m, _ := s.mouse.Load().(MouseState)
		switch int(id) {
		case 0:
			return uintptr(int16ToUint(m.DX))
		case 1:
			return uintptr(int16ToUint(m.DY))
		case 2:
			return bit(m.Buttons, 0)
		case 3:
			return bit(m.Buttons, 1)
		case 4:
			return bit(m.Buttons, 2)
		case 5, 6:
			return bit(m.Buttons, 3) // the wheel, for menus
		}
		return 0
	case DevicePointer:
		m, _ := s.mouse.Load().(MouseState)
		switch int(id) {
		case 0:
			return uintptr(m.X)
		case 1:
			return uintptr(m.Y)
		case 2:
			if m.Buttons&1 != 0 {
				return 1
			}
		}
		return 0
	}
	if int(device) != DeviceKeyboard {
		return 0
	}
	keys, _ := s.keys.Load().(map[int]bool)
	if keys != nil && keys[int(id)] {
		return 1
	}
	return 0
}

// int16ToUint hands a signed movement back as the unsigned word a core reads:
// negative movement is a very large number, and the core knows it.
func int16ToUint(v int16) uint16 {
	return uint16(v)
}

func bit(v uint8, n uint) uintptr {
	if v&(1<<n) != 0 {
		return 1
	}
	return 0
}

// audioBatchCallback takes the sound the core made this frame and hands it to
// the pipe the app's output plays from. The core counts on the batch being
// taken, so it is taken whole and the frame count is given back.
func audioBatchCallback(data, frames uintptr) uintptr {
	s := active
	if s == nil || s.audio == nil || data == 0 || frames == 0 {
		return frames
	}
	s.audio.push(unsafe.Slice((*int16)(ptr(data)), int(frames)*2))
	return frames
}

// audioSampleCallback is the one-sample form of the same call, used by cores
// that hand out a single frame at a time.
func audioSampleCallback(left, right uintptr) uintptr {
	s := active
	if s == nil || s.audio == nil {
		return 0
	}
	s.audio.push([]int16{int16(left), int16(right)})
	return 0
}

// videoCallback copies the core's picture into a frame this app draws. The
// core hands out a pointer that is only valid during the call, so the pixels
// are copied out here and the session keeps the result.
func videoCallback(data, w, h, pitch uintptr) uintptr {
	s := active
	if s == nil || w == 0 || h == 0 {
		return 0
	}
	if os.Getenv("GAMEROOM_DEBUG") != "" && s.frames < 3 {
		fmt.Fprintf(os.Stderr, "video: cb data=%v %dx%d pitch=%d\n", data != 0, w, h, pitch)
	}
	if s.vkOn {
		width, height := int(w), int(h)
		buf := vkReadFrame(width, height)
		if buf == nil {
			return 0
		}
		s.mu.Lock()
		if s.frame == nil || s.w != width || s.h != height {
			s.frame = image.NewRGBA(image.Rect(0, 0, width, height))
			s.w, s.h = width, height
		}
		s.frames++
		copy(s.frame.Pix, buf)
		s.mu.Unlock()
		return 0
	}
	if s.hwOn {
		width, height := int(w), int(h)
		if os.Getenv("GAMEROOM_READ_AFTER") != "" {
			s.pendOn = true
			s.pendW, s.pendH, s.pendPitch, s.pendBL = width, height, int(pitch), s.hwBL
			return 0
		}
		tr := time.Now()
		buf := glReadFrame(width, height, int(pitch), s.hwBL)
		s.cbNanos.Add(int64(time.Since(tr)))
		if buf == nil {
			return 0
		}
		s.mu.Lock()
		if s.frame == nil || s.w != width || s.h != height {
			s.frame = image.NewRGBA(image.Rect(0, 0, width, height))
			s.w, s.h = width, height
		}
		s.frames++
		copy(s.frame.Pix, buf)
		if os.Getenv("GAMEROOM_DEBUG") != "" && s.frames%120 == 1 {
			sum, nz := 0, 0
			for i := 0; i+3 < len(buf); i += 4 {
				v := int(buf[i]) + int(buf[i+1]) + int(buf[i+2])
				sum += v
				if v > 24 {
					nz++
				}
			}
			fmt.Fprintf(os.Stderr, "hw: frame %d %dx%d mean=%.1f lit=%d ctx=%d\n", s.frames, width, height, float64(sum)/float64(len(buf)/4*3), nz, glContextID())
		}
		s.mu.Unlock()
		return 0
	}
	if data == 0 {
		return 0
	}
	s.softFrames.Store(true)
	width, height, row := int(w), int(h), int(pitch)
	if row <= 0 {
		// A core may leave the stride out, meaning rows are simply packed.
		row = width * 4
		if s.format != pixXRGB8888 {
			row = width * 2
		}
	}
	s.mu.Lock()
	if s.frame == nil || s.w != width || s.h != height {
		s.frame = image.NewRGBA(image.Rect(0, 0, width, height))
		s.w, s.h = width, height
	}
	s.frames++
	if os.Getenv("GAMEROOM_DEBUG") != "" && s.frames%120 == 1 {
		head := ""
		for i := 0; i < 16; i++ {
			head += fmt.Sprintf("%02x ", *(*byte)(unsafe.Add(ptr(data), i)))
		}
		fmt.Fprintf(os.Stderr, "video data=0x%x %dx%d pitch=%d fmt=%d head=%s\n", data, width, height, row, s.format, head)
	}
	dst := s.frame.Pix
	stride := s.frame.Stride
	s.mu.Unlock()

	src := ptr(data)
	switch s.format {
	case pixXRGB8888:
		for y := 0; y < height; y++ {
			line := unsafe.Slice((*byte)(unsafe.Add(src, y*row)), width*4)
			out := dst[y*stride : y*stride+width*4]
			for x := 0; x < width; x++ {
				p := line[x*4:]
				out[x*4+0], out[x*4+1], out[x*4+2], out[x*4+3] = p[2], p[1], p[0], 255
			}
		}
	case pixRGB565:
		for y := 0; y < height; y++ {
			line := unsafe.Slice((*uint16)(unsafe.Pointer(unsafe.Add(src, y*row))), width)
			out := dst[y*stride : y*stride+width*4]
			for x, v := range line {
				out[x*4+0] = byte((v >> 11) << 3)
				out[x*4+1] = byte(((v >> 5) & 0x3f) << 2)
				out[x*4+2] = byte((v & 0x1f) << 3)
				out[x*4+3] = 255
			}
		}
	default:
		for y := 0; y < height; y++ {
			line := unsafe.Slice((*uint16)(unsafe.Pointer(unsafe.Add(src, y*row))), width)
			out := dst[y*stride : y*stride+width*4]
			for x, v := range line {
				out[x*4+0] = byte(((v >> 10) & 0x1f) << 3)
				out[x*4+1] = byte(((v >> 5) & 0x1f) << 3)
				out[x*4+2] = byte((v & 0x1f) << 3)
				out[x*4+3] = 255
			}
		}
	}
	return 0
}

// CorePath is the core file beside a folder that holds RetroArch's cores.
func CorePath(dir, name string) string {
	if name == "" {
		return ""
	}
	p := dir + `\cores\` + name + `_libretro.dll`
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

