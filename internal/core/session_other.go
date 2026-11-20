//go:build !windows

package core

import "errors"

// ErrUnsupported is what a build off Windows answers: the cores this launcher
// runs are Windows libraries.
var ErrUnsupported = errors.New("core: only windows runs these cores")

// Session is a game step by step; elsewhere there is none.
type Session struct{}

// Start refuses: no core here.
func Start(corePath, romPath, systemDir string) (*Session, error) { return nil, ErrUnsupported }

// Run does nothing.
func (s *Session) Run() {}

// SetKeys does nothing.
func (s *Session) SetKeys(keys map[int]bool) {}

// Frame is never a picture.
func (s *Session) Frame() any { return nil }

// OnFrame does nothing.
func (s *Session) OnFrame(f func()) {}

// Close does nothing.
func (s *Session) Close() {}

// CorePath is never a core.
func CorePath(dir, name string) string { return "" }
