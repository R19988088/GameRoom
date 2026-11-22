//go:build windows

package core

// The Vulkan half of the hardware renderer.
//
// A core that answers SET_HW_RENDER with VULKAN is not handed a window or a
// swapchain: libretro asks the frontend for a device, a queue and a handful of
// callbacks, and the core draws into images of its own and hands the finished
// one over through set_image. The picture is then copied back into memory here,
// so the rest of the app sees exactly the same frame it sees from any software
// core — filters, scaling and the window stay the same.

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	vkStructureApplicationInfo         = 0
	vkStructureInstanceCreateInfo      = 1
	vkStructureDeviceQueueCreateInfo   = 2
	vkStructureDeviceCreateInfo        = 3
	vkStructureSubmitInfo              = 4
	vkStructureMemoryAllocateInfo      = 5
	vkStructureFenceCreateInfo         = 8
	vkStructureBufferCreateInfo        = 12
	vkStructureImageViewCreateInfo     = 15
	vkStructureCommandPoolCreateInfo   = 39
	vkStructureCommandBufferAllocInfo  = 40
	vkStructureCommandBufferBeginInfo  = 42
	vkStructureImageMemoryBarrier      = 45

	vkSuccess                    = 0
	vkQueueGraphicsBit           = 0x0001
	vkMemoryHostVisibleBit       = 0x0002
	vkMemoryHostCoherentBit      = 0x0004
	vkBufferUsageTransferDst     = 0x0002
	vkSharingModeExclusive       = 0
	vkCommandPoolResetBufferBit  = 0x0002
	vkCommandBufferOneTimeSubmit = 0x0001
	vkImageLayoutGeneral         = 1
	vkImageLayoutTransferSrc     = 6
	vkAccessMemoryRead           = 0x8000
	vkAccessMemoryWrite          = 0x10000
	vkAccessTransferRead         = 0x1000
	vkPipelineAllCommands        = 0x10000
	vkImageAspectColor           = 0x0001
	vkQueueFamilyIgnored         = 0xffffffff
	vkFenceTimeout               = 2000000000 // two seconds, in nanoseconds
	vkWholeSize                  = 0xffffffff

	vkFormatRGBA8Unorm = 37
	vkFormatRGBA8Srgb  = 43
	vkFormatBGRA8Unorm = 44
	vkFormatBGRA8Srgb  = 50
)

// retroHWRenderInterfaceVulkan is the struct a Vulkan core reads after
// GET_HW_RENDER_INTERFACE. The order and padding match libretro_vulkan.h.
type retroHWRenderInterfaceVulkan struct {
	InterfaceType    uint32
	InterfaceVersion uint32
	Handle           unsafe.Pointer
	Instance         uintptr
	GPU              uintptr
	Device           uintptr
	GetDeviceProc    uintptr
	GetInstanceProc  uintptr
	Queue            uintptr
	QueueIndex       uint32
	pad0             uint32
	SetImage         uintptr
	GetSyncIndex     uintptr
	GetSyncIndexMask uintptr
	SetCommandBuffers uintptr
	WaitSyncIndex    uintptr
	LockQueue        uintptr
	UnlockQueue      uintptr
	SetSignalSemaphore uintptr
}

// retroVulkanImage is what set_image points at: the view a core finished with,
// the layout it left the image in, and the create info it used.
type retroVulkanImage struct {
	ImageView   uintptr
	ImageLayout uint32
	pad0        uint32
	CreateInfo  uintptr
}

type vkApplicationInfo struct {
	SType           uint32
	pad0            uint32
	PNext           uintptr
	AppName         uintptr
	AppVersion      uint32
	pad1            uint32
	EngineName      uintptr
	EngineVersion   uint32
	APIVersion      uint32
}

type vkInstanceCreateInfo struct {
	SType                  uint32
	pad0                   uint32
	PNext                  uintptr
	Flags                  uint32
	pad1                   uint32
	PAppInfo               uintptr
	EnabledLayerCount      uint32
	pad2                   uint32
	PPLayerNames           uintptr
	EnabledExtensionCount  uint32
	pad3                   uint32
	PPEnabledExtensionNames uintptr
}

type vkDeviceQueueCreateInfo struct {
	SType            uint32
	pad0             uint32
	PNext            uintptr
	Flags            uint32
	QueueFamilyIndex uint32
	QueueCount       uint32
	pad1             uint32
	PQueuePriorities uintptr
}

type vkDeviceCreateInfo struct {
	SType                   uint32
	pad0                    uint32
	PNext                   uintptr
	Flags                   uint32
	QueueCreateInfoCount    uint32
	PQueueCreateInfos       uintptr
	EnabledLayerCount       uint32
	pad1                    uint32
	PPEnabledLayerNames     uintptr
	EnabledExtensionCount   uint32
	pad2                    uint32
	PPEnabledExtensionNames uintptr
	PEnabledFeatures        uintptr
}

