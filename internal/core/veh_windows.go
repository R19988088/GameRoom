//go:build windows

package core

// Dolphin names its threads the C++ way: it raises 0x406D1388 and a debugger is
// expected to swallow it. Go's runtime sees an unknown exception and stops the
// program. A vectored handler registered ahead of the runtime eats exactly that
// code and lets the core continue, which is what RetroArch's own filter does.

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const exceptionThreadName = 0x406D1388
const exceptionContinueExecution = 0xFFFFFFFF

var vehOnce sync.Once

func ignoreThreadName() uintptr {
	return exceptionContinueExecution
}

func installExceptionFilter() {
	vehOnce.Do(func() {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		proc := kernel32.NewProc("AddVectoredExceptionHandler")
		cb := syscall.NewCallback(func(info uintptr) uintptr {
			if info == 0 {
				return 0 // keep looking
			}
			// EXCEPTION_POINTERS is { *EXCEPTION_RECORD, *CONTEXT }; the first
			// word of the record is its code.
			rec := *(*uintptr)(unsafe.Pointer(info))
			if rec == 0 {
				return 0
			}
			if *(*uint32)(unsafe.Pointer(rec)) == exceptionThreadName {
				if os.Getenv("GAMEROOM_DEBUG") != "" {
					fmt.Fprintln(os.Stderr, "veh: thread-name exception ignored")
				}
				return exceptionContinueExecution
			}
			if os.Getenv("GAMEROOM_DEBUG") != "" {
				// EXCEPTION_RECORD: code, flags, inner pointer, address.
				code := *(*uint32)(unsafe.Pointer(rec))
				addr := *(*uintptr)(unsafe.Pointer(rec + 16))
				fmt.Fprintf(os.Stderr, "veh: exception %#x at %#x%s\n", code, addr, moduleOf(addr))
			}
			return 0
		})
		proc.Call(1, cb)
	})
}
