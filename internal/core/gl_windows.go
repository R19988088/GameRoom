//go:build windows

package core

// A window the games are drawn in must be able to hand a core a real OpenGL
// context: the console cores (Dreamcast, PlayStation, PSP, 3DS, Wii) do all
// their drawing with OpenGL and refuse to load a game without one. The context
// is created for a window that is never shown — the picture itself is read back
// out of a framebuffer and drawn by this app, so nothing is drawn outside the
// launcher's own window and the filters still apply.
//
// Windows 的 OpenGL 只有 1.1 直接导出，3.x 的入口要靠 wglGetProcAddress 取；
// 帧缓冲对象也分 core 与 EXT 两套名字，这里按实际取到的入口用。

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The numbers are libretro's own: 1 is a compatibility OpenGL context, 3 a
// core profile one, 6 Vulkan. Treating 6 as a GL profile is what made Vulkan
// cores ask for an interface nobody offered and then die in their own Vulkan
// loader.
const (
	hwContextNone = iota
	hwContextOpenGL
	hwContextGLES2
	hwContextOpenGLCore
	hwContextGLES3
	hwContextGLESVersion
	hwContextVulkan
	hwContextD3D11
	hwContextD3D10
	hwContextD3D12
)

// hwRenderCallback mirrors retro_hw_render_callback byte for byte: two words of
// enum, three function pointers, three flags, two version numbers, the cache
// flag, the destroy pointer and the debug flag.
type hwRenderCallback struct {
	ContextType      uint32
	pad0             uint32
	ContextReset     uintptr
	GetFramebuffer   uintptr
	GetProcAddress   uintptr
	Depth            uint8
	Stencil          uint8
	BottomLeftOrigin uint8
	pad1             uint8
	VersionMajor     uint32
	VersionMinor     uint32
	CacheContext     uint8
	pad2             [3]byte
	ContextDestroy   uintptr
	DebugContext     uint8
	pad3             [7]byte
}

var (
	glOnce sync.Once
	glFail error

	glHWND   windows.Handle
	glHDC    syscall.Handle
	glRC     syscall.Handle
	glCore   bool // the context is a core-profile one
	glWidth  int
	glHeight int
	glFBO    uint32

	opengl32 = windows.NewLazySystemDLL("opengl32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")

	procChoosePixelFormat = gdi32.NewProc("ChoosePixelFormat")
	procSetPixelFormat    = gdi32.NewProc("SetPixelFormat")
	procSwapBuffers       = gdi32.NewProc("SwapBuffers")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procGetDC             = user32.NewProc("GetDC")

	procWglCreateContext       = opengl32.NewProc("wglCreateContext")
	procWglMakeCurrent         = opengl32.NewProc("wglMakeCurrent")
	procWglDeleteContext       = opengl32.NewProc("wglDeleteContext")
	procWglGetProcAddress      = opengl32.NewProc("wglGetProcAddress")
	procWglGetCurrentContext   = opengl32.NewProc("wglGetCurrentContext")
	procWglCreateContextAttrib = uintptr(0) // wglCreateContextAttribsARB, from the extensions
	procWglShareLists          = opengl32.NewProc("wglShareLists")
	procGetThreadId            = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	glThreadRC                 = map[uintptr]syscall.Handle{}

	glProcs = map[string]uintptr{}
	glProcMu sync.Mutex
)

// gproc finds a GL entry point. wglGetProcAddress answers only for the 3.x and
// extension ones; the 1.1 calls are exported from opengl32 itself, so both are
// tried, and a name is looked up once.
func gproc(name string) uintptr {
	glProcMu.Lock()
	if p, ok := glProcs[name]; ok {
		glProcMu.Unlock()
		return p
	}
	glProcMu.Unlock()
	addr := uintptr(0)
	if a, err := syscall.BytePtrFromString(name); err == nil {
		r, _, _ := procWglGetProcAddress.Call(uintptr(unsafe.Pointer(a)))
		if r > 0 && r != 1 && r != 2 && r != 3 && r != 0xffffffff {
			addr = r
		}
	}
	if addr == 0 {
		// The 1.1 calls are plain exports of opengl32, asked for by name here;
		// calling NewProc(name) would call the GL function itself with the name
		// as its first argument.
		if a, err := windows.GetProcAddress(windows.Handle(opengl32.Handle()), name); err == nil {
			addr = a
		}
	}
	glProcMu.Lock()
	glProcs[name] = addr
	glProcMu.Unlock()
	return addr
}

