//go:build !windows

package launch

// MagpieFilter is Windows-only; elsewhere nothing happens.
type MagpieFilter struct {
	Exe string
}

// TakeOver does nothing off Windows: there is no window capture tool here.
func TakeOver(pid int, exeName string, f MagpieFilter) {}
