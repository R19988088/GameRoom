//go:build !windows

package app

// mameSystem has no counterpart outside Windows: the cores are loaded as
// shared objects there, and MAME reads its files through the same UTF-8 paths
// as every other core.
func (a *App) mameSystem(string) string { return "" }