type vkCommandPoolCreateInfo struct {
	SType            uint32
	pad0             uint32
	PNext            uintptr
	Flags            uint32
	QueueFamilyIndex uint32
}

type vkCommandBufferAllocateInfo struct {
	SType              uint32
	pad0               uint32
	PNext              uintptr
	CommandPool        uintptr
	Level              uint32
	CommandBufferCount uint32
}

type vkCommandBufferBeginInfo struct {
	SType           uint32
	pad0            uint32
	PNext           uintptr
	Flags           uint32
	pad1            uint32
	PInheritanceInfo uintptr
}

type vkImageSubresourceRange struct {
	AspectMask     uint32
	BaseMipLevel   uint32
	LevelCount     uint32
	BaseArrayLayer uint32
	ArrayCount     uint32
}

type vkImageMemoryBarrier struct {
	SType               uint32
	pad0                uint32
	PNext               uintptr
	SrcAccessMask       uint32
	DstAccessMask       uint32
	OldLayout           uint32
	NewLayout           uint32
	SrcQueueFamilyIndex uint32
	DstQueueFamilyIndex uint32
	Image               uintptr
	SubresourceRange    vkImageSubresourceRange
}

type vkBufferCreateInfo struct {
	SType                 uint32
	pad0                  uint32
	PNext                 uintptr
	Flags                 uint32
	Size                  uint64
	Usage                 uint32
	SharingMode           uint32
	QueueFamilyIndexCount uint32
	pad1                  uint32
	PQueueFamilyIndices   uintptr
}

type vkMemoryRequirements struct {
	Size           uint64
	Alignment      uint64
	MemoryTypeBits uint32
}

type vkMemoryAllocateInfo struct {
	SType             uint32
	pad0              uint32
	PNext             uintptr
	AllocationSize    uint64
	MemoryTypeIndex   uint32
}

type vkImageSubresourceLayers struct {
	AspectMask     uint32
	MipLevel       uint32
	BaseArrayLayer uint32
	LayerCount     uint32
}

type vkOffset3D struct {
	X int32
	Y int32
	Z int32
}

type vkExtent3D struct {
	Width  uint32
	Height uint32
	Depth  uint32
}

type vkBufferImageCopy struct {
	BufferOffset     uint64
	BufferRowLength  uint32
	BufferImageHeight uint32
	ImageSubresource vkImageSubresourceLayers
	ImageOffset      vkOffset3D
	ImageExtent      vkExtent3D
}

type vkSubmitInfo struct {
	SType                uint32
	pad0                 uint32
	PNext                uintptr
	WaitSemaphoreCount   uint32
	pad1                 uint32
	PWaitSemaphores      uintptr
	CommandBufferCount   uint32
	pad2                 uint32
	PCommandBuffers      uintptr
	SignalSemaphoreCount uint32
	pad3                 uint32
	PSignalSemaphores    uintptr
}

type vkFenceCreateInfo struct {
	SType uint32
	pad0  uint32
	PNext uintptr
	Flags uint32
	pad1  uint32
}

type vkQueueFamilyProperties struct {
	QueueFlags                  uint32
	QueueCount                  uint32
	TimestampValidBits          uint32
	MinImageTransferGranularity [3]uint32
}

type vkMemoryType struct {
	PropertyFlags uint32
	HeapIndex     uint32
}

type vkMemoryHeap struct {
	Size  uint64
	Flags uint32
	pad0  uint32
}

type vkPhysicalDeviceMemoryProperties struct {
	MemoryTypeCount uint32
	pad0            uint32
	MemoryTypes     [32]vkMemoryType
	MemoryHeapCount uint32
	pad1            uint32
	MemoryHeaps     [16]vkMemoryHeap
}

type vkDeviceFunctions struct {
	getDeviceQueue      uintptr
	createCommandPool   uintptr
	allocateCmdBuffers  uintptr
	beginCmdBuffer      uintptr
	cmdPipelineBarrier  uintptr
	cmdCopyImageToBuffer uintptr
	endCmdBuffer        uintptr
	queueSubmit         uintptr
	waitForFences       uintptr
	resetFences         uintptr
	queueWaitIdle       uintptr
	createBuffer        uintptr
	getBufferMemoryReqs uintptr
	allocateMemory      uintptr
	bindBufferMemory    uintptr
	mapMemory           uintptr
	unmapMemory         uintptr
	destroyBuffer       uintptr
	freeMemory          uintptr
	resetCmdBuffer      uintptr
}

