//go:build windows

package core

import (
	"fmt"
	"syscall"
	"unsafe"
)

// moduleOf names the module an address falls in, so an exception inside a core
// can be told apart from one inside OpenGL or the driver.
func moduleOf(addr uintptr) string {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getMod := kernel32.NewProc("GetModuleHandleExW")
	name := kernel32.NewProc("GetModuleFileNameW")
	var h uintptr
	const fromAddress = 0x00000004
	if r, _, _ := getMod.Call(fromAddress, addr, uintptr(unsafe.Pointer(&h))); r == 0 || h == 0 {
		return ""
	}
	var buf [512]uint16
	n, _, _ := name.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	path := syscall.UTF16ToString(buf[:n])
	base := h // image base for an exe/dll handle on Win32 is the module base
	return fmt.Sprintf(" (%s+0x%x)", path, addr-base)
}
