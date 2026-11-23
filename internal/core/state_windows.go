//go:build windows

package core

import (
	"errors"
	"time"
	"unsafe"
)

// A core's save state is the whole machine in one block of bytes: retro_serialize
// writes it, retro_unserialize puts it back. It is what a save slot on disk holds,
// and it is what a rewind is made of — a row of states, the game put back at an
// older one.
//
// Both calls read or write every variable the core has, so they belong between
// two frames and never inside retro_run. This session offers exactly two such
// places: the frame callback (OnFrame, which runs on the core's own thread) and
// a task handed to Do, which Run takes at the top of a frame.

// coreTask is one piece of work the core's thread runs between frames: what to
// run, what it answered, and a door to close when it is done.
type coreTask struct {
	f    func() error
	err  error
	done chan struct{}
}

// errStopped is what a call into a core that is closed — or one whose run loop
// never started — answers, so a panel never waits on a game that is gone.
var errStopped = errors.New("核心没有运行")

// Do runs f between two frames on the core's own thread and waits for it. The
// window's thread goes through here to reach the core's state; the work lands on
// the next frame boundary, which is a frame's time away.
func (s *Session) Do(f func() error) error {
	if s == nil || f == nil {
		return nil
	}
	if s.tasks == nil {
		return errStopped
	}
	t := &coreTask{f: f, done: make(chan struct{})}
	select {
	case s.tasks <- t:
	case <-s.done:
		return errStopped
	case <-time.After(3 * time.Second):
		return errStopped
	}
	select {
	case <-t.done:
		return t.err
	case <-s.done:
		return errStopped
	case <-time.After(3 * time.Second):
		return errors.New("核心没有回应")
	}
}

// runTasks runs everything waiting at the top of a frame, oldest first. The core
// is between two frames here, which is the one place its state is whole.
func (s *Session) runTasks() {
	for {
		select {
		case t := <-s.tasks:
			t.err = t.f()
			close(t.done)
		default:
			return
		}
	}
}

// CanState reports whether this core has a save state at all: a core that
// answers 0 to retro_serialize_size has none, and neither a slot nor a rewind
// can be offered for it.
func (s *Session) CanState() bool {
	return s != nil && s.serSize != 0 && s.serialize != 0 && s.unserialize != 0
}

// StateSizeNow is how many bytes a state takes, asked without the queue: for a
// caller already on the core's thread, such as the frame callback.
func (s *Session) StateSizeNow() int {
	if s == nil || s.serSize == 0 {
		return 0
	}
	return int(s.call(s.serSize))
}

// StateSize is the same answer for the window's thread.
func (s *Session) StateSize() int {
	var n int
	if err := s.Do(func() error { n = s.StateSizeNow(); return nil }); err != nil {
		return 0
	}
	return n
}

// SaveStateNow writes the core's state into a copy of its own. It must be called
// on the core's thread — from OnFrame, or from a Do task.
func (s *Session) SaveStateNow() ([]byte, error) {
	if !s.CanState() {
		return nil, errors.New("这个核心不能存档")
	}
	n := s.StateSizeNow()
	if n <= 0 {
		return nil, errors.New("这个核心不能存档")
	}
	// The scratch buffer is the core thread's own: every caller of this is on
	// that thread, so a state is written in one place and copied out.
	if cap(s.stateBuf) < n {
		s.stateBuf = make([]byte, n)
	}
	buf := s.stateBuf[:n]
	if s.call(s.serialize, uintptr(unsafe.Pointer(&buf[0]))) == 0 {
		return nil, errors.New("核心没有写出存档")
	}
	out := make([]byte, n)
	copy(out, buf)
	return out, nil
}

// SaveState is the window's form of SaveStateNow: it answers at the next frame
// boundary, with a state of its own to keep.
func (s *Session) SaveState() ([]byte, error) {
	var data []byte
	if err := s.Do(func() error {
		var err error
		data, err = s.SaveStateNow()
		return err
	}); err != nil {
		return nil, err
	}
	return data, nil
}

// LoadStateNow puts a state back. Same rule as SaveStateNow: on the core's
// thread, between two frames.
func (s *Session) LoadStateNow(data []byte) error {
	if s == nil || s.unserialize == 0 {
		return errors.New("这个核心不能读档")
	}
	if len(data) == 0 {
		return errors.New("这个存档是空的")
	}
	if s.call(s.unserialize, uintptr(unsafe.Pointer(&data[0]))) == 0 {
		return errors.New("核心没有接受这个存档")
	}
	return nil
}

// LoadState is the window's form: the state goes back in at the next frame
// boundary, so nothing the core was half-way through is lost.
func (s *Session) LoadState(data []byte) error {
	return s.Do(func() error { return s.LoadStateNow(data) })
}
