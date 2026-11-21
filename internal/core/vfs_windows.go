// The libretro VFS: a core that does not open files with fopen of its own asks
// the frontend for one, and a frontend that says no leaves it with no way to
// reach its rom or its BIOS. Geolith and Opera are both built that way, so the
// answer here is a real file system: the same files, opened through Go, so the
// UTF-8 paths a Chinese library has work without a code page in between.
//
// Only version 1 of the interface is published. That is the version every core
// understands — open, read, write, seek, size, close — and it is all these
// cores use; the later additions (stat, mkdir, directory reading) are answered
// too, and a core that never asks for them is unaffected.
package core

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

// The interface is a table of function pointers, in the order libretro.h
// declares them. A core walks the table by that order, so the order is the
// contract: it is laid out exactly as retro_vfs_interface is.
type vfsInterface struct {
	open     uintptr
	close    uintptr
	size     uintptr
	tell     uintptr
	seek     uintptr
	read     uintptr
	write    uintptr
	flush    uintptr
	remove   uintptr
	rename   uintptr
	truncate uintptr
	stat     uintptr
	mkdir    uintptr
	opendir  uintptr
	readdir  uintptr
}

// vfsInfo is retro_vfs_interface_info: the version the core asks for, and the
// table it wants filled in.
type vfsInfo struct {
	version uint32
	_       uint32
	iface   *vfsInterface
}

// retro_vfs_interface_version is what this frontend offers. A core that asks
// for more is told the truth and can decide for itself.
const vfsVersion = 3

var vfsTable vfsInterface

// vfsFile is one open stream. A directory is the same handle with its entries
// read up front, because a core only ever asks for "the next one".
type vfsFile struct {
	f    *os.File
	dir  []os.DirEntry
	next int
}

var (
	vfsMu    sync.Mutex
	vfsFiles = map[uintptr]*vfsFile{}
	vfsID    uint64
)

func vfsAdd(v *vfsFile) uintptr {
	id := atomic.AddUint64(&vfsID, 1)
	vfsMu.Lock()
	vfsFiles[uintptr(id)] = v
	vfsMu.Unlock()
	return uintptr(id)
}

func vfsGet(id uintptr) (*vfsFile, bool) {
	vfsMu.Lock()
	v, ok := vfsFiles[id]
	vfsMu.Unlock()
	return v, ok
}

func vfsDrop(id uintptr) {
	vfsMu.Lock()
	delete(vfsFiles, id)
	vfsMu.Unlock()
}

// A core may spell a path the way its own code page does; this is the same
// name as UTF-8, which is what this process opens. A name that is already
// UTF-8 comes back unchanged, so the two spellings are tried in turn.
func utf8Name(name string) string {
	if name == "" || utf8.ValidString(name) {
		return name
	}
	b := []byte(name)
	n, _, _ := procMultiByteToWideChar.Call(0, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if int32(n) <= 0 {
		return name
	}
	u16 := make([]uint16, int32(n))
	procMultiByteToWideChar.Call(0, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&u16[0])), uintptr(int32(n)))
	return syscall.UTF16ToString(u16)
}

var procMultiByteToWideChar = syscall.NewLazyDLL("kernel32.dll").NewProc("MultiByteToWideChar")

func initVFS() *vfsInterface {
	cb := func(fn func(uintptr, uintptr, uintptr, uintptr) uintptr) uintptr {
		return syscall.NewCallback(func(a, b, c, d uintptr) uintptr { return fn(a, b, c, d) })
	}
	vfsTable = vfsInterface{
		open: cb(func(path, mode, hints, _ uintptr) uintptr {
			return vfsOpen(path, mode)
		}),
		close: cb(func(h, _, _, _ uintptr) uintptr {
			return vfsClose(h)
		}),
		size: cb(func(h, _, _, _ uintptr) uintptr {
			return vfsSize(h)
		}),
		tell: cb(func(h, _, _, _ uintptr) uintptr {
			return vfsTell(h)
		}),
		seek: cb(func(h, off, pos, _ uintptr) uintptr {
			return vfsSeek(h, int64(off), int(pos))
		}),
		read: cb(func(h, buf, n, _ uintptr) uintptr {
			return vfsRead(h, buf, n)
		}),
		write: cb(func(h, buf, n, _ uintptr) uintptr {
			return vfsWrite(h, buf, n)
		}),
		flush: cb(func(h, _, _, _ uintptr) uintptr {
			return vfsFlush(h)
		}),
		remove: cb(func(path, _, _, _ uintptr) uintptr {
			return boolRet(os.Remove(cString((*byte)(ptr(path)))) == nil)
		}),
		rename: cb(func(from, to, _, _ uintptr) uintptr {
			src, dst := cString((*byte)(ptr(from))), cString((*byte)(ptr(to)))
			return boolRet(os.Rename(src, dst) == nil)
		}),
		truncate: cb(func(h, length, _, _ uintptr) uintptr {
			return vfsTruncate(h, int64(length))
		}),
		stat: cb(func(path, out, _, _ uintptr) uintptr {
			return vfsStat(path, out)
		}),
		mkdir: cb(func(path, _, _, _ uintptr) uintptr {
			return boolRet(os.MkdirAll(cString((*byte)(ptr(path))), 0o755) == nil)
		}),
		opendir: cb(func(path, _, _, _ uintptr) uintptr {
			return vfsOpenDir(path)
		}),
		readdir: cb(func(h, _, _, _ uintptr) uintptr {
			return vfsReaddir(h)
		}),
	}
	return &vfsTable
}