var (
	vkDLL             = windows.NewLazySystemDLL("vulkan-1.dll")
	vkGetInstanceProc = vkDLL.NewProc("vkGetInstanceProcAddr")

	vkOnce    sync.Once
	vkErr     error
	vkReady   bool
	vkMutex   sync.Mutex

	vkInstance uintptr
	vkPhysical uintptr
	vkDevice   uintptr
	vkQueue    uintptr
	vkQueueIdx uint32
	vkPool     uintptr
	vkCmd      uintptr
	vkFence    uintptr
	vkFn       vkDeviceFunctions

	vkStageBuf  uintptr
	vkStageMem  uintptr
	vkStageSize uint64
	vkStagePtr  unsafe.Pointer

	vkIface retroHWRenderInterfaceVulkan

	vkFrameMu   sync.Mutex
	vkFrameView uintptr
	vkFrameLayout uint32
	vkFrameCreate uintptr
	vkPending   []uintptr
	vkLastSem   uintptr
	vkLastSems  []uintptr
)

func vkProc(inst uintptr, name string) uintptr {
	b := append([]byte(name), 0)
	r, _, _ := vkGetInstanceProc.Call(inst, uintptr(unsafe.Pointer(&b[0])))
	return r
}

func vkDevProc(name string) uintptr {
	if vkFnGetDeviceProc == 0 {
		return 0
	}
	b := append([]byte(name), 0)
	r, _, _ := syscall.SyscallN(vkFnGetDeviceProc, vkDevice, uintptr(unsafe.Pointer(&b[0])))
	return r
}

var vkFnGetDeviceProc uintptr

// vkPrepare builds the instance, device and queue a Vulkan core needs. It is
// done once per process and reused by every core, because a device is the
// expensive part and nothing here is per-game.
func vkPrepare() error {
	vkOnce.Do(func() { vkErr = vkBuild() })
	if vkErr != nil {
		return vkErr
	}
	vkReady = true
	return nil
}

// VulkanSelfTest brings the device up outside a game so a driver problem can
// be told apart from a core problem.
func VulkanSelfTest() error { return vkPrepare() }

