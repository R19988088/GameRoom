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
	init, loadGame, run, unload, deinit, getAVInfo, getSystemInfo          uintptr

	keys  atomic.Value // map[int]bool, RETROK codes held down
	mouse atomic.Value // MouseState, the pointer as a core reads it

	mu     sync.RWMutex
	frame  *image.RGBA
	frames int
	w, h   int
	format int

	closed int32
	quit   chan struct{}
	done   chan struct{}

	onFrame atomic.Value // func()
	fps     float64
	aspect  float32
	audio   *AudioPipe

	sndOnce sync.Once
	sndDone chan struct{}
}

// Aspect is how wide the picture is to be shown against its height, which on
// an old screen is not the shape of the frame: a 320×200 DOS picture belongs on
// a 4:3 glass.
func (s *Session) Aspect() float32 { return s.aspect }

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
	keepStr []string
)

// Start loads a core and its content and leaves it ready to be stepped.
func Start(corePath, romPath, systemDir string) (*Session, error) {
	workDir = systemDir
	h, err := loadLibrary(corePath)
	if err != nil {
		return nil, err
	}
	s := &Session{dll: h, quit: make(chan struct{}), done: make(chan struct{}), sndDone: make(chan struct{}), fps: 60}
	s.keys.Store(map[int]bool{})
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
	s.init = proc("retro_init")
	s.loadGame = proc("retro_load_game")
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

	var gi struct {
		path *byte
		data unsafe.Pointer
		size uint64
		meta *byte
	}
	p, err := syscall.BytePtrFromString(romPath)
	if err != nil {
		return nil, err
	}
	gi.path = p
	if ok, _, _ := syscall.SyscallN(s.loadGame, uintptr(unsafe.Pointer(&gi))); ok == 0 {
		return nil, fmt.Errorf("这个核心打不开这个游戏：%s", romPath)
	}

	var av struct {
		geometry struct{ base, max, aspect float32 }
		timing   struct{ fps, sample float64 }
	}
	if s.getAVInfo != 0 {
		s.call(s.getAVInfo, uintptr(unsafe.Pointer(&av)))
		if av.timing.fps > 1 && av.timing.fps < 240 {
			s.fps = av.timing.fps
		}
		if av.geometry.aspect > 0.4 && av.geometry.aspect < 5 {
			s.aspect = av.geometry.aspect
		}
	}
	s.audio = NewAudioPipe(int(av.timing.sample))
	return s, nil
}

var active *Session

// Run steps the core until Close. Frames are published as they come.
func (s *Session) Run() {
	defer close(s.done)
	period := time.Duration(float64(time.Second) / s.fps)
	next := time.Now()
	for {
		select {
		case <-s.quit:
			return
		default:
		}
		s.SetKeys(pollKeys())
		s.call(s.run)
		if f, ok := s.onFrame.Load().(func()); ok && f != nil {
			f()
		}
		next = next.Add(period)
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		} else if d < -period {
			next = time.Now()
		}
	}
}

// SetKeys takes the keyboard keys held down, as retrok codes.
func (s *Session) SetKeys(keys map[int]bool) { s.keys.Store(keys) }

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
	procLoadLibrary = syscall.NewLazyDLL("kernel32.dll").NewProc("LoadLibraryW")
	procGetProc     = syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcAddress")
	procSetDllDir   = syscall.NewLazyDLL("kernel32.dll").NewProc("SetDllDirectoryW")
	dllDirSet       bool
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

func envCallback(cmd, data uintptr) uintptr {
	if active == nil {
		return 0
	}
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "env: cmd=%d(0x%x) data=0x%x\n", cmd, cmd, data)
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
	case 9, 30, 31: // system, content, save directory
		if data != 0 {
			putStr(ptr(data), workDir)
		}
		return 1
	case 15: // GET_VARIABLE: no options are offered, so the core uses its own
		return 0
	case 16: // GET_VARIABLE_UPDATE
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
	case 18: // GET_LIBRETRO_PATH
		if data != 0 {
			putStr(ptr(data), curCore)
		}
		return 1
	}
	return 0
}

func putStr(dst unsafe.Pointer, s string) {
	keepStr = append(keepStr, s)
	b := append([]byte(s), 0)
	*(*unsafe.Pointer)(dst) = unsafe.Pointer(&b[0])
}

// logCallback is the one function pointer a core may call to say something.
// Its message may come with arguments, so the arguments are left alone and the
// text is not formatted: a core that logs is not a core that must be read.
var logFn uintptr

func logCallback() uintptr {
	if logFn == 0 {
		logFn = syscall.NewCallback(func(level, message uintptr) uintptr {
			if os.Getenv("GAMEROOM_DEBUG") != "" && message != 0 {
				fmt.Fprintf(os.Stderr, "core log level=%d\n", level)
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
	if s == nil || data == 0 || w == 0 || h == 0 {
		return 0
	}
	width, height, row := int(w), int(h), int(pitch)
	s.mu.Lock()
	if s.frame == nil || s.w != width || s.h != height {
		s.frame = image.NewRGBA(image.Rect(0, 0, width, height))
		s.w, s.h = width, height
	}
	s.frames++
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