func getProcAddress(name string) uintptr { return gproc(name) }

// glStubHelper runs before every GL call a foreign core makes: it is what makes
// the shared context current on that thread.
var glStubHelper = syscall.NewCallback(func() uintptr {
	glUseCurrent()
	return 0
})

var (
	stubMu   sync.Mutex
	stubMap  = map[string]uintptr{}
	procVAll = syscall.NewLazyDLL("kernel32.dll").NewProc("VirtualAlloc")
)

// glThreadStub builds, once per entry point, a stub that
//   1. saves the four integer argument registers and xmm0..3,
//   2. calls glStubHelper so this thread gets the context,
//   3. restores them, and
//   4. jumps to the driver's own function.
// The arguments therefore reach OpenGL untouched, floats included.
func glThreadStub(name string) uintptr {
	real := gproc(name)
	if real == 0 {
		return 0
	}
	stubMu.Lock()
	defer stubMu.Unlock()
	if s, ok := stubMap[name]; ok {
		return s
	}
	mem, _, _ := procVAll.Call(0, 4096, 0x3000, 0x40)
	if mem == 0 {
		return real
	}
	code := []byte{
		0x48, 0x83, 0xEC, 0x88, // sub rsp, 0x88
		0x48, 0x89, 0x4C, 0x24, 0x20, // mov [rsp+0x20], rcx
		0x48, 0x89, 0x54, 0x24, 0x28, // mov [rsp+0x28], rdx
		0x4C, 0x89, 0x44, 0x24, 0x30, // mov [rsp+0x30], r8
		0x4C, 0x89, 0x4C, 0x24, 0x38, // mov [rsp+0x38], r9
		0x0F, 0x11, 0x44, 0x24, 0x40, // movups [rsp+0x40], xmm0
		0x0F, 0x11, 0x4C, 0x24, 0x50, // movups [rsp+0x50], xmm1
		0x0F, 0x11, 0x54, 0x24, 0x60, // movups [rsp+0x60], xmm2
		0x0F, 0x11, 0x5C, 0x24, 0x70, // movups [rsp+0x70], xmm3
		0x49, 0xBB, 0, 0, 0, 0, 0, 0, 0, 0, // mov r11, helper
		0x41, 0xFF, 0xD3, // call r11
		0x0F, 0x10, 0x44, 0x24, 0x40, // movups xmm0, [rsp+0x40]
		0x0F, 0x10, 0x4C, 0x24, 0x50,
		0x0F, 0x10, 0x54, 0x24, 0x60,
		0x0F, 0x10, 0x5C, 0x24, 0x70,
		0x48, 0x8B, 0x4C, 0x24, 0x20, // mov rcx, [rsp+0x20]
		0x48, 0x8B, 0x54, 0x24, 0x28,
		0x4C, 0x8B, 0x44, 0x24, 0x30,
		0x4C, 0x8B, 0x4C, 0x24, 0x38,
		0x48, 0x83, 0xC4, 0x88, // add rsp, 0x88
		0x49, 0xBB, 0, 0, 0, 0, 0, 0, 0, 0, // mov r11, real
		0x41, 0xFF, 0xE3, // jmp r11
	}
	// Patch the two mov r11 slots: the helper call first, the driver jump last.
	first := 0
	for i := 0; i+1 < len(code); i++ {
		if code[i] == 0x49 && code[i+1] == 0xBB {
			if first == 0 {
				putU64(code[i+2:], uint64(glStubHelper))
				first = i + 1
			} else {
				putU64(code[i+2:], uint64(real))
				break
			}
		}
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(mem)), len(code)), code)
	stubMap[name] = mem
	return mem
}