func vkBuild() error {
	dbg := func(f string, a ...any) {
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "vkbld: "+f+"\n", a...)
		}
	}
	dbg("start")
	if err := vkDLL.Load(); err != nil {
		return fmt.Errorf("没有可用的 Vulkan 运行库：%w", err)
	}
	// The loader does not like a null name here, so nothing is probed: every
	// entry point below is fetched by name through vkGetInstanceProcAddr.

	app := vkApplicationInfo{SType: vkStructureApplicationInfo, AppVersion: 1, APIVersion: 0x00401000}
	instExts := vkPickInstanceExtensions()
	var instExtPtrs []uintptr
	var instExtBytes [][]byte
	for _, e := range instExts {
		b := append([]byte(e), 0)
		instExtBytes = append(instExtBytes, b)
		instExtPtrs = append(instExtPtrs, uintptr(unsafe.Pointer(&b[0])))
	}
	ci := vkInstanceCreateInfo{SType: vkStructureInstanceCreateInfo, PAppInfo: uintptr(unsafe.Pointer(&app))}
	if len(instExtPtrs) > 0 {
		ci.EnabledExtensionCount = uint32(len(instExtPtrs))
		ci.PPEnabledExtensionNames = uintptr(unsafe.Pointer(&instExtPtrs[0]))
	}
	_ = instExtBytes
	createInstance := vkProc(0, "vkCreateInstance")
	if createInstance == 0 {
		return fmt.Errorf("Vulkan 加载器缺少 vkCreateInstance")
	}
	if r, _, _ := syscall.SyscallN(createInstance, uintptr(unsafe.Pointer(&ci)), 0, uintptr(unsafe.Pointer(&vkInstance))); r != vkSuccess {
		return fmt.Errorf("创建 Vulkan 实例失败（%d）", r)
	}
	dbg("instance ok 0x%x", vkInstance)

	enumerate := vkProc(vkInstance, "vkEnumeratePhysicalDevices")
	if enumerate == 0 {
		return fmt.Errorf("Vulkan 加载器缺少设备枚举")
	}
	var count uint32
	syscall.SyscallN(enumerate, vkInstance, uintptr(unsafe.Pointer(&count)), 0)
	if count == 0 {
		return fmt.Errorf("没有 Vulkan 显卡")
	}
	devs := make([]uintptr, count)
	if r, _, _ := syscall.SyscallN(enumerate, vkInstance, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&devs[0]))); r != vkSuccess || count == 0 {
		return fmt.Errorf("枚举 Vulkan 显卡失败（%d）", r)
	}

	getFamilies := vkProc(vkInstance, "vkGetPhysicalDeviceQueueFamilyProperties")
	getFeatures := vkProc(vkInstance, "vkGetPhysicalDeviceFeatures")
	getProps := vkProc(vkInstance, "vkGetPhysicalDeviceProperties")
	getMemProps := vkProc(vkInstance, "vkGetPhysicalDeviceMemoryProperties")
	if getFamilies == 0 || getFeatures == 0 || getMemProps == 0 {
		return fmt.Errorf("Vulkan 加载器缺少设备查询接口")
	}

	var chosen uintptr
	var family uint32
	for _, d := range devs[:count] {
		var fc uint32
		syscall.SyscallN(getFamilies, d, uintptr(unsafe.Pointer(&fc)), 0)
		if fc == 0 {
			continue
		}
		fams := make([]vkQueueFamilyProperties, fc)
		syscall.SyscallN(getFamilies, d, uintptr(unsafe.Pointer(&fc)), uintptr(unsafe.Pointer(&fams[0])))
		for i := range fams {
			if fams[i].QueueFlags&vkQueueGraphicsBit != 0 && fams[i].QueueCount > 0 {
				chosen = d
				family = uint32(i)
				break
			}
		}
		if chosen != 0 {
			break
		}
	}
	if chosen == 0 {
		return fmt.Errorf("没有支持图形队列的 Vulkan 显卡")
	}
	vkPhysical = chosen
	vkQueueIdx = family

	if getProps != 0 {
		var props [512]byte
		syscall.SyscallN(getProps, chosen, uintptr(unsafe.Pointer(&props[0])))
	}

	// Every feature the card reports is asked for: a core picks what it needs
	// from the device it is given, and a missing one is a silent black screen.
	var feats [64]uint32
	syscall.SyscallN(getFeatures, chosen, uintptr(unsafe.Pointer(&feats[0])))

	var memProps vkPhysicalDeviceMemoryProperties
	syscall.SyscallN(getMemProps, chosen, uintptr(unsafe.Pointer(&memProps)))
	vkMemProps = memProps

	exts := vkPickDeviceExtensions(chosen)

	priority := float32(1.0)
	qci := vkDeviceQueueCreateInfo{
		SType:            vkStructureDeviceQueueCreateInfo,
		QueueFamilyIndex: family,
		QueueCount:       1,
		PQueuePriorities: uintptr(unsafe.Pointer(&priority)),
	}
	dci := vkDeviceCreateInfo{
		SType:                vkStructureDeviceCreateInfo,
		QueueCreateInfoCount: 1,
		PQueueCreateInfos:    uintptr(unsafe.Pointer(&qci)),
		PEnabledFeatures:     uintptr(unsafe.Pointer(&feats[0])),
	}
	var extPtrs []uintptr
	var extBytes [][]byte
	for _, e := range exts {
		b := append([]byte(e), 0)
		extBytes = append(extBytes, b)
		extPtrs = append(extPtrs, uintptr(unsafe.Pointer(&b[0])))
	}
	if len(extPtrs) > 0 {
		dci.EnabledExtensionCount = uint32(len(extPtrs))
		dci.PPEnabledExtensionNames = uintptr(unsafe.Pointer(&extPtrs[0]))
	}
	createDevice := vkProc(vkInstance, "vkCreateDevice")
	if createDevice == 0 {
		return fmt.Errorf("Vulkan 加载器缺少 vkCreateDevice")
	}
	if r, _, _ := syscall.SyscallN(createDevice, chosen, uintptr(unsafe.Pointer(&dci)), 0, uintptr(unsafe.Pointer(&vkDevice))); r != vkSuccess {
		return fmt.Errorf("创建 Vulkan 设备失败（%d）", r)
	}
	_ = extBytes
	dbg("device ok")

	vkFnGetDeviceProc = vkDevProcAddr()
	if vkFnGetDeviceProc == 0 {
		return fmt.Errorf("Vulkan 设备缺少过程地址接口")
	}
	vkFn = vkDeviceFunctions{
		getDeviceQueue:       vkDevProc("vkGetDeviceQueue"),
		createCommandPool:    vkDevProc("vkCreateCommandPool"),
		allocateCmdBuffers:   vkDevProc("vkAllocateCommandBuffers"),
		beginCmdBuffer:       vkDevProc("vkBeginCommandBuffer"),
		cmdPipelineBarrier:   vkDevProc("vkCmdPipelineBarrier"),
		cmdCopyImageToBuffer: vkDevProc("vkCmdCopyImageToBuffer"),
		endCmdBuffer:         vkDevProc("vkEndCommandBuffer"),
		queueSubmit:          vkDevProc("vkQueueSubmit"),
		waitForFences:        vkDevProc("vkWaitForFences"),
		resetFences:          vkDevProc("vkResetFences"),
		queueWaitIdle:        vkDevProc("vkQueueWaitIdle"),
		createBuffer:         vkDevProc("vkCreateBuffer"),
		getBufferMemoryReqs:  vkDevProc("vkGetBufferMemoryRequirements"),
		allocateMemory:       vkDevProc("vkAllocateMemory"),
		bindBufferMemory:     vkDevProc("vkBindBufferMemory"),
		mapMemory:            vkDevProc("vkMapMemory"),
		unmapMemory:          vkDevProc("vkUnmapMemory"),
		destroyBuffer:        vkDevProc("vkDestroyBuffer"),
		freeMemory:           vkDevProc("vkFreeMemory"),
		resetCmdBuffer:       vkDevProc("vkResetCommandBuffer"),
	}
	if vkFn.cmdCopyImageToBuffer == 0 || vkFn.queueSubmit == 0 {
		return fmt.Errorf("Vulkan 设备缺少渲染接口")
	}

	syscall.SyscallN(vkFn.getDeviceQueue, vkDevice, uintptr(family), 0, uintptr(unsafe.Pointer(&vkQueue)))
	if vkQueue == 0 {
		return fmt.Errorf("没有拿到 Vulkan 队列")
	}

	pool := vkCommandPoolCreateInfo{SType: vkStructureCommandPoolCreateInfo, Flags: vkCommandPoolResetBufferBit, QueueFamilyIndex: family}
	if r, _, _ := syscall.SyscallN(vkFn.createCommandPool, vkDevice, uintptr(unsafe.Pointer(&pool)), 0, uintptr(unsafe.Pointer(&vkPool))); r != vkSuccess {
		return fmt.Errorf("创建 Vulkan 命令池失败（%d）", r)
	}
	alloc := vkCommandBufferAllocateInfo{SType: vkStructureCommandBufferAllocInfo, CommandPool: vkPool, Level: 0, CommandBufferCount: 1}
	if r, _, _ := syscall.SyscallN(vkFn.allocateCmdBuffers, vkDevice, uintptr(unsafe.Pointer(&alloc)), uintptr(unsafe.Pointer(&vkCmd))); r != vkSuccess {
		return fmt.Errorf("分配 Vulkan 命令缓冲失败（%d）", r)
	}
	fci := vkFenceCreateInfo{SType: vkStructureFenceCreateInfo}
	createFence := vkProc(vkInstance, "vkCreateFence")
	if r, _, _ := syscall.SyscallN(createFence, vkDevice, uintptr(unsafe.Pointer(&fci)), 0, uintptr(unsafe.Pointer(&vkFence))); r != vkSuccess {
		return fmt.Errorf("创建 Vulkan 栅栏失败（%d）", r)
	}

	vkFillIface()
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "vk: instance=0x%x gpu=0x%x device=0x%x queue=0x%x family=%d exts=%d\n",
			vkInstance, vkPhysical, vkDevice, vkQueue, family, len(exts))
	}
	return nil
}

