package library

import "testing"

func TestCanonicalCore(t *testing.T) {
	cases := []struct{ platform, rom, want string }{
		{"fc", "fc/魂斗罗/魂斗罗.zip", "fceumm"},
		{"sfc", "sfc/最终幻想6.zip", "snes9x"},
		{"3DO", "3DO/game.iso", "opera"},
		// A platform folder can hold several systems: the folder decides.
		{"SEGA", "SEGA/MD/paprium/paprium.wav", "genesis_plus_gx"},
		{"SEGA", "SEGA/DC/Fighting Vipers 2.cdi", "flycast"},
		{"SEGA", "SEGA/SS/Shinobi X.cue", "mednafen_saturn"},
		{"SEGA", "SEGA/Model 2/vf2b.zip", "mame"},
		{"SEGA", "SEGA/Model 3/vf3.zip", "mame"},
		{"ps", "ps/ps/Final Fantasy VII.cue", "duckstation"},
		{"ps", "ps/ps2/Gundam.iso", "pcsx2"},
		{"任天堂", "任天堂/3ds/Hey Pikmin.cia", "citra"},
		{"任天堂", "任天堂/GB/龙珠Z.gb", "gearboy"},
		{"任天堂", "任天堂/wii/Rhythm Heaven.wbfs", "dolphin"},
		{"任天堂", "任天堂/switch/game.nsp", "ryujinx"},
		{"掌机", "掌机/GBA/节奏天国.gba", "mgba"},
		{"掌机", "掌机/PSP/2030 - 小小大星球.iso", "ppsspp"},
		{"掌机", "掌机/NGP/card fighters.ngc", "mednafen_ngp"},
		// Arcade and PC keep their own choices.
		{"arc", "arc/SNK/dino.zip", ""},
		{"微软", "微软/xbox.iso", ""},
	}
	for _, c := range cases {
		if got := CanonicalCore(c.platform, c.rom); got != c.want {
			t.Errorf("CanonicalCore(%q, %q) = %q, want %q", c.platform, c.rom, got, c.want)
		}
	}
}

func TestNormalizeConsoles(t *testing.T) {
	sims := []Sim{
		{ID: 2, PlatformKey: "fc", Name: "VirtuaNES Plus", Exe: `E:\old\VirtuaNES.exe`},
		{ID: 4, PlatformKey: "sfc", Name: "Snes9x", Exe: `E:\old\snes9x.exe`},
		{ID: 8, PlatformKey: "sfc", Name: "RetroArch", Exe: "模拟器/RetroArch/retroarch.exe", Core: "snes9x", IsDefault: true},
		{ID: 27, PlatformKey: "任天堂", Name: "Switch_ryujinx", Exe: `E:\old\Ryujinx.exe`},
		{ID: 19, PlatformKey: "arc", Name: "MAME Arcade", Exe: "模拟器/RetroArch/retroarch.exe", Core: "mamearcade", IsDefault: true},
	}
	games := []Game{
		{ID: 1, PlatformKey: "fc", RomPath: "fc/魂斗罗.zip", SimID: 2, Core: "fceumm"},
		{ID: 2, PlatformKey: "sfc", RomPath: "sfc/最终幻想6.zip", SimID: 4},
		{ID: 3, PlatformKey: "任天堂", RomPath: "任天堂/switch/game.nsp", SimID: 27},
		{ID: 4, PlatformKey: "任天堂", RomPath: "任天堂/3ds/game.cia", SimID: 0},
		{ID: 5, PlatformKey: "SEGA", RomPath: "SEGA/Model 2/vf2b.zip", SimID: 0},
		{ID: 6, PlatformKey: "arc", RomPath: "arc/SNK/dino.zip", SimID: 19},
	}
	NormalizeConsoles(&sims, games)

	byID := map[int]Sim{}
	for _, s := range sims {
		byID[s.ID] = s
	}
	simOf := map[int]Sim{}
	for _, g := range games {
		simOf[g.ID] = byID[g.SimID]
		if g.Core != "" {
			t.Errorf("game %d kept its old core override %q", g.ID, g.Core)
		}
	}

	if s := simOf[1]; s.Core != "fceumm" || s.Exe != retroArchExe {
		t.Errorf("fc game runs %+v, want FCEUmm through RetroArch", s)
	}
	if s := simOf[2]; s.Core != "snes9x" || !s.IsDefault {
		t.Errorf("sfc game runs %+v, want the default Snes9x", s)
	}
	if s := simOf[3]; s.Exe != `E:\old\Ryujinx.exe` {
		t.Errorf("switch game runs %+v, want to keep Ryujinx", s)
	}
	if s := simOf[4]; s.Core != "citra" || s.PlatformKey != "任天堂" {
		t.Errorf("3ds game runs %+v, want Citra", s)
	}
	if s := simOf[5]; s.Core != "mame" {
		t.Errorf("SEGA Model 2 game runs %+v, want MAME", s)
	}
	if s := simOf[6]; s.Core != "mamearcade" {
		t.Errorf("arcade game runs %+v, want its own arcade core", s)
	}

	// Sims nothing points at are gone, including the old external ones.
	if _, ok := byID[2]; ok {
		t.Errorf("dead external sim survived: %+v", byID[2])
	}
	if _, ok := byID[4]; ok {
		t.Errorf("dead external sim survived: %+v", byID[4])
	}
	if byID[8].Name != "Snes9x" {
		t.Errorf("name not normalized: %+v", byID[8])
	}
}
