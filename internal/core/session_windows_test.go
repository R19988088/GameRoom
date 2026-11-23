//go:build windows

package core

import "testing"

// TestJoypadAnswersItsOwnPort: two players can hold the same button id at the
// same time; each port reads only its own mask.
func TestJoypadAnswersItsOwnPort(t *testing.T) {
	s := &Session{}
	s.pads.Store(padMasks{
		0: {8: true}, // A held by player one
		1: {0: true}, // B held by player two
	})
	old := active
	active = s
	defer func() { active = old }()

	if got := inputCallback(0, DeviceJoypad, 0, 8); got != 1 {
		t.Errorf("port 0 A = %d, want 1", got)
	}
	if got := inputCallback(1, DeviceJoypad, 0, 8); got != 0 {
		t.Errorf("port 1 read player one's A: %d, want 0", got)
	}
	if got := inputCallback(1, DeviceJoypad, 0, 0); got != 1 {
		t.Errorf("port 1 B = %d, want 1", got)
	}
	if got := inputCallback(2, DeviceJoypad, 0, 8); got != 0 {
		t.Errorf("port 2 read a button: %d, want 0", got)
	}
}

// TestKeyboardAnswersItsOwnPort: the keyboard tables are per port too, and a
// port with no table reads nothing rather than another player's keys.
func TestKeyboardAnswersItsOwnPort(t *testing.T) {
	s := &Session{}
	s.keys.Store(keyMasks{1: {97: true}}) // 'a' on player two's keyboard
	old := active
	active = s
	defer func() { active = old }()

	if got := inputCallback(1, DeviceKeyboard, 0, 97); got != 1 {
		t.Errorf("port 1 'a' = %d, want 1", got)
	}
	if got := inputCallback(0, DeviceKeyboard, 0, 97); got != 0 {
		t.Errorf("port 0 read player two's 'a': %d, want 0", got)
	}
	if got := inputCallback(4, DeviceKeyboard, 0, 97); got != 0 {
		t.Errorf("port 4 answered: %d, want 0", got)
	}
}

// TestASharedKeyReachesBothPorts: the same virtual-key code in both players'
// tables is not a conflict; both keyboards read it down.
func TestASharedKeyReachesBothPorts(t *testing.T) {
	tables := bindTables{
		0: {0x50: 112}, // P on player one
		1: {0x50: 112}, // P on player two
	}
	held := keysHeld(tables, map[int]bool{0x50: true})
	if !held[0][112] {
		t.Errorf("player one did not read the shared key: %v", held[0])
	}
	if !held[1][112] {
		t.Errorf("player two did not read the shared key: %v", held[1])
	}
}