var vkMemProps vkPhysicalDeviceMemoryProperties

// vkPickInstanceExtensions enables the instance-level extensions a Vulkan core
// expects to find. Without get_physical_device_properties2 a core asks for
// vkGetPhysicalDeviceFeatures2KHR, is handed a null pointer, and calls it.
func vkPickInstanceExtensions() []string {
	enumerate := vkProc(0, "vkEnumerateInstanceExtensionProperties")
	if enumerate == 0 {
		return nil
	}
	var count uint32
	syscall.SyscallN(enumerate, 0, uintptr(unsafe.Pointer(&count)), 0)
	if count == 0 {
		return nil
	}
	type extProp struct {
		Name    [256]byte
		Version uint32
	}
	props := make([]extProp, count)
	if r, _, _ := syscall.SyscallN(enumerate, 0, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&props[0]))); r != vkSuccess {
		return nil
	}
	want := map[string]bool{
		"VK_KHR_get_physical_device_properties2":    true,
		"VK_KHR_external_memory_capabilities":       true,
		"VK_KHR_external_semaphore_capabilities":    true,
		"VK_KHR_get_surface_capabilities2":          true,
		"VK_KHR_surface":                            true,
		"VK_EXT_swapchain_colorspace":               true,
	}
	var out []string
	for i := range props {
		name := string(props[i].Name[:])
		if j := indexZero(name); j > 0 {
			name = name[:j]
		}
		if want[name] {
			out = append(out, name)
		}
	}
	return out
}

func vkDevProcAddr() uintptr {
	return vkProc(vkInstance, "vkGetDeviceProcAddr")
}

