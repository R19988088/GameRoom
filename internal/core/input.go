package core

// A core polls input by port, so the frontend keeps one set per port: four
// controllers, each with its own buttons and its own keyboard. The arrays are
// written as a whole and read on the core's own thread, never element by
// element.
const maxPorts = 4

// keyMasks is the keyboard as each port reads it: RETROK codes held down.
type keyMasks [maxPorts]map[int]bool

// padMasks is the joypad as each port reads it: RETRO_DEVICE_ID_JOYPAD ids held
// down.
type padMasks [maxPorts]map[int]bool

// bindTables is each player's keyboard, as virtual-key codes to RETROK codes.
// Two players may bind the same key; each port gets its own table, so one does
// not overwrite the other.
type bindTables [maxPorts]map[int]int