func putU64(b []byte, v uint64) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	b[4], b[5], b[6], b[7] = byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56)
}

// gdbgStubs wraps the shader calls when GAMEROOM_GLDBG is set, so a core whose
// shaders fail silently can say why: whether this thread even had our context
// current, and what the driver's own log was.
var gdbgStubs = map[string]uintptr{
	"glCompileShader": syscall.NewCallback(func(shader uintptr) uintptr {
		real := gproc("glCompileShader")
		if real == 0 {
			return 0
		}
		syscall.SyscallN(real, shader)
		var ok int32
		syscall.SyscallN(gproc("glGetShaderiv"), shader, 0x8B81, uintptr(unsafe.Pointer(&ok)))
		if ok == 0 {
			var n int32 = 4096
			var buf [4096]byte
			syscall.SyscallN(gproc("glGetShaderInfoLog"), shader, 4096, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
			ctx, _, _ := procWglGetCurrentContext.Call()
			fmt.Fprintf(os.Stderr, "gldbg: shader compile FAIL ctx=%#x log=%q\n", ctx, string(buf[:n]))
		}
		return 0
	}),
	"glLinkProgram": syscall.NewCallback(func(prog uintptr) uintptr {
		real := gproc("glLinkProgram")
		if real == 0 {
			return 0
		}
		syscall.SyscallN(real, prog)
		var ok int32
		syscall.SyscallN(gproc("glGetProgramiv"), prog, 0x8B82, uintptr(unsafe.Pointer(&ok)))
		if ok == 0 {
			var n int32 = 4096
			var buf [4096]byte
			syscall.SyscallN(gproc("glGetProgramInfoLog"), prog, 4096, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
			ctx, _, _ := procWglGetCurrentContext.Call()
			fmt.Fprintf(os.Stderr, "gldbg: program link FAIL ctx=%#x log=%q\n", ctx, string(buf[:n]))
		}
		return 0
	}),
}

// glProcName is the answer a core gets when it asks for a function by name:
// the pointer it then calls is the driver's own, so the core draws with the
// same OpenGL this window made current.
func glProcName(name uintptr) uintptr {
	if name == 0 {
		return 0
	}
	// A core resolves its GL entry points on whichever thread will draw —
	// PCSX2 does it on its GS thread. Making the shared context current here
	// means that thread has one before its first call, not after it faults.
	glUseCurrent()
	cn := cString((*byte)(ptr(name)))
	if os.Getenv("GAMEROOM_GLPROC") != "" {
		fmt.Fprintf(os.Stderr, "glproc: %s\n", cn)
	}
	if os.Getenv("GAMEROOM_GLDBG") != "" {
		if stub, ok := gdbgStubs[cn]; ok {
			return stub
		}
	}
	// PCSX2 and Flycast call OpenGL from threads they made themselves. A
	// context is current per thread, so the call from one of those threads
	// lands in the driver with nothing current and faults the process. Every
	// entry point handed to those cores is therefore a small stub that makes
	// the shared context current on the calling thread, then runs the driver's
	// own function with the arguments untouched.
	if coreName == "pcsx2" || coreName == "flycast" {
		if os.Getenv("GAMEROOM_GLSTUB") != "" {
			return glThreadStub(cn)
		}
	}
	return gproc(cn)
}

// glPrepare creates the hidden window and a context matching what the core
// asked for. It answers whether the request could be met.
func glPrepare(contextType uint32, major, minor uint32, depth, stencil bool) error {
	glOnce.Do(func() {
		glFail = glCreateWindow()
	})
	if glFail != nil {
		return glFail
	}
	if glRC != 0 {
		return nil // one context serves the app; a second game reuses it
	}
	glCore = contextType == hwContextOpenGLCore || contextType == hwContextGLES3
	if err := glCreateContext(major, minor, depth, stencil); err != nil {
		return err
	}
	return nil
}

func glWindowProc(hwnd, msg, wp, lp uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func glCreateWindow() error {
	proc := syscall.NewCallback(glWindowProc)
	name, _ := syscall.UTF16PtrFromString("GameRoomGL")
	var wc struct {
		Size            uint32
		Style           uint32
		WndProc         uintptr
		ClsExtra        int32
		WndExtra        int32
		Instance        windows.Handle
		Icon            windows.Handle
		Cursor          windows.Handle
		Background      windows.Handle
		MenuName        *uint16
		ClassName       *uint16
		IconSm          windows.Handle
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.Style = 0x0020 | 0x0002 // CS_OWNDC | CS_HREDRAW
	wc.WndProc = proc
	wc.ClassName = name
	if _, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); err != nil {
		// a class that already exists is fine
	}
	hwnd, _, callErr := procCreateWindowExW.Call(
		0x00000080, // WS_EX_TOOLWINDOW: never a taskbar entry
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(name)),
		0x80000000, // WS_POPUP, never shown
		0, 0, 1920, 1080,
		0, 0, 0, 0)
	if hwnd == 0 {
		return fmt.Errorf("无法建立 OpenGL 窗口：%v", callErr)
	}
	glHWND = windows.Handle(hwnd)
	hdc, _, _ := procGetDC.Call(hwnd)
	glHDC = syscall.Handle(hdc)
	return nil
}

// choosePixfmt sets a pixel format that suits both a legacy and a core context:
// 32-bit colour with a depth buffer is enough because every core draws into a
// framebuffer of ours, never into the window.
func choosePixfmt() error {
	var pfd struct {
		Size          uint16
		Version       uint16
		Flags         uint32
		PixelType     uint8
		ColorBits     uint8
		RedBits       uint8
		RedShift      uint8
		GreenBits     uint8
		GreenShift    uint8
		BlueBits      uint8
		BlueShift     uint8
		AlphaBits     uint8
		AlphaShift    uint8
		AccumBits     uint8
		AccumRedBits  uint8
		AccumGreen    uint8
		AccumBlue     uint8
		AccumAlpha    uint8
		DepthBits     uint8
		StencilBits   uint8
		AuxBuffers    uint8
		LayerType     uint8
		Reserved      uint8
		LayerMask     uint32
		VisibleMask   uint32
		DamageMask    uint32
	}
	pfd.Size = uint16(unsafe.Sizeof(pfd))
	pfd.Version = 1
	pfd.Flags = 0x00000004 | 0x00000020 // PFD_DRAW_TO_WINDOW | PFD_SUPPORT_OPENGL
	pfd.PixelType = 0
	pfd.ColorBits = 32
	pfd.RedBits, pfd.GreenBits, pfd.BlueBits, pfd.AlphaBits = 8, 8, 8, 8
	pfd.DepthBits = 24
	pfd.StencilBits = 8
	n, _, _ := procChoosePixelFormat.Call(uintptr(glHDC), uintptr(unsafe.Pointer(&pfd)))
	if n == 0 {
		return fmt.Errorf("找不到可用的像素格式")
	}
	if ok, _, _ := procSetPixelFormat.Call(uintptr(glHDC), n, uintptr(unsafe.Pointer(&pfd))); ok == 0 {
		return fmt.Errorf("无法设置像素格式")
	}
	return nil
}

func glCreateContext(major, minor uint32, depth, stencil bool) error {
	if err := choosePixfmt(); err != nil {
		return err
	}
	// A throwaway context first: wglCreateContextAttribsARB can only be asked for
	// from inside a current context.
	rc, _, _ := procWglCreateContext.Call(uintptr(glHDC))
	if rc == 0 {
		return fmt.Errorf("无法建立 OpenGL 上下文")
	}
	procWglMakeCurrent.Call(uintptr(glHDC), rc)
	if a, err := syscall.BytePtrFromString("wglCreateContextAttribsARB"); err == nil {
		r, _, _ := procWglGetProcAddress.Call(uintptr(unsafe.Pointer(a)))
		procWglCreateContextAttrib = r
	}
	procWglMakeCurrent.Call(0, 0)
	procWglDeleteContext.Call(rc)

	real := uintptr(0)
	if procWglCreateContextAttrib != 0 {
		ver := major
		if ver < 3 {
			ver = 3
		}
		if ver < 3 {
			ver = 3
		}
		profile := uintptr(0x00000002) // WGL_CONTEXT_CORE_PROFILE_BIT_ARB
		if !glCore {
			profile = 0x00000001 // WGL_CONTEXT_COMPATIBILITY_PROFILE_BIT_ARB
		}
		attrs := []uint32{
			0x2091, ver, // WGL_CONTEXT_MAJOR_VERSION_ARB
			0x2092, func() uint32 {
				if minor > 0 {
					return minor
				}
				return 3
			}(),
			0x9126, uint32(profile), // WGL_CONTEXT_PROFILE_MASK_ARB
			0,
		}
		real, _, _ = syscall.SyscallN(procWglCreateContextAttrib,
			uintptr(glHDC), 0, uintptr(unsafe.Pointer(&attrs[0])))
	}
	if real == 0 {
		rc, _, _ := procWglCreateContext.Call(uintptr(glHDC))
		if rc == 0 {
			return fmt.Errorf("无法建立 OpenGL 上下文")
		}
		real = rc
	}
	procWglMakeCurrent.Call(uintptr(glHDC), real)
	glRC = syscall.Handle(real)
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		ver := ""
		if gs := gproc("glGetString"); gs != 0 {
			v, _, _ := syscall.SyscallN(gs, 0x1F02) // GL_VERSION
			if v != 0 {
				ver = cString((*byte)(ptr(v)))
			}
		}
		fmt.Fprintf(os.Stderr, "gl: ctx=%v core=%v version=%q\n", real, glCore, ver)
		for _, n := range []string{"glCreateProgram", "glGenVertexArrays", "glGenBuffers", "glBindBuffer",
			"glTexImage2D", "glGenFramebuffers", "glBindFramebuffer", "glBlitFramebuffer", "glProgramBinary",
			"glGetIntegerv", "glMapBufferRange", "glDrawElements", "glUseProgram", "glGenTextures"} {
			p := gproc(n)
			if p == 0 {
				fmt.Fprintf(os.Stderr, "gl: missing %s\n", n)
			}
		}
	}
	return glMakeFBO(2048, 2048, depth, stencil)
}

// glMakeFBO gives the core a colour target it can draw into; the app reads it
// back after every frame. Depth and stencil are attached when the core asks.
func glMakeFBO(w, h int, depth, stencil bool) error {
	genFB := gproc("glGenFramebuffers")
	bindFB := gproc("glBindFramebuffer")
	texFB := gproc("glFramebufferTexture2D")
	genTex := gproc("glGenTextures")
	bindTex := gproc("glBindTexture")
	texImage := gproc("glTexImage2D")
	texParam := gproc("glTexParameteri")
	if genFB == 0 {
		genFB = gproc("glGenFramebuffersEXT")
		bindFB = gproc("glBindFramebufferEXT")
		texFB = gproc("glFramebufferTexture2DEXT")
		if genFB == 0 {
			return fmt.Errorf("这个 OpenGL 不支持帧缓冲对象")
		}
	}
	tex := uint32(0)
	syscall.SyscallN(genTex, 1, uintptr(unsafe.Pointer(&tex)))
	syscall.SyscallN(bindTex, 0x0DE1, uintptr(tex))
	syscall.SyscallN(texImage, 0x0DE1, 0, 0x8058, uintptr(w), uintptr(h), 0, 0x1908, 0x1401, 0)
	if texParam != 0 {
		syscall.SyscallN(texParam, 0x0DE1, 0x2801, 0x2601) // LINEAR
		syscall.SyscallN(texParam, 0x0DE1, 0x2800, 0x2601)
	}
	fb := uint32(0)
	syscall.SyscallN(genFB, 1, uintptr(unsafe.Pointer(&fb)))
	syscall.SyscallN(bindFB, 0x8D40, uintptr(fb))
	syscall.SyscallN(texFB, 0x8D40, 0x8CE0, 0x0DE1, uintptr(tex), 0)
	if depth || stencil {
		genRB := gproc("glGenRenderbuffers")
		bindRB := gproc("glBindRenderbuffer")
		rbStore := gproc("glRenderbufferStorage")
		rbFB := gproc("glFramebufferRenderbuffer")
		if genRB == 0 {
			genRB = gproc("glGenRenderbuffersEXT")
			bindRB = gproc("glBindRenderbufferEXT")
			rbStore = gproc("glRenderbufferStorageEXT")
			rbFB = gproc("glFramebufferRenderbufferEXT")
		}
		if genRB != 0 {
			rb := uint32(0)
			syscall.SyscallN(genRB, 1, uintptr(unsafe.Pointer(&rb)))
			syscall.SyscallN(bindRB, 0x8D41, uintptr(rb))
			format := uintptr(0x81A6) // DEPTH24_STENCIL8
			if depth && !stencil {
				format = 0x81A5 // DEPTH_COMPONENT24
			} else if !depth && stencil {
				format = 0x8D48 // STENCIL_INDEX8
			}
			syscall.SyscallN(rbStore, 0x8D41, format, uintptr(w), uintptr(h))
			attach := uintptr(0x8D00) // DEPTH_ATTACHMENT
			if !depth && stencil {
				attach = 0x8D20
			}
			syscall.SyscallN(rbFB, 0x8D40, attach, 0x8D41, uintptr(rb))
			if depth && stencil {
				syscall.SyscallN(rbFB, 0x8D40, 0x8D20, 0x8D41, uintptr(rb))
			}
		}
	}
	glFBO = fb
	glWidth, glHeight = w, h
	if st := gproc("glCheckFramebufferStatus"); st != 0 {
		status, _, _ := syscall.SyscallN(st, 0x8D40)
		if status != 0x8CD5 { // GL_FRAMEBUFFER_COMPLETE
			return fmt.Errorf("帧缓冲建不起来（0x%x）", status)
		}
	}
	return nil
}

// glCurrentFramebuffer is what the core asks for to draw into.
func glCurrentFramebuffer() uintptr {
	if s := active; s != nil {
		s.fbAsked.Store(true)
	}
	return uintptr(glFBO)
}

// glUseCurrent makes the shared context current on the calling thread. A core
// draws from the thread that runs it, not from the one that created the
// context, and OpenGL calls without a current context on that thread are a
// fault rather than a picture.
func glUseCurrent() bool {
	if glRC == 0 || glHDC == 0 {
		return false
	}
	if cur, _, _ := procWglGetCurrentContext.Call(); cur == uintptr(glRC) {
		return true
	}
	// A context is current in one thread at a time, and two of them cannot be
	// current at once. Cores such as PCSX2 and Flycast draw from a thread of
	// their own, so that thread gets its own context sharing our objects: the
	// textures and framebuffers the core made are the same, the thread simply
	// has the context it needs.
	rc := glThreadContext()
	if rc == 0 {
		return false
	}
	procWglMakeCurrent.Call(uintptr(glHDC), uintptr(rc))
	cur, _, _ := procWglGetCurrentContext.Call()
	return cur == uintptr(rc)
}

// glThreadContext returns this thread's own share-list context, creating it the
// first time the thread draws.
func glThreadContext() uintptr {
	tid, _, _ := procGetThreadId.Call()
	stubMu.Lock()
	defer stubMu.Unlock()
	if rc, ok := glThreadRC[uintptr(tid)]; ok {
		return uintptr(rc)
	}
	rc, _, _ := procWglCreateContext.Call(uintptr(glHDC))
	if rc == 0 {
		return 0
	}
	// Sharing is what lets a frame the main context reads back contain what
	// this thread drew.
	ok, _, _ := procWglShareLists.Call(uintptr(glRC), rc)
	if ok == 0 {
		procWglDeleteContext.Call(rc)
		return 0
	}
	glThreadRC[uintptr(tid)] = syscall.Handle(rc)
	return rc
}

// glBindDrawFBO puts the frame target back on before a frame runs, so a core
// that draws to "whatever is bound" lands in the picture we read back.
func glBindDrawFBO() {
	if glRC == 0 || glFBO == 0 {
		return
	}
	if !glUseCurrent() {
		// No context on this thread: calling GL now would fault the process,
		// and there is nothing to draw into anyway.
		return
	}
	if bindFB := gproc("glBindFramebuffer"); bindFB != 0 {
		syscall.SyscallN(bindFB, 0x8D40, uintptr(glFBO))
	}
}

// glReadFrame reads the colour target back as RGBA rows, top row first. The core
// draws bottom-up (OpenGL's own orientation) unless it said otherwise, so the
// rows are flipped here once rather than twice further up.
func glReadFrame(w, h, pitch int, bottomLeft bool) []byte {
	if glRC == 0 || w <= 0 || h <= 0 {
		return nil
	}
	glUseCurrent()
	if w > glWidth || h > glHeight {
		return nil // a frame larger than the target: the core keeps the old picture
	}
	bindFB := gproc("glBindFramebuffer")
	if bindFB == 0 {
		bindFB = gproc("glBindFramebufferEXT")
	}
	readBuf := gproc("glReadBuffer")
	readPix := gproc("glReadPixels")
	if readPix == 0 || bindFB == 0 {
		return nil
	}
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		if geti := gproc("glGetIntegerv"); geti != 0 {
			var bound int32
			syscall.SyscallN(geti, 0x8CA6, uintptr(unsafe.Pointer(&bound)))
			var vp [4]int32
			syscall.SyscallN(geti, 0x0BA2, uintptr(unsafe.Pointer(&vp[0])))
			fmt.Fprintf(os.Stderr, "gl: drew into fb=%d viewport=%d,%d %dx%d (ours=%d) ask=%dx%d\n",
				bound, vp[0], vp[1], vp[2], vp[3], glFBO, w, h)
		}
	}
	// A core either draws into the framebuffer we hand out or straight into the
	// window's own one; whichever it used is the one to read back from. The
	// window is created at a usable size for exactly this case.
	target := glFBO
	readFrom := uintptr(0x8CE0) // GL_COLOR_ATTACHMENT0
	if geti := gproc("glGetIntegerv"); geti != 0 {
		var bound int32
		syscall.SyscallN(geti, 0x8CA6, uintptr(unsafe.Pointer(&bound)))
		if bound == 0 {
			target = 0
			readFrom = 0x0405 // GL_BACK
		}
	}
	syscall.SyscallN(bindFB, 0x8D40, uintptr(target))
	if readBuf != 0 {
		syscall.SyscallN(readBuf, readFrom)
	}
	buf := make([]byte, w*h*4)
	syscall.SyscallN(readPix, 0, 0, uintptr(w), uintptr(h), 0x1908, 0x1401, uintptr(unsafe.Pointer(&buf[0])))
	if !bottomLeft {
		row := make([]byte, w*4)
		for y := 0; y < h/2; y++ {
			top := y * w * 4
			bot := (h - 1 - y) * w * 4
			copy(row, buf[top:top+w*4])
			copy(buf[top:top+w*4], buf[bot:bot+w*4])
			copy(buf[bot:bot+w*4], row)
		}
	}
	return buf
}