func vkPickDeviceExtensions(phys uintptr) []string {
	enumerate := vkProc(vkInstance, "vkEnumerateDeviceExtensionProperties")
	if enumerate == 0 {
		return nil
	}
	var count uint32
	syscall.SyscallN(enumerate, phys, 0, uintptr(unsafe.Pointer(&count)), 0)
	if count == 0 {
		return nil
	}
	type extProp struct {
		Name    [256]byte
		Version uint32
	}
	props := make([]extProp, count)
	if r, _, _ := syscall.SyscallN(enumerate, phys, 0, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&props[0]))); r != vkSuccess {
		return nil
	}
	// Everything the driver offers is enabled: a core picks the extensions it
	// was built against, and a missing one shows up as a null function pointer
	// that it then calls.
	var out []string
	for i := range props {
		name := string(props[i].Name[:])
		if j := indexZero(name); j > 0 {
			name = name[:j]
		}
		out = append(out, name)
	}
	return out
}

func indexZero(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return i
		}
	}
	return -1
}

func vkFillIface() {
	vkIface = retroHWRenderInterfaceVulkan{
		InterfaceType:     0, // RETRO_HW_RENDER_INTERFACE_VULKAN
		InterfaceVersion:  1,
		Handle:            unsafe.Pointer(&vkIface),
		Instance:          vkInstance,
		GPU:               vkPhysical,
		Device:            vkDevice,
		GetDeviceProc:     syscall.NewCallback(vkGetDeviceProcCb),
		GetInstanceProc:   syscall.NewCallback(vkGetInstanceProcCb),
		Queue:             vkQueue,
		QueueIndex:        vkQueueIdx,
		SetImage:          syscall.NewCallback(vkSetImage),
		GetSyncIndex:      syscall.NewCallback(vkSyncIndex),
		GetSyncIndexMask:  syscall.NewCallback(vkSyncIndex),
		SetCommandBuffers: syscall.NewCallback(vkSetCommandBuffers),
		WaitSyncIndex:     syscall.NewCallback(vkNoop),
		LockQueue:         syscall.NewCallback(vkNoop),
		UnlockQueue:       syscall.NewCallback(vkNoop),
		SetSignalSemaphore: syscall.NewCallback(vkSetSignalSemaphore),
	}
}

func vkNoop(handle uintptr) uintptr { return 0 }

func vkCString(p uintptr) string {
	if p == 0 {
		return "<null>"
	}
	var b []byte
	for i := uintptr(0); i < 256; i++ {
		c := *(*byte)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

// The two proc lookups are wrapped only so GAMEROOM_DEBUG can show which
// entry point a core asks for right before it crashes; they forward straight
// to the loader.
func vkGetDeviceProcCb(dev, name uintptr) uintptr {
	n := vkCString(name)
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "vk: devproc %s\n", n)
	}
	return vkDevProc(n)
}

func vkGetInstanceProcCb(inst, name uintptr) uintptr {
	n := vkCString(name)
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "vk: instproc %s\n", n)
	}
	return vkProc(inst, n)
}

func vkSyncIndex(handle uintptr) uintptr { return 0 }

func vkSetSignalSemaphore(handle, sem uintptr) uintptr {
	vkFrameMu.Lock()
	vkLastSem = sem
	vkFrameMu.Unlock()
	return 0
}

func vkSetCommandBuffers(handle, num, cmds uintptr) uintptr {
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "vk: set_command_buffers n=%d\n", num)
	}
	if num == 0 || cmds == 0 {
		return 0
	}
	list := unsafe.Slice((*uintptr)(unsafe.Pointer(cmds)), int(num))
	vkFrameMu.Lock()
	vkPending = append(vkPending, list...)
	vkFrameMu.Unlock()
	return 0
}

func vkSetImage(handle, image, numsems, sems uintptr) uintptr {
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "vk: set_image img=0x%x sems=%d\n", image, numsems)
	}
	if image == 0 {
		return 0
	}
	img := (*retroVulkanImage)(unsafe.Pointer(image))
	vkFrameMu.Lock()
	vkFrameView = img.ImageView
	vkFrameLayout = img.ImageLayout
	vkFrameCreate = img.CreateInfo
	if numsems > 0 && sems != 0 {
		vkLastSems = append([]uintptr(nil), unsafe.Slice((*uintptr)(unsafe.Pointer(sems)), int(numsems))...)
	}
	vkFrameMu.Unlock()
	return 0
}