// The interface reports a few things as a boolean, and libretro spells "no" as
// zero and "yes" as one.
func boolRet(ok bool) uintptr {
	if ok {
		return 1
	}
	return 0
}

// fail is -1 as a return value of the int64-returning calls.
func fail() uintptr { return ^uintptr(0) }

func vfsOpen(path, mode uintptr) uintptr {
	name := cString((*byte)(ptr(path)))
	if name == "" {
		return 0
	}
	// Modes are read, write and update: the last one opens read/write and
	// creates the file if it is not there, which is what a save state write is.
	flag := os.O_RDONLY
	switch {
	case mode&2 != 0: // WRITE
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case mode&4 != 0: // UPDATE
		flag = os.O_RDWR | os.O_CREATE
	}
	f, err := os.OpenFile(name, flag, 0o644)
	if err != nil {
		// A core may spell a path the way its own code page does; the same name
		// in UTF-8 is what this process can open.
		if alt := utf8Name(name); alt != name {
			f, err = os.OpenFile(alt, flag, 0o644)
		}
		if err != nil {
			if os.Getenv("GAMEROOM_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "vfs open FAIL mode=%d %q: %v\n", mode, name, err)
			}
			return 0
		}
	}
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "vfs open mode=%d %q\n", mode, name)
	}
	return vfsAdd(&vfsFile{f: f})
}

func vfsClose(h uintptr) uintptr {
	v, ok := vfsGet(h)
	if !ok {
		return 0
	}
	if v.f != nil {
		v.f.Close()
	}
	vfsDrop(h)
	return 0
}

func vfsSize(h uintptr) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.f == nil {
		return 0
	}
	st, err := v.f.Stat()
	if err != nil {
		return fail()
	}
	return uintptr(st.Size())
}

func vfsTell(h uintptr) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.f == nil {
		return fail()
	}
	off, err := v.f.Seek(0, 1)
	if err != nil {
		return fail()
	}
	return uintptr(off)
}

func vfsSeek(h uintptr, off int64, where int) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.f == nil {
		return fail()
	}
	whence := 0
	switch where {
	case 1:
		whence = 1
	case 2:
		whence = 2
	}
	if _, err := v.f.Seek(off, whence); err != nil {
		return fail()
	}
	return 0
}

func vfsRead(h, buf, n uintptr) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.f == nil || buf == 0 {
		return 0
	}
	b := unsafe.Slice((*byte)(ptr(buf)), int(n))
	got, err := v.f.Read(b)
	if got == 0 && err != nil {
		return 0
	}
	return uintptr(got)
}

func vfsWrite(h, buf, n uintptr) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.f == nil || buf == 0 {
		return 0
	}
	b := unsafe.Slice((*byte)(ptr(buf)), int(n))
	put, err := v.f.Write(b)
	if err != nil && put == 0 {
		return fail()
	}
	return uintptr(put)
}

func vfsFlush(h uintptr) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.f == nil {
		return 0
	}
	if err := v.f.Sync(); err != nil {
		return fail()
	}
	return 0
}

func vfsTruncate(h uintptr, length int64) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.f == nil {
		return fail()
	}
	if err := v.f.Truncate(length); err != nil {
		return fail()
	}
	return 0
}

func vfsStat(path, out uintptr) uintptr {
	name := cString((*byte)(ptr(path)))
	st, err := os.Stat(name)
	if err != nil {
		if alt := utf8Name(name); alt != name {
			st, err = os.Stat(alt)
		}
		if err != nil {
			return fail()
		}
	}
	if out != 0 {
		*(*int32)(ptr(out)) = int32(st.Size())
	}
	return 0
}

func vfsOpenDir(path uintptr) uintptr {
	name := cString((*byte)(ptr(path)))
	entries, err := os.ReadDir(name)
	if err != nil {
		if alt := utf8Name(name); alt != name {
			entries, err = os.ReadDir(alt)
		}
		if err != nil {
			return 0
		}
	}
	return vfsAdd(&vfsFile{dir: entries})
}

func vfsReaddir(h uintptr) uintptr {
	v, ok := vfsGet(h)
	if !ok || v.next >= len(v.dir) {
		return 0
	}
	v.next++
	return 1
}
