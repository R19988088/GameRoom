package library

import (
	"path/filepath"
	"strings"
)

// One core per system. A platform folder often holds several systems
// (SEGA has Mega Drive, Saturn, Dreamcast and arcade boards; 掌机 holds
// GBA, PSP and Neo Geo Pocket), and the old frontend let each game point
// at whatever emulator its owner had picked. The launcher instead runs
// everything through RetroArch and picks the core by what the rom
// actually is: its folder first, its extension second. That keeps the CRT
// shader identical across the whole library.
//
// The cores named here ship in the bundled cores/ folder on every
// platform, so a library stays portable. Ryujinx is the exception: there
// is no libretro core for Switch, so those games keep their own emulator.
var coreNames = map[string]string{
	"mesen":           "Mesen",
	"snes9x":          "Snes9x",
	"genesis_plus_gx": "Genesis Plus GX",
	"duckstation":     "DuckStation",
	"pcsx2":           "PCSX2",
	"flycast":         "Flycast",
	"mednafen_saturn": "Mednafen Saturn",
	"mame":            "MAME",
	"opera":           "Opera",
	"gearboy":         "Gearboy",
	"mgba":            "mGBA",
	"melonds":         "melonDS",
	"mednafen_ngp":    "Mednafen NGP",
	"ppsspp":          "PPSSPP",
	"citra":           "Citra",
	"dolphin":         "Dolphin",
}

// retroArchExe is where every core-bearing sim points.
const retroArchExe = "模拟器/RetroArch/retroarch.exe"

// consolePlatforms are the platforms this normalization covers. Arcade
// folders keep their hand-tuned MAME/FBNeo choices, PC is left alone.
var consolePlatforms = map[string]bool{
	"fc": true, "sfc": true, "3DO": true, "ps": true,
	"SEGA": true, "任天堂": true, "掌机": true,
}

// CanonicalCore returns the core that should run this rom, "" when the
// platform is not one we normalize.
func CanonicalCore(platform, romPath string) string {
	if !consolePlatforms[platform] {
		return ""
	}
	p := strings.ToLower(strings.ReplaceAll(romPath, "\\", "/"))
	ext := strings.TrimPrefix(filepath.Ext(p), ".")

	switch platform {
	case "fc":
		return "mesen"
	case "sfc":
		return "snes9x"
	case "3DO":
		return "opera"
	case "ps":
		if strings.Contains(p, "/ps2/") || ext == "iso" {
			return "pcsx2"
		}
		return "duckstation"
	case "SEGA":
		// Model 2, Model 3 and ST-V are arcade boards that happen to live
		// in a SEGA folder; MAME is the only core that runs them.
		if strings.Contains(p, "/model 2/") || strings.Contains(p, "/model 3/") || strings.Contains(p, "/stv/") {
			return "mame"
		}
		if strings.Contains(p, "/dc/") || ext == "gdi" || ext == "cdi" {
			return "flycast"
		}
		if strings.Contains(p, "/ss/") {
			return "mednafen_saturn"
		}
		return "genesis_plus_gx"
	case "任天堂":
		switch {
		case strings.Contains(p, "/3ds/"), ext == "cia", ext == "cci":
			return "citra"
		case strings.Contains(p, "/gb/"), ext == "gb", ext == "gbc":
			return "gearboy"
		case strings.Contains(p, "/switch/"), ext == "nsp", ext == "xci":
			return "ryujinx"
		case strings.Contains(p, "/wii/"), strings.Contains(p, "/gc/"), ext == "rvz", ext == "wbfs", ext == "iso":
			return "dolphin"
		case strings.Contains(p, "/nds/"), ext == "nds":
			return "melonds"
		}
		return "citra"
	case "掌机":
		switch {
		case strings.Contains(p, "/psp/"), ext == "iso", ext == "cso":
			return "ppsspp"
		case strings.Contains(p, "/ngp/"), ext == "ngc":
			return "mednafen_ngp"
		case strings.Contains(p, "/gba/"), ext == "gba":
			return "mgba"
		}
		return "mgba"
	}
	return ""
}

// NormalizeConsoles rewrites the sims of a freshly imported library so
// every console and handheld game runs the canonical core for its system:
// one sim per (platform, core), each game pointed at the right one, one
// default per platform, and only sims that are actually used kept. Old
// per-game core overrides are dropped, since replacing them is the point;
// arcade and PC games are untouched.
func NormalizeConsoles(sims *[]Sim, games []Game) {
	// Sims to reuse, keyed "platform\x00core".
	byCore := map[string]int{}
	nextID := 1
	for _, s := range *sims {
		if s.ID >= nextID {
			nextID = s.ID + 1
		}
		if !consolePlatforms[s.PlatformKey] || s.Core == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(filepath.ToSlash(s.Exe)), "retroarch") {
			continue
		}
		key := s.PlatformKey + "\x00" + s.Core
		if _, ok := byCore[key]; !ok {
			byCore[key] = s.ID
		}
	}
	// The external emulators we keep: no libretro core exists for them.
	external := map[string]int{}
	for _, s := range *sims {
		if !consolePlatforms[s.PlatformKey] || s.Core != "" {
			continue
		}
		if strings.Contains(strings.ToLower(filepath.ToSlash(s.Exe)), "ryujinx") {
			external[s.PlatformKey+"\x00ryujinx"] = s.ID
		}
	}

	used := map[int]bool{}
	counts := map[int]int{}
	for i := range games {
		g := &games[i]
		core := CanonicalCore(g.PlatformKey, g.RomPath)
		if core == "" {
			continue
		}
		if core == "ryujinx" {
			if id, ok := external[g.PlatformKey+"\x00ryujinx"]; ok {
				g.SimID = id
				g.Core = ""
				used[id] = true
			}
			continue
		}
		key := g.PlatformKey + "\x00" + core
		id, ok := byCore[key]
		if !ok {
			id = nextID
			nextID++
			*sims = append(*sims, Sim{
				ID: id, PlatformKey: g.PlatformKey, Name: coreNames[core],
				Exe: retroArchExe, Core: core, Args: "{RomFullPath}",
			})
			byCore[key] = id
		}
		g.SimID = id
		g.Core = ""
		used[id] = true
		counts[id]++
	}

	// Sim names follow the core, so the UI cannot show two names for one
	// core, and every core-bearing sim points at the bundled RetroArch.
	for i := range *sims {
		s := &(*sims)[i]
		if !consolePlatforms[s.PlatformKey] || s.Core == "" {
			continue
		}
		if name, ok := coreNames[s.Core]; ok {
			s.Name = name
			s.Exe = retroArchExe
		}
	}

	// One default per platform: the sim most of its games point at.
	platformOf := map[int]string{}
	for _, s := range *sims {
		platformOf[s.ID] = s.PlatformKey
	}
	for platform := range consolePlatforms {
		best, bestN := 0, 0
		for id, n := range counts {
			if platformOf[id] != platform || n <= bestN {
				continue
			}
			best, bestN = id, n
		}
		for i := range *sims {
			if (*sims)[i].PlatformKey == platform {
				(*sims)[i].IsDefault = (*sims)[i].ID == best
			}
		}
	}

	// Keep every sim a game points at; console sims nothing uses are gone.
	var kept []Sim
	for _, s := range *sims {
		if consolePlatforms[s.PlatformKey] && !used[s.ID] {
			continue
		}
		kept = append(kept, s)
	}
	*sims = kept
}