func vkDestroy() {
	if vkDevice == 0 {
		return
	}
	if vkFn.queueWaitIdle != 0 {
		syscall.SyscallN(vkFn.queueWaitIdle, vkDevice)
	}
	if vkStageBuf != 0 && vkFn.destroyBuffer != 0 {
		if vkStagePtr != nil && vkFn.unmapMemory != 0 {
			syscall.SyscallN(vkFn.unmapMemory, vkDevice, vkStageMem)
			vkStagePtr = nil
		}
		syscall.SyscallN(vkFn.destroyBuffer, vkDevice, vkStageBuf, 0)
		vkStageBuf = 0
	}
	if vkStageMem != 0 && vkFn.freeMemory != 0 {
		syscall.SyscallN(vkFn.freeMemory, vkDevice, vkStageMem, 0)
		vkStageMem = 0
	}
}

// vkReadFrame copies the last image a core handed over into RGBA rows. It is
// called from the video callback with the size the core reported.
func vkReadFrame(w, h int) []byte {
	if vkDevice == 0 || w <= 0 || h <= 0 {
		return nil
	}
	vkFrameMu.Lock()
	view := vkFrameView
	layout := vkFrameLayout
	create := vkFrameCreate
	pending := vkPending
	vkPending = nil
	sems := vkLastSems
	vkLastSems = nil
	vkFrameMu.Unlock()
	if view == 0 || create == 0 {
		return nil
	}

	format := *(*uint32)(unsafe.Pointer(create + 32))
	if format != vkFormatRGBA8Unorm && format != vkFormatRGBA8Srgb &&
		format != vkFormatBGRA8Unorm && format != vkFormatBGRA8Srgb {
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "vk: unsupported image format %d\n", format)
		}
		return nil
	}

	size := uint64(w) * uint64(h) * 4
	if err := vkEnsureStaging(size); err != nil {
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "vk: staging buffer: %v\n", err)
		}
		return nil
	}

	if err := vkRecordCopy(w, h, layout, pending); err != nil {
		if os.Getenv("GAMEROOM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "vk: copy failed: %v\n", err)
		}
		return nil
	}
	_ = sems

	out := make([]byte, w*h*4)
	src := unsafe.Slice((*byte)(vkStagePtr), w*h*4)
	if format == vkFormatRGBA8Unorm || format == vkFormatRGBA8Srgb {
		for i := 0; i < len(out); i += 4 {
			out[i+0] = src[i+0]
			out[i+1] = src[i+1]
			out[i+2] = src[i+2]
			out[i+3] = 255
		}
	} else {
		for i := 0; i < len(out); i += 4 {
			out[i+0] = src[i+2]
			out[i+1] = src[i+1]
			out[i+2] = src[i+0]
			out[i+3] = 255
		}
	}
	return out
}

func vkEnsureStaging(size uint64) error {
	if vkStageBuf != 0 && vkStageSize >= size {
		return nil
	}
	if vkStageBuf != 0 && vkFn.destroyBuffer != 0 {
		if vkStagePtr != nil {
			syscall.SyscallN(vkFn.unmapMemory, vkDevice, vkStageMem)
			vkStagePtr = nil
		}
		syscall.SyscallN(vkFn.destroyBuffer, vkDevice, vkStageBuf, 0)
		syscall.SyscallN(vkFn.freeMemory, vkDevice, vkStageMem, 0)
		vkStageBuf, vkStageMem, vkStageSize = 0, 0, 0
	}
	info := vkBufferCreateInfo{
		SType:       vkStructureBufferCreateInfo,
		Size:        size,
		Usage:       vkBufferUsageTransferDst,
		SharingMode: vkSharingModeExclusive,
	}
	if r, _, _ := syscall.SyscallN(vkFn.createBuffer, vkDevice, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&vkStageBuf))); r != vkSuccess {
		return fmt.Errorf("创建缓冲失败（%d）", r)
	}
	var req vkMemoryRequirements
	syscall.SyscallN(vkFn.getBufferMemoryReqs, vkDevice, vkStageBuf, uintptr(unsafe.Pointer(&req)))
	idx := -1
	for i := 0; i < int(vkMemProps.MemoryTypeCount); i++ {
		flags := vkMemProps.MemoryTypes[i].PropertyFlags
		if flags&vkMemoryHostVisibleBit != 0 && flags&vkMemoryHostCoherentBit != 0 && req.MemoryTypeBits&(1<<uint(i)) != 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("没有可读回的主机内存")
	}
	alloc := vkMemoryAllocateInfo{SType: vkStructureMemoryAllocateInfo, AllocationSize: req.Size, MemoryTypeIndex: uint32(idx)}
	if r, _, _ := syscall.SyscallN(vkFn.allocateMemory, vkDevice, uintptr(unsafe.Pointer(&alloc)), 0, uintptr(unsafe.Pointer(&vkStageMem))); r != vkSuccess {
		return fmt.Errorf("分配缓冲内存失败（%d）", r)
	}
	if r, _, _ := syscall.SyscallN(vkFn.bindBufferMemory, vkDevice, vkStageBuf, vkStageMem, 0); r != vkSuccess {
		return fmt.Errorf("绑定缓冲内存失败（%d）", r)
	}
	var mapped unsafe.Pointer
	if r, _, _ := syscall.SyscallN(vkFn.mapMemory, vkDevice, vkStageMem, 0, uintptr(req.Size), 0, uintptr(unsafe.Pointer(&mapped))); r != vkSuccess {
		return fmt.Errorf("映射缓冲内存失败（%d）", r)
	}
	vkStagePtr = mapped
	vkStageSize = req.Size
	return nil
}

func vkRecordCopy(w, h int, layout uint32, pending []uintptr) error {
	if vkFn.resetCmdBuffer != 0 {
		syscall.SyscallN(vkFn.resetCmdBuffer, vkCmd, 0)
	}
	begin := vkCommandBufferBeginInfo{SType: vkStructureCommandBufferBeginInfo, Flags: vkCommandBufferOneTimeSubmit}
	if r, _, _ := syscall.SyscallN(vkFn.beginCmdBuffer, vkCmd, uintptr(unsafe.Pointer(&begin))); r != vkSuccess {
		return fmt.Errorf("命令缓冲开始失败（%d）", r)
	}

	toTransfer := vkImageMemoryBarrier{
		SType:               vkStructureImageMemoryBarrier,
		SrcAccessMask:       vkAccessMemoryRead | vkAccessMemoryWrite,
		DstAccessMask:       vkAccessTransferRead,
		OldLayout:           layout,
		NewLayout:           vkImageLayoutTransferSrc,
		SrcQueueFamilyIndex: vkQueueFamilyIgnored,
		DstQueueFamilyIndex: vkQueueFamilyIgnored,
		Image:               vkFrameViewImage(),
		SubresourceRange:    vkImageSubresourceRange{AspectMask: vkImageAspectColor, LevelCount: 1, ArrayCount: 1},
	}
	syscall.SyscallN(vkFn.cmdPipelineBarrier, vkCmd, vkPipelineAllCommands, vkPipelineAllCommands,
		0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&toTransfer)))

	copyRegion := vkBufferImageCopy{
		ImageSubresource: vkImageSubresourceLayers{AspectMask: vkImageAspectColor, LayerCount: 1},
		ImageExtent:      vkExtent3D{Width: uint32(w), Height: uint32(h), Depth: 1},
	}
	syscall.SyscallN(vkFn.cmdCopyImageToBuffer, vkCmd, vkFrameViewImage(), vkImageLayoutTransferSrc,
		vkStageBuf, 1, uintptr(unsafe.Pointer(&copyRegion)))

	back := toTransfer
	back.OldLayout = vkImageLayoutTransferSrc
	back.NewLayout = layout
	back.SrcAccessMask = vkAccessTransferRead
	back.DstAccessMask = vkAccessMemoryRead
	syscall.SyscallN(vkFn.cmdPipelineBarrier, vkCmd, vkPipelineAllCommands, vkPipelineAllCommands,
		0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&back)))

	if r, _, _ := syscall.SyscallN(vkFn.endCmdBuffer, vkCmd); r != vkSuccess {
		return fmt.Errorf("命令缓冲结束失败（%d）", r)
	}

	cmds := append([]uintptr(nil), pending...)
	cmds = append(cmds, vkCmd)
	submit := vkSubmitInfo{
		SType:              vkStructureSubmitInfo,
		CommandBufferCount: uint32(len(cmds)),
		PCommandBuffers:    uintptr(unsafe.Pointer(&cmds[0])),
	}
	if r, _, _ := syscall.SyscallN(vkFn.queueSubmit, vkQueue, 1, uintptr(unsafe.Pointer(&submit)), vkFence); r != vkSuccess {
		return fmt.Errorf("提交失败（%d）", r)
	}
	if r, _, _ := syscall.SyscallN(vkFn.waitForFences, vkDevice, 1, uintptr(unsafe.Pointer(&vkFence)), 1, uintptr(vkFenceTimeout)); r != vkSuccess {
		return fmt.Errorf("等待显卡失败（%d）", r)
	}
	syscall.SyscallN(vkFn.resetFences, vkDevice, 1, uintptr(unsafe.Pointer(&vkFence)))
	return nil
}

// vkFrameViewImage keeps the image itself alongside the view the core handed
// over: set_image only publishes the view, but copying needs the image, which
// the core's create_info names.
func vkFrameViewImage() uintptr {
	vkFrameMu.Lock()
	create := vkFrameCreate
	vkFrameMu.Unlock()
	if create == 0 {
		return 0
	}
	// VkImageViewCreateInfo: sType, pNext, flags, image, ...
	return *(*uintptr)(unsafe.Pointer(create + 24))
}
